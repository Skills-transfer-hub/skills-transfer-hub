package cmd

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"sth/internal/fs"
	sthhttp "sth/internal/http"
	"sth/internal/tui"
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
			return fmt.Errorf("usage: sth telemetry status")
		}
		return a.runTelemetryStatus()
	case "on":
		// Deprecated alias for `anon` — kept so scripts written against the
		// pre-3-tier CLI keep working. Emit a one-line stderr warning so the
		// deprecation is visible but not breaking.
		if len(args) != 1 {
			return fmt.Errorf("usage: sth telemetry on")
		}
		a.writeStderr("`sth telemetry on` is deprecated — use `sth telemetry anon` (or `connect`).")
		return a.runTelemetryMode(fs.TelemetryAnonymous)
	case "off":
		if len(args) != 1 {
			return fmt.Errorf("usage: sth telemetry off")
		}
		return a.runTelemetryMode(fs.TelemetryOff)
	case "anon":
		if len(args) != 1 {
			return fmt.Errorf("usage: sth telemetry anon")
		}
		return a.runTelemetryMode(fs.TelemetryAnonymous)
	case "connect":
		if len(args) != 1 {
			return fmt.Errorf("usage: sth telemetry connect")
		}
		return a.runTelemetryConnect()
	case "output":
		return a.runTelemetryOutput(args[1:])
	case "help", "--help", "-h":
		a.writeStdout(telemetryHelpText())
		return nil
	default:
		return fmt.Errorf("unknown telemetry subcommand: %q (see `sth telemetry help`)", args[0])
	}
}

func (a *app) runTelemetryStatus() error {
	settings, err := fs.ReadUserSettings()
	if err != nil {
		return err
	}
	if settings == nil || strings.TrimSpace(settings.Telemetry.ConsentedAt) == "" {
		a.writeStdout(
			"Telemetry: not configured yet.",
			"The next `sth init` will ask you to choose a mode.",
			fmt.Sprintf("Details: %s", TelemetryDocsURL()),
		)
		return nil
	}

	mode := fs.TelemetryModeActive()
	a.writeStdout(
		fmt.Sprintf("Telemetry: %s mode.", mode),
		fmt.Sprintf("Decision recorded on %s (CLI %s).",
			settings.Telemetry.ConsentedAt,
			valueOrDefault(settings.Telemetry.CLIVersionAtConsent, "?"),
		),
	)

	switch mode {
	case fs.TelemetryConnected:
		cfg, _ := fs.ReadProjectConfigV2()
		if _, found := findCloudProviderProfile(cfg); found {
			a.writeStdout("STH Cloud account linked: yes (view server-side organization data with `sth telemetry output --server`).")
		} else {
			a.writeStdout("No STH Cloud account is linked to this project — run `sth login` here.")
		}
	case fs.TelemetryAnonymous:
		a.writeStdout("Anonymous mode: no server dashboard because submissions are unattributed, but a local log is available.")
	}
	a.writeStdout("Recent submissions from this machine: `sth telemetry output`.")
	a.writeStdout(fmt.Sprintf("Details: %s", TelemetryDocsURL()))
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
			"Telemetry disabled. Commands will no longer send usage data.",
			fmt.Sprintf("Preferences: %s", path),
		)
	case fs.TelemetryAnonymous:
		a.writeStdout(
			"Anonymous telemetry enabled. No dashboard is available; switch with `sth telemetry connect` at any time.",
			fmt.Sprintf("Preferences: %s", path),
			fmt.Sprintf("Details: %s", TelemetryDocsURL()),
		)
	case fs.TelemetryConnected:
		a.writeStdout(
			"Connected mode enabled.",
			fmt.Sprintf("Preferences: %s", path),
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
	a.promptCloudLoginIfNeeded(tui.NewSession(a.stdin, a.stdout), fs.TelemetryConnected)
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
			a.writeStdout("Telemetry disabled — no submissions recorded locally.")
		} else {
			a.writeStdout("No telemetry submissions have been recorded locally yet.")
		}
		return nil
	}
	if fs.TelemetryModeActive() == fs.TelemetryOff {
		a.writeStdout("Telemetry disabled — previous submissions from this machine:")
	}
	mine := make([]sthhttp.UsageMineRecord, len(records))
	for i, record := range records {
		mine[i] = sthhttp.UsageMineRecord{Ts: record.Ts, CLIVersion: record.CLIVersion, CommandName: record.CommandName}
	}
	a.writeStdout(formatTelemetryTable(mine))
	a.writeStdout("(source: local log on this machine)")
	a.writeStdout(fmt.Sprintf("%d submission(s) recorded locally.", len(mine)))
	return nil
}

