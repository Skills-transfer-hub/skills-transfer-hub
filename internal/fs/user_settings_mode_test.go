package fs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestTelemetryModeActive_AbsentSettings_ReturnsOff(t *testing.T) {
	withTempUserConfig(t)
	if got := TelemetryModeActive(); got != TelemetryOff {
		t.Fatalf("absent settings should yield off, got %q", got)
	}
}

func TestTelemetryModeActive_LegacyConsentedTrue_MigratesToAnonymous(t *testing.T) {
	// Pre-rework settings.json where only Consented was tracked.
	dir := withTempUserConfig(t)
	seedLegacySettings(t, dir, `{"telemetry":{"consented":true,"consented_at":"2026-01-01T00:00:00Z"}}`)
	if got := TelemetryModeActive(); got != TelemetryAnonymous {
		t.Fatalf("legacy consented:true should migrate to anonymous, got %q", got)
	}
}

func TestTelemetryModeActive_LegacyConsentedFalse_MigratesToOff(t *testing.T) {
	dir := withTempUserConfig(t)
	seedLegacySettings(t, dir, `{"telemetry":{"consented":false}}`)
	if got := TelemetryModeActive(); got != TelemetryOff {
		t.Fatalf("legacy consented:false should migrate to off, got %q", got)
	}
}

func TestTelemetryModeActive_ExplicitModeWinsOverLegacy(t *testing.T) {
	// If both are present, Mode is the source of truth.
	dir := withTempUserConfig(t)
	seedLegacySettings(t, dir, `{"telemetry":{"mode":"connected","consented":false}}`)
	if got := TelemetryModeActive(); got != TelemetryConnected {
		t.Fatalf("explicit mode should win, got %q", got)
	}
}

func TestSetTelemetryMode_RoundTrip(t *testing.T) {
	withTempUserConfig(t)
	for _, mode := range []TelemetryMode{TelemetryOff, TelemetryAnonymous, TelemetryConnected} {
		if _, err := SetTelemetryMode(mode, "v1.0.0"); err != nil {
			t.Fatalf("SetTelemetryMode(%q): %v", mode, err)
		}
		if got := TelemetryModeActive(); got != mode {
			t.Errorf("round-trip mismatch: wrote %q, read %q", mode, got)
		}
	}
}

func TestSetTelemetryMode_KeepsLegacyConsentedInSync(t *testing.T) {
	// Older binaries reading the same settings file rely on Consented.
	// Make sure SetTelemetryMode writes it consistently.
	withTempUserConfig(t)
	cases := []struct {
		mode      TelemetryMode
		wantBool  bool
	}{
		{TelemetryOff, false},
		{TelemetryAnonymous, true},
		{TelemetryConnected, true},
	}
	for _, tc := range cases {
		t.Run(string(tc.mode), func(t *testing.T) {
			if _, err := SetTelemetryMode(tc.mode, "v1.0.0"); err != nil {
				t.Fatal(err)
			}
			s, _ := ReadUserSettings()
			if s == nil {
				t.Fatal("settings nil after write")
			}
			if s.Telemetry.Consented != tc.wantBool {
				t.Errorf("Consented = %v, want %v", s.Telemetry.Consented, tc.wantBool)
			}
		})
	}
}

func TestSetTelemetryConsent_LegacyHelper_StillMapsToMode(t *testing.T) {
	// SetTelemetryConsent(true) must yield TelemetryAnonymous on read so
	// callers that have not migrated keep working.
	withTempUserConfig(t)
	if _, err := SetTelemetryConsent(true, "v1.0.0"); err != nil {
		t.Fatal(err)
	}
	if got := TelemetryModeActive(); got != TelemetryAnonymous {
		t.Fatalf("SetTelemetryConsent(true) should land on anonymous, got %q", got)
	}
	if _, err := SetTelemetryConsent(false, "v1.0.0"); err != nil {
		t.Fatal(err)
	}
	if got := TelemetryModeActive(); got != TelemetryOff {
		t.Fatalf("SetTelemetryConsent(false) should land on off, got %q", got)
	}
}

func TestTelemetrySettings_ModeOmittedWhenEmpty(t *testing.T) {
	// Legacy settings files that never set Mode must round-trip without
	// gaining a `"mode": ""` field on read+write.
	s := UserSettings{Telemetry: TelemetrySettings{Consented: true}}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	_ = json.Unmarshal(data, &raw)
	telem, _ := raw["telemetry"].(map[string]any)
	if _, ok := telem["mode"]; ok {
		t.Fatalf("empty mode should be omitted, got %v", telem)
	}
}

func seedLegacySettings(t *testing.T, configDir, content string) {
	t.Helper()
	dir := filepath.Join(configDir, "sth")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
