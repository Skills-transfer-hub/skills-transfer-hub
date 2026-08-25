package fs

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var ErrManagedMarkdownRegionNotFound = errors.New("managed Markdown region not found")
var ErrManagedMarkdownRegionMalformed = errors.New("invalid managed Markdown region markers")

type managedMarkdownRegionLocation struct {
	start int
	end   int
	found bool
}

func BuildManagedMarkdownRegion(id string, body []byte) ([]byte, error) {
	return buildManagedMarkdownRegion(id, body, false)
}

func buildManagedMarkdownRegion(id string, body []byte, includeSeparator bool) ([]byte, error) {
	startMarker, endMarker, err := managedMarkdownRegionMarkers(id)
	if err != nil {
		return nil, err
	}
	if bytes.Contains(body, []byte("<!-- sth:managed:")) {
		return nil, fmt.Errorf("%w for %q: content contains a reserved marker", ErrManagedMarkdownRegionMalformed, id)
	}

	separatorMarker := managedMarkdownRegionSeparatorMarker(id)
	block := make([]byte, 0, len(startMarker)+len(separatorMarker)+len(body)+len(endMarker)+4)
	if includeSeparator {
		block = append(block, '\n')
	}
	block = append(block, startMarker...)
	block = append(block, '\n')
	if includeSeparator {
		block = append(block, separatorMarker...)
		block = append(block, '\n')
	}
	block = append(block, body...)
	if len(body) > 0 && body[len(body)-1] != '\n' {
		block = append(block, '\n')
	}
	block = append(block, endMarker...)
	return block, nil
}

func MergeManagedMarkdownRegion(document []byte, id string, body []byte) ([]byte, error) {
	location, err := locateManagedMarkdownRegion(document, id)
	if err != nil {
		return nil, err
	}
	includeSeparator := len(document) > 0
	if location.found {
		includeSeparator = location.start > 0
	}
	block, err := buildManagedMarkdownRegion(id, body, includeSeparator)
	if err != nil {
		return nil, err
	}
	if !location.found {
		merged := make([]byte, 0, len(document)+len(block))
		merged = append(merged, document...)
		merged = append(merged, block...)
		return merged, nil
	}
	return replaceManagedMarkdownRegionAt(document, location, block), nil
}

func ReplaceManagedMarkdownRegion(document []byte, id string, body []byte) ([]byte, error) {
	location, err := locateManagedMarkdownRegion(document, id)
	if err != nil {
		return nil, err
	}
	if !location.found {
		return nil, fmt.Errorf("%w : %s", ErrManagedMarkdownRegionNotFound, id)
	}
	block, err := buildManagedMarkdownRegion(id, body, location.start > 0)
	if err != nil {
		return nil, err
	}
	return replaceManagedMarkdownRegionAt(document, location, block), nil
}

func RemoveManagedMarkdownRegion(document []byte, id string) ([]byte, error) {
	location, err := locateManagedMarkdownRegion(document, id)
	if err != nil {
		return nil, err
	}
	if !location.found {
		return nil, fmt.Errorf("%w : %s", ErrManagedMarkdownRegionNotFound, id)
	}
	updated := make([]byte, 0, len(document)-(location.end-location.start))
	updated = append(updated, document[:location.start]...)
	updated = append(updated, document[location.end:]...)
	return updated, nil
}

func ManagedMarkdownRegionSHA256(document []byte, id string) (string, error) {
	location, err := locateManagedMarkdownRegion(document, id)
	if err != nil {
		return "", err
	}
	if !location.found {
		return "", fmt.Errorf("%w : %s", ErrManagedMarkdownRegionNotFound, id)
	}
	digest := sha256.Sum256(document[location.start:location.end])
	return fmt.Sprintf("%x", digest), nil
}

