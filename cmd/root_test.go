package cmd

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func assertEmpty(t *testing.T, got string) {
	t.Helper()
	if got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func runCLI(t *testing.T, workDir, homeDir string, args ...string) cliResult {
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

	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}

	oldStdout := os.Stdout
	oldStderr := os.Stderr
	os.Stdout = stdoutWriter
	os.Stderr = stderrWriter

	runErr := Run(args)

	_ = stdoutWriter.Close()
	_ = stderrWriter.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr

	stdout, err := io.ReadAll(stdoutReader)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	stderr, err := io.ReadAll(stderrReader)
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}

	return cliResult{
		stdout: string(stdout),
		stderr: string(stderr),
		err:    runErr,
	}
}

func TestRun_noArgumentsShowsHelp(t *testing.T) {
	result := runCLI(t, "", "")

	if result.err != nil {
		t.Fatalf("Run() error = %v", result.err)
	}
	assertContains(t, result.stdout, asciiGuide)
	assertContains(t, result.stdout, "Commandes disponibles")
	assertContains(t, result.stdout, "loreskills init")
	assertEmpty(t, result.stderr)
}

func TestRun_unknownCommandShowsErrorAndHelp(t *testing.T) {
	result := runCLI(t, "", "", "bogus")

	if result.err == nil {
		t.Fatal("Run() error = nil, want error")
	}
	assertContains(t, result.stderr, asciiGuide)
	assertContains(t, result.stderr, "commande inconnue")
	assertContains(t, result.stderr, "Commandes disponibles")
}

func TestRun_helpCommandRejectsUnknownTarget(t *testing.T) {
	result := runCLI(t, "", "", "help", "bogus")

	if result.err == nil {
		t.Fatal("Run() error = nil, want error")
	}
	assertContains(t, result.stderr, "commande inconnue")
	assertContains(t, result.stderr, "help")
}
