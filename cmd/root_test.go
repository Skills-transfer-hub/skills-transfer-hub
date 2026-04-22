package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"loreskills/internal/fs"
)

type cliResult struct {
	stdout string
	stderr string
	err    error
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
	return runCLIWithInput(t, workDir, homeDir, "", args...)
}

func runCLIWithInput(t *testing.T, workDir, homeDir, input string, args ...string) cliResult {
	t.Helper()

	if workDir == "" {
		workDir = t.TempDir()
	}
	if homeDir == "" {
		homeDir = t.TempDir()
	}

	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	t.Setenv("HOMEDRIVE", filepath.VolumeName(homeDir))
	t.Setenv("HOMEPATH", strings.TrimPrefix(homeDir, filepath.VolumeName(homeDir)))

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("chdir %s: %v", workDir, err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := newApp(strings.NewReader(input), &stdout, &stderr, nil)
	runErr := app.run(args)

	return cliResult{stdout: stdout.String(), stderr: stderr.String(), err: runErr}
}

func writeProjectProfileForCommandTest(t *testing.T, projectDir string, profile fs.ProjectProfile) {
	t.Helper()

	configDir := filepath.Join(projectDir, ".loreskills")
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
	if err := os.WriteFile(filepath.Join(targetPath, "lore-state.json"), data, 0o644); err != nil {
		t.Fatalf("WriteFile(lore-state.json): %v", err)
	}
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

func TestRun_noArgumentsShowsVersionSignature(t *testing.T) {
	result := runCLI(t, "", "")

	if result.err != nil {
		t.Fatalf("Run() error = %v", result.err)
	}
	assertContains(t, result.stdout, "LoreSkills dev")
	assertContains(t, result.stdout, versionByline)
	assertNotContains(t, result.stdout, "Commandes visibles")
	assertEmpty(t, result.stderr)
}

func TestRun_unknownCommandShowsErrorAndHelp(t *testing.T) {
	result := runCLI(t, "", "", "bogus")

	if result.err == nil {
		t.Fatal("Run() error = nil, want error")
	}
	assertContains(t, result.stderr, "commande inconnue")
	assertContains(t, result.stderr, "Commandes visibles")
	assertNotContains(t, result.stderr, "stockholm")
}

func TestRun_helpCommandRejectsUnknownTarget(t *testing.T) {
	result := runCLI(t, "", "", "help", "bogus")

	if result.err == nil {
		t.Fatal("Run() error = nil, want error")
	}
	assertContains(t, result.stderr, "commande inconnue")
	assertContains(t, result.stderr, "help")
}
