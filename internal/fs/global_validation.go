package fs

import (
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

type globalOwnedFile struct {
	File  GlobalFile
	Owner string
	Path  string
}

func GlobalFilePath(context GlobalContext, rootID, relativePath string) (string, error) {
	root, exists := context.Roots[rootID]
	if rootID == "provenance" {
		root = filepath.Join(context.StateRoot, "provenance")
		exists = true
	}
	if !exists {
		return "", fmt.Errorf("unknown global root %q", rootID)
	}
	if rootID != "provenance" {
		relativeRoot, err := filepath.Rel(context.Home, root)
		if err != nil || relativeRoot == "." || relativeRoot == ".." || strings.HasPrefix(relativeRoot, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("global root outside home: %s", root)
		}
		if err := validateGlobalCasePath(context.Home, filepath.ToSlash(relativeRoot)); err != nil {
			return "", err
		}
	}
	if err := validateGlobalRelativePath(relativePath); err != nil {
		return "", err
	}
	target := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := validateGlobalAbsolutePath(target); err != nil {
		return "", err
	}
	if err := validateGlobalCasePath(root, relativePath); err != nil {
		return "", err
	}
	return target, nil
}

func validateGlobalRelativePath(relativePath string) error {
	if !utf8.ValidString(relativePath) || len(relativePath) > 4096 || relativePath == "" || relativePath == "." || path.Clean(relativePath) != relativePath || path.IsAbs(relativePath) || strings.ContainsAny(relativePath, `\:*?"<>|`) {
		return fmt.Errorf("unsafe global relative path %q", relativePath)
	}
	for _, segment := range strings.Split(relativePath, "/") {
		if segment == "." || segment == ".." || segment == "" || len(segment) > 255 || strings.TrimRight(segment, ". ") != segment {
			return fmt.Errorf("unsafe global path segment %q", segment)
		}
		for _, character := range segment {
			if unicode.IsControl(character) {
				return fmt.Errorf("control character in global path")
			}
		}
		base := strings.ToUpper(strings.SplitN(segment, ".", 2)[0])
		reserved := base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || base == "CLOCK$" || (len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9')
		if strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT") {
			suffix := strings.TrimPrefix(strings.TrimPrefix(base, "COM"), "LPT")
			reserved = reserved || suffix == "¹" || suffix == "²" || suffix == "³"
		}
		if reserved {
			return fmt.Errorf("reserved Windows path segment %q", segment)
		}
	}
	return nil
}

func validateGlobalCasePath(root, relativePath string) error {
	current := root
	for _, segment := range strings.Split(relativePath, "/") {
		entries, err := os.ReadDir(current)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("inspect destination %s: %w", current, err)
		}
		for _, entry := range entries {
			if globalPathKey(entry.Name()) == globalPathKey(segment) && entry.Name() != segment {
				return fmt.Errorf("case collision at %s: %s and %s", current, entry.Name(), segment)
			}
		}
		current = filepath.Join(current, segment)
	}
	return nil
}

func globalPathKey(value string) string {
	return strings.ToLower(norm.NFC.String(value))
}

