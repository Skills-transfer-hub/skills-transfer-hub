package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"sth/internal/fs"
	sthhttp "sth/internal/http"
)

const (
	// alphaGateTimeout caps the GitHub releases lookup so a slow network can't
	// hang the CLI indefinitely. Short enough to feel snappy, long enough that
	// a healthy connection always succeeds.
	alphaGateTimeout = 3 * time.Second

	// alphaGateCacheTTLh shortens the shared update-check cache for gate-driven
	// writes. The `update` command keeps its 24h TTL; gate writes carry 1h so a
	// fresh release reaches users within the hour even without `update self`.
	alphaGateCacheTTLh = 1

	// alphaGateSkipEnvVar disables the gate when set to "0". Used by the release
	// CI pipeline and as an escape hatch for power users who accept the risk.
	alphaGateSkipEnvVar = "STH_ALPHA_GATE"
)

// shouldBypassAlphaGate keeps a minimal set of commands always usable so users
// can never get fully bricked: read-only inspection (version/help) and the two
// commands needed to recover (update self to upgrade, logout to detach the key).
func shouldBypassAlphaGate(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "version", "help", "--help", "-h", "logout":
		return true
	case "update":
		return len(args) >= 2 && args[1] == "self"
	}
	return false
}

// enforceAlphaGate refuses to run if the binary is older than the latest
// GitHub release, or if the version check itself fails. Returns nil for dev
// builds (no release metadata baked in), bypassed commands, and when the env
// escape hatch is set.
func (a *app) enforceAlphaGate(ctx context.Context, args []string) error {
	if shouldBypassAlphaGate(args) {
		return nil
	}
	if strings.TrimSpace(os.Getenv(alphaGateSkipEnvVar)) == "0" {
		return nil
	}

	metadata := currentReleaseMetadata()
	if strings.TrimSpace(metadata.ReleasesAPIURL) == "" {
		// Dev build (`go run`, tests, untagged binaries) — no way to check, so
		// the gate is intentionally inert here.
		return nil
	}

	now := time.Now().UTC()
	if cached, err := fs.ReadUpdateCheckState(); err == nil && cached != nil && cached.IsFresh(now, metadata.CurrentVersion) {
		switch strings.TrimSpace(cached.Status) {
		case "no-update":
			return nil
		case "update-available":
			return alphaGateOutdatedError(metadata.CurrentVersion, cached.LatestStableVersion)
		}
		// "error" or unknown status: fall through and re-check fresh.
	}

	checkCtx, cancel := context.WithTimeout(ctx, alphaGateTimeout)
	defer cancel()
	release, err := a.client.CheckForStableUpdate(checkCtx, metadata)
	if err != nil {
		writeAlphaGateState(fs.NewErrorUpdateCheckState(now, metadata.CurrentVersion, err.Error()))
		return alphaGateUnreachableError(err)
	}
	if release != nil {
		writeAlphaGateState(fs.NewAvailableUpdateCheckState(
			now, metadata.CurrentVersion, release.Version,
			release.DownloadURL, release.ChecksumURL, release.AssetName,
		))
		return alphaGateOutdatedError(metadata.CurrentVersion, release.Version)
	}
	writeAlphaGateState(fs.NewNoUpdateCheckState(now, metadata.CurrentVersion))
	return nil
}

// writeAlphaGateState persists the cache entry with the gate's shorter TTL so
// the next invocation reuses the result for up to alphaGateCacheTTLh hours.
// Errors are swallowed: the gate can re-check on the next run.
func writeAlphaGateState(state fs.UpdateCheckState) {
	state.TTLHours = alphaGateCacheTTLh
	_, _ = fs.WriteUpdateCheckState(state)
}

// osExecutable is package-level so tests can stub it without touching the
// global os.Executable function.
var osExecutable = os.Executable

// upgradeCommand returns the shell command users should run to get the latest
// version, picking the right package-manager hint when the binary is owned by
// brew or scoop, and falling back to `sth update self` otherwise.
func upgradeCommand() string {
	if execPath, err := osExecutable(); err == nil {
		if pm := detectPackageManager(execPath); pm.name != "" {
			return pm.upgradeHint
		}
	}
	return "sth update self"
}

func alphaGateOutdatedError(current, latest string) error {
	return fmt.Errorf(
		"Alpha : version %s obsolète, dernière version %s. Lance `%s` pour mettre à jour",
		valueOrDefault(current, "dev"),
		valueOrDefault(latest, "inconnue"),
		upgradeCommand(),
	)
}

func alphaGateUnreachableError(cause error) error {
	return fmt.Errorf(
		"Alpha : impossible de vérifier la dernière version (%w). Pendant la phase Alpha, sth refuse de tourner sans confirmation que tu es à jour. Vérifie ta connexion et relance, ou exporte STH_ALPHA_GATE=0 si tu sais ce que tu fais",
		cause,
	)
}

// Compile-time guard so a future refactor of the http client surface trips a
// build error here instead of silently breaking the gate.
var _ = func(c *sthhttp.Client) error {
	_, err := c.CheckForStableUpdate(context.Background(), sthhttp.ReleaseMetadata{})
	return err
}
