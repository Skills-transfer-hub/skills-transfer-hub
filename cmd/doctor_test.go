package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestDoctor_cloudProviderReachability(t *testing.T) {
	cases := []struct {
		name       string
		statusCode int
		wantPass   bool
	}{
		{name: "unauthorized means reachable", statusCode: stdhttp.StatusUnauthorized, wantPass: true},
		{name: "not found means reachable", statusCode: stdhttp.StatusNotFound, wantPass: true},
		{name: "server error fails", statusCode: stdhttp.StatusInternalServerError, wantPass: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			projectDir := t.TempDir()
			writeProjectConfigV2ForTest(t, projectDir, fs.ProjectConfigV2{
				SchemaVersion: 2,
				Providers: []fs.ProviderEntry{
					{ID: "cloud", Provider: "sth-cloud", TargetDir: ".claude/skills"},
				},
			})
			t.Setenv("STH_TOKEN", "dummy")

			server := httptest.NewServer(stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, _ *stdhttp.Request) {
				writer.WriteHeader(testCase.statusCode)
			}))
			defer server.Close()
			t.Setenv("STH_CLOUD_BASE_URL", server.URL)

			res := runCLI(t, projectDir, t.TempDir(), "doctor")
			if testCase.wantPass {
				if res.err != nil {
					t.Fatalf("err = %v\nstdout=%s", res.err, res.stdout)
				}
				if !strings.Contains(res.stdout, "[✓] Provider reachability (cloud)") {
					t.Fatalf("missing cloud reachability pass line: %q", res.stdout)
				}
				if !strings.Contains(res.stdout, "(reachable)") {
					t.Fatalf("missing reachable detail: %q", res.stdout)
				}
				if strings.Contains(res.stdout, "auth required") {
					t.Fatalf("sth-cloud must not surface an auth failure: %q", res.stdout)
				}
				return
			}
			if res.err == nil {
				t.Fatalf("expected non-zero exit on HTTP %d\nstdout=%s", testCase.statusCode, res.stdout)
			}
			if !strings.Contains(res.stdout, "[✗] Provider reachability (cloud)") {
				t.Fatalf("missing cloud reachability fail line: %q", res.stdout)
			}
		})
	}
}

func TestDoctor_failsOnExpiredLicense(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("STH_USER_CONFIG_DIR", configDir)
	if err := fs.SaveEntitlements(fs.EntitlementsCache{
		Plan:         "pro",
		ExpiresAt:    time.Now().UTC().Add(-48 * time.Hour),
		Capabilities: []string{"pack_init"},
		CheckedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SaveEntitlements: %v", err)
	}
	res := runCLIWithOptions(t, cliOptions{userConfigDir: configDir, args: []string{"doctor"}})
	if res.err == nil {
		t.Fatal("expected non-zero exit on expired license")
	}
	if !strings.Contains(res.stdout, "[✗] License expiry") {
		t.Fatalf("missing expired license fail line: %q", res.stdout)
	}
	if !strings.Contains(res.stdout, "Regenerate your key") {
		t.Fatalf("missing remediation: %q", res.stdout)
	}
}

func TestDoctor_passesWithNearExpiryDetail(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("STH_USER_CONFIG_DIR", configDir)
	if err := fs.SaveEntitlements(fs.EntitlementsCache{
		Plan:         "pro",
		ExpiresAt:    time.Now().UTC().Add(5 * 24 * time.Hour),
		Capabilities: []string{"pack_init"},
		CheckedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SaveEntitlements: %v", err)
	}
	res := runCLIWithOptions(t, cliOptions{userConfigDir: configDir, args: []string{"doctor"}})
	if !strings.Contains(res.stdout, "[✓] License expiry") {
		t.Fatalf("missing license expiry pass line: %q", res.stdout)
	}
	if !strings.Contains(res.stdout, "expires in") {
		t.Fatalf("missing near-expiry detail: %q", res.stdout)
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
