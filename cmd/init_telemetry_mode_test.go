package cmd

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"sth/internal/fs"
)

// newPromptApp builds a minimal app suitable for driving promptTelemetryMode
// in isolation: in-memory stdin/stdout and an explicit interactive setting.
func newPromptApp(input string, interactive bool) (*app, *bytes.Buffer) {
	stdout := &bytes.Buffer{}
	a := newApp(strings.NewReader(input), stdout, &bytes.Buffer{}, nil)
	a.interactive = func() bool { return interactive }
	return a, stdout
}

func TestPromptTelemetryMode_DefaultOff(t *testing.T) {
	withTempUserConfig(t)
	a, stdout := newPromptApp("\n", true) // empty input → default "1"

	mode, err := a.promptTelemetryMode(bufio.NewReader(a.stdin))
	if err != nil {
		t.Fatalf("promptTelemetryMode: %v", err)
	}
	if mode != fs.TelemetryOff {
		t.Fatalf("mode = %q, want off", mode)
	}
	if !strings.Contains(stdout.String(), "Télémétrie désactivée") {
		t.Errorf("missing confirmation, got:\n%s", stdout.String())
	}
	if fs.TelemetryModeActive() != fs.TelemetryOff {
		t.Error("mode not persisted")
	}
}

func TestPromptTelemetryMode_Anonymous(t *testing.T) {
	withTempUserConfig(t)
	a, stdout := newPromptApp("2\n", true)

	mode, err := a.promptTelemetryMode(bufio.NewReader(a.stdin))
	if err != nil {
		t.Fatal(err)
	}
	if mode != fs.TelemetryAnonymous {
		t.Fatalf("mode = %q, want anonymous", mode)
	}
	if !strings.Contains(stdout.String(), "anonyme activée") {
		t.Errorf("missing anonymous confirmation, got:\n%s", stdout.String())
	}
	if fs.TelemetryModeActive() != fs.TelemetryAnonymous {
		t.Error("mode not persisted")
	}
}

func TestPromptTelemetryMode_Connected(t *testing.T) {
	withTempUserConfig(t)
	a, stdout := newPromptApp("3\n", true)

	mode, err := a.promptTelemetryMode(bufio.NewReader(a.stdin))
	if err != nil {
		t.Fatal(err)
	}
	if mode != fs.TelemetryConnected {
		t.Fatalf("mode = %q, want connected", mode)
	}
	if !strings.Contains(stdout.String(), "connecté") {
		t.Errorf("missing connected hint, got:\n%s", stdout.String())
	}
}

func TestPromptTelemetryMode_InvalidThenValid(t *testing.T) {
	withTempUserConfig(t)
	a, stdout := newPromptApp("9\n2\n", true)

	mode, err := a.promptTelemetryMode(bufio.NewReader(a.stdin))
	if err != nil {
		t.Fatal(err)
	}
	if mode != fs.TelemetryAnonymous {
		t.Fatalf("mode = %q, want anonymous after re-prompt", mode)
	}
	if !strings.Contains(stdout.String(), "Choix invalide") {
		t.Errorf("re-prompt message missing, got:\n%s", stdout.String())
	}
}

func TestPromptTelemetryMode_NonInteractive_PersistsOffSilently(t *testing.T) {
	withTempUserConfig(t)
	a, stdout := newPromptApp("", false)

	mode, err := a.promptTelemetryMode(bufio.NewReader(a.stdin))
	if err != nil {
		t.Fatal(err)
	}
	if mode != fs.TelemetryOff {
		t.Fatalf("non-interactive must default to off, got %q", mode)
	}
	if strings.Contains(stdout.String(), "Bienvenue dans STH") {
		t.Errorf("non-interactive must not render the welcome buddy/menu, got:\n%s", stdout.String())
	}
	if fs.TelemetryModeActive() != fs.TelemetryOff {
		t.Error("mode not persisted")
	}
}

func TestPromptTelemetryMode_ThreeInvalidsFallBackToOff(t *testing.T) {
	withTempUserConfig(t)
	a, _ := newPromptApp("bad1\nbad2\nbad3\n", true)

	mode, err := a.promptTelemetryMode(bufio.NewReader(a.stdin))
	if err != nil {
		t.Fatal(err)
	}
	if mode != fs.TelemetryOff {
		t.Fatalf("3 invalids should fall back to off, got %q", mode)
	}
}
