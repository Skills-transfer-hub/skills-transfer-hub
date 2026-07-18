package fs

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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

// ReadLocalLibrary reads the STH Code-compatible local library without
// creating state or following a symlink out of the project. Missing state is
// an empty library so Starter works fully offline on a fresh checkout.
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
		return nil, errors.New("la bibliothèque locale doit être un fichier régulier")
	}
	if info.Size() > localLibraryMaxBytes {
		return nil, errors.New("la bibliothèque locale dépasse 8 Mio")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var document localLibraryDocument
	if err := json.Unmarshal(data, &document); err != nil || document.Version != 1 {
		return nil, errors.New("la bibliothèque locale est invalide")
	}
	items := make([]LocalLibraryItem, 0, len(document.Items))
	for _, item := range document.Items {
		if validateLocalLibraryItem(item) == nil {
			items = append(items, item)
		}
	}
	return items, nil
}

// SaveLocalLibraryItem performs a voluntary local capture. It writes the same
// version-1 document consumed by STH Code and never contacts the cloud.
func SaveLocalLibraryItem(projectRoot string, input LocalLibrarySaveInput) (LocalLibraryItem, error) {
	localLibraryMutationMutex.Lock()
	defer localLibraryMutationMutex.Unlock()

	kind := strings.TrimSpace(input.Kind)
	if kind == "" {
		kind = "prompt"
	}
	if kind != "prompt" && kind != "artifact" {
		return LocalLibraryItem{}, errors.New("le type local doit être prompt ou artifact")
	}
	title := strings.TrimSpace(input.Title)
	body := strings.TrimSpace(input.Body)
	if title == "" || len(title) > 200 {
		return LocalLibraryItem{}, errors.New("le titre local est invalide")
	}
	if body == "" || len([]byte(body)) > localLibraryMaxBody {
		return LocalLibraryItem{}, errors.New("le contenu local est vide ou dépasse 1 Mio")
	}
	tags := make([]string, 0, len(input.Tags))
	seenTags := map[string]struct{}{}
	for _, rawTag := range input.Tags {
		tag := strings.TrimSpace(rawTag)
		if tag == "" || len(tag) > 64 {
			return LocalLibraryItem{}, errors.New("un tag local est invalide")
		}
		if _, exists := seenTags[tag]; exists {
			continue
		}
		seenTags[tag] = struct{}{}
		tags = append(tags, tag)
	}
	if len(tags) > 20 {
		return LocalLibraryItem{}, errors.New("la capture locale contient trop de tags")
	}

	directory, err := localCodeStateDirectory(projectRoot, true)
	if err != nil {
		return LocalLibraryItem{}, err
	}
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
		return LocalLibraryItem{}, errors.New("la bibliothèque locale dépasserait 8 Mio")
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
			return "", fmt.Errorf("le chemin d'état local %s n'est pas un dossier sûr", current)
		}
		if create {
			_ = os.Chmod(current, 0o700)
		}
	}
	return current, nil
}

func validateLocalLibraryItem(item LocalLibraryItem) error {
	if strings.TrimSpace(item.ID) == "" || (item.Kind != "prompt" && item.Kind != "artifact") {
		return errors.New("item local invalide")
	}
	if strings.TrimSpace(item.Title) == "" || strings.TrimSpace(item.Body) == "" {
		return errors.New("item local incomplet")
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
