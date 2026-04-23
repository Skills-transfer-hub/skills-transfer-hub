package http

import "testing"

func assertDiscoveryPath(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("discovery path = %q, want %q", got, want)
	}
}

func TestSplitSkillEntrypoint_acceptsCategoryFolderSkillPath(t *testing.T) {
	categoryName, folderName, err := splitSkillEntrypoint("react/form-wizard/SKILL.md")
	if err != nil {
		t.Fatalf("splitSkillEntrypoint() error = %v", err)
	}
	assertDiscoveryPath(t, categoryName, "react")
	assertDiscoveryPath(t, folderName, "form-wizard")
}

func TestSplitSkillEntrypoint_rejectsInvalidLayouts(t *testing.T) {
	for _, resourcePath := range []string{"react/SKILL.md", "../react/form-wizard/SKILL.md", "react/form-wizard/readme.md"} {
		if _, _, err := splitSkillEntrypoint(resourcePath); err == nil {
			t.Fatalf("splitSkillEntrypoint(%q) error = nil, want error", resourcePath)
		}
	}
}

func TestNewCatalogDiscoverySource_setsPrimaryPrecedence(t *testing.T) {
	source := newCatalogDiscoverySource("SKILLS.md", "catalog")
	if source.Kind != discoveryKindCatalog {
		t.Fatalf("source.Kind = %q, want %q", source.Kind, discoveryKindCatalog)
	}
	if source.Precedence != 1 {
		t.Fatalf("source.Precedence = %d, want 1", source.Precedence)
	}
}

func TestCatalogEntriesFromRepositoryTree_discoversSkillBundles(t *testing.T) {
	entries, err := catalogEntriesFromRepositoryTree([]repositoryTreeEntry{
		{Path: "react/form-wizard/SKILL.md", Type: "blob"},
		{Path: "agents/reviewer/SKILL.md", Type: "blob"},
		{Path: "react/form-wizard/README.md", Type: "blob"},
	})
	if err != nil {
		t.Fatalf("catalogEntriesFromRepositoryTree() error = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}
	if entries[0].CatalogID != "bundle::agents::reviewer" {
		t.Fatalf("entries[0].CatalogID = %q, want bundle::agents::reviewer", entries[0].CatalogID)
	}
	if entries[0].FolderName != "agents" || entries[0].Name != "reviewer" {
		t.Fatalf("entries[0] = %+v, want folder=agents name=reviewer", entries[0])
	}
	if entries[1].SourcePath != "react/form-wizard/SKILL.md" {
		t.Fatalf("entries[1].SourcePath = %q, want react/form-wizard/SKILL.md", entries[1].SourcePath)
	}
}