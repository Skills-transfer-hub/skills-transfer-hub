package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	sthhttp "sth/internal/http"
	"strings"
	"testing"

	"sth/internal/fs"
)

type cliResult struct {
	stdout string
	stderr string
	err    error
}

type cliOptions struct {
	workDir      string
	homeDir      string
	input        string
	args         []string
	client       *sthhttp.Client
	interactive  func() bool
	userCacheDir string
	// userConfigDir overrides STH_USER_CONFIG_DIR. When empty, a per-test
	// tempdir is used automatically so tests never touch the real user config.
	userConfigDir string
	// skipTelemetrySeed leaves the telemetry decision unrecorded so the test
	// exercises the first-init consent prompt. Default false: the helper
	// pre-records a refusal so existing tests don't have to feed an extra
	// "n" into the prompt sequence.
	skipTelemetrySeed bool
}

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("got %q, want substring %q", got, want)
	}
}

func assertNotContains(t *testing.T, got, want string) {
	t.Helper()
	if strings.Contains(got, want) {
		t.Fatalf("got %q, did not want substring %q", got, want)
	}
}

func assertEmpty(t *testing.T, got string) {
	t.Helper()
	if got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func runCLI(t *testing.T, workDir, homeDir string, args ...string) cliResult {
	t.Helper()
	return runCLIWithOptions(t, cliOptions{
		workDir: workDir,
		homeDir: homeDir,
		args:    args,
	})
}

func runCLIWithInput(t *testing.T, workDir, homeDir, input string, args ...string) cliResult {
	t.Helper()
	return runCLIWithOptions(t, cliOptions{
		workDir: workDir,
		homeDir: homeDir,
		input:   input,
		args:    args,
	})
}

func runCLIWithOptions(t *testing.T, options cliOptions) cliResult {
	t.Helper()

	if options.workDir == "" {
		options.workDir = t.TempDir()
	}
	if options.homeDir == "" {
		options.homeDir = t.TempDir()
	}

	t.Setenv("HOME", options.homeDir)
	t.Setenv("USERPROFILE", options.homeDir)
	t.Setenv("HOMEDRIVE", filepath.VolumeName(options.homeDir))
	t.Setenv("HOMEPATH", strings.TrimPrefix(options.homeDir, filepath.VolumeName(options.homeDir)))
	if options.userCacheDir != "" {
		t.Setenv("STH_USER_CACHE_DIR", options.userCacheDir)
	}
	if options.userConfigDir == "" {
		options.userConfigDir = t.TempDir()
	}
	t.Setenv("STH_USER_CONFIG_DIR", options.userConfigDir)
	if !options.skipTelemetrySeed {
		if _, err := fs.SetTelemetryConsent(false, "test"); err != nil {
			t.Fatalf("seed telemetry decision: %v", err)
		}
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(options.workDir); err != nil {
		t.Fatalf("chdir %s: %v", options.workDir, err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := newApp(strings.NewReader(options.input), &stdout, &stderr, options.client)
	if options.interactive != nil {
		app.interactive = options.interactive
	}
	runErr := app.run(options.args)

	return cliResult{stdout: stdout.String(), stderr: stderr.String(), err: runErr}
}

func writeProjectProfileForCommandTest(t *testing.T, projectDir string, profile fs.ProjectProfile) {
	t.Helper()

	configDir := filepath.Join(projectDir, ".sth")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", configDir, err)
	}
	data, err := json.Marshal(profile)
	if err != nil {
		t.Fatalf("json.Marshal(): %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "project.json"), data, 0o600); err != nil {
		t.Fatalf("WriteFile(project profile): %v", err)
	}
}

func writeProjectConfigV2ForTest(t *testing.T, projectDir string, cfg fs.ProjectConfigV2) {
	t.Helper()

	configDir := filepath.Join(projectDir, ".sth")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", configDir, err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent(): %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "project.json"), append(data, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile(project.json): %v", err)
	}
}

func writeInstalledStateForCommandTest(t *testing.T, projectDir, targetDir string, state fs.InstalledResourceState) {
	t.Helper()

	targetPath := filepath.Join(projectDir, filepath.FromSlash(targetDir))
	if err := os.MkdirAll(targetPath, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", targetPath, err)
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("json.Marshal(): %v", err)
	}
	if err := os.WriteFile(filepath.Join(targetPath, "sth-state.json"), data, 0o644); err != nil {
		t.Fatalf("WriteFile(sth-state.json): %v", err)
	}
}

func readProjectConfigForTest(t *testing.T, projectDir string) fs.ProjectConfigV2 {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(projectDir, ".sth", "project.json"))
	if err != nil {
		t.Fatalf("ReadFile(project.json): %v", err)
	}
	var config fs.ProjectConfigV2
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("json.Unmarshal(project.json): %v", err)
	}
	return config
}

func testCatalogMarkdown(rows ...string) string {
	lines := []string{
		"# SKILLS",
		"",
		"| Type | Name | Folder | Source | Description |",
		"| ---- | ---- | ------ | ------ | ----------- |",
	}
	lines = append(lines, rows...)
	return strings.Join(lines, "\n") + "\n"
}

func TestRun_noArgumentsNonInteractiveShowsNextStepGuidance(t *testing.T) {
	result := runCLI(t, "", "")

	if result.err != nil {
		t.Fatalf("Run() error = %v", result.err)
	}
	assertContains(t, result.stdout, "Project state: not configured")
	assertContains(t, result.stdout, "Recommended action: sth init")
	assertNotContains(t, result.stdout, "STH dev")
	assertEmpty(t, result.stderr)
}

func TestRun_noArgumentsInteractiveShowsGuidedMenuAndAllowsQuit(t *testing.T) {
	result := runCLIWithOptions(t, cliOptions{
		input:       "3\n",
		interactive: func() bool { return true },
	})

	if result.err != nil {
		t.Fatalf("Run() error = %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}
	assertContains(t, result.stdout, "STH guided menu")
	assertContains(t, result.stdout, "Configure this project (recommended)")
	assertContains(t, result.stdout, "Quit")
	assertNotContains(t, result.stdout, "1.")
	assertEmpty(t, result.stderr)
}

func TestRun_unknownCommandShowsErrorAndHelp(t *testing.T) {
	result := runCLI(t, "", "", "bogus")

	if result.err == nil {
		t.Fatal("Run() error = nil, want error")
	}
	assertContains(t, result.stderr, "unknown command")
	assertContains(t, result.stderr, "Run `sth help`")
	assertNotContains(t, result.stderr, "Available commands")
	assertNotContains(t, result.stderr, "stockholm")
}

func TestRun_unknownCommandSuggestsClosestMatch(t *testing.T) {
	result := runCLI(t, "", "", "instal")

	if result.err == nil {
		t.Fatal("Run() error = nil, want error")
	}
	assertContains(t, result.stderr, "`sth install`")

	tooFar := runCLI(t, "", "", "xkjvzz")
	assertNotContains(t, tooFar.stderr, "Did you mean")
}

func TestRun_helpCommandRejectsUnknownTarget(t *testing.T) {
	result := runCLI(t, "", "", "help", "instal")

	if result.err == nil {
		t.Fatal("Run() error = nil, want error")
	}
	assertContains(t, result.stderr, "unknown command")
	assertContains(t, result.stderr, "help")
	assertContains(t, result.stderr, "`sth install`")
}
