package cmd

import (
	"context"
	"io"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"sth/internal/auth"
	"sth/internal/fs"
)

// On dev builds (no release metadata baked in) the Pro gate is intentionally
// inert — same policy as the alpha gate — so `go test ./...` never hits a
// gated error path. The matching production code path is the
// "STH_PRO_GATE=0" escape hatch, which we exercise below as a documented
// bypass for power users.
func TestEnforceProGate_BypassesOnDevBuild(t *testing.T) {
	a := &app{}
	if err := a.enforceProGate(context.Background()); err != nil {
		t.Errorf("dev build should bypass Pro gate, got %v", err)
	}
}

func TestEnforceProGate_EnvVarBypass(t *testing.T) {
	t.Setenv(proGateSkipEnvVar, "0")
	a := &app{}
	if err := a.enforceProGate(context.Background()); err != nil {
		t.Errorf("STH_PRO_GATE=0 should bypass, got %v", err)
	}
}

// proGateNotEntitledError is the message users see when they hit the gate on
// a release build without an active entitlement. The wording is part of the
// product contract — keep it stable so docs / Pro upsell can quote it.
func TestProGateNotEntitledError_MentionsEnvVarAndDocs(t *testing.T) {
	err := proGateNotEntitledError()
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	msg := err.Error()
	if !strings.Contains(msg, proGateSkipEnvVar) {
		t.Errorf("error should mention the bypass env var %q, got %q", proGateSkipEnvVar, msg)
	}
	if !strings.Contains(msg, "Pro") {
		t.Errorf("error should mention Pro, got %q", msg)
	}
}

// ─── enforceCapability live-path tests (Wire 1) ────────────────────────────
//
// These tests have to opt out of the dev-build bypass enforced by
// enforceCapability — we do that by setting a release-metadata URL on the
// fly via setReleaseMetadataForTest. They isolate the keychain, config dir,
// and STH_CLOUD_BASE_URL so the live branch is exercised end-to-end.

func enableProGateForTest(t *testing.T) {
	t.Helper()
	// currentReleaseMetadata() reads releaseAPIURL (a package var set at
	// build time via -ldflags). Override it for the duration of the test so
	// the gate's "no release metadata → bypass" branch does not fire.
	prev := releaseAPIURL
	t.Cleanup(func() { releaseAPIURL = prev })
	releaseAPIURL = "https://example.invalid/releases"
}

func setupEntitlementsServer(t *testing.T, handler stdhttp.HandlerFunc) *httptest.Server {
	t.Helper()
	t.Setenv("STH_USER_CONFIG_DIR", t.TempDir())
	t.Setenv(auth.EnvAPIKey, "lsk_live_PROGATETEST")
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	t.Setenv("STH_CLOUD_BASE_URL", srv.URL)
	enableProGateForTest(t)
	return srv
}

func TestEnforceCapability_LiveAllowsWhenCapPresent(t *testing.T) {
	setupEntitlementsServer(t, func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		_, _ = io.WriteString(w, `{"plan":"pro","expires_at":"2026-12-31T23:59:59Z","capabilities":["pack_init"]}`)
	})
	a := &app{}
	if err := a.enforceCapability(context.Background(), "pack_init"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Cache should now be populated.
	settings, _ := fs.ReadUserSettings()
	if settings == nil || settings.Entitlements == nil {
		t.Fatal("entitlements cache should be populated after live fetch")
	}
	if !fs.HasCapability("pack_init") {
		t.Fatal("pack_init should be granted after live success")
	}
}

func TestEnforceCapability_LiveDeniesWhenCapMissing(t *testing.T) {
	setupEntitlementsServer(t, func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		_, _ = io.WriteString(w, `{"plan":"starter","expires_at":"2026-12-31T23:59:59Z","capabilities":[]}`)
	})
	a := &app{}
	err := a.enforceCapability(context.Background(), "pack_init")
	if err == nil {
		t.Fatal("expected deny when capability is missing")
	}
	if !strings.Contains(err.Error(), "pack_init") {
		t.Errorf("error should mention the missing capability, got %q", err)
	}
}