func validateGlobalState(context GlobalContext, state GlobalState) (map[string]globalOwnedFile, error) {
	if state.SchemaVersion != 1 {
		return nil, fmt.Errorf("unsupported global state schema version %d", state.SchemaVersion)
	}
	files := make(map[string]globalOwnedFile)
	identities := make(map[string]bool)
	placements := make(map[string]string)
	prefixes := make(map[string]string)
	for _, installation := range state.Installations {
		if installation.ID == "" || identities[installation.ID] {
			return nil, fmt.Errorf("missing or duplicate global installation identity %q", installation.ID)
		}
		identities[installation.ID] = true
		if err := validateGlobalRelativePath(installation.SkillName); err != nil || strings.Contains(installation.SkillName, "/") {
			return nil, fmt.Errorf("invalid global skill name %q", installation.SkillName)
		}
		for _, placement := range installation.Placements {
			root, exists := context.Roots[placement.RootID]
			if !exists || placement.RelativePath != installation.SkillName {
				return nil, fmt.Errorf("invalid placement for %s", installation.ID)
			}
			if placement.ResolvedRoot != root {
				return nil, fmt.Errorf("global root relocation for %s: recorded %q, current %q", installation.ID, placement.ResolvedRoot, root)
			}
			placementPath, err := GlobalFilePath(context, placement.RootID, placement.RelativePath)
			if err != nil {
				return nil, err
			}
			key := globalPathKey(placementPath)
			if owner, exists := placements[key]; exists {
				return nil, fmt.Errorf("source collision at %s between %s and %s", placementPath, owner, installation.ID)
			}
			placements[key] = installation.ID
			for _, file := range placement.Files {
				if file.RootID != placement.RootID || !strings.HasPrefix(file.RelativePath, placement.RelativePath+"/") {
					return nil, fmt.Errorf("file outside declared placement for %s: %s", installation.ID, file.RelativePath)
				}
				if err := addGlobalOwnedFile(context, files, prefixes, installation.ID, file); err != nil {
					return nil, err
				}
			}
		}
		for _, file := range installation.Provenance {
			if file.RootID != "provenance" {
				return nil, fmt.Errorf("invalid provenance root for %s", installation.ID)
			}
			if err := addGlobalOwnedFile(context, files, prefixes, installation.ID, file); err != nil {
				return nil, err
			}
		}
	}
	return files, nil
}

func addGlobalOwnedFile(context GlobalContext, files map[string]globalOwnedFile, prefixes map[string]string, owner string, file GlobalFile) error {
	target, err := GlobalFilePath(context, file.RootID, file.RelativePath)
	if err != nil {
		return err
	}
	hash, err := hex.DecodeString(file.SHA256)
	if err != nil || len(hash) != 32 || strings.ToLower(file.SHA256) != file.SHA256 || file.Mode&^0755 != 0 || file.Mode&0600 != 0600 {
		return fmt.Errorf("invalid file hash or mode: %s", target)
	}
	for prefix := target; filepath.Dir(prefix) != prefix; prefix = filepath.Dir(prefix) {
		key := globalPathKey(prefix)
		if previous, exists := prefixes[key]; exists && previous != prefix {
			return fmt.Errorf("case or Unicode collision between %s and %s", previous, prefix)
		}
		prefixes[key] = prefix
	}
	key := globalPathKey(target)
	if existing, exists := files[key]; exists {
		return fmt.Errorf("source or case collision at %s between %s and %s", target, existing.Owner, owner)
	}
	files[key] = globalOwnedFile{File: file, Owner: owner, Path: target}
	return nil
}

func InspectGlobalInstallation(context GlobalContext, installation GlobalInstallation) ([]GlobalFileStatus, error) {
	if _, err := validateGlobalState(context, GlobalState{SchemaVersion: 1, Installations: []GlobalInstallation{installation}}); err != nil {
		return nil, err
	}
	statuses := []GlobalFileStatus{}
	for _, placement := range installation.Placements {
		for _, file := range placement.Files {
			target, err := GlobalFilePath(context, file.RootID, file.RelativePath)
			if err != nil {
				return nil, err
			}
			status := GlobalFileStatus{GlobalFile: file, Path: target, Status: "ok"}
			content, err := os.ReadFile(target)
			if os.IsNotExist(err) {
				status.Status = "missing"
			} else if err != nil {
				return nil, err
			} else {
				info, err := os.Stat(target)
				if err != nil {
					return nil, err
				}
				if ComputeSHA256(content) != file.SHA256 || !globalModesEqual(uint32(info.Mode().Perm()), file.Mode) {
					status.Status = "modified-locally"
				}
			}
			statuses = append(statuses, status)
		}
	}
	return statuses, nil
}
