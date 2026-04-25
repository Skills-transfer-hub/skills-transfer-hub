package fs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type testPaths struct {
	homeDir    string
	projectDir string
	targetDir  string
}

func newTestCatalogEntry(resourceType, folderName, name, sourcePath string) CatalogEntry {
	return CatalogEntry{
		CatalogID:    CatalogID(resourceType, folderName, name),
		ResourceType: resourceType,
		Name:         name,
		FolderName:   folderName,
		SourcePath:   sourcePath,
	}
}

func writeInstalledStateForTest(t *testing.T, targetDir string, state InstalledResourceState) string {
	t.Helper()

	statePath := filepath.Join(filepath.Clean(targetDir), stateFileName)
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", filepath.Dir(statePath), err)
	}

	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("json.Marshal(): %v", err)
	}
	if err := os.WriteFile(statePath, data, 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", statePath, err)
	}

	return statePath
}

func newTestPaths(t *testing.T) testPaths {
	t.Helper()

	homeDir := t.TempDir()
	projectDir := t.TempDir()

	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	t.Setenv("HOMEDRIVE", filepath.VolumeName(homeDir))
	t.Setenv("HOMEPATH", strings.TrimPrefix(homeDir, filepath.VolumeName(homeDir)))

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(projectDir); err != nil {
		t.Fatalf("chdir %s: %v", projectDir, err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	return testPaths{
		homeDir:    homeDir,
		projectDir: projectDir,
		targetDir:  ".claude/resources",
	}
}

func TestWriteDefaultConfig_createsDefaultJSON(t *testing.T) {
	paths := newTestPaths(t)

	configPath, err := WriteDefaultConfig()
	if err != nil {
		t.Fatalf("WriteDefaultConfig() error = %v", err)
	}
	if configPath != filepath.Join(paths.homeDir, ".sthrc") {
		t.Fatalf("configPath = %s, want %s", configPath, filepath.Join(paths.homeDir, ".sthrc"))
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", configPath, err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("json.Unmarshal(): %v", err)
	}
	if config.RepoRawURL == "" {
		t.Fatal("RepoRawURL is empty")
	}
	if config.TargetDir == "" {
		t.Fatal("TargetDir is empty")
	}
}

func TestWriteDefaultConfig_refusesToOverwriteExistingConfig(t *testing.T) {
	paths := newTestPaths(t)
	configPath := filepath.Join(paths.homeDir, ".sthrc")
	original := []byte("{\"repo_raw_url\":\"https://example.invalid\",\"target_dir\":\".claude/skills\"}\n")
	if err := os.WriteFile(configPath, original, 0o600); err != nil {
		t.Fatalf("WriteFile(%s): %v", configPath, err)
	}

	_, err := WriteDefaultConfig()
	if err == nil {
		t.Fatal("WriteDefaultConfig() error = nil, want error")
	}

	data, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatalf("ReadFile(%s): %v", configPath, readErr)
	}
	if string(data) != string(original) {
		t.Fatalf("config was modified:\n got: %q\nwant: %q", data, original)
	}
}

func TestWriteProjectProfile_roundTrips(t *testing.T) {
	paths := newTestPaths(t)
	profile := ProjectProfile{
		Provider:     "github",
		RepositoryID: "owner/repo",
		Ref:          "main",
		CatalogPath:  "SKILLS.md",
		TargetDir:    ".claude/resources",
	}

	profilePath, err := WriteProjectProfile(profile)
	if err != nil {
		t.Fatalf("WriteProjectProfile() error = %v", err)
	}
	expectedPath := filepath.Join(paths.projectDir, ".sth", "project.json")
	profileInfo, err := os.Stat(profilePath)
	if err != nil {
		t.Fatalf("Stat(%s): %v", profilePath, err)
	}
	expectedInfo, err := os.Stat(expectedPath)
	if err != nil {
		t.Fatalf("Stat(%s): %v", expectedPath, err)
	}
	if !os.SameFile(profileInfo, expectedInfo) {
		t.Fatalf("profilePath = %s, want same file as %s", profilePath, expectedPath)
	}

	loaded, err := ReadProjectProfile()
	if err != nil {
		t.Fatalf("ReadProjectProfile() error = %v", err)
	}
	if loaded.Provider != profile.Provider || loaded.RepositoryID != profile.RepositoryID || loaded.Ref != profile.Ref || loaded.CatalogPath != profile.CatalogPath || loaded.TargetDir != filepath.Clean(profile.TargetDir) || loaded.MigratedFromLegacy != profile.MigratedFromLegacy {
		t.Fatalf("loaded profile = %#v, want %#v", *loaded, profile)
	}
}

func TestLegacyProfileFromConfig_parsesGitHubRawURL(t *testing.T) {
	profile, err := LegacyProfileFromConfig(Config{
		RepoRawURL: "https://raw.githubusercontent.com/owner/repo/main/skills",
		TargetDir:  ".claude/resources",
	})
	if err != nil {
		t.Fatalf("LegacyProfileFromConfig() error = %v", err)
	}
	if profile.Provider != "github" {
		t.Fatalf("profile.Provider = %q, want github", profile.Provider)
	}
	if profile.RepositoryID != "owner/repo" {
		t.Fatalf("profile.RepositoryID = %q, want owner/repo", profile.RepositoryID)
	}
	if profile.CatalogPath != "skills/SKILLS.md" {
		t.Fatalf("profile.CatalogPath = %q, want skills/SKILLS.md", profile.CatalogPath)
	}
}

func TestInstallCatalogEntry_writesManagedHeaderAndState(t *testing.T) {
	paths := newTestPaths(t)
	entry := CatalogEntry{
		CatalogID:    CatalogID("skill", "skills", "alpha"),
		ResourceType: "skill",
		Name:         "alpha",
		FolderName:   "skills",
		SourcePath:   "skills/alpha.md",
	}

	record, err := InstallCatalogEntry(paths.targetDir, entry, []byte("# Alpha\n"))
	if err != nil {
		t.Fatalf("InstallCatalogEntry() error = %v", err)
	}
	if record.InstalledPath != ".claude/resources/skills/alpha.md" {
		t.Fatalf("record.InstalledPath = %q, want .claude/resources/skills/alpha.md", record.InstalledPath)
	}

	data, err := os.ReadFile(filepath.Join(paths.projectDir, ".claude", "resources", "skills", "alpha.md"))
	if err != nil {
		t.Fatalf("ReadFile(installed resource): %v", err)
	}
	if !strings.HasPrefix(string(data), skillHeader) {
		t.Fatalf("installed resource %q does not contain managed header", data)
	}

	state, err := ReadInstalledState(paths.targetDir)
	if err != nil {
		t.Fatalf("ReadInstalledState() error = %v", err)
	}
	if len(state.Entries) != 1 {
		t.Fatalf("len(state.Entries) = %d, want 1", len(state.Entries))
	}
	if state.Entries[0].CatalogID != entry.CatalogID {
		t.Fatalf("state.Entries[0].CatalogID = %q, want %q", state.Entries[0].CatalogID, entry.CatalogID)
	}
}

func TestReadInstalledState_missingFileReturnsEmptyState(t *testing.T) {
	paths := newTestPaths(t)

	state, err := ReadInstalledState(paths.targetDir)
	if err != nil {
		t.Fatalf("ReadInstalledState() error = %v", err)
	}
	if len(state.Entries) != 0 {
		t.Fatalf("state.Entries = %#v, want empty", state.Entries)
	}
}

func TestReadInstalledState_readsLegacyMap(t *testing.T) {
	paths := newTestPaths(t)
	targetPath := filepath.Join(paths.projectDir, ".claude", "resources")
	if err := os.MkdirAll(targetPath, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", targetPath, err)
	}
	statePath := filepath.Join(targetPath, "sth-state.json")
	legacyState, err := json.Marshal(map[string]string{"alpha": "installed"})
	if err != nil {
		t.Fatalf("json.Marshal(): %v", err)
	}
	if err := os.WriteFile(statePath, legacyState, 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", statePath, err)
	}

	state, err := ReadInstalledState(paths.targetDir)
	if err != nil {
		t.Fatalf("ReadInstalledState() error = %v", err)
	}
	if len(state.Entries) != 1 {
		t.Fatalf("len(state.Entries) = %d, want 1", len(state.Entries))
	}
	if state.Entries[0].SourcePath != "skills/alpha.md" {
		t.Fatalf("state.Entries[0].SourcePath = %q, want skills/alpha.md", state.Entries[0].SourcePath)
	}
}

// T034
func TestInstallCatalogEntryV2_WritesManagedFiles(t *testing.T) {
	paths := newTestPaths(t)
	entry := CatalogEntry{
		CatalogID:    CatalogID("skill", "coding", "test"),
		ResourceType: "skill",
		Name:         "test",
		FolderName:   "coding",
		SourcePath:   "coding/test.md",
	}
	content := []byte("# Test Skill\nsome content")

	_, err := InstallCatalogEntryV2(paths.targetDir, entry, content, "default")
	if err != nil {
		t.Fatalf("InstallCatalogEntryV2() error = %v", err)
	}

	state, err := ReadInstalledStateV2(paths.targetDir)
	if err != nil {
		t.Fatalf("ReadInstalledStateV2() error = %v", err)
	}
	if len(state.Entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(state.Entries))
	}
	if len(state.Entries[0].ManagedFiles) != 1 {
		t.Fatalf("len(managed_files) = %d, want 1", len(state.Entries[0].ManagedFiles))
	}

	// .md file gets skillHeader prepended; compute expected SHA256 of actual written bytes.
	fileContent := append([]byte(skillHeader), content...)
	expectedSHA := ComputeSHA256(fileContent)
	if got := state.Entries[0].ManagedFiles[0].SHA256; got != expectedSHA {
		t.Fatalf("SHA256 = %q, want %q", got, expectedSHA)
	}
	if path := state.Entries[0].ManagedFiles[0].Path; path == "" {
		t.Fatal("ManagedFiles[0].Path is empty")
	}
}