func TestEnforceCapability_NetworkErrFallsBackToCache(t *testing.T) {
	// Use port 1 (refused) so the live fetch fails; but seed a fresh cache.
	t.Setenv("STH_USER_CONFIG_DIR", t.TempDir())
	t.Setenv(auth.EnvAPIKey, "lsk_live_X")
	t.Setenv("STH_CLOUD_BASE_URL", "http://127.0.0.1:1")
	enableProGateForTest(t)

	if err := fs.SaveEntitlements(fs.EntitlementsCache{
		Plan:         "pro",
		ExpiresAt:    time.Now().UTC().Add(30 * 24 * time.Hour),
		Capabilities: []string{"pack_init"},
		// CheckedAt within the 24h hot TTL — the gate must short-circuit
		// BEFORE attempting the network at all.
		CheckedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SaveEntitlements: %v", err)
	}

	a := &app{}
	if err := a.enforceCapability(context.Background(), "pack_init"); err != nil {
		t.Fatalf("expected hot-cache path to allow without network; got %v", err)
	}
}

func TestEnforceCapability_NetworkErrStaleAllowsWithinSilentWindow(t *testing.T) {
	t.Setenv("STH_USER_CONFIG_DIR", t.TempDir())
	t.Setenv(auth.EnvAPIKey, "lsk_live_X")
	t.Setenv("STH_CLOUD_BASE_URL", "http://127.0.0.1:1")
	enableProGateForTest(t)

	// Cache is stale (>24h) but within the 7-day silent-allow window.
	if err := fs.SaveEntitlements(fs.EntitlementsCache{
		Plan:         "pro",
		ExpiresAt:    time.Now().UTC().Add(30 * 24 * time.Hour),
		Capabilities: []string{"pack_init"},
		CheckedAt:    time.Now().UTC().Add(-(48 * time.Hour)),
	}); err != nil {
		t.Fatalf("SaveEntitlements: %v", err)
	}

	a := &app{}
	if err := a.enforceCapability(context.Background(), "pack_init"); err != nil {
		t.Fatalf("expected stale cache + offline to silently allow; got %v", err)
	}
}

func TestEnforceCapability_NetworkErrNoCacheDenies(t *testing.T) {
	t.Setenv("STH_USER_CONFIG_DIR", t.TempDir())
	t.Setenv(auth.EnvAPIKey, "lsk_live_X")
	t.Setenv("STH_CLOUD_BASE_URL", "http://127.0.0.1:1")
	enableProGateForTest(t)

	a := &app{}
	if err := a.enforceCapability(context.Background(), "pack_init"); err == nil {
		t.Fatal("expected deny when there is no cache and the network is down")
	}
}

func TestEnforceCapability_Unauthorized_ClearsKey(t *testing.T) {
	setupEntitlementsServer(t, func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		w.WriteHeader(stdhttp.StatusUnauthorized)
	})
	// Note: env fallback still holds the key after DeleteAPIKey wipes the
	// keychain. We assert behaviour by checking the error message instead
	// of trying to verify the keychain delete (which is a no-op against
	// the env-backed fallback path used in tests).
	a := &app{}
	err := a.enforceCapability(context.Background(), "pack_init")
	if err == nil || !strings.Contains(err.Error(), "sth login") {
		t.Fatalf("expected re-login message, got %v", err)
	}
}

func TestEnforceCapability_GenericCapability_MultiTarget(t *testing.T) {
	// Wire 2 will gate multi-target install behind "multi_target". Make
	// sure the helper distinguishes capabilities correctly.
	setupEntitlementsServer(t, func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		_, _ = io.WriteString(w, `{"plan":"pro","expires_at":"2026-12-31T23:59:59Z","capabilities":["multi_target"]}`)
	})
	a := &app{}
	if err := a.enforceCapability(context.Background(), "multi_target"); err != nil {
		t.Fatalf("multi_target should be granted: %v", err)
	}
	if err := a.enforceCapability(context.Background(), "fleet_sync"); err == nil {
		t.Fatal("fleet_sync must be denied — server only advertised multi_target")
	}
}

func TestEnforceProGate_DelegatesToEnforceCapability_PackInit(t *testing.T) {
	setupEntitlementsServer(t, func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		_, _ = io.WriteString(w, `{"plan":"pro","expires_at":"2026-12-31T23:59:59Z","capabilities":["pack_init"]}`)
	})
	a := &app{}
	if err := a.enforceProGate(context.Background()); err != nil {
		t.Fatalf("enforceProGate must delegate to enforceCapability(\"pack_init\"); got %v", err)
	}
}

func TestEnforceCapability_StaleSilentWindow_Boundary(t *testing.T) {
	t.Setenv("STH_USER_CONFIG_DIR", t.TempDir())
	t.Setenv(auth.EnvAPIKey, "lsk_live_X")
	t.Setenv("STH_CLOUD_BASE_URL", "http://127.0.0.1:1")
	enableProGateForTest(t)

	// Just BEFORE the 7-day boundary: should silently allow.
	if err := fs.SaveEntitlements(fs.EntitlementsCache{
		Plan:         "pro",
		ExpiresAt:    time.Now().UTC().Add(30 * 24 * time.Hour),
		Capabilities: []string{"pack_init"},
		CheckedAt:    time.Now().UTC().Add(-(proGateStaleSilentWindow - time.Hour)),
	}); err != nil {
		t.Fatalf("SaveEntitlements: %v", err)
	}
	a := &app{}
	if err := a.enforceCapability(context.Background(), "pack_init"); err != nil {
		t.Fatalf("just-inside-silent-window must allow: %v", err)
	}

	// Just AFTER the 7-day boundary: must deny.
	if err := fs.SaveEntitlements(fs.EntitlementsCache{
		Plan:         "pro",
		ExpiresAt:    time.Now().UTC().Add(30 * 24 * time.Hour),
		Capabilities: []string{"pack_init"},
		CheckedAt:    time.Now().UTC().Add(-(proGateStaleSilentWindow + time.Hour)),
	}); err != nil {
		t.Fatalf("SaveEntitlements: %v", err)
	}
	if err := a.enforceCapability(context.Background(), "pack_init"); err == nil {
		t.Fatal("just-past-silent-window must deny")
	}
}
