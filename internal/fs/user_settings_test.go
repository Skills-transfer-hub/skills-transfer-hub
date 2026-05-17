package fs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
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

func TestReadUserSettings_MalformedJSONIsTolerated(t *testing.T) {
	dir := withTempUserConfig(t)
	settingsPath := filepath.Join(dir, "sth", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(settingsPath, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// A corrupt settings file must NOT block sth init — ReadUserSettings
	// returns nil-nil so the wizard treats it as "no decision yet" and
	// re-prompts. TelemetryConsented stays false (fail-closed).
	got, err := ReadUserSettings()
	if err != nil {
		t.Fatalf("expected nil, nil on malformed JSON; got err=%v", err)
	}
	if got != nil {
		t.Fatalf("expected nil settings on malformed JSON, got %+v", got)
	}
	if TelemetryConsented() {
		t.Fatal("TelemetryConsented must be false (fail-closed) on malformed JSON")
	}
}

func TestReadUserSettings_EmptyFileIsTolerated(t *testing.T) {
	dir := withTempUserConfig(t)
	settingsPath := filepath.Join(dir, "sth", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(settingsPath, []byte("   \n\t"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := ReadUserSettings()
	if err != nil || got != nil {
		t.Fatalf("expected nil, nil on empty file; got %v / %v", got, err)
	}
}

// ─── HasCapability + SaveEntitlements (Wire 1) ────────────────────────────

func TestHasCapability_FreshCacheTrue(t *testing.T) {
	withTempUserConfig(t)
	if err := SaveEntitlements(EntitlementsCache{
		Plan:         "pro",
		ExpiresAt:    time.Now().UTC().Add(30 * 24 * time.Hour),
		Capabilities: []string{"pack_init", "multi_target"},
		CheckedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SaveEntitlements: %v", err)
	}
	if !HasCapability("pack_init") {
		t.Fatal("expected pack_init to be granted")
	}
	if HasCapability("fleet_sync") {
		t.Fatal("fleet_sync must NOT be granted on a pro cache")
	}
}

func TestHasCapability_StaleReturnsFalse(t *testing.T) {
	withTempUserConfig(t)
	if err := SaveEntitlements(EntitlementsCache{
		Plan:         "pro",
		ExpiresAt:    time.Now().UTC().Add(30 * 24 * time.Hour),
		Capabilities: []string{"pack_init"},
		CheckedAt:    time.Now().UTC().Add(-(entitlementsHotTTL + time.Hour)),
	}); err != nil {
		t.Fatalf("SaveEntitlements: %v", err)
	}
	if HasCapability("pack_init") {
		t.Fatal("a stale cache must not grant capabilities")
	}
}

func TestHasCapability_CapMissingReturnsFalse(t *testing.T) {
	withTempUserConfig(t)
	if err := SaveEntitlements(EntitlementsCache{
		Plan:         "starter",
		ExpiresAt:    time.Now().UTC().Add(30 * 24 * time.Hour),
		Capabilities: []string{},
		CheckedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SaveEntitlements: %v", err)
	}
	if HasCapability("pack_init") {
		t.Fatal("starter cache must not grant pack_init")
	}
}

// Legacy migration: a settings file written by the pre-capabilities CLI
// will have ProActive=true / ProCheckedAt within TTL but no Entitlements
// block. HasCapability should still credit the legacy pro caps for one
// cache cycle so the user is not asked to re-login after a CLI upgrade.
func TestHasCapability_LegacyProActiveMigrates(t *testing.T) {
	dir := withTempUserConfig(t)
	settingsPath := filepath.Join(dir, "sth", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Write a settings file *only* with the legacy fields — no entitlements key.
	raw := `{"telemetry":{"consented":true,"consented_at":"2026-01-02T03:04:05Z"},"pro_active":true,"pro_checked_at":"` + time.Now().UTC().Format(time.RFC3339) + `"}`
	if err := os.WriteFile(settingsPath, []byte(raw), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if !HasCapability("pack_init") {
		t.Fatal("legacy pro_active=true within TTL must migrate to pack_init grant")
	}
	if !HasCapability("private_repos") {
		t.Fatal("legacy migration must include private_repos in the pro cap set")
	}
	if HasCapability("fleet_sync") {
		t.Fatal("legacy migration must NOT grant fleet_sync (team-only)")
	}
}

func TestHasCapability_LegacyProActiveExpiredDoesNotMigrate(t *testing.T) {
	dir := withTempUserConfig(t)
	settingsPath := filepath.Join(dir, "sth", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	stale := time.Now().UTC().Add(-(entitlementsHotTTL + time.Hour))
	raw := `{"telemetry":{"consented":true},"pro_active":true,"pro_checked_at":"` + stale.Format(time.RFC3339) + `"}`
	if err := os.WriteFile(settingsPath, []byte(raw), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if HasCapability("pack_init") {
		t.Fatal("expired legacy cache must not grant capabilities")
	}
}

func TestSaveEntitlements_ClearsLegacyFields(t *testing.T) {
	// Settings file with legacy fields already populated.
	dir := withTempUserConfig(t)
	settingsPath := filepath.Join(dir, "sth", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	raw := `{"telemetry":{"consented":true},"pro_active":true,"pro_checked_at":"2026-01-01T00:00:00Z"}`
	if err := os.WriteFile(settingsPath, []byte(raw), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// Persist a fresh capability cache.
	if err := SaveEntitlements(EntitlementsCache{
		Plan:         "team",
		ExpiresAt:    time.Now().UTC().Add(30 * 24 * time.Hour),
		Capabilities: []string{"pack_init", "fleet_sync"},
		CheckedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SaveEntitlements: %v", err)
	}
	// Re-read raw bytes and confirm pro_active was wiped so HasCapability
	// cannot accidentally double-credit via the legacy migration path.
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var got UserSettings
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ProActive {
		t.Fatal("ProActive must be cleared after SaveEntitlements")
	}
	if !got.ProCheckedAt.IsZero() {
		t.Fatal("ProCheckedAt must be zeroed after SaveEntitlements")
	}
	if got.Entitlements == nil || got.Entitlements.Plan != "team" {
		t.Fatalf("Entitlements block missing or wrong: %+v", got.Entitlements)
	}
}

// Concurrent writes: 50 goroutines hammering SaveEntitlements must always
// leave the file in a valid JSON state (atomic rename-into-place) and
// containing one of the written cache values, never a half-written mix.
func TestSaveEntitlements_ConcurrentWrites(t *testing.T) {
	dir := withTempUserConfig(t)
	settingsPath := filepath.Join(dir, "sth", "settings.json")

	var wg sync.WaitGroup
	const n = 50
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			plan := "pro"
			if i%3 == 0 {
				plan = "team"
			}
			_ = SaveEntitlements(EntitlementsCache{
				Plan:         plan,
				ExpiresAt:    time.Now().UTC().Add(30 * 24 * time.Hour),
				Capabilities: []string{"pack_init"},
				CheckedAt:    time.Now().UTC(),
			})
		}(i)
	}
	wg.Wait()

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var got UserSettings
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("file is not valid JSON after concurrent writes: %v\nraw=%s", err, data)
	}
	if got.Entitlements == nil {
		t.Fatal("expected Entitlements block to be populated")
	}
	if got.Entitlements.Plan != "pro" && got.Entitlements.Plan != "team" {
		t.Fatalf("unexpected plan after concurrent writes: %q", got.Entitlements.Plan)
	}
}

// Round-trip: marshal → unmarshal → marshal must be byte-identical so the
// schema can't drift accidentally (omitempty placement matters).
func TestUserSettings_RoundTrip_JSONStability(t *testing.T) {
	original := UserSettings{
		Telemetry: TelemetrySettings{
			Mode:                TelemetryConnected,
			Consented:           true,
			ConsentedAt:         "2026-05-17T10:00:00Z",
			CLIVersionAtConsent: "v9.9.9",
		},
		Entitlements: &EntitlementsCache{
			Plan:         "pro",
			ExpiresAt:    time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC),
			Capabilities: []string{"pack_init", "multi_target"},
			CheckedAt:    time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC),
		},
	}
	out1, err := json.MarshalIndent(original, "", "  ")
	if err != nil {
		t.Fatalf("marshal 1: %v", err)
	}
	var roundtrip UserSettings
	if err := json.Unmarshal(out1, &roundtrip); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	out2, err := json.MarshalIndent(roundtrip, "", "  ")
	if err != nil {
		t.Fatalf("marshal 2: %v", err)
	}
	if string(out1) != string(out2) {
		t.Fatalf("round-trip drift:\nfirst:\n%s\nsecond:\n%s", out1, out2)
	}
}

// A settings file written by a pre-Wire-1 CLI (no `entitlements` field at
// all) must load without error so users do not get blocked after upgrading.
func TestUserSettings_BackwardCompat_NoEntitlementsField(t *testing.T) {
	dir := withTempUserConfig(t)
	settingsPath := filepath.Join(dir, "sth", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	raw := `{"telemetry":{"consented":true,"consented_at":"2026-01-02T03:04:05Z"}}`
	if err := os.WriteFile(settingsPath, []byte(raw), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	s, err := ReadUserSettings()
	if err != nil || s == nil {
		t.Fatalf("expected to read pre-Wire-1 file, got %v / %v", s, err)
	}
	if s.Entitlements != nil {
		t.Fatalf("Entitlements should be nil for a pre-Wire-1 file, got %+v", s.Entitlements)
	}
	if HasCapability("pack_init") {
		t.Fatal("a settings file with no entitlements + no legacy pro_active must not grant anything")
	}
}
