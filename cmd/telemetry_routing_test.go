package cmd

import (
	"bytes"
	stdhttp "net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"sth/internal/fs"
	sthhttp "sth/internal/http"
)

// telemetryHits counts requests to each endpoint so the 4-cases switch in
// reportCloudUsageBestEffort can be verified without inspecting headers.
type telemetryHits struct {
	mu   sync.Mutex
	anon int
	auth int
}

func (h *telemetryHits) inc(anon bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if anon {
		h.anon++
	} else {
		h.auth++
	}
}

func newTelemetryHits(t *testing.T) (*telemetryHits, *httptest.Server) {
	t.Helper()
	hits := &telemetryHits{}
	srv := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		switch r.URL.Path {
		case "/v1/usage/anon":
			hits.inc(true)
		case "/v1/usage":
			hits.inc(false)
		default:
			stdhttp.NotFound(w, r)
			return
		}
		w.WriteHeader(stdhttp.StatusAccepted)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("STH_CLOUD_BASE_URL", srv.URL)
	return hits, srv
}

func newTestApp(t *testing.T, srv *httptest.Server) *app {
	t.Helper()
	client := sthhttp.NewClient(srv.Client())
	return newApp(nil, &bytes.Buffer{}, &bytes.Buffer{}, client)
}

func TestReportCloudUsageBestEffort_OffMode_NoCalls(t *testing.T) {
	withTempUserConfig(t)
	if _, err := fs.SetTelemetryMode(fs.TelemetryOff, "test"); err != nil {
		t.Fatal(err)
	}

	hits, srv := newTelemetryHits(t)
	a := newTestApp(t, srv)
	a.reportCloudUsageBestEffort("list")

	if hits.anon != 0 || hits.auth != 0 {
		t.Fatalf("off mode must not send anything, got anon=%d auth=%d", hits.anon, hits.auth)
	}
	if records, _ := fs.ReadUsageLog(10); len(records) != 0 {
		t.Fatalf("off mode must not write the local log, got %d entries", len(records))
	}
}

func TestReportCloudUsageBestEffort_AnonymousMode_CallsAnonymousEndpoint(t *testing.T) {
	withTempUserConfig(t)
	if _, err := fs.SetTelemetryMode(fs.TelemetryAnonymous, "test"); err != nil {
		t.Fatal(err)
	}
	// Anonymous mode is project-agnostic: no .sth/project.json needed.

	hits, srv := newTelemetryHits(t)
	a := newTestApp(t, srv)
	a.reportCloudUsageBestEffort("list")

	if hits.anon != 1 {
		t.Errorf("anonymous mode should hit /v1/usage/anon once, got %d", hits.anon)
	}
	if hits.auth != 0 {
		t.Errorf("anonymous mode must never hit /v1/usage (auth), got %d", hits.auth)
	}

	records, err := fs.ReadUsageLog(10)
	if err != nil {
		t.Fatalf("read local log: %v", err)
	}
	if len(records) != 1 || records[0].CommandName != "list" {
		t.Fatalf("anonymous send must append one local-log entry for %q, got %+v", "list", records)
	}
}

func TestReportCloudUsageBestEffort_ConnectedMode_NoProvider_NoCalls(t *testing.T) {
	withTempUserConfig(t)
	if _, err := fs.SetTelemetryMode(fs.TelemetryConnected, "test"); err != nil {
		t.Fatal(err)
	}
	// No sth-cloud provider in project: connected mode degrades silently.

	hits, srv := newTelemetryHits(t)
	a := newTestApp(t, srv)
	a.reportCloudUsageBestEffort("list")

	if hits.anon != 0 || hits.auth != 0 {
		t.Fatalf("connected mode without provider must not send anything, got anon=%d auth=%d", hits.anon, hits.auth)
	}
	if records, _ := fs.ReadUsageLog(10); len(records) != 0 {
		t.Fatalf("connected-without-provider must not write the local log, got %d entries", len(records))
	}
}

func TestReportCloudUsageBestEffort_SkipsBlacklistedCommands(t *testing.T) {
	withTempUserConfig(t)
	// Anonymous mode would normally send, but blacklisted commands must NOT.
	if _, err := fs.SetTelemetryMode(fs.TelemetryAnonymous, "test"); err != nil {
		t.Fatal(err)
	}

	hits, srv := newTelemetryHits(t)
	a := newTestApp(t, srv)
	for _, cmd := range []string{"login", "logout", "version", "help", "telemetry"} {
		a.reportCloudUsageBestEffort(cmd)
	}

	if hits.anon != 0 || hits.auth != 0 {
		t.Fatalf("blacklisted commands must not send anything, got anon=%d auth=%d", hits.anon, hits.auth)
	}
}

// withTempUserConfig is a thin wrapper around what root_test.go does via
// runCLIWithOptions, so the test can isolate just the routing logic without
// going through app.run.
func withTempUserConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("STH_USER_CONFIG_DIR", dir)
	return dir
}
