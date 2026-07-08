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
	sthhttp "sth/internal/http"
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

func TestDoctor_cloudReachabilityTargetsConfiguredCloudBase(t *testing.T) {
	projectDir := t.TempDir()
	writeProjectConfigV2ForTest(t, projectDir, fs.ProjectConfigV2{
		SchemaVersion: 2,
		Providers: []fs.ProviderEntry{
			{ID: "sth-cloud", Provider: "sth-cloud", TargetDir: ".claude/skills"},
		},
	})
	t.Setenv("STH_TOKEN", "dummy")

	srv := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		w.WriteHeader(stdhttp.StatusOK)
	}))
	defer srv.Close()
	t.Setenv("STH_CLOUD_BASE_URL", srv.URL)

	res := runCLI(t, projectDir, t.TempDir(), "doctor")
	if res.err != nil {
		t.Fatalf("err = %v\nstdout=%s", res.err, res.stdout)
	}
	if !strings.Contains(res.stdout, "[✓] Provider reachability (sth-cloud)") {
		t.Fatalf("missing cloud reachability pass line: %q", res.stdout)
	}
	if !strings.Contains(res.stdout, srv.URL) {
		t.Fatalf("reachability check did not target STH_CLOUD_BASE_URL %q: %q", srv.URL, res.stdout)
	}
}

func TestDoctor_cloudReachabilityPassesWhenAuthRequired(t *testing.T) {
	projectDir := t.TempDir()
	writeProjectConfigV2ForTest(t, projectDir, fs.ProjectConfigV2{
		SchemaVersion: 2,
		Providers: []fs.ProviderEntry{
			{ID: "sth-cloud", Provider: "sth-cloud", TargetDir: ".claude/skills"},
		},
	})
	t.Setenv("STH_TOKEN", "dummy")

	srv := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		w.WriteHeader(stdhttp.StatusUnauthorized)
	}))
	defer srv.Close()
	t.Setenv("STH_CLOUD_BASE_URL", srv.URL)

	res := runCLI(t, projectDir, t.TempDir(), "doctor")
	if res.err != nil {
		t.Fatalf("err = %v, want doctor success on 401 from sth-cloud\nstdout=%s", res.err, res.stdout)
	}
	if !strings.Contains(res.stdout, "[✓] Provider reachability (sth-cloud)") {
		t.Fatalf("missing cloud reachability pass line: %q", res.stdout)
	}
	if !strings.Contains(res.stdout, "authentication required") {
		t.Fatalf("missing authentication detail: %q", res.stdout)
	}
}

func TestProviderReachabilityURL_sthCloudMatchesCloudClientBase(t *testing.T) {
	t.Setenv("STH_CLOUD_BASE_URL", "")
	url := providerReachabilityURL(fs.ProviderEntry{Provider: "sth-cloud"})
	if url != sthhttp.CloudBaseURL() {
		t.Fatalf("providerReachabilityURL = %q, want cloud client base %q", url, sthhttp.CloudBaseURL())
	}
	if strings.Contains(url, ".example") {
		t.Fatalf("providerReachabilityURL still points at a placeholder host: %q", url)
	}

	t.Setenv("STH_CLOUD_BASE_URL", "http://127.0.0.1:4321/cli/")
	overridden := providerReachabilityURL(fs.ProviderEntry{Provider: "sth-cloud"})
	if overridden != "http://127.0.0.1:4321/cli" {
		t.Fatalf("providerReachabilityURL with override = %q, want http://127.0.0.1:4321/cli", overridden)
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
