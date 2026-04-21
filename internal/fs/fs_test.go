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
		targetDir:  ".claude/skills",
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

func TestSaveSkill_writesManagedHeader(t *testing.T) {
	paths := newTestPaths(t)

	if err := EnsureTargetDir(paths.targetDir); err != nil {
		t.Fatalf("EnsureTargetDir() error = %v", err)
	}
	if err := SaveSkill(paths.targetDir, "alpha", []byte("# Alpha\n")); err != nil {
		t.Fatalf("SaveSkill() error = %v", err)
	}

	skillPath := filepath.Join(paths.projectDir, ".claude", "skills", "alpha.md")
	data, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", skillPath, err)
	}
	if !strings.HasPrefix(string(data), skillHeader) {
		t.Fatalf("skill file %q does not contain managed header", data)
	}
}

func TestUpdateState_persistsInstalledSkill(t *testing.T) {
	paths := newTestPaths(t)

	if err := EnsureTargetDir(paths.targetDir); err != nil {
		t.Fatalf("EnsureTargetDir() error = %v", err)
	}
	if err := UpdateState(paths.targetDir, "alpha"); err != nil {
		t.Fatalf("UpdateState() error = %v", err)
	}

	state, err := ReadState(paths.targetDir)
	if err != nil {
		t.Fatalf("ReadState() error = %v", err)
	}
	if got := state["alpha"]; got != "installed" {
		t.Fatalf("state[alpha] = %q, want installed", got)
	}
}

func TestReadState_missingFileReturnsNil(t *testing.T) {
	paths := newTestPaths(t)

	state, err := ReadState(paths.targetDir)
	if err != nil {
		t.Fatalf("ReadState() error = %v", err)
	}
	if state != nil {
		t.Fatalf("state = %#v, want nil", state)
	}
}

func TestReadState_malformedFileFails(t *testing.T) {
	paths := newTestPaths(t)
	targetPath := filepath.Join(paths.projectDir, ".claude", "skills")
	if err := os.MkdirAll(targetPath, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", targetPath, err)
	}
	statePath := filepath.Join(targetPath, "lore-state.json")
	if err := os.WriteFile(statePath, []byte("{invalid"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", statePath, err)
	}

	_, err := ReadState(paths.targetDir)
	if err == nil {
		t.Fatal("ReadState() error = nil, want error")
	}
}
