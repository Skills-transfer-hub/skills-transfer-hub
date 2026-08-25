package cmd

import (
	"fmt"

	"sth/internal/fs"
	"sth/internal/tui"
)

func (a *app) promptTelemetryMode(session *tui.Session) (fs.TelemetryMode, error) {
	if !a.isInteractiveSession() {
		return a.persistTelemetryMode(fs.TelemetryOff)
	}

	a.writeStdout(
		renderBuddy(fs.BuddyAllGood),
		"",
		"Welcome to STH (Alpha).",
		fmt.Sprintf("Docs : %s", DocsURL()),
		fmt.Sprintf("Full details and opt-out: %s", TelemetryDocsURL()),
		"",
	)

	index, err := tui.Select(session, tui.SelectConfig{
		Title:       "How do you want to contribute to STH?",
		Options:     telemetryOptions(),
		MaxAttempts: 3,
	})
	if err != nil {
		if err == tui.ErrAborted {
			return a.persistTelemetryMode(fs.TelemetryOff)
		}
		return "", err
	}
	return a.persistTelemetryMode(telemetryModeForIndex(index))
}

func telemetryOptions() []tui.Option {
	return []tui.Option{
		{
			Label:   "No telemetry — nothing is sent",
			Aliases: []string{"1", "off", "none"},
		},
		{
			Label:   "Anonymous — product stats, no dashboard",
			Aliases: []string{"2", "anon", "anonymous"},
		},
		{
			Label:   "Connected — anonymous + dashboard (STH Cloud key required)",
			Aliases: []string{"3", "connect", "connecte"},
		},
	}
}

func telemetryModeForIndex(index int) fs.TelemetryMode {
	switch index {
	case 1:
		return fs.TelemetryAnonymous
	case 2:
		return fs.TelemetryConnected
	default:
		return fs.TelemetryOff
	}
}

func (a *app) persistTelemetryMode(mode fs.TelemetryMode) (fs.TelemetryMode, error) {
	persistedMode, err := a.saveTelemetryMode(mode)
	if err != nil {
		return "", err
	}
	a.writeStdout(telemetryModeConfirmation(persistedMode)...)
	return persistedMode, nil
}

func (a *app) saveTelemetryMode(mode fs.TelemetryMode) (fs.TelemetryMode, error) {
	cliVersion := valueOrDefault(version, "dev")
	if _, err := fs.SetTelemetryMode(mode, cliVersion); err != nil {
		return "", newInitUserError("could not save the telemetry preference", err)
	}
	return mode, nil
}

func telemetryModeConfirmation(mode fs.TelemetryMode) []string {
	lines := make([]string, 0, 2)
	switch mode {
	case fs.TelemetryOff:
		lines = append(lines, "Telemetry disabled. Re-enable via `sth telemetry anon` or `sth telemetry connect`.")
	case fs.TelemetryAnonymous:
		lines = append(lines, "Anonymous telemetry enabled. No dashboard; switch to `sth telemetry connect` anytime.")
	case fs.TelemetryConnected:
		lines = append(lines, "Connected mode selected — you will be asked to paste your STH Cloud key.")
	}
	return append(lines, "")
}
