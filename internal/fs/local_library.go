package fs

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	localLibraryMaxBytes = 8 * 1024 * 1024
	localLibraryMaxBody  = 1024 * 1024
)

var localLibraryMutationMutex sync.Mutex

type LocalLibraryItem struct {
	ID        string   `json:"id"`
	Kind      string   `json:"kind"`
	Title     string   `json:"title"`
	Body      string   `json:"body"`
	Tags      []string `json:"tags"`
	CreatedAt string   `json:"createdAt"`
	UpdatedAt string   `json:"updatedAt"`
	Source    string   `json:"source"`
}

type LocalLibrarySaveInput struct {
	Kind  string
	Title string
	Body  string
	Tags  []string
}

type localLibraryDocument struct {
	Version int                `json:"version"`
	Items   []LocalLibraryItem `json:"items"`
}

func ReadLocalLibrary(projectRoot string) ([]LocalLibraryItem, error) {
	directory, err := localCodeStateDirectory(projectRoot, false)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []LocalLibraryItem{}, nil
		}
		return nil, err
	}
	path := filepath.Join(directory, "library.json")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return []LocalLibraryItem{}, nil
	}
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, errors.New("local library must be a regular file")
	}
	if info.Size() > localLibraryMaxBytes {
		return nil, errors.New("local library exceeds 8 MiB")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var document localLibraryDocument
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil || document.Version != 1 {
		return nil, errors.New("local library is invalid")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("local library is invalid")
	}
	for itemIndex, item := range document.Items {
		if err := validateLocalLibraryItem(item); err != nil {
			return nil, fmt.Errorf("local library contains an invalid item at index %d: %w", itemIndex, err)
		}
	}
	return append([]LocalLibraryItem(nil), document.Items...), nil
}

func SaveLocalLibraryItem(projectRoot string, input LocalLibrarySaveInput) (LocalLibraryItem, error) {
	localLibraryMutationMutex.Lock()
	defer localLibraryMutationMutex.Unlock()

	kind := strings.TrimSpace(input.Kind)
	if kind == "" {
		kind = "prompt"
	}
	if kind != "prompt" && kind != "artifact" {
		return LocalLibraryItem{}, errors.New("local type must be prompt or artifact")
	}
	title := strings.TrimSpace(input.Title)
	body := strings.TrimSpace(input.Body)
	if title == "" || len(title) > 200 {
		return LocalLibraryItem{}, errors.New("local title is invalid")
	}
	if body == "" || len([]byte(body)) > localLibraryMaxBody {
		return LocalLibraryItem{}, errors.New("local content is empty or exceeds 1 MiB")
	}
	tags := make([]string, 0, len(input.Tags))
	seenTags := map[string]struct{}{}
	for _, rawTag := range input.Tags {
		tag := strings.TrimSpace(rawTag)
		if tag == "" || len(tag) > 64 {
			return LocalLibraryItem{}, errors.New("a local tag is invalid")
		}
		if _, exists := seenTags[tag]; exists {
			continue
		}
		seenTags[tag] = struct{}{}
		tags = append(tags, tag)
	}
	if len(tags) > 20 {
		return LocalLibraryItem{}, errors.New("local capture contains too many tags")
	}

	directory, err := localCodeStateDirectory(projectRoot, true)
	if err != nil {
		return LocalLibraryItem{}, err
	}
	releaseLock, err := AcquireStateLock(directory)
	if err != nil {
		return LocalLibraryItem{}, err
	}
	defer releaseLock()
	items, err := ReadLocalLibrary(projectRoot)
	if err != nil {
		return LocalLibraryItem{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	item := LocalLibraryItem{
		ID:        localLibraryID(),
		Kind:      kind,
		Title:     title,
		Body:      body,
		Tags:      tags,
		CreatedAt: now,
		UpdatedAt: now,
		Source:    "local",
	}
	document := localLibraryDocument{Version: 1, Items: append([]LocalLibraryItem{item}, items...)}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return LocalLibraryItem{}, err
	}
	data = append(data, '\n')
	if len(data) > localLibraryMaxBytes {
		return LocalLibraryItem{}, errors.New("local library would exceed 8 MiB")
	}
	if err := atomicWriteFile(filepath.Join(directory, "library.json"), data, 0o600); err != nil {
		return LocalLibraryItem{}, err
	}
	return item, nil
}

func localCodeStateDirectory(projectRoot string, create bool) (string, error) {
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return "", err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	current := root
	for _, segment := range []string{".sth", "local", "code"} {
		current = filepath.Join(current, segment)
		info, statErr := os.Lstat(current)
		if errors.Is(statErr, os.ErrNotExist) && create {
			if err := os.Mkdir(current, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
				return "", err
			}
			info, statErr = os.Lstat(current)
		}
		if statErr != nil {
			return "", statErr
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return "", fmt.Errorf("local state path %s is not a safe directory", current)
		}
		if create {
			_ = os.Chmod(current, 0o700)
		}
	}
	return current, nil
}

func validateLocalLibraryItem(item LocalLibraryItem) error {
	if strings.TrimSpace(item.ID) == "" || (item.Kind != "prompt" && item.Kind != "artifact") {
		return errors.New("invalid local item")
	}
	if strings.TrimSpace(item.Title) == "" || strings.TrimSpace(item.Body) == "" {
		return errors.New("incomplete local item")
	}
	return nil
}

func localLibraryID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("local-%d", time.Now().UnixNano())
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	hexValue := hex.EncodeToString(bytes)
	return fmt.Sprintf("%s-%s-%s-%s-%s", hexValue[:8], hexValue[8:12], hexValue[12:16], hexValue[16:20], hexValue[20:])
}
