package fs

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withCwd swaps the process working directory for the duration of the test.
// HasLegacyProjectDir / MigrateLegacyProjectDir both rely on os.Getwd().
func withCwd(t *testing.T, dir string) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir(%s): %v", dir, err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(previous)
	})
}

func writeLegacyProfile(t *testing.T, root string, profile ProjectProfile) {
	t.Helper()
	dir := filepath.Join(root, legacyProjectConfigDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", dir, err)
	}
	data, err := json.Marshal(profile)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, projectConfigFileName), data, 0o644); err != nil {
		t.Fatalf("WriteFile project.json: %v", err)
	}
}

func TestHasLegacyProjectDir_DetectsLegacyOnly(t *testing.T) {
	root := t.TempDir()
	withCwd(t, root)

	got, err := HasLegacyProjectDir()
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got {
		t.Fatal("HasLegacyProjectDir = true, want false (no legacy dir)")
	}

	writeLegacyProfile(t, root, ProjectProfile{
		Provider:     "github",
		RepositoryID: "owner/repo",
		Ref:          "main",
		CatalogPath:  "SKILLS.md",
		TargetDir:    ".claude/skills",
	})

	got, err = HasLegacyProjectDir()
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !got {
		t.Fatal("HasLegacyProjectDir = false, want true after writing .loreskills/project.json")
	}
}

func TestHasLegacyProjectDir_ReturnsFalseWhenSthExists(t *testing.T) {
	root := t.TempDir()
	withCwd(t, root)

	writeLegacyProfile(t, root, ProjectProfile{
		Provider:     "github",
		RepositoryID: "owner/repo",
		Ref:          "main",
		CatalogPath:  "SKILLS.md",
		TargetDir:    ".claude/skills",
	})

	sthDir := filepath.Join(root, sthProjectConfigDirName)
	if err := os.MkdirAll(sthDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", sthDir, err)
	}
	if err := os.WriteFile(filepath.Join(sthDir, projectConfigFileName), []byte("{}"), 0o644); err != nil {
		t.Fatalf("WriteFile sth project.json: %v", err)
	}

	got, err := HasLegacyProjectDir()
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got {
		t.Fatal("HasLegacyProjectDir = true, want false when .sth/project.json exists")
	}
}

func TestMigrateLegacyProjectDir_CopiesProfileAndLeavesLegacyInPlace(t *testing.T) {
	root := t.TempDir()
	withCwd(t, root)

	profile := ProjectProfile{
		Provider:     "github",
		RepositoryID: "owner/repo",
		Ref:          "main",
		CatalogPath:  "SKILLS.md",
		TargetDir:    ".claude/skills",
	}
	writeLegacyProfile(t, root, profile)

	var out bytes.Buffer
	if err := MigrateLegacyProjectDir(profile.TargetDir, &out); err != nil {
		t.Fatalf("MigrateLegacyProjectDir: %v", err)
	}

	sthCfg := filepath.Join(root, sthProjectConfigDirName, projectConfigFileName)
	if _, err := os.Stat(sthCfg); err != nil {
		t.Fatalf("expected %s to exist after migration: %v", sthCfg, err)
	}

	legacyDir := filepath.Join(root, legacyProjectConfigDirName)
	if _, err := os.Stat(legacyDir); err != nil {
		t.Fatalf("expected legacy dir %s to remain in place: %v", legacyDir, err)
	}

	bakDir := legacyDir + ".bak"
	if _, err := os.Stat(bakDir); err == nil {
		t.Fatalf("expected no .bak dir, but %s exists", bakDir)
	}

	msg := out.String()
	if !strings.Contains(msg, "Configuration migrée") {
		t.Errorf("expected migration message, got %q", msg)
	}
	if !strings.Contains(msg, "manuellement") {
		t.Errorf("expected manual-removal hint, got %q", msg)
	}
}

func TestMigrateLegacyProjectDir_RenamesLegacyStateFiles(t *testing.T) {
	root := t.TempDir()
	withCwd(t, root)

	profile := ProjectProfile{
		Provider:     "github",
		RepositoryID: "owner/repo",
		Ref:          "main",
		CatalogPath:  "SKILLS.md",
		TargetDir:    ".claude/skills",
	}
	writeLegacyProfile(t, root, profile)

	targetAbs := filepath.Join(root, profile.TargetDir)
	if err := os.MkdirAll(targetAbs, 0o755); err != nil {
		t.Fatalf("MkdirAll target: %v", err)
	}
	if err := os.WriteFile(filepath.Join(targetAbs, legacyStateFileName), []byte(`{"entries":[]}`), 0o644); err != nil {
		t.Fatalf("write legacy state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(targetAbs, legacyStateFileNameV2), []byte(`{"schema_version":2}`), 0o644); err != nil {
		t.Fatalf("write legacy v2 state: %v", err)
	}

	if err := MigrateLegacyProjectDir(profile.TargetDir, nil); err != nil {
		t.Fatalf("MigrateLegacyProjectDir: %v", err)
	}

	for _, name := range []string{stateFileName, stateFileNameV2} {
		if _, err := os.Stat(filepath.Join(targetAbs, name)); err != nil {
			t.Errorf("expected %s in target dir: %v", name, err)
		}
	}
	for _, name := range []string{legacyStateFileName, legacyStateFileNameV2} {
		if _, err := os.Stat(filepath.Join(targetAbs, name)); err == nil {
			t.Errorf("expected %s to be gone after rename", name)
		}
	}
}

func TestMigrateLegacyProjectDir_SecondInvocationIsNoop(t *testing.T) {
	root := t.TempDir()
	withCwd(t, root)

	profile := ProjectProfile{
		Provider:     "github",
		RepositoryID: "owner/repo",
		Ref:          "main",
		CatalogPath:  "SKILLS.md",
		TargetDir:    ".claude/skills",
	}
	writeLegacyProfile(t, root, profile)

	if err := MigrateLegacyProjectDir(profile.TargetDir, nil); err != nil {
		t.Fatalf("first migration: %v", err)
	}

	got, err := HasLegacyProjectDir()
	if err != nil {
		t.Fatalf("HasLegacyProjectDir: %v", err)
	}
	if got {
		t.Fatal("HasLegacyProjectDir = true after migration; want false (re-prompt suppressed)")
	}
}
