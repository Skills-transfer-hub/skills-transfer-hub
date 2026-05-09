package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	stdhttp "net/http"
	"net/http/httptest"

	"sth/internal/fs"
)

func TestDoctor_skipsAllWhenNoProject(t *testing.T) {
	res := runCLI(t, t.TempDir(), t.TempDir(), "doctor")
	// no project → fail (project config check), but state/credential/provider
	// checks should be skipped and the command should still finish.
	if res.err == nil {
		t.Fatal("err = nil, want failure on missing project config")
	}
	if !strings.Contains(res.stdout, "[✗] Project configuration") {
		t.Fatalf("missing fail marker for project config: %q", res.stdout)
	}
	if !strings.Contains(res.stdout, "[SKIP]") {
		t.Fatalf("expected at least one [SKIP] check: %q", res.stdout)
	}
}

func TestDoctor_failsOnCorruptStateFile(t *testing.T) {
	projectDir := t.TempDir()
	targetDir := ".claude/resources"
	writeProjectConfigV2ForTest(t, projectDir, fs.ProjectConfigV2{
		SchemaVersion: 2,
		Providers: []fs.ProviderEntry{
			{ID: "main", Provider: "github", RepositoryID: "owner/repo", CatalogRef: "main", CatalogPath: "SKILLS.md", TargetDir: targetDir},
		},
	})
	statePath := filepath.Join(projectDir, filepath.FromSlash(targetDir))
	if err := os.MkdirAll(statePath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(statePath, "sth-state-v2.json"), []byte("{not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := runCLI(t, projectDir, t.TempDir(), "doctor")
	if res.err == nil {
		t.Fatal("expected non-zero exit on corrupt state")
	}
	if !strings.Contains(res.stdout, "[✗] State file integrity") {
		t.Fatalf("missing state fail line: %q", res.stdout)
	}
}

func TestDoctor_passesOnHealthyProject(t *testing.T) {
	projectDir := t.TempDir()
	targetDir := ".claude/resources"
	writeProjectConfigV2ForTest(t, projectDir, fs.ProjectConfigV2{
		SchemaVersion: 2,
		Providers: []fs.ProviderEntry{
			{ID: "main", Provider: "github", RepositoryID: "owner/repo", CatalogRef: "main", CatalogPath: "SKILLS.md", TargetDir: targetDir},
		},
	})
	t.Setenv("STH_TOKEN", "dummy")

	srv := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		w.WriteHeader(stdhttp.StatusOK)
	}))
	defer srv.Close()
	t.Setenv("STH_GITHUB_API_BASE_URL", srv.URL)

	res := runCLI(t, projectDir, t.TempDir(), "doctor")
	if res.err != nil {
		t.Fatalf("err = %v\nstdout=%s", res.err, res.stdout)
	}
	if !strings.Contains(res.stdout, "All checks passed.") {
		t.Fatalf("missing pass summary: %q", res.stdout)
	}
}

func TestDoctor_credentialFailWhenNoToken(t *testing.T) {
	projectDir := t.TempDir()
	targetDir := ".claude/resources"
	writeProjectConfigV2ForTest(t, projectDir, fs.ProjectConfigV2{
		SchemaVersion: 2,
		Providers: []fs.ProviderEntry{
			{ID: "main", Provider: "github", RepositoryID: "owner/repo", CatalogRef: "main", CatalogPath: "SKILLS.md", TargetDir: targetDir},
		},
	})
	t.Setenv("STH_TOKEN", "")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", "")

	res := runCLI(t, projectDir, t.TempDir(), "doctor")
	if res.err == nil {
		t.Fatal("expected non-zero exit when no creds")
	}
	if !strings.Contains(res.stdout, "[✗] Credential validity") {
		t.Fatalf("missing credential fail line: %q", res.stdout)
	}
	if !strings.Contains(res.stdout, "Set a valid STH_TOKEN") {
		t.Fatalf("missing remediation: %q", res.stdout)
	}
}
