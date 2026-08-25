package fs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureManagedGitignore_CreatesWhenGitRepo(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	changed, err := EnsureManagedGitignore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Errorf("first call should create the block")
	}
	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{"**/sth-state-v2.json", "**/.sth-state.lock", ".sth/last-operation.json"} {
		if !strings.Contains(content, want) {
			t.Errorf("gitignore missing %q: %s", want, content)
		}
	}
	if strings.Contains(content, "project.json") {
		t.Errorf("project.json must not be ignored: %s", content)
	}
}

func TestEnsureManagedGitignore_Idempotent(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := EnsureManagedGitignore(dir); err != nil {
		t.Fatal(err)
	}
	changed, err := EnsureManagedGitignore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Errorf("second call should be a no-op")
	}
	data, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if strings.Count(string(data), gitignoreManagedHeader) != 1 {
		t.Errorf("managed block duplicated: %s", string(data))
	}
}

func TestEnsureManagedGitignore_MigratesLegacyHeaderIdempotently(t *testing.T) {
	directory := t.TempDir()
	if err := os.MkdirAll(filepath.Join(directory, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	gitignorePath := filepath.Join(directory, ".gitignore")
	legacyBlock := strings.Replace(managedGitignoreBlock(), gitignoreManagedHeader, gitignoreLegacyManagedHeader, 1)
	if err := os.WriteFile(gitignorePath, []byte("node_modules/\n\n"+legacyBlock+"*.log\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	changed, err := EnsureManagedGitignore(directory)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("legacy header migration should report a change")
	}
	secondChange, err := EnsureManagedGitignore(directory)
	if err != nil {
		t.Fatal(err)
	}
	if secondChange {
		t.Fatal("migrated block should be idempotent")
	}
	data, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if strings.Count(content, gitignoreManagedHeader) != 1 || strings.Contains(content, gitignoreLegacyManagedHeader) {
		t.Fatalf("legacy header was not migrated exactly once: %s", content)
	}
	if !strings.Contains(content, "node_modules/") || !strings.Contains(content, "*.log") {
		t.Fatalf("entries outside the managed block were not preserved: %s", content)
	}
}

func TestEnsureManagedGitignore_PreservesExisting(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	gitignorePath := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(gitignorePath, []byte("node_modules/\n*.log\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := EnsureManagedGitignore(dir); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(gitignorePath)
	content := string(data)
	if !strings.Contains(content, "node_modules/") || !strings.Contains(content, "*.log") {
		t.Errorf("existing entries not preserved: %s", content)
	}
	if !strings.Contains(content, "**/sth-state-v2.json") {
		t.Errorf("managed block not appended: %s", content)
	}
}

func TestEnsureManagedGitignore_SkipsWithoutGit(t *testing.T) {
	dir := t.TempDir()
	changed, err := EnsureManagedGitignore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Errorf("should skip when no .git present")
	}
	if _, statErr := os.Stat(filepath.Join(dir, ".gitignore")); statErr == nil {
		t.Errorf(".gitignore should not be created without a git repo")
	}
}
