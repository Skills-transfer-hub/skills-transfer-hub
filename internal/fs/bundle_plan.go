package fs

import (
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

func PlanManagedBundle(targetDir string, entry CatalogEntry, files map[string][]byte) ([]ManagedFile, error) {
	if err := validateTargetDir(targetDir); err != nil {
		return nil, err
	}
	if err := validateCatalogEntry(entry); err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no bundle files to synchronize for %s", entry.CatalogID)
	}

	sourceRoot := path.Dir(path.Clean(strings.ReplaceAll(strings.TrimSpace(entry.SourcePath), "\\", "/")))
	if sourceRoot == "." || sourceRoot == "/" || sourceRoot == "" {
		return nil, fmt.Errorf("invalid bundle source_path: %q", entry.SourcePath)
	}
	managedRoot, err := BuildManagedRoot(targetDir, entry.FolderName, entry.Name)
	if err != nil {
		return nil, err
	}

	sourcePaths := make([]string, 0, len(files))
	for sourcePath := range files {
		sourcePaths = append(sourcePaths, sourcePath)
	}
	sort.Strings(sourcePaths)

	plannedFiles := make([]ManagedFile, 0, len(sourcePaths))
	for _, sourcePath := range sourcePaths {
		normalizedSourcePath := path.Clean(strings.ReplaceAll(strings.TrimSpace(sourcePath), "\\", "/"))
		relativePath, err := relativeRepositoryPath(sourceRoot, normalizedSourcePath)
		if err != nil {
			return nil, fmt.Errorf("unable to resolve bundle file %s: %w", sourcePath, err)
		}
		installedPath, err := ResolveManagedPath(managedRoot, relativePath)
		if err != nil {
			return nil, err
		}
		content := addManagedHeaderIfMarkdown(installedPath, files[sourcePath])
		plannedFiles = append(plannedFiles, ManagedFile{
			Path:   filepath.ToSlash(installedPath),
			SHA256: ComputeSHA256(content),
		})
	}
	return plannedFiles, nil
}
