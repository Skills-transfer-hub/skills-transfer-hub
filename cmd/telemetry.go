package cmd

import (
	"bufio"
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"sth/internal/fs"
	sthhttp "sth/internal/http"
)

const telemetryFetchTimeout = 10 * time.Second

// runTelemetry dispatches the `sth telemetry <subcommand>` family. Kept
// in-file (no separate cobra) to match the pattern of cmd/login.go.
func (a *app) runTelemetry(args []string) error {
	if len(args) == 0 {
		return a.runTelemetryStatus()
	}
	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "status":
		if len(args) != 1 {
			return fmt.Errorf("usage : sth telemetry status")
		}
		return a.runTelemetryStatus()
	case "on":
		// Deprecated alias for `anon` — kept so scripts written against the
		// pre-3-tier CLI keep working. Emit a one-line stderr warning so the
		// deprecation is visible but not breaking.
		if len(args) != 1 {
			return fmt.Errorf("usage : sth telemetry on")
		}
		a.writeStderr("`sth telemetry on` est déprécié — utilise `sth telemetry anon` (ou `connect`).")
		return a.runTelemetryMode(fs.TelemetryAnonymous)
	case "off":
		if len(args) != 1 {
			return fmt.Errorf("usage : sth telemetry off")
		}
		return a.runTelemetryMode(fs.TelemetryOff)
	case "anon":
		if len(args) != 1 {
			return fmt.Errorf("usage : sth telemetry anon")
		}
		return a.runTelemetryMode(fs.TelemetryAnonymous)
	case "connect":
		if len(args) != 1 {
			return fmt.Errorf("usage : sth telemetry connect")
		}
		return a.runTelemetryConnect()
	case "output":
		return a.runTelemetryOutput(args[1:])
	case "help", "--help", "-h":
		a.writeStdout(telemetryHelpText())
		return nil
	default:
		return fmt.Errorf("sous-commande inconnue : %q (voir `sth telemetry help`)", args[0])
	}
}

func (a *app) runTelemetryStatus() error {
	settings, err := fs.ReadUserSettings()
	if err != nil {
		return err
	}
	if settings == nil || strings.TrimSpace(settings.Telemetry.ConsentedAt) == "" {
		a.writeStdout(
			"Télémétrie : pas encore configurée.",
			"Le prochain `sth init` te demandera ton choix.",
			fmt.Sprintf("Détails : %s", TelemetryDocsURL()),
		)
		return nil
	}

	mode := fs.TelemetryModeActive()
	a.writeStdout(
		fmt.Sprintf("Télémétrie : mode %s.", mode),
		fmt.Sprintf("Décision enregistrée le %s (CLI %s).",
			settings.Telemetry.ConsentedAt,
			valueOrDefault(settings.Telemetry.CLIVersionAtConsent, "?"),
		),
	)

	switch mode {
	case fs.TelemetryConnected:
		cfg, _ := fs.ReadProjectConfigV2()
		if _, found := findCloudProviderProfile(cfg); found {
			a.writeStdout("Compte STH Cloud rattaché : oui (vue org côté serveur via `sth telemetry output --server`).")
		} else {
			a.writeStdout("Compte STH Cloud non rattaché à ce projet — lance `sth login` ici.")
		}
	case fs.TelemetryAnonymous:
		a.writeStdout("Mode anonyme : pas de dashboard serveur (envois non attribués), mais journal local disponible.")
	}
	a.writeStdout("Derniers envois de cette machine : `sth telemetry output`.")
	a.writeStdout(fmt.Sprintf("Détails : %s", TelemetryDocsURL()))
	return nil
}

// runTelemetryMode persists the new mode and prints a confirmation tailored
// to the choice. Shared between `sth telemetry off`, `anon`, and the (legacy)
// `on` alias.
func (a *app) runTelemetryMode(mode fs.TelemetryMode) error {
	cliVersion := valueOrDefault(version, "dev")
	path, err := fs.SetTelemetryMode(mode, cliVersion)
	if err != nil {
		return err
	}
	switch mode {
	case fs.TelemetryOff:
		a.writeStdout(
			"Télémétrie désactivée. Plus aucune commande ne déclenchera d'envoi.",
			fmt.Sprintf("Préférences : %s", path),
		)
	case fs.TelemetryAnonymous:
		a.writeStdout(
			"Télémétrie anonyme activée. Pas de dashboard ; passe en `sth telemetry connect` quand tu veux.",
			fmt.Sprintf("Préférences : %s", path),
			fmt.Sprintf("Détails : %s", TelemetryDocsURL()),
		)
	case fs.TelemetryConnected:
		a.writeStdout(
			"Mode connecté activé.",
			fmt.Sprintf("Préférences : %s", path),
		)
	}
	return nil
}

// runTelemetryConnect persists mode=connected and chains the cloud-login
// sub-prompt so the user lands fully wired in one command.
func (a *app) runTelemetryConnect() error {
	cliVersion := valueOrDefault(version, "dev")
	if _, err := fs.SetTelemetryMode(fs.TelemetryConnected, cliVersion); err != nil {
		return err
	}
	a.promptCloudLoginIfNeeded(bufio.NewReader(a.stdin), fs.TelemetryConnected)
	return nil
}

