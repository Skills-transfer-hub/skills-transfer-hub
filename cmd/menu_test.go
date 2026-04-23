package cmd

import (
	"bufio"
	"bytes"
	stdhttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"loreskills/internal/fs"
	lorehttp "loreskills/internal/http"
)

func assertStartupState(t *testing.T, got, want string) {
	t.Helper()
	assertContains(t, got, want)
}

func TestLoadGuidedStartupState_unconfiguredProject(t *testing.T) {
	projectDir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd(): %v", err)
	}
	if err := os.Chdir(projectDir); err != nil {
		t.Fatalf("Chdir(%s): %v", projectDir, err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	a := newApp(nil, nil, nil, nil)
	a.interactive = func() bool { return true }

	state := a.loadGuidedStartupState()
	if state.ProjectState != projectStateUnconfigured {
		t.Fatalf("state.ProjectState = %q, want %q", state.ProjectState, projectStateUnconfigured)
	}
	if state.PrimaryAction != "init" {
		t.Fatalf("state.PrimaryAction = %q, want init", state.PrimaryAction)
	}
}

func TestLoadGuidedStartupState_configuredReadyProject(t *testing.T) {
	projectDir := t.TempDir()
	writeProjectProfileForCommandTest(t, projectDir, fs.ProjectProfile{
		Provider:     "github",
		RepositoryID: "owner/repo",
		Ref:          "main",
		CatalogPath:  "SKILLS.md",
		TargetDir:    ".claude/resources",
	})

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd(): %v", err)
	}
	if err := os.Chdir(projectDir); err != nil {
		t.Fatalf("Chdir(%s): %v", projectDir, err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	a := newApp(nil, nil, nil, nil)
	state := a.loadGuidedStartupState()
	if state.ProjectState != projectStateConfiguredReady {
		t.Fatalf("state.ProjectState = %q, want %q", state.ProjectState, projectStateConfiguredReady)
	}
	if state.PrimaryAction != "list" {
		t.Fatalf("state.PrimaryAction = %q, want list", state.PrimaryAction)
	}
	assertStartupState(t, state.Detail, "Provider configuré : github")
}

func TestLoadGuidedStartupState_invalidProjectProfile(t *testing.T) {
	projectDir := t.TempDir()
	configDir := filepath.Join(projectDir, ".loreskills")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", configDir, err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "project.json"), []byte("{"), 0o600); err != nil {
		t.Fatalf("WriteFile(project.json): %v", err)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd(): %v", err)
	}
	if err := os.Chdir(projectDir); err != nil {
		t.Fatalf("Chdir(%s): %v", projectDir, err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	a := newApp(nil, nil, nil, nil)
	state := a.loadGuidedStartupState()
	if state.ProjectState != projectStateConfiguredBad {
		t.Fatalf("state.ProjectState = %q, want %q", state.ProjectState, projectStateConfiguredBad)
	}
	if state.PrimaryAction != "init" {
		t.Fatalf("state.PrimaryAction = %q, want init", state.PrimaryAction)
	}
}

func TestRenderGuidedMenu_unconfiguredProject(t *testing.T) {
	lines := renderGuidedMenuLines(guidedStartupState{
		InteractionMode:  "interactive",
		ProjectState:     projectStateUnconfigured,
		PrimaryAction:    "init",
		AvailableActions: []string{"init", "help", "quit"},
		Detail:           "Configuration projet absente.",
	})
	menu := renderMessage(lines...)
	assertStartupState(t, menu, "Menu guidé LoreSkills")
	assertStartupState(t, menu, "Etat projet : unconfigured")
	assertStartupState(t, menu, "1. init (recommandé)")
	assertStartupState(t, menu, "3. quit")
}

func TestRenderGuidedMenu_configuredInvalidProject(t *testing.T) {
	lines := renderGuidedMenuLines(guidedStartupState{
		InteractionMode:  "interactive",
		ProjectState:     projectStateConfiguredBad,
		PrimaryAction:    "init",
		AvailableActions: []string{"init", "help", "quit"},
		Detail:           "profil projet malformé",
	})
	menu := renderMessage(lines...)
	assertStartupState(t, menu, "Etat projet : configured-invalid")
	assertStartupState(t, menu, "profil projet malformé")
	assertStartupState(t, menu, "1. init (recommandé)")
}

func TestRenderGuidedMenu_configuredReadyProject(t *testing.T) {
	lines := renderGuidedMenuLines(guidedStartupState{
		InteractionMode:  "interactive",
		ProjectState:     projectStateConfiguredReady,
		PrimaryAction:    "list",
		AvailableActions: []string{"list", "install", "update", "show-config", "provider-guide", "init", "help", "quit"},
		Detail:           "Provider configuré : github",
	})
	menu := renderMessage(lines...)
	assertStartupState(t, menu, "Etat projet : configured-ready")
	assertStartupState(t, menu, "1. list (recommandé)")
	assertStartupState(t, menu, "4. show-config")
	assertStartupState(t, menu, "5. provider-guide")
	assertStartupState(t, menu, "8. quit")
}

func TestPromptGuidedMenuAction_acceptsTwoDigitSelection(t *testing.T) {
	state := guidedStartupState{
		PrimaryAction: "action-1",
		AvailableActions: []string{
			"action-1",
			"action-2",
			"action-3",
			"action-4",
			"action-5",
			"action-6",
			"action-7",
			"action-8",
			"action-9",
			"action-10",
		},
	}

	var stdout bytes.Buffer
	a := newApp(strings.NewReader("10\n"), &stdout, nil, nil)

	action, err := promptGuidedMenuAction(bufio.NewReader(strings.NewReader("10\n")), a, state)
	if err != nil {
		t.Fatalf("promptGuidedMenuAction() error = %v", err)
	}
	if action != "action-10" {
		t.Fatalf("action = %q, want action-10", action)
	}
}

func TestRunGuidedMenu_configuredReadyShowsProviderGuidance(t *testing.T) {
	projectDir := t.TempDir()
	writeProjectProfileForCommandTest(t, projectDir, fs.ProjectProfile{
		Provider:     "github",
		RepositoryID: "owner/repo",
		Ref:          "main",
		CatalogPath:  "SKILLS.md",
		TargetDir:    ".claude/resources",
	})

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd(): %v", err)
	}
	if err := os.Chdir(projectDir); err != nil {
		t.Fatalf("Chdir(%s): %v", projectDir, err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	a := newApp(strings.NewReader("5\n8\n"), &stdout, &stderr, nil)
	a.interactive = func() bool { return true }

	if err := a.runGuidedMenu(); err != nil {
		t.Fatalf("runGuidedMenu() error = %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	assertStartupState(t, stdout.String(), "https://github.com/settings/personal-access-tokens/new")
	if count := strings.Count(stdout.String(), "Menu guidé LoreSkills"); count < 2 {
		t.Fatalf("guided menu should have been rendered twice, got %d occurrences in %q", count, stdout.String())
	}
	assertEmpty(t, stderr.String())
}

func TestRunGuidedMenu_reusesFreshUpdateCacheWithin24Hours(t *testing.T) {
	projectDir := t.TempDir()
	cacheDir := t.TempDir()
	writeProjectProfileForCommandTest(t, projectDir, fs.ProjectProfile{
		Provider:     "github",
		RepositoryID: "owner/repo",
		Ref:          "main",
		CatalogPath:  "SKILLS.md",
		TargetDir:    ".claude/resources",
	})

	assetName := defaultReleaseAssetName()
	releaseRequests := 0
	serverURL := ""
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		switch r.URL.Path {
		case "/releases":
			releaseRequests++
			_, _ = w.Write([]byte(`[
  {"tag_name":"v2.0.0-beta.1","prerelease":true,"assets":[{"name":"` + assetName + `","browser_download_url":"` + serverURL + `/beta"}]},
  {"tag_name":"v1.2.0","prerelease":false,"assets":[{"name":"` + assetName + `","browser_download_url":"` + serverURL + `/asset"},{"name":"SHA256SUMS","browser_download_url":"` + serverURL + `/checksum"}]}
]`))
		default:
			stdhttp.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	previousVersion := version
	previousReleaseAPIURL := releaseAPIURL
	previousReleaseAssetName := releaseAssetName
	version = "v1.0.0"
	releaseAPIURL = server.URL + "/releases"
	releaseAssetName = assetName
	t.Cleanup(func() {
		version = previousVersion
		releaseAPIURL = previousReleaseAPIURL
		releaseAssetName = previousReleaseAssetName
	})

	options := cliOptions{
		workDir:      projectDir,
		homeDir:      t.TempDir(),
		input:        "9\n",
		client:       lorehttp.NewClient(server.Client()),
		interactive:  func() bool { return true },
		userCacheDir: cacheDir,
	}

	first := runCLIWithOptions(t, options)
	if first.err != nil {
		t.Fatalf("first Run() error = %v\nstdout=%s\nstderr=%s", first.err, first.stdout, first.stderr)
	}
	assertContains(t, first.stdout, "Mise à jour stable disponible : v1.2.0")
	assertContains(t, first.stdout, "update-self")
	if releaseRequests != 1 {
		t.Fatalf("releaseRequests after first run = %d, want 1", releaseRequests)
	}

	second := runCLIWithOptions(t, options)
	if second.err != nil {
		t.Fatalf("second Run() error = %v\nstdout=%s\nstderr=%s", second.err, second.stdout, second.stderr)
	}
	assertContains(t, second.stdout, "Mise à jour stable disponible : v1.2.0")
	if releaseRequests != 1 {
		t.Fatalf("releaseRequests after second run = %d, want cache reuse without new network call", releaseRequests)
	}
}
