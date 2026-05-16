package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"sth/internal/fs"
)

const (
	// proGateSkipEnvVar disables the Pro gate when set to "0". Mirrors the
	// alpha gate pattern so power users / CI only need to remember one
	// disable mechanism per gate (here: this var is the dev escape hatch
	// for everything Pro-only).
	proGateSkipEnvVar = "STH_PRO_GATE"

	// proGateCacheTTL is the lifetime of a successful entitlement check
	// stored in user settings. Inside this window we let Pro commands run
	// even if the user is offline (degraded silent mode); past this window
	// an offline failure blocks the command.
	proGateCacheTTL = 24 * time.Hour
)

// enforceProGate refuses to run a Pro-only command unless the user is on a
// Pro plan. Resolution order:
//
//  1. STH_PRO_GATE=0 — bypass (CI and power users).
//  2. Dev build (no release metadata) — bypass (tests, `go run`).
//  3. Cached "pro_active=true" within proGateCacheTTL — pass.
//  4. Live entitlement check — TODO once the entitlements endpoint ships;
//     today this returns an explicit "not Pro" error pointing the user at
//     the docs and the env-var escape hatch.
//  5. Cached "pro_active=true" past TTL + offline — block with a clear
//     "retry online" message.
func (a *app) enforceProGate(_ context.Context) error {
	if strings.TrimSpace(os.Getenv(proGateSkipEnvVar)) == "0" {
		return nil
	}
	if strings.TrimSpace(currentReleaseMetadata().ReleasesAPIURL) == "" {
		return nil
	}

	settings, _ := fs.ReadUserSettings()
	now := time.Now().UTC()
	if settings != nil && settings.ProActive && !settings.ProCheckedAt.IsZero() && now.Sub(settings.ProCheckedAt) < proGateCacheTTL {
		return nil
	}

	return proGateNotEntitledError()
}

func proGateNotEntitledError() error {
	return errors.New(
		"`sth pack` est réservé à l'offre Pro. " +
			"Pour scaffolder un repo de skills manuellement, voir " + DocsURL() + " . " +
			"Si tu travailles sur le binaire en local, exporte " + proGateSkipEnvVar + "=0",
	)
}

// proGateMarkEntitled writes a fresh entitlement cache entry into the user
// settings file. Wired by the entitlements endpoint once it lands; exposed
// here so tests can drive the cached-pass branch without hitting the API.
//
//nolint:deadcode,unused // Intentional: hook point for the entitlements endpoint.
func proGateMarkEntitled() error {
	settings, err := fs.ReadUserSettings()
	if err != nil {
		return err
	}
	if settings == nil {
		settings = &fs.UserSettings{}
	}
	settings.ProActive = true
	settings.ProCheckedAt = time.Now().UTC()
	_, err = fs.WriteUserSettings(*settings)
	if err != nil {
		return fmt.Errorf("écriture des settings utilisateur échouée : %w", err)
	}
	return nil
}