func (a *app) runTelemetryOutput(args []string) error {
	server, rest := stripServerFlag(args)
	limit, err := parseTelemetryLimit(rest)
	if err != nil {
		return err
	}
	if server {
		return a.runTelemetryOutputServer(limit)
	}
	return a.runTelemetryOutputLocal(limit)
}

func (a *app) runTelemetryOutputLocal(limit int) error {
	records, err := fs.ReadUsageLog(limit)
	if err != nil {
		return err
	}
	if len(records) == 0 {
		if fs.TelemetryModeActive() == fs.TelemetryOff {
			a.writeStdout("Télémétrie désactivée — aucun envoi enregistré localement.")
		} else {
			a.writeStdout("Aucun envoi de télémétrie enregistré localement pour le moment.")
		}
		return nil
	}
	if fs.TelemetryModeActive() == fs.TelemetryOff {
		a.writeStdout("Télémétrie désactivée — historique des envois passés de cette machine :")
	}
	mine := make([]sthhttp.UsageMineRecord, len(records))
	for i, record := range records {
		mine[i] = sthhttp.UsageMineRecord{Ts: record.Ts, CLIVersion: record.CLIVersion, CommandName: record.CommandName}
	}
	a.writeStdout(formatTelemetryTable(mine))
	a.writeStdout("(source : journal local de cette machine)")
	a.writeStdout(fmt.Sprintf("%d envoi(s) enregistré(s) localement.", len(mine)))
	return nil
}

func (a *app) runTelemetryOutputServer(limit int) error {
	cfg, err := fs.ReadProjectConfigV2()
	if err != nil {
		return err
	}
	profile, found := findCloudProviderProfile(cfg)
	if !found {
		return fmt.Errorf("--server nécessite le mode connected + un provider sth-cloud — lance `sth login` d'abord")
	}

	ctx, cancel := context.WithTimeout(context.Background(), telemetryFetchTimeout)
	defer cancel()
	records, err := a.client.FetchOwnUsage(ctx, profile, limit)
	if err != nil {
		return err
	}
	if len(records) == 0 {
		a.writeStdout("Aucun enregistrement de télémétrie côté serveur pour cette clé.")
		return nil
	}
	a.writeStdout(formatTelemetryTable(records))
	a.writeStdout(fmt.Sprintf("%d enregistrement(s) côté serveur.", len(records)))
	return nil
}

func stripServerFlag(args []string) (bool, []string) {
	server := false
	rest := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--server" {
			server = true
			continue
		}
		rest = append(rest, arg)
	}
	return server, rest
}

// parseTelemetryLimit reads `--limit=N` from the args. Defaults to 50 — small
// enough to fit a typical terminal window without paging, large enough to spot
// a recent burst of activity. Iterates the full args list so unknown trailing
// flags surface a clear error instead of being silently ignored.
func parseTelemetryLimit(args []string) (int, error) {
	const defaultLimit = 50
	limit := defaultLimit
	seen := false
	for _, a := range args {
		switch {
		case a == "--limit":
			return 0, fmt.Errorf("usage : --limit=N")
		case strings.HasPrefix(a, "--limit="):
			if seen {
				return 0, fmt.Errorf("--limit ne peut être passé qu'une fois")
			}
			raw := strings.TrimPrefix(a, "--limit=")
			n, err := strconv.Atoi(raw)
			if err != nil || n <= 0 {
				return 0, fmt.Errorf("--limit=N requiert un entier positif (reçu %q)", raw)
			}
			limit = n
			seen = true
		default:
			return 0, fmt.Errorf("argument inattendu : %q", a)
		}
	}
	return limit, nil
}

// formatTelemetryTable produces a compact terminal-friendly table. We don't
// pull in a tablewriter dep: 3 columns of fixed-ish width is plenty for the
// volumes involved (capped at MAX_LIMIT=500 server-side).
func formatTelemetryTable(records []sthhttp.UsageMineRecord) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-25s  %-12s  %s\n", "Horodatage", "CLI", "Commande")
	fmt.Fprintf(&b, "%-25s  %-12s  %s\n", strings.Repeat("-", 25), strings.Repeat("-", 12), strings.Repeat("-", 12))
	for _, r := range records {
		cmd := strings.TrimSpace(r.CommandName)
		if cmd == "" {
			cmd = "—"
		}
		fmt.Fprintf(&b, "%-25s  %-12s  %s\n", r.Ts, r.CLIVersion, cmd)
	}
	return strings.TrimRight(b.String(), "\n")
}

func telemetryHelpText() string {
	return strings.Join([]string{
		"Usage : sth telemetry <status|off|anon|connect|output>",
		"",
		"Sous-commandes :",
		"  status              Affiche le mode actif (off / anonymous / connected).",
		"  off                 Désactive complètement l'envoi de télémétrie.",
		"  anon                Active le mode anonyme (stats produit, pas de dashboard).",
		"  connect             Active le mode connecté et lance le sub-prompt `sth login`.",
		"  on                  (déprécié) Alias de `anon` pour les anciens scripts.",
		"  output [--limit=N]  Affiche le journal local des envois de cette machine",
		"                      (tous modes, hors-ligne). Ajoute --server pour la vue",
		"                      org côté serveur (mode connected + provider sth-cloud).",
		"",
		"Politique détaillée : voir " + TelemetryDocsURL() + ".",
	}, "\n")
}