func WriteManagedMarkdownRegion(path, id string, body []byte) (ManagedRegion, error) {
	document, fileMode, err := readManagedMarkdownDocument(path, true)
	if err != nil {
		return ManagedRegion{}, err
	}
	updated, err := MergeManagedMarkdownRegion(document, id, body)
	if err != nil {
		return ManagedRegion{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return ManagedRegion{}, fmt.Errorf("failed to create directory for %s: %w", path, err)
	}
	if err := atomicWriteFile(path, updated, fileMode); err != nil {
		return ManagedRegion{}, fmt.Errorf("failed to write managed region to %s: %w", path, err)
	}
	regionSHA256, err := ManagedMarkdownRegionSHA256(updated, id)
	if err != nil {
		return ManagedRegion{}, err
	}
	return ManagedRegion{Path: path, ID: id, SHA256: regionSHA256}, nil
}

func RemoveManagedMarkdownRegionFile(path, id string) ([]byte, error) {
	document, fileMode, err := readManagedMarkdownDocument(path, false)
	if err != nil {
		return nil, err
	}
	updated, err := RemoveManagedMarkdownRegion(document, id)
	if err != nil {
		return nil, err
	}
	if err := atomicWriteFile(path, updated, fileMode); err != nil {
		return nil, fmt.Errorf("failed to remove managed region from %s: %w", path, err)
	}
	return updated, nil
}

func managedMarkdownRegionMarkers(id string) ([]byte, []byte, error) {
	if !validManagedMarkdownRegionID(id) {
		return nil, nil, fmt.Errorf("invalid managed Markdown region identifier: %q", id)
	}
	startMarker := []byte("<!-- sth:managed:" + id + ":start -->")
	endMarker := []byte("<!-- sth:managed:" + id + ":end -->")
	return startMarker, endMarker, nil
}

func managedMarkdownRegionSeparatorMarker(id string) []byte {
	return []byte("<!-- sth:managed:" + id + ":separator -->")
}

func validManagedMarkdownRegionID(id string) bool {
	if len(id) == 0 || id[0] < 'a' || id[0] > 'z' {
		return false
	}
	for _, character := range []byte(id[1:]) {
		if character >= 'a' && character <= 'z' {
			continue
		}
		if character >= '0' && character <= '9' {
			continue
		}
		if character == '-' {
			continue
		}
		return false
	}
	return true
}

func locateManagedMarkdownRegion(document []byte, id string) (managedMarkdownRegionLocation, error) {
	startMarker, endMarker, err := managedMarkdownRegionMarkers(id)
	if err != nil {
		return managedMarkdownRegionLocation{}, err
	}
	startCount := bytes.Count(document, startMarker)
	endCount := bytes.Count(document, endMarker)
	separatorMarker := managedMarkdownRegionSeparatorMarker(id)
	separatorCount := bytes.Count(document, separatorMarker)
	if startCount == 0 && endCount == 0 && separatorCount == 0 {
		return managedMarkdownRegionLocation{}, nil
	}
	if startCount != 1 || endCount != 1 || separatorCount > 1 {
		return managedMarkdownRegionLocation{}, fmt.Errorf("%w for %q: %d start marker(s), %d separator(s), %d end marker(s)", ErrManagedMarkdownRegionMalformed, id, startCount, separatorCount, endCount)
	}
	startPosition := bytes.Index(document, startMarker)
	endPosition := bytes.Index(document, endMarker)
	if endPosition < startPosition {
		return managedMarkdownRegionLocation{}, fmt.Errorf("%w for %q: end marker before start marker", ErrManagedMarkdownRegionMalformed, id)
	}
	managedStartPosition := startPosition
	if separatorCount == 1 {
		separatorPosition := bytes.Index(document, separatorMarker)
		expectedSeparatorPosition := startPosition + len(startMarker) + 1
		if startPosition == 0 || document[startPosition-1] != '\n' || separatorPosition != expectedSeparatorPosition {
			return managedMarkdownRegionLocation{}, fmt.Errorf("%w for %q: misplaced separator", ErrManagedMarkdownRegionMalformed, id)
		}
		managedStartPosition = startPosition - 1
	}
	return managedMarkdownRegionLocation{
		start: managedStartPosition,
		end:   endPosition + len(endMarker),
		found: true,
	}, nil
}

func replaceManagedMarkdownRegionAt(document []byte, location managedMarkdownRegionLocation, block []byte) []byte {
	updated := make([]byte, 0, len(document)-(location.end-location.start)+len(block))
	updated = append(updated, document[:location.start]...)
	updated = append(updated, block...)
	updated = append(updated, document[location.end:]...)
	return updated
}

func readManagedMarkdownDocument(path string, allowMissing bool) ([]byte, os.FileMode, error) {
	fileInfo, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) && allowMissing {
		return nil, 0o644, nil
	}
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read %s: %w", path, err)
	}
	if fileInfo.Mode()&os.ModeSymlink != 0 || !fileInfo.Mode().IsRegular() {
		return nil, 0, fmt.Errorf("invalid managed Markdown file: %s", path)
	}
	document, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read %s: %w", path, err)
	}
	fileMode := fileInfo.Mode().Perm() | fileInfo.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky)
	return document, fileMode, nil
}
