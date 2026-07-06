package fs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFixtureFile(t *testing.T, root string, relativePath, content string) {
	t.Helper()
	fullPath := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWalkLocalSkillTree_ListsFilesAndDirsWithSlashPaths(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "prompts/code-review/SKILL.md", "content")
	writeFixtureFile(t, root, "prompts/deep-research/SKILL.md", "content")
	writeFixtureFile(t, root, "conventions/go-style/SKILL.md", "content")
	writeFixtureFile(t, root, "README.md", "noise")

	entries, err := WalkLocalSkillTree(root)
	if err != nil {
		t.Fatalf("WalkLocalSkillTree: %v", err)
	}

	paths := make(map[string]string, len(entries))
	for _, entry := range entries {
		paths[entry.Path] = entry.Type
	}

	if paths["prompts/code-review/SKILL.md"] != "blob" {
		t.Errorf("expected blob at prompts/code-review/SKILL.md, got %q", paths["prompts/code-review/SKILL.md"])
	}
	if paths["prompts"] != "tree" {
		t.Errorf("expected tree at prompts, got %q", paths["prompts"])
	}
	if paths["README.md"] != "blob" {
		t.Errorf("expected blob at README.md, got %q", paths["README.md"])
	}
}

func TestWalkLocalSkillTree_SkipsHiddenDirectories(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, ".git/hooks/SKILL.md", "trap")
	writeFixtureFile(t, root, ".claude/skills/SKILL.md", "trap")
	writeFixtureFile(t, root, "prompts/code-review/SKILL.md", "content")

	entries, err := WalkLocalSkillTree(root)
	if err != nil {
		t.Fatalf("WalkLocalSkillTree: %v", err)
	}

	for _, entry := range entries {
		if strings.HasPrefix(entry.Path, ".git") || strings.HasPrefix(entry.Path, ".claude") {
			t.Errorf("hidden directory leaked into tree: %s", entry.Path)
		}
	}
}

func TestReadSkillDescription_ReadsFrontMatter(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "prompts/code-review/SKILL.md", "---\nname: code-review\ndescription: Review a diff for bugs\n---\n\nBody.\n")

	description := ReadSkillDescription(filepath.Join(root, "prompts", "code-review", "SKILL.md"))
	if description != "Review a diff for bugs" {
		t.Errorf("description = %q, want %q", description, "Review a diff for bugs")
	}
}

func TestReadSkillDescription_EmptyWhenNoFrontMatterOrMissingFile(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "prompts/raw/SKILL.md", "No front matter here.\n")

	if description := ReadSkillDescription(filepath.Join(root, "prompts", "raw", "SKILL.md")); description != "" {
		t.Errorf("description = %q, want empty", description)
	}
	if description := ReadSkillDescription(filepath.Join(root, "does", "not", "exist.md")); description != "" {
		t.Errorf("description for missing file = %q, want empty", description)
	}
}

func TestRenderSkillsMarkdown_EmitsSingleTable(t *testing.T) {
	entries := []CatalogEntry{
		{ResourceType: "skill", Name: "go-style", FolderName: "conventions", SourcePath: "conventions/go-style/SKILL.md", Description: ""},
		{ResourceType: "skill", Name: "code-review", FolderName: "prompts", SourcePath: "prompts/code-review/SKILL.md", Description: "Review a diff"},
	}

	rendered := RenderSkillsMarkdown(entries)

	if !strings.Contains(rendered, "| Type | Name | Folder | Source | Description |") {
		t.Error("missing table header")
	}
	if !strings.Contains(rendered, "| skill | code-review | prompts | prompts/code-review/SKILL.md | Review a diff |") {
		t.Error("missing code-review row")
	}
	if !strings.Contains(rendered, "| skill | go-style | conventions | conventions/go-style/SKILL.md |  |") {
		t.Error("missing go-style row with empty description")
	}
	if strings.Count(rendered, "| ---- |") != 1 {
		t.Error("expected exactly one table separator row")
	}
}

func TestRenderSkillsMarkdown_SanitizesPipesAndNewlines(t *testing.T) {
	entries := []CatalogEntry{
		{ResourceType: "skill", Name: "tricky", FolderName: "prompts", SourcePath: "prompts/tricky/SKILL.md", Description: "left | right\nsecond line"},
	}

	rendered := RenderSkillsMarkdown(entries)

	if !strings.Contains(rendered, "| left / right second line |") {
		t.Errorf("description not sanitized: %s", rendered)
	}
}
