package cmd

import (
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sth/internal/fs"
)

func TestRunTelemetry_Status_NotYetConfigured(t *testing.T) {
	result := runCLIWithOptions(t, cliOptions{
		args:              []string{"telemetry", "status"},
		skipTelemetrySeed: true,
	})
	if result.err != nil {
		t.Fatalf("Run() error = %v", result.err)
	}
	if !strings.Contains(result.stdout, "pas encore configurée") {
		t.Fatalf("expected 'pas encore configurée' in stdout, got: %s", result.stdout)
	}
}

func TestRunTelemetry_Status_WhenConsented(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STH_USER_CONFIG_DIR", dir)
	if _, err := fs.SetTelemetryConsent(true, "v9.9.9"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	result := runCLIWithOptions(t, cliOptions{
		args:              []string{"telemetry", "status"},
		skipTelemetrySeed: true,
		userConfigDir:     dir,
	})
	if result.err != nil {
		t.Fatalf("Run() error = %v", result.err)
	}
	// Legacy SetTelemetryConsent(true) maps to anonymous mode; status now
	// reports the mode by name rather than the binary "activée".
	if !strings.Contains(result.stdout, "mode anonymous") {
		t.Fatalf("expected 'mode anonymous' in stdout, got: %s", result.stdout)
	}
	if !strings.Contains(result.stdout, "v9.9.9") {
		t.Fatalf("expected CLI version in stdout, got: %s", result.stdout)
	}
}

func TestRunTelemetry_On_PersistsConsent(t *testing.T) {
	// `on` is the deprecated alias for `anon`; the new confirmation wording
	// reflects the actual mode that gets persisted.
	result := runCLIWithOptions(t, cliOptions{
		args:              []string{"telemetry", "on"},
		skipTelemetrySeed: true,
	})
	if result.err != nil {
		t.Fatalf("Run() error = %v", result.err)
	}
	if !strings.Contains(result.stdout, "anonyme activée") {
		t.Fatalf("expected 'anonyme activée' in stdout, got: %s", result.stdout)
	}
	if !strings.Contains(result.stderr, "déprécié") {
		t.Fatalf("expected deprecation warning in stderr, got: %s", result.stderr)
	}
	if fs.TelemetryModeActive() != fs.TelemetryAnonymous {
		t.Fatalf("mode = %q, want anonymous after `sth telemetry on`", fs.TelemetryModeActive())
	}
}

func TestRunTelemetry_Off_PersistsRefusal(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STH_USER_CONFIG_DIR", dir)
	if _, err := fs.SetTelemetryConsent(true, "v1"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	result := runCLIWithOptions(t, cliOptions{
		args:              []string{"telemetry", "off"},
		skipTelemetrySeed: true,
		userConfigDir:     dir,
	})
	if result.err != nil {
		t.Fatalf("Run() error = %v", result.err)
	}
	if !strings.Contains(result.stdout, "Télémétrie désactivée") {
		t.Fatalf("expected confirmation in stdout, got: %s", result.stdout)
	}
	if fs.TelemetryConsented() {
		t.Fatal("TelemetryConsented should be false after `sth telemetry off`")
	}
}

func TestRunTelemetry_Output_Server_NoCloudProvider(t *testing.T) {
	result := runCLIWithOptions(t, cliOptions{
		args: []string{"telemetry", "output", "--server"},
	})
	if result.err == nil {
		t.Fatal("expected error when --server is used without an sth-cloud provider")
	}
	if !strings.Contains(result.err.Error(), "sth-cloud") {
		t.Fatalf("expected 'sth-cloud' in error, got: %v", result.err)
	}
}

func TestRunTelemetry_Output_Local_RendersSeededLog(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STH_USER_CONFIG_DIR", dir)
	if _, err := fs.SetTelemetryMode(fs.TelemetryAnonymous, "v1"); err != nil {
		t.Fatalf("seed mode: %v", err)
	}
	for _, command := range []string{"init", "list"} {
		if err := fs.AppendUsageLogEntry(fs.UsageLogRecord{Ts: "2026-06-03T10:00:00Z", CLIVersion: "v0.2.0", CommandName: command}); err != nil {
			t.Fatalf("seed log: %v", err)
		}
	}

	result := runCLIWithOptions(t, cliOptions{
		args:              []string{"telemetry", "output"},
		skipTelemetrySeed: true,
		userConfigDir:     dir,
	})
	if result.err != nil {
		t.Fatalf("Run() error = %v\nstderr=%s", result.err, result.stderr)
	}
	if !strings.Contains(result.stdout, "init") || !strings.Contains(result.stdout, "list") {
		t.Fatalf("expected both commands from local log, got: %s", result.stdout)
	}
	if !strings.Contains(result.stdout, "journal local") {
		t.Fatalf("expected local-source note, got: %s", result.stdout)
	}
	if !strings.Contains(result.stdout, "2 envoi") {
		t.Fatalf("expected local count footer, got: %s", result.stdout)
	}
}

func TestRunTelemetry_Output_Local_EmptyWhenOff(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STH_USER_CONFIG_DIR", dir)
	if _, err := fs.SetTelemetryMode(fs.TelemetryOff, "v1"); err != nil {
		t.Fatalf("seed mode: %v", err)
	}
	result := runCLIWithOptions(t, cliOptions{
		args:              []string{"telemetry", "output"},
		skipTelemetrySeed: true,
		userConfigDir:     dir,
	})
	if result.err != nil {
		t.Fatalf("Run() error = %v", result.err)
	}
	if !strings.Contains(result.stdout, "désactivée") {
		t.Fatalf("expected disabled message, got: %s", result.stdout)
	}
}

func TestRunTelemetry_Output_RendersServerRecords(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	// Stand up a fake STH Cloud server that returns two usage records.
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if r.URL.Path != "/v1/usage/mine" {
			stdhttp.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"records": []map[string]any{
				{"ts": "2026-05-11T10:00:00Z", "cli_version": "v0.1.2", "command_name": "init"},
				{"ts": "2026-05-11T10:01:00Z", "cli_version": "v0.1.2", "command_name": "list"},
			},
		})
	}))
	defer server.Close()
	t.Setenv("STH_CLOUD_BASE_URL", server.URL)
	t.Setenv("STH_CLOUD_API_KEY", "lsk_live_test")

	// Project has a sth-cloud provider so FetchOwnUsage resolves credentials.
	writeProjectConfigV2ForTest(t, projectDir, fs.ProjectConfigV2{
		SchemaVersion: 2,
		Providers: []fs.ProviderEntry{
			{ID: "cloud", Provider: "sth-cloud", TargetDir: ".claude/skills"},
		},
		Resources: []fs.ResourceEntry{},
	})

	result := runCLIWithOptions(t, cliOptions{
		workDir: projectDir,
		homeDir: homeDir,
		args:    []string{"telemetry", "output", "--server"},
	})
	if result.err != nil {
		t.Fatalf("Run() error = %v\nstderr=%s", result.err, result.stderr)
	}
	if !strings.Contains(result.stdout, "init") || !strings.Contains(result.stdout, "list") {
		t.Fatalf("expected both commands in output, got: %s", result.stdout)
	}
	if !strings.Contains(result.stdout, "v0.1.2") {
		t.Fatalf("expected cli_version in output, got: %s", result.stdout)
	}
	if !strings.Contains(result.stdout, "2 enregistrement") {
		t.Fatalf("expected count footer in output, got: %s", result.stdout)
	}
}

