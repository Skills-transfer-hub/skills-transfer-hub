package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"sth/internal/auth"
	"sth/internal/cloud"
	"sth/internal/fs"
)

const (
	// proGateSkipEnvVar disables the gate when set to "0". Single escape
	// hatch shared by every capability check.
	proGateSkipEnvVar = "STH_PRO_GATE"

	// proGateCacheTTL is the hot-cache window for a successful entitlement
	// check. Inside this window we never re-fetch (network is best-effort).
	proGateCacheTTL = 24 * time.Hour

	// proGateStaleSilentWindow lets the cache silently authorize commands
	// when the network is down up to 7 days after the last successful
	// check. Beyond that an offline gate fails closed — defensive against
	// a long-revoked key that happens to still cache a "pack_init" grant.
	proGateStaleSilentWindow = 7 * 24 * time.Hour
)

// enforceCapability refuses to run a feature gated behind the given
// capability key (e.g. "pack_init" for Pro, "multi_target" for Wire 2,
// "fleet_sync" for Team).
//
// Resolution order:
//  1. STH_PRO_GATE=0  → bypass (CI, power users, devs hacking the binary).
//  2. Dev build (no release metadata baked in) → bypass.
//  3. Hot cache (< proGateCacheTTL) with capability present → allow.
//  4. Live GET /v1/entitlements:
//      4a. HTTP 401 → clear keychain and tell user to re-login.
//      4b. Network error + cache <7d with capability → allow (degraded).
//      4c. Network error + nothing usable → deny with docs link.
//      4d. HTTP 200 → persist cache, allow iff capability is present.
func (a *app) enforceCapability(ctx context.Context, capability string) error {
	if strings.TrimSpace(os.Getenv(proGateSkipEnvVar)) == "0" {
		return nil
	}
	if strings.TrimSpace(currentReleaseMetadata().ReleasesAPIURL) == "" {
		return nil
	}

	now := time.Now().UTC()
	settings, _ := fs.ReadUserSettings()

	// Hot cache check.
	if settings != nil && settings.Entitlements != nil &&
		!settings.Entitlements.CheckedAt.IsZero() &&
		now.Sub(settings.Entitlements.CheckedAt) < proGateCacheTTL &&
		containsCapability(settings.Entitlements.Capabilities, capability) {
		return nil
	}

	httpCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	ent, err := cloud.FetchEntitlements(httpCtx, nil)
	if err != nil {
		if errors.Is(err, cloud.ErrUnauthorized) {
			// The key is no longer valid — wipe it so the next interactive
			// command doesn't keep trying. Best-effort delete.
			_ = auth.DeleteAPIKey()
			return errors.New(
				"clé STH Cloud refusée par le serveur — relance `sth login` " +
					"avec une clé valide (ou exporte " + proGateSkipEnvVar + "=0 pour bypass).",
			)
		}
		// Network failure: tolerate a stale cache if it grants the capability.
		if settings != nil && settings.Entitlements != nil &&
			!settings.Entitlements.CheckedAt.IsZero() &&
			now.Sub(settings.Entitlements.CheckedAt) < proGateStaleSilentWindow &&
			containsCapability(settings.Entitlements.Capabilities, capability) {
			return nil
		}
		return notEntitledError(capability)
	}

	if saveErr := fs.SaveEntitlements(fs.EntitlementsCache{
		Plan:         ent.Plan,
		ExpiresAt:    ent.ExpiresAt,
		Capabilities: ent.Capabilities,
		CheckedAt:    now,
	}); saveErr != nil {
		a.writeStderr(fmt.Sprintf("Avertissement : cache entitlements non écrit : %v", saveErr))
	}

	if !containsCapability(ent.Capabilities, capability) {
		return notEntitledError(capability)
	}
	return nil
}

// enforceProGate is the historical entry point. Kept as a thin wrapper so
// the dozens of call sites in cmd/pack*.go don't have to change.
func (a *app) enforceProGate(ctx context.Context) error {
	return a.enforceCapability(ctx, "pack_init")
}

// proGateMarkEntitled primes the cache after a successful `sth login`. Calls
// the live endpoint and persists whatever the server returns — best-effort,
// callers warn on failure but never block the login.
func proGateMarkEntitled(ctx context.Context) error {
	httpCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	ent, err := cloud.FetchEntitlements(httpCtx, nil)
	if err != nil {
		return err
	}
	return fs.SaveEntitlements(fs.EntitlementsCache{
		Plan:         ent.Plan,
		ExpiresAt:    ent.ExpiresAt,
		Capabilities: ent.Capabilities,
		CheckedAt:    time.Now().UTC(),
	})
}

func proGateNotEntitledError() error {
	// Preserved for any external caller / test that still references the
	// pack_init message verbatim. New code paths go through notEntitledError.
	return notEntitledError("pack_init")
}

func notEntitledError(capability string) error {
	return fmt.Errorf(
		"cette commande requiert la capability `%s` (offre Pro / Team). "+
			"Voir %s pour souscrire, ou exporte %s=0 si tu travailles sur le binaire en local.",
		capability, DocsURL(), proGateSkipEnvVar,
	)
}

func containsCapability(haystack []string, needle string) bool {
	for _, c := range haystack {
		if c == needle {
			return true
		}
	}
	return false
}