// T035
func TestInstallCatalogEntryV2_NonMarkdown_NoHeader(t *testing.T) {
	paths := newTestPaths(t)
	entry := CatalogEntry{
		CatalogID:    CatalogID("skill", "coding", "test"),
		ResourceType: "skill",
		Name:         "test",
		FolderName:   "coding",
		SourcePath:   "coding/test.txt",
	}
	content := []byte("some plain text content")

	_, err := InstallCatalogEntryV2(paths.targetDir, entry, content, "default")
	if err != nil {
		t.Fatalf("InstallCatalogEntryV2() error = %v", err)
	}

	state, err := ReadInstalledStateV2(paths.targetDir)
	if err != nil {
		t.Fatalf("ReadInstalledStateV2() error = %v", err)
	}
	if len(state.Entries) != 1 || len(state.Entries[0].ManagedFiles) != 1 {
		t.Fatalf("unexpected state: %+v", state.Entries)
	}

	// For non-.md files, no header is added; SHA256 must match raw content.
	expectedSHA := ComputeSHA256(content)
	if got := state.Entries[0].ManagedFiles[0].SHA256; got != expectedSHA {
		t.Fatalf("SHA256 = %q, want %q (no header expected for .txt)", got, expectedSHA)
	}
}

// T037
func TestInstallCatalogEntryV2_SHA256MatchesActualFileContent(t *testing.T) {
	paths := newTestPaths(t)
	entry := CatalogEntry{
		CatalogID:    CatalogID("skill", "skills", "beta"),
		ResourceType: "skill",
		Name:         "beta",
		FolderName:   "skills",
		SourcePath:   "skills/beta.md",
	}
	content := []byte("# Beta Skill\ncontent here")

	_, err := InstallCatalogEntryV2(paths.targetDir, entry, content, "default")
	if err != nil {
		t.Fatalf("InstallCatalogEntryV2() error = %v", err)
	}

	// Read actual bytes written to disk.
	installedPath := filepath.Join(paths.projectDir, ".claude", "resources", "skills", "beta.md")
	fileBytes, err := os.ReadFile(installedPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", installedPath, err)
	}

	computedSHA := ComputeSHA256(fileBytes)

	state, err := ReadInstalledStateV2(paths.targetDir)
	if err != nil {
		t.Fatalf("ReadInstalledStateV2() error = %v", err)
	}
	if len(state.Entries) != 1 || len(state.Entries[0].ManagedFiles) != 1 {
		t.Fatalf("unexpected state: %+v", state.Entries)
	}
	if got := state.Entries[0].ManagedFiles[0].SHA256; got != computedSHA {
		t.Fatalf("stored SHA256 %q != computed SHA256 %q", got, computedSHA)
	}
}