func TestRunTelemetry_Output_LimitFlag(t *testing.T) {
	for _, tc := range []struct {
		args      []string
		wantErr   bool
		errSubstr string
	}{
		{args: []string{"telemetry", "output", "--limit=10"}, wantErr: false},
		{args: []string{"telemetry", "output", "--limit"}, wantErr: true, errSubstr: "--limit=N"},
		{args: []string{"telemetry", "output", "--limit=abc"}, wantErr: true, errSubstr: "entier positif"},
		{args: []string{"telemetry", "output", "--limit=-1"}, wantErr: true, errSubstr: "entier positif"},
		{args: []string{"telemetry", "output", "--unknown"}, wantErr: true, errSubstr: "inattendu"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			result := runCLIWithOptions(t, cliOptions{args: tc.args})
			if tc.wantErr {
				if result.err == nil || !strings.Contains(result.err.Error(), tc.errSubstr) {
					t.Fatalf("expected error containing %q, got: %v", tc.errSubstr, result.err)
				}
			} else if result.err != nil && !strings.Contains(result.err.Error(), "sth-cloud") {
				t.Fatalf("unexpected non-cloud error: %v", result.err)
			}
		})
	}
}

func TestRunTelemetry_Help(t *testing.T) {
	result := runCLIWithOptions(t, cliOptions{
		args: []string{"telemetry", "help"},
	})
	if result.err != nil {
		t.Fatalf("Run() error = %v", result.err)
	}
	for _, want := range []string{"status", "on", "off", "output"} {
		if !strings.Contains(result.stdout, want) {
			t.Fatalf("help output missing %q: %s", want, result.stdout)
		}
	}
}

func TestRunTelemetry_UnknownSubcommand(t *testing.T) {
	result := runCLIWithOptions(t, cliOptions{
		args: []string{"telemetry", "wat"},
	})
	if result.err == nil {
		t.Fatal("expected error on unknown subcommand")
	}
	if !strings.Contains(result.err.Error(), "wat") {
		t.Fatalf("expected unknown-subcommand error, got: %v", result.err)
	}
}
