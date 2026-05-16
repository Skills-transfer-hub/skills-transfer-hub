package cmd

import (
	"bufio"
	"bytes"
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sth/internal/fs"
	sthhttp "sth/internal/http"

	"github.com/zalando/go-keyring"
)

// cloudLoginEnv chdir into a fresh project dir + isolated user config,
// optionally sets STH_CLOUD_API_KEY, optionally pre-writes a project.json
// already carrying the sth-cloud provider, and returns the httptest server
// the AccessCheck call hits.
type cloudLoginEnv struct {
	t              *testing.T
	projectDir     string
	server         *httptest.Server
	accessCheckErr int // HTTP status to return (0 = 200 OK)
}

func setupCloudLoginEnv(t *testing.T, opts cloudLoginEnv) cloudLoginEnv {
	t.Helper()

	// go-keyring MockInit replaces the OS keychain with an in-memory store
	// for the rest of this process — required for SaveAPIKey to succeed
	// in tests on Windows / headless CI where no real keychain is available.
	keyring.MockInit()

	if opts.projectDir == "" {
		opts.projectDir = t.TempDir()
	}
	t.Setenv("STH_USER_CONFIG_DIR", t.TempDir())

	// Isolate cwd so ensureCloudProviderEntry writes to our project.json.
	oldWD, _ := os.Getwd()
	if err := os.Chdir(opts.projectDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	srv := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if r.URL.Path == "/v1/access-check" {
			if opts.accessCheckErr != 0 {
				w.WriteHeader(opts.accessCheckErr)
				return
			}
			w.WriteHeader(stdhttp.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"org":{"id":"o","name":"Acme"},"plan":"pro","expires_at":"2099-01-01"}`))
			return
		}
		stdhttp.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("STH_CLOUD_BASE_URL", srv.URL)

	opts.server = srv
	opts.t = t
	return opts
}

func seedCloudProvider(t *testing.T, projectDir string) {
	t.Helper()
	cfg := fs.ProjectConfigV2{
		SchemaVersion: 2,
		Providers: []fs.ProviderEntry{
			{ID: "sth-cloud", Provider: "sth-cloud", TargetDir: ".claude/skills"},
		},
		Resources: []fs.ResourceEntry{},
	}
	data, _ := json.Marshal(cfg)
	if err := os.MkdirAll(filepath.Join(projectDir, ".sth"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".sth", "project.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func newCloudLoginApp(input string, interactive bool, srv *httptest.Server) (*app, *bytes.Buffer, *bytes.Buffer) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	client := sthhttp.NewClient(srv.Client())
	a := newApp(strings.NewReader(input), stdout, stderr, client)
	a.interactive = func() bool { return interactive }
	return a, stdout, stderr
}

func TestCloudLogin_OtherModeNoOp(t *testing.T) {
	env := setupCloudLoginEnv(t, cloudLoginEnv{})
	a, stdout, _ := newCloudLoginApp("", true, env.server)

	mode := a.promptCloudLoginIfNeeded(bufio.NewReader(a.stdin), fs.TelemetryAnonymous)
	if mode != fs.TelemetryAnonymous {
		t.Fatalf("anonymous mode should pass through unchanged, got %q", mode)
	}
	if stdout.Len() != 0 {
		t.Errorf("non-connected mode must not render anything, got:\n%s", stdout.String())
	}
}

func TestCloudLogin_KeyAndProviderPresent_NoPromptNoOutput(t *testing.T) {
	env := setupCloudLoginEnv(t, cloudLoginEnv{})
	seedCloudProvider(t, env.projectDir)
	t.Setenv("STH_CLOUD_API_KEY", "lsk_live_seed")

	a, stdout, _ := newCloudLoginApp("", true, env.server)
	mode := a.promptCloudLoginIfNeeded(bufio.NewReader(a.stdin), fs.TelemetryConnected)

	if mode != fs.TelemetryConnected {
		t.Errorf("mode = %q, want connected (no-op path)", mode)
	}
	if stdout.Len() != 0 {
		t.Errorf("idempotent path must stay silent, got:\n%s", stdout.String())
	}
}

func TestCloudLogin_KeyPresentProviderMissing_AttachesSilently(t *testing.T) {
	env := setupCloudLoginEnv(t, cloudLoginEnv{})
	t.Setenv("STH_CLOUD_API_KEY", "lsk_live_seed")
	// No seed of project.json: provider is missing.

	a, stdout, _ := newCloudLoginApp("", true, env.server)
	mode := a.promptCloudLoginIfNeeded(bufio.NewReader(a.stdin), fs.TelemetryConnected)

	if mode != fs.TelemetryConnected {
		t.Errorf("mode = %q, want connected (silent attach)", mode)
	}
	if !strings.Contains(stdout.String(), "rattaché") {
		t.Errorf("expected silent attach confirmation, got:\n%s", stdout.String())
	}
	// Confirm project.json now carries the sth-cloud provider.
	cfg, err := fs.ReadProjectConfigV2()
	if err != nil {
		t.Fatal(err)
	}
	if _, found := findCloudProviderProfile(cfg); !found {
		t.Error("provider not attached after silent attach branch")
	}
}

func TestCloudLogin_NonInteractive_NoKey_DowngradesToAnonymous(t *testing.T) {
	env := setupCloudLoginEnv(t, cloudLoginEnv{})

	a, _, stderr := newCloudLoginApp("", false, env.server)
	mode := a.promptCloudLoginIfNeeded(bufio.NewReader(a.stdin), fs.TelemetryConnected)

	if mode != fs.TelemetryAnonymous {
		t.Fatalf("mode = %q, want anonymous (non-interactive fallback)", mode)
	}
	if !strings.Contains(stderr.String(), "sth login") {
		t.Errorf("expected hint mentioning `sth login`, got stderr:\n%s", stderr.String())
	}
	if fs.TelemetryModeActive() != fs.TelemetryAnonymous {
		t.Error("mode not persisted on downgrade")
	}
}

func TestCloudLogin_RefusalDowngradesToAnonymous(t *testing.T) {
	env := setupCloudLoginEnv(t, cloudLoginEnv{})

	// "n\n" answers the y/N prompt with the default "no".
	a, _, stderr := newCloudLoginApp("n\n", true, env.server)
	mode := a.promptCloudLoginIfNeeded(bufio.NewReader(a.stdin), fs.TelemetryConnected)

	if mode != fs.TelemetryAnonymous {
		t.Fatalf("refusal should downgrade to anonymous, got %q", mode)
	}
	if !strings.Contains(stderr.String(), "Mode bascule sur anonymous") {
		t.Errorf("missing downgrade hint in stderr, got:\n%s", stderr.String())
	}
}

func TestCloudLogin_ValidKey_SavesAndUpgradesToConnected(t *testing.T) {
	env := setupCloudLoginEnv(t, cloudLoginEnv{})

	// "y\n" accepts the sub-prompt, then we feed a valid key on the next line.
	a, stdout, _ := newCloudLoginApp("y\nlsk_live_valid\n", true, env.server)
	mode := a.promptCloudLoginIfNeeded(bufio.NewReader(a.stdin), fs.TelemetryConnected)

	if mode != fs.TelemetryConnected {
		t.Fatalf("valid key + AccessCheck OK should stay connected, got %q", mode)
	}
	if !strings.Contains(stdout.String(), "Connecté à Acme") {
		t.Errorf("missing success message, got:\n%s", stdout.String())
	}
	// Provider entry should now exist in project.json.
	cfg, _ := fs.ReadProjectConfigV2()
	if _, found := findCloudProviderProfile(cfg); !found {
		t.Error("provider not attached after successful login")
	}
}

func TestCloudLogin_AccessCheck401_DowngradesToAnonymous(t *testing.T) {
	env := setupCloudLoginEnv(t, cloudLoginEnv{accessCheckErr: stdhttp.StatusUnauthorized})

	a, _, stderr := newCloudLoginApp("y\nlsk_live_invalid\n", true, env.server)
	mode := a.promptCloudLoginIfNeeded(bufio.NewReader(a.stdin), fs.TelemetryConnected)

	if mode != fs.TelemetryAnonymous {
		t.Fatalf("401 from AccessCheck should downgrade to anonymous, got %q", mode)
	}
	if !strings.Contains(stderr.String(), "Mode bascule sur anonymous") {
		t.Errorf("missing downgrade hint, got:\n%s", stderr.String())
	}
}

func TestCloudLogin_InvalidPrefix_DowngradesToAnonymous(t *testing.T) {
	env := setupCloudLoginEnv(t, cloudLoginEnv{})

	a, _, stderr := newCloudLoginApp("y\nnotaprefix\n", true, env.server)
	mode := a.promptCloudLoginIfNeeded(bufio.NewReader(a.stdin), fs.TelemetryConnected)

	if mode != fs.TelemetryAnonymous {
		t.Fatalf("bad prefix should downgrade to anonymous, got %q", mode)
	}
	if !strings.Contains(stderr.String(), "Mode bascule") {
		t.Errorf("missing hint on stderr, got:\n%s", stderr.String())
	}
}
