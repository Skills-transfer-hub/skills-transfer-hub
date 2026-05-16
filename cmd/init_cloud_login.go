package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"strings"

	"sth/internal/auth"
	"sth/internal/fs"
)

// promptCloudLoginIfNeeded is the sub-prompt that runs after the user picked
// telemetry mode "connected" in the init wizard. It guarantees that, at the
// end of the call, the user is in one of two consistent states:
//
//   - mode = connected, keychain key present, provider sth-cloud attached;
//   - mode = anonymous (gracefully downgraded), with a hint pointing at
//     `sth login` to upgrade later.
//
// The function never returns a non-nil error to the caller — failures degrade
// the mode silently rather than aborting `sth init`. The returned mode is
// the persisted one after this call.
func (a *app) promptCloudLoginIfNeeded(reader *bufio.Reader, current fs.TelemetryMode) fs.TelemetryMode {
	if current != fs.TelemetryConnected {
		return current
	}

	hasKey := false
	if _, err := auth.LoadAPIKey(); err == nil {
		hasKey = true
	}

	cfg, _ := fs.ReadProjectConfigV2()
	_, providerAttached := findCloudProviderProfile(cfg)

	switch {
	case hasKey && providerAttached:
		// Nothing to do: the user is already fully wired for connected mode.
		return fs.TelemetryConnected
	case hasKey && !providerAttached:
		// Key already in keychain (probably from a previous `sth login` on
		// another project on the same machine). Silently attach the provider.
		if err := ensureCloudProviderEntry(); err != nil {
			a.writeStderr(fmt.Sprintf("Avertissement : provider sth-cloud non rattaché : %v", err))
			return a.downgradeToAnonymous("STH Cloud injoignable", err)
		}
		a.writeStdout("Provider sth-cloud rattaché au projet.", "")
		return fs.TelemetryConnected
	}

	// No key yet. In non-interactive sessions we cannot prompt — degrade now
	// with a hint instead of hanging.
	if !a.isInteractiveSession() {
		a.writeStderr("Astuce : lance `sth login` pour activer le dashboard STH Cloud.")
		return a.downgradeToAnonymous("non-interactif", nil)
	}

	a.writeStdout(
		renderBuddy(fs.BuddyAllGood),
		"",
		"Pour voir tes commandes dans le dashboard STH Cloud, connecte-toi maintenant.",
		fmt.Sprintf("Récupère ta clé `lsk_live_…` sur %s", DashboardLicensesURL()),
		"Tu peux refuser et le faire plus tard avec `sth login`.",
		"",
	)
	doIt, err := promptYesNo(reader, a.stdout, "Se connecter à STH Cloud maintenant ?", false)
	if err != nil {
		return a.downgradeToAnonymous("entrée utilisateur indisponible", err)
	}
	if !doIt {
		a.writeStdout("OK. Lance `sth login` quand tu veux activer ton dashboard.", "")
		return a.downgradeToAnonymous("refus", nil)
	}

	if err := a.promptCloudLoginInteractive(reader); err != nil {
		return a.downgradeToAnonymous("login échoué", err)
	}
	return fs.TelemetryConnected
}

// promptCloudLoginInteractive runs the interactive cloud-login flow: prompt
// for the key (masked TTY when possible), validate it via /v1/access-check,
// persist it in the keychain, and attach the provider entry in project.json.
//
// Shared between the init sub-prompt and `sth login` so the two paths cannot
// drift on validation rules, success messaging, or persistence order.
func (a *app) promptCloudLoginInteractive(reader *bufio.Reader) error {
	key, err := a.promptAPIKey(reader)
	if err != nil {
		return err
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return errors.New("clé vide")
	}
	if !strings.HasPrefix(key, "lsk_live_") {
		return errors.New("clé invalide : doit commencer par lsk_live_")
	}

	ctx, cancel := context.WithTimeout(context.Background(), accessCheckTimeout)
	defer cancel()
	resp, err := a.client.AccessCheck(ctx, key)
	if err != nil {
		return fmt.Errorf("vérification de la clé échouée : %w", err)
	}
	if !resp.OK {
		return errors.New("clé refusée par le serveur")
	}

	if err := auth.SaveAPIKey(key); err != nil {
		return err
	}

	if err := ensureCloudProviderEntry(); err != nil {
		// Non-fatal: keychain is set, user can still call commands. Warn only.
		a.writeStderr(fmt.Sprintf("Avertissement : configuration projet non écrite : %v", err))
	}

	a.writeStdout(
		fmt.Sprintf("Connecté à %s (plan %s, expire le %s).", resp.Org.Name, resp.Plan, resp.ExpiresAt),
		"Clé stockée dans le keychain de l'OS.",
		"",
	)
	return nil
}

// downgradeToAnonymous persists telemetry mode = anonymous and prints a
// one-line stderr hint explaining why the connected path was abandoned.
// Any error from persistence is swallowed (best-effort).
func (a *app) downgradeToAnonymous(reason string, cause error) fs.TelemetryMode {
	_, _ = fs.SetTelemetryMode(fs.TelemetryAnonymous, valueOrDefault(version, "dev"))
	msg := fmt.Sprintf("Mode bascule sur anonymous (%s).", reason)
	if cause != nil {
		msg = fmt.Sprintf("Mode bascule sur anonymous (%s : %v).", reason, cause)
	}
	a.writeStderr(msg, "Tu pourras réessayer plus tard avec `sth login`.")
	return fs.TelemetryAnonymous
}

// (DashboardLicensesURL was moved to cmd/docs_url.go alongside DocsURL et al.
// so all customer-facing URLs share one override mechanism.)
