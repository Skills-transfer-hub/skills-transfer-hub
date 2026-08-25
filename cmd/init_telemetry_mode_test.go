package cmd

import (
	"bytes"
	"strings"
	"testing"

	"sth/internal/fs"
	"sth/internal/tui"
)

func newPromptApp(input string, interactive bool) (*app, *bytes.Buffer) {
	stdout := &bytes.Buffer{}
	application := newApp(strings.NewReader(input), stdout, &bytes.Buffer{}, nil)
	application.interactive = func() bool { return interactive }
	return application, stdout
}

func TestPromptTelemetryMode_DefaultOff(t *testing.T) {
	withTempUserConfig(t)
	application, stdout := newPromptApp("\n", true)

	mode, err := application.promptTelemetryMode(tui.NewSession(application.stdin, stdout))
	if err != nil {
		t.Fatalf("promptTelemetryMode: %v", err)
	}
	if mode != fs.TelemetryOff {
		t.Fatalf("mode = %q, want off", mode)
	}
	if !strings.Contains(stdout.String(), "Telemetry disabled") {
		t.Errorf("missing confirmation, got:\n%s", stdout.String())
	}
	if fs.TelemetryModeActive() != fs.TelemetryOff {
		t.Error("mode not persisted")
	}
}

func TestPromptTelemetryMode_Anonymous(t *testing.T) {
	withTempUserConfig(t)
	application, stdout := newPromptApp("2\n", true)

	mode, err := application.promptTelemetryMode(tui.NewSession(application.stdin, stdout))
	if err != nil {
		t.Fatal(err)
	}
	if mode != fs.TelemetryAnonymous {
		t.Fatalf("mode = %q, want anonymous", mode)
	}
	if !strings.Contains(stdout.String(), "Anonymous telemetry enabled") {
		t.Errorf("missing anonymous confirmation, got:\n%s", stdout.String())
	}
	if fs.TelemetryModeActive() != fs.TelemetryAnonymous {
		t.Error("mode not persisted")
	}
}

func TestPromptTelemetryMode_Connected(t *testing.T) {
	withTempUserConfig(t)
	application, stdout := newPromptApp("3\n", true)

	mode, err := application.promptTelemetryMode(tui.NewSession(application.stdin, stdout))
	if err != nil {
		t.Fatal(err)
	}
	if mode != fs.TelemetryConnected {
		t.Fatalf("mode = %q, want connected", mode)
	}
	if !strings.Contains(stdout.String(), "Connected mode selected") {
		t.Errorf("missing connected hint, got:\n%s", stdout.String())
	}
}

func TestPromptTelemetryMode_InvalidThenValid(t *testing.T) {
	withTempUserConfig(t)
	application, stdout := newPromptApp("9\n2\n", true)

	mode, err := application.promptTelemetryMode(tui.NewSession(application.stdin, stdout))
	if err != nil {
		t.Fatal(err)
	}
	if mode != fs.TelemetryAnonymous {
		t.Fatalf("mode = %q, want anonymous after re-prompt", mode)
	}
	if !strings.Contains(stdout.String(), "number out of range: 9") {
		t.Errorf("re-prompt message missing, got:\n%s", stdout.String())
	}
}

func TestPromptTelemetryMode_NonInteractive_PersistsOffSilently(t *testing.T) {
	withTempUserConfig(t)
	application, stdout := newPromptApp("", false)

	mode, err := application.promptTelemetryMode(tui.NewSession(application.stdin, stdout))
	if err != nil {
		t.Fatal(err)
	}
	if mode != fs.TelemetryOff {
		t.Fatalf("non-interactive must default to off, got %q", mode)
	}
	if strings.Contains(stdout.String(), "Welcome to STH") {
		t.Errorf("non-interactive must not render the welcome buddy/menu, got:\n%s", stdout.String())
	}
	if fs.TelemetryModeActive() != fs.TelemetryOff {
		t.Error("mode not persisted")
	}
}

func TestPromptTelemetryMode_ThreeInvalidsFallBackToOff(t *testing.T) {
	withTempUserConfig(t)
	application, stdout := newPromptApp("bad1\nbad2\nbad3\n", true)

	mode, err := application.promptTelemetryMode(tui.NewSession(application.stdin, stdout))
	if err != nil {
		t.Fatal(err)
	}
	if mode != fs.TelemetryOff {
		t.Fatalf("3 invalids should fall back to off, got %q", mode)
	}
}
