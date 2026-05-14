package fs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func withTempUserConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("STH_USER_CONFIG_DIR", dir)
	return dir
}

func TestReadUserSettings_AbsentFileReturnsNil(t *testing.T) {
	withTempUserConfig(t)
	got, err := ReadUserSettings()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil settings on empty config dir, got %+v", got)
	}
}

func TestTelemetryConsented_DefaultsFalse(t *testing.T) {
	withTempUserConfig(t)
	if TelemetryConsented() {
		t.Fatal("expected false when no settings file exists")
	}
}

func TestHasTelemetryDecision_FalseWhenAbsent(t *testing.T) {
	withTempUserConfig(t)
	if HasTelemetryDecision() {
		t.Fatal("expected false when no settings file exists")
	}
}

func TestSetTelemetryConsent_PersistsTrue(t *testing.T) {
	dir := withTempUserConfig(t)
	path, err := SetTelemetryConsent(true, "v1.2.3")
	if err != nil {
		t.Fatalf("SetTelemetryConsent: %v", err)
	}
	expectedPath := filepath.Join(dir, "sth", "settings.json")
	if path != expectedPath {
		t.Fatalf("path = %q, want %q", path, expectedPath)
	}
	if !TelemetryConsented() {
		t.Fatal("TelemetryConsented should be true after SetTelemetryConsent(true)")
	}
	if !HasTelemetryDecision() {
		t.Fatal("HasTelemetryDecision should be true after SetTelemetryConsent")
	}
	s, err := ReadUserSettings()
	if err != nil || s == nil {
		t.Fatalf("ReadUserSettings: %v / %v", err, s)
	}
	if s.Telemetry.CLIVersionAtConsent != "v1.2.3" {
		t.Fatalf("cli_version_at_consent = %q, want %q", s.Telemetry.CLIVersionAtConsent, "v1.2.3")
	}
	if strings.TrimSpace(s.Telemetry.ConsentedAt) == "" {
		t.Fatal("consented_at should not be empty")
	}
}

func TestSetTelemetryConsent_PersistsFalseExplicitly(t *testing.T) {
	withTempUserConfig(t)
	if _, err := SetTelemetryConsent(false, "v0.0.0"); err != nil {
		t.Fatalf("SetTelemetryConsent: %v", err)
	}
	if TelemetryConsented() {
		t.Fatal("TelemetryConsented should be false after explicit refusal")
	}
	if !HasTelemetryDecision() {
		t.Fatal("HasTelemetryDecision should be true even after refusal — the user has answered")
	}
}

func TestSetTelemetryConsent_FlipsValue(t *testing.T) {
	withTempUserConfig(t)
	if _, err := SetTelemetryConsent(true, "v1"); err != nil {
		t.Fatalf("first set: %v", err)
	}
	if _, err := SetTelemetryConsent(false, "v1"); err != nil {
		t.Fatalf("second set: %v", err)
	}
	if TelemetryConsented() {
		t.Fatal("expected false after flip")
	}
}

func TestUserSettings_RoundTripPreservesUnknownFields(t *testing.T) {
	dir := withTempUserConfig(t)
	settingsPath := filepath.Join(dir, "sth", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	raw := `{"telemetry":{"consented":true,"consented_at":"2026-01-02T03:04:05Z","cli_version_at_consent":"v9.9.9"}}`
	if err := os.WriteFile(settingsPath, []byte(raw), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	s, err := ReadUserSettings()
	if err != nil || s == nil {
		t.Fatalf("read: %v / %v", err, s)
	}
	if !s.Telemetry.Consented || s.Telemetry.CLIVersionAtConsent != "v9.9.9" {
		t.Fatalf("unexpected read: %+v", s)
	}
}

func TestReadUserSettings_MalformedJSON(t *testing.T) {
	dir := withTempUserConfig(t)
	settingsPath := filepath.Join(dir, "sth", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(settingsPath, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := ReadUserSettings(); err == nil {
		t.Fatal("expected error on malformed JSON")
	}
	if TelemetryConsented() {
		t.Fatal("TelemetryConsented must be false (fail-closed) on read error")
	}
}
