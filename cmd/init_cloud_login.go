package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"sth/internal/auth"
	"sth/internal/fs"
	"sth/internal/tui"
)

func (a *app) promptCloudLoginIfNeeded(session *tui.Session, current fs.TelemetryMode) fs.TelemetryMode {
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
		return fs.TelemetryConnected
	case hasKey && !providerAttached:
		if err := ensureCloudProviderEntry(); err != nil {
			a.writeStderr("Warning: could not attach the sth-cloud provider.")
			return a.downgradeToAnonymous("STH Cloud unavailable", err)
		}
		a.writeStdout("The sth-cloud provider is now attached to this project.", "")
		return fs.TelemetryConnected
	}

	if !a.isInteractiveSession() {
		a.writeStderr("Tip: run `sth login` to enable the STH Cloud dashboard.")
		return a.downgradeToAnonymous("non-interactive session", nil)
	}

	a.writeStdout(
		renderBuddy(fs.BuddyAllGood),
		"",
		"Sign in now to see your commands in the STH Cloud dashboard.",
		fmt.Sprintf("Get your `lsk_live_…` key from %s", DashboardLicensesURL()),
		"You can decline and run `sth login` later.",
		"",
	)
	doIt, err := tui.Confirm(session, tui.ConfirmConfig{Question: "Connect to STH Cloud now?", DefaultYes: false})
	if err != nil {
		return a.downgradeToAnonymous("input unavailable", err)
	}
	if !doIt {
		a.writeStdout("OK. Run `sth login` whenever you want to enable your dashboard.", "")
		return a.downgradeToAnonymous("declined", nil)
	}

	if err := a.promptCloudLoginInteractive(session); err != nil {
		return a.downgradeToAnonymous("sign-in failed", err)
	}
	return fs.TelemetryConnected
}

func (a *app) promptCloudLoginInteractive(session *tui.Session) error {
	secretSession, closeSecretSession := a.secureSecretSession(session)
	defer closeSecretSession()
	key, err := tui.Secret(secretSession, tui.SecretConfig{
		Label: "Paste your STH Cloud key (lsk_live_…)",
	})
	if err != nil {
		return err
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return errors.New("key is empty")
	}
	if !strings.HasPrefix(key, "lsk_live_") {
		return errors.New("invalid key: it must start with lsk_live_")
	}

	ctx, cancel := context.WithTimeout(context.Background(), accessCheckTimeout)
	defer cancel()
	resp, err := a.client.AccessCheck(ctx, key)
	if err != nil {
		return newInitUserError("key verification failed", err)
	}
	if !resp.OK {
		return errors.New("the server rejected the key")
	}

	if err := auth.SaveAPIKey(key); err != nil {
		return newInitUserError("could not store the key in the OS keychain", err)
	}

	if err := ensureCloudProviderEntry(); err != nil {
		a.writeStderr("Warning: could not write the project configuration.")
	}

	a.writeStdout(
		fmt.Sprintf("Connected to %s (plan %s, expires %s).", resp.Org.Name, resp.Plan, resp.ExpiresAt),
		"Key stored in the OS keychain.",
		"",
	)
	return nil
}

func (a *app) downgradeToAnonymous(reason string, _ error) fs.TelemetryMode {
	_, _ = fs.SetTelemetryMode(fs.TelemetryAnonymous, valueOrDefault(version, "dev"))
	msg := fmt.Sprintf("Switched to anonymous mode (%s).", reason)
	a.writeStderr(msg, "You can try again later with `sth login`.")
	return fs.TelemetryAnonymous
}