// SKILL.md with YAML frontmatter must remain Claude-compatible after install:
// the `---` opening fence stays on line 1 and the STH header is injected
// after the closing fence rather than before the opening one.
func TestInstallCatalogEntryV2_PreservesFrontmatterOnLineOne(t *testing.T) {
	paths := newTestPaths(t)
	entry := CatalogEntry{
		CatalogID:    CatalogID("skill", "react", "form-wizard"),
		ResourceType: "skill",
		Name:         "form-wizard",
		FolderName:   "react",
		SourcePath:   "react/form-wizard/SKILL.md",
	}
	content := []byte("---\nname: form-wizard\ndescription: builds React forms\n---\n# Body\n")

	_, err := InstallCatalogEntryV2(paths.targetDir, entry, content, "default")
	if err != nil {
		t.Fatalf("InstallCatalogEntryV2() error = %v", err)
	}

	installedPath := filepath.Join(paths.projectDir, ".claude", "resources", "react", "SKILL.md")
	fileBytes, err := os.ReadFile(installedPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", installedPath, err)
	}

	if !strings.HasPrefix(string(fileBytes), "---\n") {
		t.Fatalf("first line of installed SKILL.md must be `---`; got:\n%s", fileBytes)
	}
	if !strings.Contains(string(fileBytes), "---\nname: form-wizard\ndescription: builds React forms\n---\n") {
		t.Fatalf("frontmatter block was altered on disk; got:\n%s", fileBytes)
	}
	if !strings.Contains(string(fileBytes), skillHeader) {
		t.Fatalf("STH managed-header is missing from installed SKILL.md; got:\n%s", fileBytes)
	}
	// Header must sit between closing fence and body, not at top.
	idxOpen := strings.Index(string(fileBytes), "---\nname:")
	idxHeader := strings.Index(string(fileBytes), skillHeader)
	if idxHeader < idxOpen {
		t.Fatalf("STH header injected before frontmatter; got:\n%s", fileBytes)
	}
}

