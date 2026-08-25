package cmd

import (
	"bytes"
	"errors"
	"io"
	stdhttp "net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/zalando/go-keyring"

	"sth/internal/fs"
	sthhttp "sth/internal/http"
	"sth/internal/tui"
)

type initRoundTripFunc func(*stdhttp.Request) (*stdhttp.Response, error)

func (roundTrip initRoundTripFunc) RoundTrip(request *stdhttp.Request) (*stdhttp.Response, error) {
	return roundTrip(request)
}

func initHTTPResponse(request *stdhttp.Request, statusCode int, body string) *stdhttp.Response {
	return &stdhttp.Response{
		StatusCode: statusCode,
		Header:     make(stdhttp.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
}

func initRegressionHTTPClient() *sthhttp.Client {
	httpClient := &stdhttp.Client{Transport: initRoundTripFunc(func(request *stdhttp.Request) (*stdhttp.Response, error) {
		if request.URL.Path == "/v1/access-check" {
			body := `{"ok":true,"org":{"id":"org","name":"Acme"},"plan":"pro","expires_at":"2099-01-01"}`
			return initHTTPResponse(request, stdhttp.StatusOK, body), nil
		}
		catalog := testCatalogMarkdown("| skill | alpha | skills | skills/alpha.md | Alpha |")
		return initHTTPResponse(request, stdhttp.StatusOK, catalog), nil
	})}
	return sthhttp.NewClient(httpClient)
}

func TestRunInitFirstCloudLoginPreservesCloudProvider(t *testing.T) {
	keyring.MockInit()
	t.Setenv("STH_CLOUD_API_KEY", "")
	t.Setenv("STH_CLOUD_BASE_URL", "https://cloud.test")
	t.Setenv("STH_GITHUB_RAW_BASE_URL", "https://git.test")

	projectDir := t.TempDir()
	result := runCLIWithOptions(t, cliOptions{
		workDir:           projectDir,
		input:             "3\ny\nlsk_live_valid\n\nowner/repo\n\n\n\npublic\n\n",
		args:              []string{"init"},
		client:            initRegressionHTTPClient(),
		interactive:       func() bool { return true },
		skipTelemetrySeed: true,
	})

	if result.err != nil {
		t.Fatalf("Run() error = %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}
	config, err := fs.ReadProjectConfigV2()
	if err != nil {
		t.Fatalf("ReadProjectConfigV2() error = %v", err)
	}
	providerIDs := make(map[string]bool, len(config.Providers))
	for _, provider := range config.Providers {
		providerIDs[provider.ID] = true
	}
	if len(config.Providers) != 2 || !providerIDs["sth-cloud"] || !providerIDs["default"] {
		t.Fatalf("providers = %#v, want sth-cloud and default", config.Providers)
	}
}

func TestRunInitCancellationAfterCloudLoginIsHonest(t *testing.T) {
	keyring.MockInit()
	t.Setenv("STH_CLOUD_API_KEY", "")
	t.Setenv("STH_CLOUD_BASE_URL", "https://cloud.test")
	t.Setenv("STH_GITHUB_RAW_BASE_URL", "https://git.test")

	result := runCLIWithOptions(t, cliOptions{
		workDir:           t.TempDir(),
		input:             "3\ny\nlsk_live_valid\n\nowner/repo\n\n\n\npublic\nn\n",
		args:              []string{"init"},
		client:            initRegressionHTTPClient(),
		interactive:       func() bool { return true },
		skipTelemetrySeed: true,
	})

	if result.err != nil {
		t.Fatalf("Run() error = %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}
	assertContains(t, result.stdout, "setup cancelled.")
	assertNotContains(t, result.stdout, "No file was modified")
	config, err := fs.ReadProjectConfigV2()
	if err != nil {
		t.Fatalf("ReadProjectConfigV2() error = %v", err)
	}
	if len(config.Providers) != 1 || config.Providers[0].ID != "sth-cloud" {
		t.Fatalf("providers = %#v, want only sth-cloud", config.Providers)
	}
}

func TestInitFlowLegacyImportIsAskedOnce(t *testing.T) {
	t.Setenv("STH_USER_CONFIG_DIR", t.TempDir())
	if _, err := fs.SetTelemetryConsent(false, "test"); err != nil {
		t.Fatalf("SetTelemetryConsent() error = %v", err)
	}
	defaults := fs.ProjectProfile{Provider: "github", Ref: "main", CatalogPath: "SKILLS.md", TargetDir: ".sth"}
	legacyProfile := fs.ProjectProfile{
		Provider:     "gitlab",
		RepositoryID: "group/catalog",
		Ref:          "stable",
		CatalogPath:  "LEGACY.md",
		TargetDir:    ".legacy",
	}
	application := newApp(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}, nil)
	model := newInitFlowModel(application, 80, false, fs.ProjectConfigV2{SchemaVersion: 2}, defaults, "default", &legacyProfile)

	if model.kind != stepLegacyImportConfirm {
		t.Fatalf("initial step = %v, want legacy import confirmation", model.kind)
	}
	updatedTeaModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updatedModel := updatedTeaModel.(initFlowModel)
	if updatedModel.legacyState != legacyImportHandled {
		t.Fatalf("legacy state = %v, want handled", updatedModel.legacyState)
	}
	if updatedModel.kind != stepProvider {
		t.Fatalf("step after legacy import = %v, want provider", updatedModel.kind)
	}
	if updatedModel.draft.RepositoryID != legacyProfile.RepositoryID {
		t.Fatalf("repository = %q, want %q", updatedModel.draft.RepositoryID, legacyProfile.RepositoryID)
	}
}

func TestInitFlowChecksKeyOnlyAfterConnectedSelection(t *testing.T) {
	t.Setenv("STH_USER_CONFIG_DIR", t.TempDir())
	defaults := fs.ProjectProfile{Provider: "github", Ref: "main", CatalogPath: "SKILLS.md", TargetDir: ".sth"}
	application := newApp(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}, nil)
	model := newInitFlowModel(application, 80, false, fs.ProjectConfigV2{SchemaVersion: 2}, defaults, "default", nil)

	updatedModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updatedModel.(initFlowModel)
	updatedModel, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updatedModel.(initFlowModel)
	updatedModel, keyCheckCommand := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updatedModel.(initFlowModel)
	if model.kind != stepExistingCloudKeyChecking || keyCheckCommand == nil {
		t.Fatalf("connected selection = step %v, command nil %v", model.kind, keyCheckCommand == nil)
	}

	updatedModel, _ = model.Update(existingCloudKeyDoneMsg{available: false})
	model = updatedModel.(initFlowModel)
	if model.kind != stepCloudLoginAsk {
		t.Fatalf("step after missing key = %v, want cloud login prompt", model.kind)
	}
}

func TestInitFlowRecapViewShowsDraftValues(t *testing.T) {
	draft := wizardProfile{
		Alias:        "work",
		Provider:     "gitlab",
		RepositoryID: "group/catalog",
		Ref:          "stable",
		CatalogPath:  "catalog/SKILLS.md",
		Targets:      []string{"claude", "codex"},
		IsPrivate:    true,
	}
	model := initFlowModel{draft: draft}
	model.enter(stepRecap)
	view := model.View()
	expectedValues := []string{
		"Alias: work",
		"Provider: gitlab",
		"Repository: group/catalog",
		"Git reference: stable",
		"Catalog: catalog/SKILLS.md",
		"Targets: claude, codex",
		"Access: private",
		"Token: none",
	}
	for _, expectedValue := range expectedValues {
		if !strings.Contains(view, expectedValue) {
			t.Fatalf("recap view %q does not contain %q", view, expectedValue)
		}
	}
	recapPosition := strings.Index(view, "Alias: work")
	savePosition := strings.Index(view, "Save this configuration")
	if recapPosition < 0 || savePosition < 0 || recapPosition > savePosition {
		t.Fatalf("recap values must appear before the actions: %q", view)
	}
}

func TestInitFlowCtrlCDuringValidationCannotWriteAfterCancellation(t *testing.T) {
	projectDir := t.TempDir()
	previousWorkingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(projectDir); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(previousWorkingDir)
	})
	t.Setenv("STH_USER_CONFIG_DIR", t.TempDir())
	t.Setenv("STH_GITHUB_RAW_BASE_URL", "https://git.test")

	validationStarted := make(chan struct{})
	httpClient := &stdhttp.Client{Transport: initRoundTripFunc(func(request *stdhttp.Request) (*stdhttp.Response, error) {
		close(validationStarted)
		<-request.Context().Done()
		return nil, request.Context().Err()
	})}
	application := newApp(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}, sthhttp.NewClient(httpClient))
	draft := wizardProfile{
		Alias:        "default",
		Provider:     "github",
		RepositoryID: "owner/repo",
		Ref:          "main",
		CatalogPath:  "SKILLS.md",
		TargetDir:    ".sth",
		Targets:      []string{"claude"},
	}
	model := initFlowModel{app: application, draft: draft}
	model.enter(stepRecap)
	validatingTeaModel, validationCmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	validatingModel := validatingTeaModel.(initFlowModel)
	if validationCmd == nil || validatingModel.kind != stepValidating {
		t.Fatalf("validation did not start: step=%v command=%v", validatingModel.kind, validationCmd)
	}

	validationResult := make(chan tea.Msg, 1)
	go func() {
		validationResult <- validationCmd()
	}()
	select {
	case <-validationStarted:
	case <-time.After(time.Second):
		t.Fatal("validation did not reach the HTTP client")
	}
	cancelledTeaModel, _ := validatingModel.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	cancelledModel := cancelledTeaModel.(initFlowModel)
	if !errors.Is(cancelledModel.flowErr, tui.ErrAborted) {
		t.Fatalf("flow error = %v, want aborted", cancelledModel.flowErr)
	}

	var validationMessage tea.Msg
	select {
	case validationMessage = <-validationResult:
	case <-time.After(time.Second):
		t.Fatal("cancelled validation did not return")
	}
	configPath := filepath.Join(projectDir, ".sth", "project.json")
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("config exists before validation result is delivered: %v", err)
	}
	ignoredTeaModel, _ := cancelledModel.Update(validationMessage)
	ignoredModel := ignoredTeaModel.(initFlowModel)
	if !errors.Is(ignoredModel.flowErr, tui.ErrAborted) {
		t.Fatalf("flow error after late validation = %v, want aborted", ignoredModel.flowErr)
	}
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("config exists after late validation result: %v", err)
	}
}

