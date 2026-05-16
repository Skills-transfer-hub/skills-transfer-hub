package http

import (
	"context"
	"encoding/json"
	"io"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"
)

// Anonymous telemetry must NEVER carry an Authorization header — that's the
// whole point of the mode.
func TestReportCloudUsageAnonymous_PostsWithoutBearer(t *testing.T) {
	var sawAuth bool
	srv := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if r.URL.Path != "/v1/usage/anon" {
			t.Errorf("path = %q, want /v1/usage/anon", r.URL.Path)
		}
		if r.Method != stdhttp.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			sawAuth = true
			t.Errorf("Authorization header must be absent, got %q", got)
		}
		w.WriteHeader(stdhttp.StatusAccepted)
	}))
	defer srv.Close()
	t.Setenv("STH_CLOUD_BASE_URL", srv.URL)

	c := NewClient(srv.Client())
	if err := c.ReportCloudUsageAnonymous(context.Background(), "v1.0.0", "list"); err != nil {
		t.Fatalf("ReportCloudUsageAnonymous: %v", err)
	}
	if sawAuth {
		t.Fatal("Authorization header should never appear on anonymous payloads")
	}
}

// Payload shape must mirror /v1/usage exactly — same JSON keys, same
// commands array — so the backend can reuse the existing ingest schema.
func TestReportCloudUsageAnonymous_PayloadShape(t *testing.T) {
	var captured struct {
		Commands []struct {
			Ts          string `json:"ts"`
			CLIVersion  string `json:"cli_version"`
			CommandName string `json:"command_name"`
		} `json:"commands"`
	}
	srv := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &captured); err != nil {
			t.Fatalf("payload not JSON: %v (raw=%s)", err, body)
		}
		w.WriteHeader(stdhttp.StatusOK)
	}))
	defer srv.Close()
	t.Setenv("STH_CLOUD_BASE_URL", srv.URL)

	c := NewClient(srv.Client())
	if err := c.ReportCloudUsageAnonymous(context.Background(), "v1.2.3", "install"); err != nil {
		t.Fatal(err)
	}
	if len(captured.Commands) != 1 {
		t.Fatalf("expected 1 command in payload, got %d", len(captured.Commands))
	}
	cmd := captured.Commands[0]
	if cmd.CommandName != "install" {
		t.Errorf("command_name = %q, want install", cmd.CommandName)
	}
	if cmd.CLIVersion != "v1.2.3" {
		t.Errorf("cli_version = %q, want v1.2.3", cmd.CLIVersion)
	}
	if cmd.Ts == "" {
		t.Error("ts must be populated (RFC3339)")
	}
}

// A backend that has not rolled out the /v1/usage/anon endpoint yet returns
// 404 or 501. The CLI must swallow that — shipping the CLI ahead of the
// backend is an expected ops scenario.
func TestReportCloudUsageAnonymous_Swallows404(t *testing.T) {
	srv := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		w.WriteHeader(stdhttp.StatusNotFound)
	}))
	defer srv.Close()
	t.Setenv("STH_CLOUD_BASE_URL", srv.URL)

	c := NewClient(srv.Client())
	if err := c.ReportCloudUsageAnonymous(context.Background(), "v1.0.0", "list"); err != nil {
		t.Fatalf("404 should be swallowed, got %v", err)
	}
}

func TestReportCloudUsageAnonymous_Swallows501(t *testing.T) {
	srv := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		w.WriteHeader(stdhttp.StatusNotImplemented)
	}))
	defer srv.Close()
	t.Setenv("STH_CLOUD_BASE_URL", srv.URL)

	c := NewClient(srv.Client())
	if err := c.ReportCloudUsageAnonymous(context.Background(), "v1.0.0", "list"); err != nil {
		t.Fatalf("501 should be swallowed, got %v", err)
	}
}

// Other 4xx/5xx still surface as errors (the caller swallows them, but here
// we verify the contract so a future call site that wants to log them can).
func TestReportCloudUsageAnonymous_SurfacesOther5xx(t *testing.T) {
	srv := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		w.WriteHeader(stdhttp.StatusBadGateway)
	}))
	defer srv.Close()
	t.Setenv("STH_CLOUD_BASE_URL", srv.URL)

	c := NewClient(srv.Client())
	if err := c.ReportCloudUsageAnonymous(context.Background(), "v1.0.0", "list"); err == nil {
		t.Fatal("502 should surface as error so caller can choose to log it")
	}
}