// T038 – RefreshManagedFileHashes updates SHA256 for markdown (header included)
func TestRefreshManagedFileHashes_MarkdownIncludesHeader(t *testing.T) {
	record := InstalledResourceRecordV2{
		ResourceName: "alpha",
		ManagedFiles: []ManagedFile{{Path: ".claude/skills/skills/alpha.md", SHA256: "old"}},
	}
	content := []byte("# Alpha\ncontent")
	updated := RefreshManagedFileHashes(record, content)

	expectedContent := addManagedHeaderIfMarkdown(".claude/skills/skills/alpha.md", content)
	want := ComputeSHA256(expectedContent)
	if got := updated.ManagedFiles[0].SHA256; got != want {
		t.Errorf("SHA256 = %q, want %q", got, want)
	}
}

// T039 – RefreshManagedFileHashes updates SHA256 for non-markdown (raw content)
func TestRefreshManagedFileHashes_NonMarkdownRawHash(t *testing.T) {
	record := InstalledResourceRecordV2{
		ResourceName: "cfg",
		ManagedFiles: []ManagedFile{{Path: ".claude/skills/cfg/config.txt", SHA256: "old"}},
	}
	content := []byte("config=value")
	updated := RefreshManagedFileHashes(record, content)

	want := ComputeSHA256(content)
	if got := updated.ManagedFiles[0].SHA256; got != want {
		t.Errorf("SHA256 = %q, want %q", got, want)
	}
}

// T040 – RefreshManagedFileHashes is a no-op for records with no managed files
func TestRefreshManagedFileHashes_NoManagedFiles(t *testing.T) {
	record := InstalledResourceRecordV2{ResourceName: "empty"}
	updated := RefreshManagedFileHashes(record, []byte("data"))
	if len(updated.ManagedFiles) != 0 {
		t.Errorf("expected 0 managed files, got %d", len(updated.ManagedFiles))
	}
}

func TestResolveCatalogEntry_reportsAmbiguousNames(t *testing.T) {
	_, err := ResolveCatalogEntry([]CatalogEntry{
		{CatalogID: CatalogID("skill", "skills", "alpha"), Name: "alpha", FolderName: "skills", ResourceType: "skill", SourcePath: "skills/alpha.md"},
		{CatalogID: CatalogID("agent", "agents", "alpha"), Name: "alpha", FolderName: "agents", ResourceType: "agent", SourcePath: "agents/alpha.md"},
	}, "alpha")
	if err == nil {
		t.Fatal("ResolveCatalogEntry() error = nil, want ambiguity error")
	}
	if !strings.Contains(err.Error(), "ambiguë") {
		t.Fatalf("error %q does not contain ambiguity detail", err)
	}
}

// T030
func TestUpsertResourceEntry_AddNew(t *testing.T) {
	cfg := ProjectConfigV2{
		SchemaVersion: 2,
		Providers:     []ProviderEntry{{ID: "default", Provider: "github"}},
		Resources:     []ResourceEntry{},
	}

	err := UpsertResourceEntry(&cfg, ResourceEntry{ResourceName: "test", ProviderID: "default"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Resources) != 1 {
		t.Fatalf("Resources length: got %d, want 1", len(cfg.Resources))
	}
	if cfg.Resources[0].ResourceName != "test" {
		t.Errorf("ResourceName: got %q, want %q", cfg.Resources[0].ResourceName, "test")
	}
}

// T031
func TestUpsertResourceEntry_UpdateExisting(t *testing.T) {
	cfg := ProjectConfigV2{
		SchemaVersion: 2,
		Providers:     []ProviderEntry{{ID: "default", Provider: "github"}},
		Resources:     []ResourceEntry{{ResourceName: "test", ProviderID: "default", PinnedVersion: "v1"}},
	}

	err := UpsertResourceEntry(&cfg, ResourceEntry{ResourceName: "test", ProviderID: "default", PinnedVersion: "v2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Resources) != 1 {
		t.Fatalf("Resources length: got %d, want 1 (no duplicate)", len(cfg.Resources))
	}
	if cfg.Resources[0].PinnedVersion != "v2" {
		t.Errorf("PinnedVersion: got %q, want %q", cfg.Resources[0].PinnedVersion, "v2")
	}
}