func TestInitFlowCloudLoginRunsAsCancellableCommand(t *testing.T) {
	keyring.MockInit()
	t.Setenv("STH_CLOUD_API_KEY", "")
	t.Setenv("STH_USER_CONFIG_DIR", t.TempDir())
	t.Setenv("STH_CLOUD_BASE_URL", "https://cloud.test")
	loginStarted := make(chan struct{})
	httpClient := &stdhttp.Client{Transport: initRoundTripFunc(func(request *stdhttp.Request) (*stdhttp.Response, error) {
		close(loginStarted)
		<-request.Context().Done()
		return nil, request.Context().Err()
	})}
	application := newApp(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}, sthhttp.NewClient(httpClient))
	model := initFlowModel{app: application, assignedAlias: "default"}
	model.enter(stepCloudKey)

	typedTeaModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("lsk_live_valid")})
	typedModel := typedTeaModel.(initFlowModel)
	checkingTeaModel, loginCommand := typedModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	checkingModel := checkingTeaModel.(initFlowModel)
	if checkingModel.kind != stepCloudKeyChecking || loginCommand == nil {
		t.Fatalf("cloud login did not start asynchronously: step=%v command=%v", checkingModel.kind, loginCommand)
	}

	loginResult := make(chan tea.Msg, 1)
	go func() {
		loginResult <- loginCommand()
	}()
	select {
	case <-loginStarted:
	case <-time.After(time.Second):
		t.Fatal("cloud login did not reach the HTTP client")
	}
	cancelledTeaModel, _ := checkingModel.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	cancelledModel := cancelledTeaModel.(initFlowModel)
	if !errors.Is(cancelledModel.flowErr, tui.ErrAborted) {
		t.Fatalf("flow error = %v, want aborted", cancelledModel.flowErr)
	}
	select {
	case loginMessage := <-loginResult:
		lateTeaModel, _ := cancelledModel.Update(loginMessage)
		lateModel := lateTeaModel.(initFlowModel)
		if !errors.Is(lateModel.flowErr, tui.ErrAborted) {
			t.Fatalf("late cloud result changed cancellation: %v", lateModel.flowErr)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled cloud login did not return")
	}
}

func TestInitFlowCloudLoginCommitCannotReportFalseCancellation(t *testing.T) {
	model := initFlowModel{kind: stepCloudKeySaving}
	updatedTeaModel, command := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	updatedModel := updatedTeaModel.(initFlowModel)
	if command != nil || updatedModel.kind != stepCloudKeySaving || updatedModel.flowErr != nil {
		t.Fatalf("commit cancellation = step %v, error %v, command %v", updatedModel.kind, updatedModel.flowErr, command)
	}
}
