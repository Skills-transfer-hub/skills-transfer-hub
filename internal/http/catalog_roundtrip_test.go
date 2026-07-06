package http

import (
	"testing"

	"sth/internal/fs"
)

func TestRenderSkillsMarkdown_RoundTripsThroughCatalogParser(t *testing.T) {
	generated := []fs.CatalogEntry{
		{ResourceType: "skill", Name: "go-style", FolderName: "conventions", SourcePath: "conventions/go-style/SKILL.md"},
		{ResourceType: "skill", Name: "code-review", FolderName: "prompts", SourcePath: "prompts/code-review/SKILL.md", Description: "Review a diff for bugs"},
		{ResourceType: "skill", Name: "deep-research", FolderName: "prompts", SourcePath: "prompts/deep-research/SKILL.md", Description: "Multi-source research"},
	}

	parsed, err := parseCatalogEntries(fs.RenderSkillsMarkdown(generated))
	if err != nil {
		t.Fatalf("parseCatalogEntries sur un SKILLS.md généré : %v", err)
	}

	if len(parsed) != len(generated) {
		t.Fatalf("len(parsed) = %d, want %d", len(parsed), len(generated))
	}
	for index, parsedEntry := range parsed {
		expected := generated[index]
		if parsedEntry.Name != expected.Name ||
			parsedEntry.FolderName != expected.FolderName ||
			parsedEntry.SourcePath != expected.SourcePath ||
			parsedEntry.ResourceType != expected.ResourceType ||
			parsedEntry.Description != expected.Description {
			t.Errorf("parsed[%d] = %+v, want %+v", index, parsedEntry, expected)
		}
	}
}

func TestRenderSkillsMarkdown_RoundTripPreservesEmptyDescription(t *testing.T) {
	generated := []fs.CatalogEntry{
		{ResourceType: "skill", Name: "bare", FolderName: "prompts", SourcePath: "prompts/bare/SKILL.md"},
	}

	parsed, err := parseCatalogEntries(fs.RenderSkillsMarkdown(generated))
	if err != nil {
		t.Fatalf("parseCatalogEntries : %v", err)
	}
	if len(parsed) != 1 || parsed[0].Description != "" {
		t.Fatalf("parsed = %+v", parsed)
	}
}