func (a *app) runTelemetryOutputServer(limit int) error {
	cfg, err := fs.ReadProjectConfigV2()
	if err != nil {
		return err
	}
	profile, found := findCloudProviderProfile(cfg)
	if !found {
		return fmt.Errorf("--server requires connected mode and an sth-cloud provider — run `sth login` first")
	}

	ctx, cancel := context.WithTimeout(context.Background(), telemetryFetchTimeout)
	defer cancel()
	records, err := a.client.FetchOwnUsage(ctx, profile, limit)
	if err != nil {
		return err
	}
	if len(records) == 0 {
		a.writeStdout("No server-side telemetry records were found for this key.")
		return nil
	}
	a.writeStdout(formatTelemetryTable(records))
	a.writeStdout(fmt.Sprintf("%d server-side record(s).", len(records)))
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
	for _, arg := range args {
		switch {
		case arg == "--limit":
			return 0, fmt.Errorf("usage: --limit=N")
		case strings.HasPrefix(arg, "--limit="):
			if seen {
				return 0, fmt.Errorf("--limit may only be specified once")
			}
			raw := strings.TrimPrefix(arg, "--limit=")
			parsedLimit, err := strconv.Atoi(raw)
			if err != nil || parsedLimit <= 0 {
				return 0, fmt.Errorf("--limit=N requires a positive integer (received %q)", raw)
			}
			limit = parsedLimit
			seen = true
		default:
			return 0, fmt.Errorf("unexpected argument: %q", arg)
		}
	}
	return limit, nil
}

// formatTelemetryTable produces a compact terminal-friendly table. We don't
// pull in a tablewriter dep: 3 columns of fixed-ish width is plenty for the
// volumes involved (capped at MAX_LIMIT=500 server-side).
func formatTelemetryTable(records []sthhttp.UsageMineRecord) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "%-25s  %-12s  %s\n", "Timestamp", "CLI", "Command")
	fmt.Fprintf(&builder, "%-25s  %-12s  %s\n", strings.Repeat("-", 25), strings.Repeat("-", 12), strings.Repeat("-", 12))
	for _, record := range records {
		command := strings.TrimSpace(record.CommandName)
		if command == "" {
			command = "—"
		}
		fmt.Fprintf(&builder, "%-25s  %-12s  %s\n", record.Ts, record.CLIVersion, command)
	}
	return strings.TrimRight(builder.String(), "\n")
}

func telemetryHelpText() string {
	return strings.Join([]string{
		"Usage: sth telemetry <status|off|anon|connect|output>",
		"",
		"Subcommands:",
		"  status              Show the active mode (off, anonymous, or connected).",
		"  off                 Disable all telemetry submissions.",
		"  anon                Enable anonymous mode (product statistics, no dashboard).",
		"  connect             Enable connected mode and open the `sth login` prompt.",
		"  on                  (deprecated) Alias for `anon`, kept for existing scripts.",
		"  output [--limit=N]  Show the local submission log for this machine",
		"                      in any mode and without network access. Add --server",
		"                      for the server-side organization view (connected mode",
		"                      and an sth-cloud provider required).",
		"",
		"Detailed policy: " + TelemetryDocsURL() + ".",
	}, "\n")
}
