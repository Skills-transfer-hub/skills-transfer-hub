package scanner

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

// FlatSkillScanner discovers skill bundles laid out as `category/skill/SKILL.md`.
// This is the historical STH layout and the convention used by the anthropics/skills
// reference repository.
type FlatSkillScanner struct{}

// Name implements RepoScanner.
func (FlatSkillScanner) Name() string { return "flat" }

// Detect returns true when at least one entry matches the flat skill convention.
// The cheap HasSuffix check avoids the path normalization cost of
// IsFlatSkillPath for the typical case where most tree entries are not skills.
func (FlatSkillScanner) Detect(tree []TreeEntry) bool {
	for _, entry := range tree {
		if !strings.HasSuffix(entry.Path, "SKILL.md") {
			continue
		}
		if IsFlatSkillPath(entry.Path) {
			return true
		}
	}
	return false
}

// Scan returns one Artifact per discovered `category/skill/SKILL.md` entry.
// Entries are deduplicated and sorted by (category, name).
// Same HasSuffix guard as Detect — keeps the per-entry cost flat on large
// trees where the vast majority of paths aren't skills. Directory entries
// (Type == "tree" / "directory") are also skipped so a folder literally
// named SKILL.md never gets mistaken for a skill entrypoint.
func (FlatSkillScanner) Scan(tree []TreeEntry) ([]Artifact, error) {
	seen := make(map[string]Artifact)
	for _, entry := range tree {
		if !strings.HasSuffix(entry.Path, "SKILL.md") {
			continue
		}
		if isDirEntry(entry.Type) {
			continue
		}
		category, name, err := SplitFlatSkillPath(entry.Path)
		if err != nil {
			continue
		}
		key := category + "/" + name
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = Artifact{
			Kind:       KindSkill,
			Name:       name,
			Category:   category,
			Files:      []string{path.Join(category, name, "SKILL.md")},
			Entrypoint: path.Join(category, name, "SKILL.md"),
		}
	}

	if len(seen) == 0 {
		return nil, fmt.Errorf("aucun bundle compatible détecté dans l'arborescence du dépôt")
	}

	results := make([]Artifact, 0, len(seen))
	for _, artifact := range seen {
		results = append(results, artifact)
	}
	sort.Slice(results, func(left, right int) bool {
		if results[left].Category == results[right].Category {
			return results[left].Name < results[right].Name
		}
		return results[left].Category < results[right].Category
	})
	return results, nil
}

// IsFlatSkillPath reports whether resourcePath matches the flat skill convention.
func IsFlatSkillPath(resourcePath string) bool {
	category, name, err := SplitFlatSkillPath(resourcePath)
	return err == nil && category != "" && name != ""
}

// SplitFlatSkillPath splits a `category/skill/SKILL.md` path into (category, skill).
// It returns an error when the path does not match the convention.
func SplitFlatSkillPath(resourcePath string) (string, string, error) {
	normalized := NormalizePath(resourcePath)
	if normalized == "" || path.IsAbs(normalized) || normalized == ".." || strings.HasPrefix(normalized, "../") {
		return "", "", fmt.Errorf("resource path invalide : %q", resourcePath)
	}

	parts := strings.Split(normalized, "/")
	if len(parts) != 3 || parts[2] != "SKILL.md" {
		return "", "", fmt.Errorf("resource path incompatible : %q", resourcePath)
	}
	if strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", fmt.Errorf("resource path incompatible : %q", resourcePath)
	}

	return parts[0], parts[1], nil
}

// NormalizePath cleans a resource path: trims whitespace, converts backslashes
// to forward slashes, and resolves redundant separators.
func NormalizePath(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	return path.Clean(strings.ReplaceAll(trimmed, "\\", "/"))
}

// isDirEntry reports whether a TreeEntry.Type marks a directory rather than
// a file. Upstream providers use different vocabularies ("tree" on GitHub,
// "directory" on Bitbucket/Azure); the substring match catches all known
// dialects without coupling the scanner to a specific provider.
func isDirEntry(entryType string) bool {
	normalized := strings.ToLower(strings.TrimSpace(entryType))
	return strings.Contains(normalized, "tree") || strings.Contains(normalized, "directory")
}
