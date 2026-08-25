package fs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sth/internal/scanner"
)

func WalkLocalSkillTree(root string) ([]scanner.TreeEntry, error) {
	entries := make([]scanner.TreeEntry, 0)
	walkErr := filepath.WalkDir(root, func(currentPath string, dirEntry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if currentPath == root {
			return nil
		}
		if dirEntry.IsDir() && strings.HasPrefix(dirEntry.Name(), ".") {
			return filepath.SkipDir
		}
		relativePath, relErr := filepath.Rel(root, currentPath)
		if relErr != nil {
			return relErr
		}
		entryType := "blob"
		if dirEntry.IsDir() {
			entryType = "tree"
		}
		entries = append(entries, scanner.TreeEntry{
			Path: filepath.ToSlash(relativePath),
			Type: entryType,
		})
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("failed to scan %s: %w", root, walkErr)
	}
	return entries, nil
}

func ReadSkillDescription(skillMDPath string) string {
	content, err := os.ReadFile(skillMDPath)
	if err != nil {
		return ""
	}
	frontmatter, found := scanner.ExtractFrontmatter(content)
	if !found {
		return ""
	}
	return frontmatter.Description
}

func RenderSkillsMarkdown(entries []CatalogEntry) string {
	var builder strings.Builder
	builder.WriteString("# SKILLS.md\n\n")
	builder.WriteString("| Type | Name | Folder | Source | Description |\n")
	builder.WriteString("| ---- | ---- | ------ | ------ | ----------- |\n")
	for _, entry := range entries {
		builder.WriteString(fmt.Sprintf(
			"| %s | %s | %s | %s | %s |\n",
			sanitizeTableCell(entry.ResourceType),
			sanitizeTableCell(entry.Name),
			sanitizeTableCell(entry.FolderName),
			sanitizeTableCell(entry.SourcePath),
			sanitizeTableCell(entry.Description),
		))
	}
	return builder.String()
}

func sanitizeTableCell(value string) string {
	cleaned := strings.ReplaceAll(value, "|", "/")
	cleaned = strings.ReplaceAll(cleaned, "\n", " ")
	return strings.TrimSpace(cleaned)
}
