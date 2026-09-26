package fs

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeProjectConfigFile(t *testing.T, contents string) string {
	t.Helper()
	projectDir := t.TempDir()
	configDir := filepath.Join(projectDir, ".sth")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "project.json"), []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	withCwd(t, projectDir)
	return projectDir
}

func TestReadProjectProfileAcceptsCloudOnlyConfig(t *testing.T) {
	writeProjectConfigFile(t, `{
		"schema_version": 2,
		"providers": [{"id": "sth-cloud", "provider": "sth-cloud", "target_dir": ".claude/skills"}],
		"resources": []
	}`)

	profile, err := ReadProjectProfile()
	if err != nil {
		t.Fatalf("ReadProjectProfile() error = %v, want nil", err)
	}
	if profile.Provider != CloudProviderName {
		t.Fatalf("provider = %q, want %q", profile.Provider, CloudProviderName)
	}
	if profile.TargetDir != filepath.Clean(".claude/skills") {
		t.Fatalf("target_dir = %q, want .claude/skills", profile.TargetDir)
	}
}

func TestReadProjectProfileSkipsUnusableFirstProvider(t *testing.T) {
	writeProjectConfigFile(t, `{
		"schema_version": 2,
		"providers": [
			{"id": "broken", "provider": "github", "catalog_ref": "main", "catalog_path": "SKILLS.md", "target_dir": ".claude/skills"},
			{"id": "internal", "provider": "gitlab", "repository_id": "acme/skills", "catalog_ref": "main", "catalog_path": "SKILLS.md", "target_dir": ".claude/skills"}
		],
		"resources": []
	}`)

	profile, err := ReadProjectProfile()
	if err != nil {
		t.Fatalf("ReadProjectProfile() error = %v, want nil", err)
	}
	if profile.Provider != "gitlab" {
		t.Fatalf("provider = %q, want gitlab (first usable)", profile.Provider)
	}
}

func multiProviderConfig() ProjectConfigV2 {
	return ProjectConfigV2{
		SchemaVersion: 2,
		Providers: []ProviderEntry{
			{ID: "broken", Provider: "github", CatalogRef: "main", CatalogPath: "SKILLS.md", TargetDir: ".claude/skills"},
			{ID: "public", Provider: "github", RepositoryID: "octo/public", CatalogRef: "main", CatalogPath: "SKILLS.md", TargetDir: ".claude/skills"},
			{ID: "cloud", Provider: "sth-cloud", TargetDir: ".sth/cloud"},
		},
	}
}

func TestDefaultProviderIDSkipsUnusableProviders(t *testing.T) {
	if got := DefaultProviderID(multiProviderConfig()); got != "public" {
		t.Fatalf("DefaultProviderID() = %q, want public", got)
	}
}

func TestDefaultProviderIDFallsBackWhenNoProvider(t *testing.T) {
	if got := DefaultProviderID(ProjectConfigV2{SchemaVersion: 2}); got != FallbackProviderID {
		t.Fatalf("DefaultProviderID() = %q, want %q", got, FallbackProviderID)
	}
}

func TestProviderProfileResolvesEachProvider(t *testing.T) {
	config := multiProviderConfig()

	cloudProfile, err := ProviderProfile(config, "cloud")
	if err != nil {
		t.Fatalf("ProviderProfile(cloud) error = %v", err)
	}
	if cloudProfile.Provider != CloudProviderName || cloudProfile.TargetDir != filepath.Clean(".sth/cloud") {
		t.Fatalf("cloud profile = %+v", cloudProfile)
	}

	if _, err := ProviderProfile(config, "broken"); err == nil {
		t.Fatal("ProviderProfile(broken) error = nil, want misconfiguration error")
	}
	if _, err := ProviderProfile(config, "missing"); err == nil {
		t.Fatal("ProviderProfile(missing) error = nil, want unknown provider error")
	}
}

