package cmd

import (
	"bufio"
	"fmt"
	"strings"

	"sth/internal/fs"
)

// promptTelemetryMode runs on the user's very first `sth init`. It shows a
// buddy frame, the link to the privacy docs, and a 3-choice menu so the user
// picks how (if at all) they want to contribute telemetry:
//
//	1) Aucune télémétrie         → mode off (default)
//	2) Anonyme — stats produit   → mode anonymous
//	3) Connecté — + dashboard    → mode connected (followed by the cloud-login sub-prompt)
//
// Returns the chosen mode so the caller can chain the cloud-login prompt
// when the user picked "connected". Non-interactive sessions persist
// TelemetryOff silently and return it: no surprises, the user can always
// `sth telemetry anon` later.
//
// The decision is persisted regardless of the answer — recording a refusal
// still counts as "the user has been asked", which is what keeps `runInit`
// from re-prompting on subsequent runs.
func (a *app) promptTelemetryMode(reader *bufio.Reader) (fs.TelemetryMode, error) {
	if !a.isInteractiveSession() {
		return a.persistTelemetryMode(fs.TelemetryOff)
	}

	a.writeStdout(
		renderBuddy(fs.BuddyAllGood),
		"",
		"Bienvenue dans STH (Alpha).",
		fmt.Sprintf("Docs : %s", DocsURL()),
		"",
		"Comment veux-tu contribuer à STH ?",
		"  1) Aucune télémétrie (défaut)",
		"  2) Anonyme — envoie ts, version CLI, nom de commande. Aide à améliorer STH. Pas de dashboard.",
		"  3) Connecté — idem anonyme + alimente ton dashboard. Nécessite une clé STH Cloud.",
		"",
		fmt.Sprintf("Détails complets et opt-out : %s", TelemetryDocsURL()),
		"",
	)

	mode, err := readTelemetryChoice(reader, a.stdout)
	if err != nil {
		return "", err
	}
	return a.persistTelemetryMode(mode)
}

// persistTelemetryMode writes the chosen mode and prints a one-line
// confirmation tailored to the choice.
func (a *app) persistTelemetryMode(mode fs.TelemetryMode) (fs.TelemetryMode, error) {
	cliVersion := valueOrDefault(version, "dev")
	if _, err := fs.SetTelemetryMode(mode, cliVersion); err != nil {
		return "", fmt.Errorf("enregistrement du consentement télémétrie échoué : %w", err)
	}
	switch mode {
	case fs.TelemetryOff:
		a.writeStdout("Télémétrie désactivée. Réactivable via `sth telemetry anon` ou `sth telemetry connect`.")
	case fs.TelemetryAnonymous:
		a.writeStdout("Télémétrie anonyme activée. Pas de dashboard ; passe en `sth telemetry connect` quand tu veux.")
	case fs.TelemetryConnected:
		a.writeStdout("Mode connecté choisi — tu vas être invité à coller ta clé STH Cloud.")
	}
	a.writeStdout("")
	return mode, nil
}

// readTelemetryChoice loops on the menu until the user enters a valid choice.
// Empty input maps to the default (off). After 3 invalid attempts we fall
// back to off so a script piping garbage cannot hang init forever.
func readTelemetryChoice(reader *bufio.Reader, stdout interface{ Write([]byte) (int, error) }) (fs.TelemetryMode, error) {
	const maxAttempts = 3
	for attempt := 0; attempt < maxAttempts; attempt++ {
		raw, err := promptValue(reader, stdout, "Choix [1]", "1")
		if err != nil {
			// promptValue treats empty as "required" — here empty IS the default.
			// On EOF we'd see io.EOF; treat that as "off" too (no interaction).
			return fs.TelemetryOff, nil
		}
		switch strings.TrimSpace(raw) {
		case "1":
			return fs.TelemetryOff, nil
		case "2":
			return fs.TelemetryAnonymous, nil
		case "3":
			return fs.TelemetryConnected, nil
		}
		_, _ = stdout.Write([]byte("Choix invalide. Tape 1, 2 ou 3.\n"))
	}
	return fs.TelemetryOff, nil
}
