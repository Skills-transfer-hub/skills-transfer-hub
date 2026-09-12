package cmd

import (
	"bytes"
	"errors"
	stdhttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/bubbletea"

	"sth/internal/fs"
	sthhttp "sth/internal/http"
	"sth/internal/tui"
)

func assertStartupState(t *testing.T, got, want string) {
	t.Helper()
	assertContains(t, got, want)
}

func TestExecuteGuidedMenuAction_quitSignalsExit(t *testing.T) {
	application := newApp(nil, nil, nil, nil)
	session := tui.NewSession(strings.NewReader(""), &bytes.Buffer{})

	err := application.executeGuidedMenuAction(session, "quit")
	if !errors.Is(err, errMenuQuit) {
		t.Fatalf("quit doit signaler la sortie, obtenu : %v", err)
	}
}

func TestGuidedMenuModelSelectionQuitsBeforeActionExecution(t *testing.T) {
	state := guidedStartupState{
		PrimaryAction:    "init",
		AvailableActions: []string{"init", "help", "quit"},
	}
	model := guidedMenuModel{
		state: state,
		list:  tui.NewSelectModel(guidedSelectConfig(state)),
	}

	navigatedModel, navigationCommand := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	if navigationCommand != nil {
		t.Fatal("navigation should not quit the menu")
	}
	selectedModel, selectionCommand := navigatedModel.(guidedMenuModel).Update(tea.KeyMsg{Type: tea.KeyEnter})
	if selectionCommand == nil {
		t.Fatal("selection should release the menu program")
	}
	if _, ok := selectionCommand().(tea.QuitMsg); !ok {
		t.Fatalf("selection command type = %T, want tea.QuitMsg", selectionCommand())
	}
	result := selectedModel.(guidedMenuModel)
	if result.selectedAction != "help" {
		t.Fatalf("selected action = %q, want help", result.selectedAction)
	}
}

func TestGuidedMenuModelQQuitsImmediately(t *testing.T) {
	state := guidedStartupState{
		PrimaryAction:    "init",
		AvailableActions: []string{"init", "help", "quit"},
	}
	model := guidedMenuModel{
		state: state,
		list:  tui.NewSelectModel(guidedSelectConfig(state)),
	}

	finalModel, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if command == nil {
		t.Fatal("q should quit the menu immediately")
	}
	if _, ok := command().(tea.QuitMsg); !ok {
		t.Fatalf("q command type = %T, want tea.QuitMsg", command())
	}
	result := finalModel.(guidedMenuModel)
	if result.selectedAction != "quit" {
		t.Fatalf("selected action = %q, want quit", result.selectedAction)
	}
}

func TestGuidedMenuOptions_carryFrenchLabelsAndSlugAliases(t *testing.T) {
	state := guidedStartupState{
		AvailableActions: []string{"list", "quit"},
	}
	options := guidedMenuOptions(state)
	if options[0].Label != "List catalog resources" {
		t.Fatalf("list label = %q", options[0].Label)
	}
	if len(options[1].Aliases) != 1 || options[1].Aliases[0] != "quit" {
		t.Fatalf("alias quit = %v", options[1].Aliases)
	}
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
	if state.PrimaryAction != "install" {
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
	if state.PrimaryAction != "install" {
		t.Fatalf("state.PrimaryAction = %q, want list", state.PrimaryAction)
	}
	assertStartupState(t, state.Detail, "Configured provider: github")
}

func TestLoadGuidedStartupState_invalidProjectProfile(t *testing.T) {
	projectDir := t.TempDir()
	configDir := filepath.Join(projectDir, ".sth")
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
	assertStartupState(t, menu, "STH guided menu")
	assertStartupState(t, menu, "Project state: not configured")
	assertNotContains(t, menu, "1. init")
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
	assertStartupState(t, menu, "Project state: invalid configuration")
	assertStartupState(t, menu, "profil projet malformé")
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
	assertStartupState(t, menu, "Project state: ready")
	assertStartupState(t, menu, "Provider configuré : github")
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
	session := tui.NewSession(strings.NewReader("10\n"), &stdout)
	application := newApp(nil, nil, nil, nil)

	action, err := application.promptGuidedMenuAction(session, state)
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
	a := newApp(strings.NewReader("advanced\nprovider-guide\nquit\n"), &stdout, &stderr, nil)
	a.interactive = func() bool { return true }

	if err := a.runGuidedMenu(); err != nil {
		t.Fatalf("runGuidedMenu() error = %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	assertStartupState(t, stdout.String(), "https://github.com/settings/personal-access-tokens/new")
	if count := strings.Count(stdout.String(), "What do you want to do?"); count < 2 {
		t.Fatalf("guided menu should have been rendered twice, got %d occurrences in %q", count, stdout.String())
	}
	assertEmpty(t, stderr.String())
}

func TestRunGuidedMenu_releaseMetadataNeverBlocksLocalUse(t *testing.T) {
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
	var releaseRequests atomic.Int64
	serverURL := ""
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		switch r.URL.Path {
		case "/releases":
			releaseRequests.Add(1)
			_, _ = w.Write([]byte(`[
  {"tag_name":"v2.0.0-beta.1","prerelease":true,"assets":[{"name":"` + assetName + `","browser_download_url":"` + serverURL + `/beta"}]},
  {"tag_name":"v1.2.0","prerelease":false,"assets":[{"name":"` + assetName + `","browser_download_url":"` + serverURL + `/asset"},{"name":"SHA256SUMS","browser_download_url":"` + serverURL + `/checksum"},{"name":"SHA256SUMS.sig","browser_download_url":"` + serverURL + `/signature"},{"name":"sth-code-engine.json","browser_download_url":"` + serverURL + `/manifest"}]}
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
		client:       sthhttp.NewClient(server.Client()),
		interactive:  func() bool { return true },
		userCacheDir: cacheDir,
	}

	first := runCLIWithOptions(t, options)
	if first.err != nil {
		t.Fatalf("first Run() error = %v, want local menu to remain usable\nstdout=%s\nstderr=%s", first.err, first.stdout, first.stderr)
	}
	assertNotContains(t, first.stderr, "obsolète")
	assertContains(t, first.stdout, "STH guided menu")

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		cachedState, cacheErr := fs.ReadUpdateCheckState()
		if cacheErr == nil && cachedState != nil && cachedState.LatestStableVersion == "v1.2.0" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if requestCount := releaseRequests.Load(); requestCount != 1 {
		t.Fatalf("releaseRequests after first run = %d, want 1 (background refresh)", requestCount)
	}
	cachedState, cacheErr := fs.ReadUpdateCheckState()
	if cacheErr != nil || cachedState == nil || cachedState.LatestStableVersion != "v1.2.0" {
		t.Fatalf("background refresh cache = %+v, error = %v", cachedState, cacheErr)
	}

	second := runCLIWithOptions(t, options)
	if second.err != nil {
		t.Fatalf("second Run() error = %v, want cached offer to remain non-blocking\nstdout=%s\nstderr=%s", second.err, second.stdout, second.stderr)
	}
	assertNotContains(t, second.stderr, "obsolète")
	assertContains(t, second.stdout, "STH guided menu")
	if requestCount := releaseRequests.Load(); requestCount != 1 {
		t.Fatalf("releaseRequests after second run = %d, want cache reuse without new network call", requestCount)
	}
}
