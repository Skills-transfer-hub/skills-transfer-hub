package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sth/internal/fs"
)

func TestUpdate_FailOnChanges_exitsNonZeroWhenLocallyModified(t *testing.T) {
	projectDir := t.TempDir()
	targetDir := ".claude/resources"
	targetPath := filepath.Join(projectDir, filepath.FromSlash(targetDir))
	if err := os.MkdirAll(targetPath, 0o755); err != nil {
		t.Fatal(err)
	}
	skillRel := filepath.Join("skills", "alpha.md")
	skillAbs := filepath.Join(targetPath, skillRel)
	if err := os.MkdirAll(filepath.Dir(skillAbs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skillAbs, []byte("# Original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	originalSHA := fs.ComputeSHA256([]byte("# Original\n"))

	writeProjectConfigV2ForTest(t, projectDir, fs.ProjectConfigV2{
		SchemaVersion: 2,
		Providers: []fs.ProviderEntry{
			{ID: "main", Provider: "github", RepositoryID: "owner/repo", CatalogRef: "main", CatalogPath: "SKILLS.md", TargetDir: targetDir},
		},
	})

	state := fs.InstalledResourceStateV2{
		SchemaVersion: 2,
		Entries: []fs.InstalledResourceRecordV2{
			{
				ResourceName:    "alpha",
				ProviderID:      "main",
				ResolvedVersion: "abc",
				ManagedFiles:    []fs.ManagedFile{{Path: filepath.ToSlash(skillAbs), SHA256: originalSHA}},
			},
		},
	}
	stateData, _ := os.ReadFile(filepath.Join(targetPath, "sth-state-v2.json"))
	_ = stateData
	if err := fs.WriteInstalledStateV2(targetDir, state); err != nil {
		// WriteInstalledStateV2 takes targetDir relative to cwd; switch.
		oldWD, _ := os.Getwd()
		_ = os.Chdir(projectDir)
		_ = fs.WriteInstalledStateV2(targetDir, state)
		_ = os.Chdir(oldWD)
	}

	// Hand-edit the file to flip its SHA256.
	if err := os.WriteFile(skillAbs, []byte("# Hand-edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := runCLI(t, projectDir, t.TempDir(), "update", "--fail-on-changes")
	if res.err == nil {
		t.Fatalf("expected non-zero exit, stdout=%s stderr=%s", res.stdout, res.stderr)
	}
	if !strings.Contains(res.stdout, "Modified files detected") {
		t.Fatalf("missing detection line: %q", res.stdout)
	}
	if !strings.Contains(res.stdout, "alpha") {
		t.Fatalf("missing resource name in output: %q", res.stdout)
	}
}