func TestStateTargetDirsDeduplicates(t *testing.T) {
	dirs := StateTargetDirs(multiProviderConfig())
	want := []string{".claude/skills", ".sth/cloud"}
	if len(dirs) != len(want) {
		t.Fatalf("StateTargetDirs() = %v, want %v", dirs, want)
	}
	for index := range want {
		if dirs[index] != want[index] {
			t.Fatalf("StateTargetDirs() = %v, want %v", dirs, want)
		}
	}
}

func TestParseCatalogRef(t *testing.T) {
	cases := []struct {
		input   string
		want    CatalogRef
		wantErr bool
	}{
		{input: "reviewer", want: CatalogRef{Name: "reviewer"}},
		{input: "agents/reviewer", want: CatalogRef{FolderName: "agents", Name: "reviewer"}},
		{input: "internal:reviewer", want: CatalogRef{ProviderID: "internal", Name: "reviewer"}},
		{input: "internal:agents/reviewer", want: CatalogRef{ProviderID: "internal", FolderName: "agents", Name: "reviewer"}},
		{input: "internal:", wantErr: true},
		{input: ":reviewer", wantErr: true},
		{input: "a/b/c", wantErr: true},
		{input: "   ", wantErr: true},
	}
	for _, testCase := range cases {
		got, err := ParseCatalogRef(testCase.input)
		if testCase.wantErr {
			if err == nil {
				t.Fatalf("ParseCatalogRef(%q) error = nil, want error", testCase.input)
			}
			continue
		}
		if err != nil {
			t.Fatalf("ParseCatalogRef(%q) error = %v", testCase.input, err)
		}
		if got != testCase.want {
			t.Fatalf("ParseCatalogRef(%q) = %+v, want %+v", testCase.input, got, testCase.want)
		}
	}
}

func crossProviderEntries() []CatalogEntry {
	return []CatalogEntry{
		{CatalogID: "agent::agents::reviewer", ProviderID: "public", ResourceType: "agent", Name: "reviewer", FolderName: "agents", SourcePath: "agents/reviewer.md"},
		{CatalogID: "agent::agents::reviewer", ProviderID: "internal", ResourceType: "agent", Name: "reviewer", FolderName: "agents", SourcePath: "agents/reviewer.md"},
		{CatalogID: "agent::agents::solo", ProviderID: "internal", ResourceType: "agent", Name: "solo", FolderName: "agents", SourcePath: "agents/solo.md"},
	}
}

func TestResolveCatalogEntryForProviderDisambiguates(t *testing.T) {
	entries := crossProviderEntries()

	entry, err := ResolveCatalogEntryForProvider(entries, "reviewer", "internal")
	if err != nil {
		t.Fatalf("ResolveCatalogEntryForProvider() error = %v", err)
	}
	if entry.ProviderID != "internal" {
		t.Fatalf("provider = %q, want internal", entry.ProviderID)
	}

	entry, err = ResolveCatalogEntryForProvider(entries, "public:agents/reviewer", "internal")
	if err != nil {
		t.Fatalf("ResolveCatalogEntryForProvider() error = %v", err)
	}
	if entry.ProviderID != "public" {
		t.Fatalf("provider = %q, want public (qualified ref wins over preference)", entry.ProviderID)
	}

	entry, err = ResolveCatalogEntryForProvider(entries, "solo", "")
	if err != nil {
		t.Fatalf("ResolveCatalogEntryForProvider() error = %v", err)
	}
	if entry.ProviderID != "internal" {
		t.Fatalf("provider = %q, want internal", entry.ProviderID)
	}
}

func TestResolveCatalogEntryForProviderReportsAmbiguity(t *testing.T) {
	_, err := ResolveCatalogEntryForProvider(crossProviderEntries(), "reviewer", "")
	var ambiguous *AmbiguousCatalogRefError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("error = %v, want AmbiguousCatalogRefError", err)
	}
	if len(ambiguous.Candidates) != 2 {
		t.Fatalf("candidates = %d, want 2", len(ambiguous.Candidates))
	}
}

func TestResolveCatalogEntryForProviderUnknownProvider(t *testing.T) {
	if _, err := ResolveCatalogEntryForProvider(crossProviderEntries(), "reviewer", "nope"); err == nil {
		t.Fatal("error = nil, want not-found error")
	}
}
