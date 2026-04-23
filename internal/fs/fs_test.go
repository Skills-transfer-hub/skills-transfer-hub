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
	if configPath != filepath.Join(paths.homeDir, ".loreskillsrc") {
		t.Fatalf("configPath = %s, want %s", configPath, filepath.Join(paths.homeDir, ".loreskillsrc"))
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
	configPath := filepath.Join(paths.homeDir, ".loreskillsrc")
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
	expectedPath := filepath.Join(paths.projectDir, ".loreskills", "project.json")
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
	statePath := filepath.Join(targetPath, "lore-state.json")
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

func TestBuildManagedRoot_returnsCategoryFolderRoot(t *testing.T) {
	root, err := BuildManagedRoot(".claude/resources", "react", "form-wizard")
	if err != nil {
		t.Fatalf("BuildManagedRoot() error = %v", err)
	}
	want := filepath.Join(".claude", "resources", "react", "form-wizard")
	if root != want {
		t.Fatalf("root = %q, want %q", root, want)
	}
}

func TestResolveManagedPath_rejectsEscapes(t *testing.T) {
	_, err := ResolveManagedPath(filepath.Join(".claude", "resources", "react", "form-wizard"), "../escape.txt")
	if err == nil {
		t.Fatal("ResolveManagedPath() error = nil, want error")
	}
}

func TestBundleID_normalizesCaseAndWhitespace(t *testing.T) {
	if got := BundleID(" React ", " Form-Wizard "); got != "react::form-wizard" {
		t.Fatalf("BundleID() = %q, want react::form-wizard", got)
	}
}

func TestNormalizeRepositoryID_supportsHostedURLsAndRejectsSelfHosted(t *testing.T) {
	githubID, err := NormalizeRepositoryID("github", "https://github.com/owner/repo")
	if err != nil {
		t.Fatalf("NormalizeRepositoryID(github) error = %v", err)
	}
	if githubID != "owner/repo" {
		t.Fatalf("githubID = %q, want owner/repo", githubID)
	}

	if _, err := NormalizeRepositoryID("gitlab", "https://git.example.com/group/project"); err == nil {
		t.Fatal("NormalizeRepositoryID(gitlab, self-hosted) error = nil, want error")
	}
}
