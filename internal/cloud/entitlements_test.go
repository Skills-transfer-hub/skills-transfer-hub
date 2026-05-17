package cloud

import (
	"context"
	"errors"
	"fmt"
	"io"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"sth/internal/auth"
)

// Each test isolates STH_CLOUD_BASE_URL + STH_CLOUD_API_KEY + the user config
// dir so the keychain helper (auth.LoadAPIKey) sees a deterministic state.
func setupTestEnv(t *testing.T) {
	t.Helper()
	t.Setenv("STH_USER_CONFIG_DIR", t.TempDir())
	// Don't leak whatever key the dev machine has in its keychain into tests.
	t.Setenv(auth.EnvAPIKey, "lsk_live_TESTKEY")
}

// newTestServer wraps a httptest.NewServer + sets STH_CLOUD_BASE_URL so the
// production CloudBaseURL() helper picks it up transparently.
func newTestServer(t *testing.T, h stdhttp.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	t.Setenv("STH_CLOUD_BASE_URL", srv.URL)
	return srv
}

// ─── happy path × 3 plans ─────────────────────────────────────────────────

func TestFetchEntitlements_OK_Pro(t *testing.T) {
	setupTestEnv(t)
	newTestServer(t, func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if r.URL.Path != "/v1/entitlements" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"plan":"pro","expires_at":"2026-12-31T23:59:59Z","capabilities":["private_repos","pack_init","multi_target"]}`)
	})
	ent, err := FetchEntitlements(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ent.Plan != "pro" || len(ent.Capabilities) != 3 {
		t.Fatalf("unexpected response: %+v", ent)
	}
}

func TestFetchEntitlements_OK_Team(t *testing.T) {
	setupTestEnv(t)
	newTestServer(t, func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		_, _ = io.WriteString(w, `{"plan":"team","expires_at":"2027-01-15T23:59:59Z","capabilities":["private_repos","pack_init","multi_target","fleet_sync"]}`)
	})
	ent, err := FetchEntitlements(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ent.Plan != "team" || len(ent.Capabilities) != 4 {
		t.Fatalf("unexpected response: %+v", ent)
	}
}

func TestFetchEntitlements_OK_Starter(t *testing.T) {
	setupTestEnv(t)
	newTestServer(t, func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		_, _ = io.WriteString(w, `{"plan":"starter","expires_at":"2026-12-31T23:59:59Z","capabilities":[]}`)
	})
	ent, err := FetchEntitlements(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ent.Plan != "starter" || len(ent.Capabilities) != 0 {
		t.Fatalf("unexpected response: %+v", ent)
	}
}

// ─── HTTP status branches ─────────────────────────────────────────────────

func TestFetchEntitlements_Unauthorized(t *testing.T) {
	setupTestEnv(t)
	newTestServer(t, func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		w.WriteHeader(stdhttp.StatusUnauthorized)
	})
	_, err := FetchEntitlements(context.Background(), nil)
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestFetchEntitlements_Forbidden(t *testing.T) {
	setupTestEnv(t)
	newTestServer(t, func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		w.WriteHeader(stdhttp.StatusForbidden)
	})
	_, err := FetchEntitlements(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "HTTP 403") {
		t.Fatalf("expected HTTP 403 error, got %v", err)
	}
}

func TestFetchEntitlements_ServerError(t *testing.T) {
	setupTestEnv(t)
	newTestServer(t, func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		w.WriteHeader(stdhttp.StatusInternalServerError)
	})
	_, err := FetchEntitlements(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("expected HTTP 500 error, got %v", err)
	}
}

// ─── Body / parsing branches ──────────────────────────────────────────────

func TestFetchEntitlements_BadJSON(t *testing.T) {
	setupTestEnv(t)
	newTestServer(t, func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		_, _ = io.WriteString(w, `{not valid json`)
	})
	_, err := FetchEntitlements(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "JSON invalide") {
		t.Fatalf("expected JSON parse error, got %v", err)
	}
}

func TestFetchEntitlements_EmptyBody(t *testing.T) {
	setupTestEnv(t)
	newTestServer(t, func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		w.Header().Set("Content-Length", "0")
	})
	_, err := FetchEntitlements(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error on empty body")
	}
}

func TestFetchEntitlements_BodyTooLarge(t *testing.T) {
	setupTestEnv(t)
	// Send 2 MiB so the io.LimitReader (1 MiB) truncates the payload,
	// producing an invalid JSON tail that must surface as a parse error.
	huge := strings.Repeat("a", 2<<20)
	newTestServer(t, func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		_, _ = fmt.Fprintf(w, `{"plan":"pro","expires_at":"2026-12-31T23:59:59Z","capabilities":["%s"]}`, huge)
	})
	_, err := FetchEntitlements(context.Background(), nil)
	if err == nil {
		t.Fatal("expected JSON parse error after LimitReader truncation, got nil")
	}
}

func TestFetchEntitlements_BadDate_NonRFC3339(t *testing.T) {
	setupTestEnv(t)
	newTestServer(t, func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		_, _ = io.WriteString(w, `{"plan":"pro","expires_at":"Sun, 31 Dec 2026 23:59:59 GMT","capabilities":["pack_init"]}`)
	})
	_, err := FetchEntitlements(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "expires_at") {
		t.Fatalf("expected expires_at parse error, got %v", err)
	}
}

func TestFetchEntitlements_BadDate_Empty(t *testing.T) {
	setupTestEnv(t)
	newTestServer(t, func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		_, _ = io.WriteString(w, `{"plan":"pro","expires_at":"","capabilities":["pack_init"]}`)
	})
	_, err := FetchEntitlements(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "expires_at") {
		t.Fatalf("expected empty expires_at error, got %v", err)
	}
}

func TestFetchEntitlements_BadDate_FutureFarParsesOK(t *testing.T) {
	// A date 100 years in the future is still RFC3339 — the parser must
	// accept it. The enforcer decides what to do with it (it doesn't).
	setupTestEnv(t)
	newTestServer(t, func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		_, _ = io.WriteString(w, `{"plan":"team","expires_at":"2125-12-31T23:59:59Z","capabilities":["fleet_sync"]}`)
	})
	ent, err := FetchEntitlements(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ent.ExpiresAt.Year() != 2125 {
		t.Fatalf("unexpected year: %d", ent.ExpiresAt.Year())
	}
}

// ─── Auth / no-key branch ─────────────────────────────────────────────────

func TestFetchEntitlements_NoKey(t *testing.T) {
	t.Setenv("STH_USER_CONFIG_DIR", t.TempDir())
	t.Setenv(auth.EnvAPIKey, "")
	// httptest server not needed — LoadAPIKey fails before any request.
	_, err := FetchEntitlements(context.Background(), nil)
	if !errors.Is(err, auth.ErrKeyNotFound) {
		t.Fatalf("expected ErrKeyNotFound, got %v", err)
	}
	// Sanity check on the package-level alias.
	if ErrNoKey != auth.ErrKeyNotFound {
		t.Fatal("ErrNoKey must alias auth.ErrKeyNotFound")
	}
}

// ─── Cancellation / network failure ───────────────────────────────────────

func TestFetchEntitlements_ContextCancelled(t *testing.T) {
	setupTestEnv(t)
	newTestServer(t, func(_ stdhttp.ResponseWriter, _ *stdhttp.Request) {
		// Hold the request open; the cancelled context should abort us.
		time.Sleep(2 * time.Second)
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	_, err := FetchEntitlements(ctx, nil)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestFetchEntitlements_NetworkRefused(t *testing.T) {
	setupTestEnv(t)
	// Port 1 is reserved and typically refuses immediately.
	t.Setenv("STH_CLOUD_BASE_URL", "http://127.0.0.1:1")
	_, err := FetchEntitlements(context.Background(), nil)
	if err == nil {
		t.Fatal("expected connection-refused error")
	}
}

// ─── Header forwarding contract ───────────────────────────────────────────

func TestFetchEntitlements_BearerHeaderSent(t *testing.T) {
	setupTestEnv(t)
	t.Setenv(auth.EnvAPIKey, "lsk_live_HEADERCHECK")
	gotAuth := ""
	newTestServer(t, func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = io.WriteString(w, `{"plan":"starter","expires_at":"2026-12-31T23:59:59Z","capabilities":[]}`)
	})
	if _, err := FetchEntitlements(context.Background(), nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth != "Bearer lsk_live_HEADERCHECK" {
		t.Fatalf("expected Bearer header to be forwarded, got %q", gotAuth)
	}
}

// ─── DecodeEntitlements: pure parser tests (also driven by FuzzDecodeEntitlements) ──

func TestDecodeEntitlements_NilCapabilitiesBecomesEmpty(t *testing.T) {
	// Server omitted the field entirely; we want callers to be able to
	// safely range over Capabilities without a nil-check.
	body := []byte(`{"plan":"starter","expires_at":"2026-12-31T23:59:59Z"}`)
	ent, err := DecodeEntitlements(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ent.Capabilities == nil || len(ent.Capabilities) != 0 {
		t.Fatalf("expected empty (non-nil) capabilities, got %v", ent.Capabilities)
	}
}

func TestDecodeEntitlements_TrimsPlanWhitespace(t *testing.T) {
	body := []byte(`{"plan":"  pro  ","expires_at":"2026-12-31T23:59:59Z","capabilities":[]}`)
	ent, err := DecodeEntitlements(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ent.Plan != "pro" {
		t.Fatalf("plan = %q, want 'pro'", ent.Plan)
	}
}

// ─── Fuzz: the decoder must never panic on garbage input ──────────────────

func FuzzDecodeEntitlements(f *testing.F) {
	// Seeds: well-formed, missing fields, mixed types.
	f.Add([]byte(`{"plan":"pro","expires_at":"2026-12-31T23:59:59Z","capabilities":["pack_init"]}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`null`))
	f.Add([]byte(`[]`))
	f.Add([]byte(`{"plan":42}`))
	f.Add([]byte(`{"capabilities":"not-an-array"}`))
	f.Fuzz(func(t *testing.T, body []byte) {
		// Allowed outcomes: err != nil, or err == nil with a sane struct.
		// What we forbid: a panic.
		_, _ = DecodeEntitlements(body)
	})
}
