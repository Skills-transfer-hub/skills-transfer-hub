// Package cloud houses HTTP clients for the STH Cloud REST API. Today it
// covers the entitlements endpoint that drives the Pro gate; access-check
// still lives under internal/http for legacy reasons.
package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	stdhttp "net/http"
	"strings"
	"time"

	"sth/internal/auth"
	sthhttp "sth/internal/http"
)

// Entitlements is the decoded response of GET /v1/entitlements. Returned to
// the Pro gate which persists it in user settings (see fs.EntitlementsCache)
// and consults HasCapability("pack_init") to decide.
type Entitlements struct {
	Plan         string    `json:"plan"`
	ExpiresAt    time.Time `json:"expires_at"`
	Capabilities []string  `json:"capabilities"`
}

// ErrUnauthorized signals an HTTP 401 from /v1/entitlements. The caller MUST
// clear the keychain and surface a "run `sth login`" message — the key has
// either been revoked, expired, or is unknown to the server.
var ErrUnauthorized = errors.New("entitlements: clé refusée (HTTP 401)")

// ErrNoKey is the alias of auth.ErrKeyNotFound surfaced from this package so
// callers can switch on a single sentinel set. Returned when neither the
// keychain nor STH_CLOUD_API_KEY has a value.
var ErrNoKey = auth.ErrKeyNotFound

const fetchTimeout = 10 * time.Second
const maxResponseBytes = 1 << 20

// FetchEntitlements calls GET $STH_CLOUD_BASE_URL/v1/entitlements with the
// API key from auth.LoadAPIKey(). hc is optional — pass nil to use the
// default http.Client with a 10s timeout (httptest passes its own server's
// client for deterministic tests).
func FetchEntitlements(ctx context.Context, hc *stdhttp.Client) (Entitlements, error) {
	if hc == nil {
		hc = &stdhttp.Client{Timeout: fetchTimeout}
	}
	key, err := auth.LoadAPIKey()
	if err != nil {
		return Entitlements{}, err
	}
	url := sthhttp.CloudBaseURL() + "/v1/entitlements"
	req, err := stdhttp.NewRequestWithContext(ctx, stdhttp.MethodGet, url, nil)
	if err != nil {
		return Entitlements{}, fmt.Errorf("entitlements: préparation requête : %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")

	resp, err := hc.Do(req)
	if err != nil {
		return Entitlements{}, fmt.Errorf("entitlements: requête vers %s échouée : %w", url, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case stdhttp.StatusOK:
		// fall through to body decode
	case stdhttp.StatusUnauthorized:
		return Entitlements{}, ErrUnauthorized
	default:
		return Entitlements{}, fmt.Errorf("entitlements: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return Entitlements{}, fmt.Errorf("entitlements: lecture du corps : %w", err)
	}
	return DecodeEntitlements(body)
}

// DecodeEntitlements parses the raw JSON body returned by /v1/entitlements.
// Extracted so Go fuzz tests (FuzzDecodeEntitlements) can exercise the parser
// in isolation and so the unit tests can assert error messages directly.
func DecodeEntitlements(body []byte) (Entitlements, error) {
	var raw struct {
		Plan         string   `json:"plan"`
		ExpiresAt    string   `json:"expires_at"`
		Capabilities []string `json:"capabilities"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return Entitlements{}, fmt.Errorf("entitlements: JSON invalide : %w", err)
	}
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(raw.ExpiresAt))
	if err != nil {
		return Entitlements{}, fmt.Errorf("entitlements: expires_at invalide (%q) : %w", raw.ExpiresAt, err)
	}
	caps := raw.Capabilities
	if caps == nil {
		caps = []string{}
	}
	return Entitlements{
		Plan:         strings.TrimSpace(raw.Plan),
		ExpiresAt:    t,
		Capabilities: caps,
	}, nil
}
