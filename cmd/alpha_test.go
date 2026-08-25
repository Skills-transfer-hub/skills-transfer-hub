package cmd

import (
	"bytes"
	"io"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sth/internal/auth"
)

func TestRunAlphaRequest_PendingPrintsConfirmation(t *testing.T) {
	t.Setenv("STH_USER_CONFIG_DIR", t.TempDir())
	t.Setenv(auth.EnvAPIKey, "lsk_live_ALPHATEST")
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		_, _ = io.WriteString(w, `{"status":"pending","created_at":"2026-06-09T10:00:00Z"}`)
	}))
	defer server.Close()
	t.Setenv("STH_CLOUD_BASE_URL", server.URL)

	var stdout bytes.Buffer
	application := newApp(nil, &stdout, io.Discard, nil)
	if err := application.runAlphaRequest(nil); err != nil {
		t.Fatalf("runAlphaRequest: %v", err)
	}
	if !strings.Contains(stdout.String(), "Request submitted") {
		t.Fatalf("expected confirmation, got %q", stdout.String())
	}
}

func TestRunAlphaRequest_AlreadyActive(t *testing.T) {
	t.Setenv("STH_USER_CONFIG_DIR", t.TempDir())
	t.Setenv(auth.EnvAPIKey, "lsk_live_ALPHATEST")
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		_, _ = io.WriteString(w, `{"status":"already_active","plan":"pro"}`)
	}))
	defer server.Close()
	t.Setenv("STH_CLOUD_BASE_URL", server.URL)

	var stdout bytes.Buffer
	application := newApp(nil, &stdout, io.Discard, nil)
	if err := application.runAlphaRequest(nil); err != nil {
		t.Fatalf("runAlphaRequest: %v", err)
	}
	if !strings.Contains(stdout.String(), "already have access") {
		t.Fatalf("expected already-active message, got %q", stdout.String())
	}
}

func TestRunAlpha_UnknownSubcommand(t *testing.T) {
	var stdout bytes.Buffer
	application := newApp(nil, &stdout, io.Discard, nil)
	if err := application.runAlpha([]string{"bogus"}); err == nil {
		t.Fatal("expected error for unknown alpha subcommand")
	} else if !strings.Contains(err.Error(), "unknown alpha subcommand") {
		t.Fatalf("unexpected unknown-subcommand error: %v", err)
	}
}
