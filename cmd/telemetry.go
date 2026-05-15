package cmd

import (
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
		if len(args) != 1 {
			return fmt.Errorf("usage : sth telemetry on")
		}
		return a.runTelemetrySet(true)
	case "off":
		if len(args) != 1 {
			return fmt.Errorf("usage : sth telemetry off")
		}
		return a.runTelemetrySet(false)
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
			"Le prochain `sth init` te demandera ton consentement.",
			fmt.Sprintf("Détails : %s", TelemetryDocsURL()),
		)
		return nil
	}
	state := "désactivée"
	if settings.Telemetry.Consented {
		state = "activée"
	}
	a.writeStdout(
		fmt.Sprintf("Télémétrie : %s.", state),
		fmt.Sprintf("Décision enregistrée le %s (CLI %s).",
			settings.Telemetry.ConsentedAt,
			valueOrDefault(settings.Telemetry.CLIVersionAtConsent, "?"),
		),
		fmt.Sprintf("Détails : %s", TelemetryDocsURL()),
	)
	return nil
}

func (a *app) runTelemetrySet(consent bool) error {
	cliVersion := valueOrDefault(version, "dev")
	path, err := fs.SetTelemetryConsent(consent, cliVersion)
	if err != nil {
		return err
	}
	if consent {
		a.writeStdout(
			"Télémétrie activée.",
			fmt.Sprintf("Préférences : %s", path),
			fmt.Sprintf("Détails : %s", TelemetryDocsURL()),
		)
	} else {
		a.writeStdout(
			"Télémétrie désactivée. Plus aucune commande ne déclenchera d'envoi.",
			fmt.Sprintf("Préférences : %s", path),
		)
	}
	return nil
}

func (a *app) runTelemetryOutput(args []string) error {
	limit, err := parseTelemetryLimit(args)
	if err != nil {
		return err
	}

	cfg, err := fs.ReadProjectConfigV2()
	if err != nil {
		return err
	}
	profile, found := findCloudProviderProfile(cfg)
	if !found {
		return fmt.Errorf("aucun provider sth-cloud configuré — lance `sth login` d'abord")
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
	a.writeStdout(fmt.Sprintf("%d enregistrement(s).", len(records)))
	return nil
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
		"Usage : sth telemetry <status|on|off|output>",
		"",
		"Sous-commandes :",
		"  status           Affiche l'état du consentement local.",
		"  on               Active l'envoi de télémétrie.",
		"  off              Désactive l'envoi de télémétrie.",
		"  output [--limit=N]  Récupère les derniers enregistrements depuis le serveur",
		"                   (nécessite un provider sth-cloud configuré).",
		"",
		"Politique détaillée : voir " + TelemetryDocsURL() + ".",
	}, "\n")
}
