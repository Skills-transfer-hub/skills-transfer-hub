package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"sth/internal/auth"
	"sth/internal/cloud"
	"sth/internal/fs"
	"sth/internal/tui"
)

const (
	loginProviderName  = "sth-cloud"
	loginDefaultProvID = "sth-cloud"
	accessCheckTimeout = 10 * time.Second
)

// runLogin prompts for an API key, validates it via /v1/access-check, persists
// it in the OS keychain, ensures a `sth-cloud` provider entry exists in
// .sth/project.json, and switches telemetry to connected mode so the user's
// dashboard starts filling on the next command.
func (a *app) runLogin(args []string) error {
	if len(args) > 0 && isHelpFlag(args[0]) {
		a.writeStdout(loginHelpText())
		return nil
	}
	if len(args) > 1 {
		return fmt.Errorf("usage: sth login [<api-key>]")
	}

	// Surface the mode transition up front when the user was running in
	// anonymous mode — they should know `sth login` is what flips them to
	// connected so their dashboard becomes populated.
	if fs.TelemetryModeActive() == fs.TelemetryAnonymous {
		a.writeStdout("You are currently in anonymous mode. `sth login` will switch to connected mode.")
	}

	var key string
	if len(args) == 1 {
		key = strings.TrimSpace(args[0])
		a.writeStderr("Warning: a key passed as an argument may remain in your shell history. Prefer `sth login` without an argument so the input is hidden.")
	} else {
		promptedKey, err := a.promptAPIKey()
		if err != nil {
			return err
		}
		key = promptedKey
	}
	if key == "" {
		return fmt.Errorf("empty key")
	}
	if !strings.HasPrefix(key, "lsk_live_") {
		return fmt.Errorf("invalid key: must start with lsk_live_")
	}

	ctx, cancel := context.WithTimeout(context.Background(), accessCheckTimeout)
	defer cancel()
	resp, err := a.client.AccessCheck(ctx, key)
	if err != nil {
		return fmt.Errorf("key verification failed: %w", err)
	}
	if !resp.OK {
		return fmt.Errorf("key rejected by the server")
	}

	if err := auth.SaveAPIKey(key); err != nil {
		return err
	}

	// Prime the entitlements cache so the next Pro-gated command runs
	// without an extra round-trip. Best-effort: a network blip here must
	// NOT fail the login (the key is already persisted and access-check
	// has already succeeded above).
	if primeErr := proGateMarkEntitled(ctx); primeErr != nil {
		a.writeStderr(fmt.Sprintf("Warning: entitlements cache was not primed: %v", primeErr))
	}

	attachment, attachErr := ensureCloudProviderEntry()
	if attachErr != nil {
		// Non-fatal: keychain is set, user can still call commands. Warn only.
		a.writeStderr(fmt.Sprintf("Warning: project configuration was not written: %v", attachErr))
	}

	// Auto-promote to connected mode: a successful login is a strong opt-in
	// signal. If the user was anonymous they now get their dashboard; if they
	// were off and still ran `sth login` they probably want telemetry on.
	if _, modeErr := fs.SetTelemetryMode(fs.TelemetryConnected, valueOrDefault(version, "dev")); modeErr != nil {
		a.writeStderr(fmt.Sprintf("Warning: telemetry mode was not switched: %v", modeErr))
	}

	loginLines := []string{
		fmt.Sprintf("Connected to %s (plan %s, expires on %s).", resp.Org.Name, resp.Plan, resp.ExpiresAt),
		"Key stored in the OS keychain.",
	}
	a.writeStdout(append(loginLines, attachment.announcement()...)...)
	if notice := licenseExpiryWarning(); notice != "" {
		a.writeStderr(notice)
	}
	a.hintMultipleOrgs()
	return nil
}

func (a *app) hintMultipleOrgs() {
	if !experimentalEnabled() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	orgs, err := cloud.ListOrgs(ctx, nil)
	if err != nil || len(orgs) <= 1 {
		return
	}
	a.writeStdout(fmt.Sprintf(
		"You belong to %d organizations. Use `sth org use <name|id>` to choose the active organization.",
		len(orgs),
	))
}

// runLogout deletes the keychain entry. Idempotent.
func (a *app) runLogout(args []string) error {
	if len(args) > 0 && isHelpFlag(args[0]) {
		a.writeStdout("Usage: sth logout — remove the STH Cloud key from the keychain.")
		return nil
	}
	if len(args) > 0 {
		return fmt.Errorf("usage: sth logout")
	}
	if err := auth.DeleteAPIKey(); err != nil {
		return err
	}
	a.writeStdout("Logged out. Key removed from the keychain.")
	return nil
}

func (a *app) promptAPIKey() (string, error) {
	baseSession := tui.NewSession(a.stdin, a.stdout)
	defer baseSession.Close()
	secretSession, closeSecretSession := a.secureSecretSession(baseSession)
	defer closeSecretSession()
	key, err := tui.Secret(secretSession, tui.SecretConfig{Label: "Paste your STH Cloud key (lsk_live_…)"})
	if err != nil {
		return "", fmt.Errorf("failed to read key: %w", err)
	}
	return strings.TrimSpace(key), nil
}

// findCloudProviderProfile returns the sth-cloud provider profile from the
// given project config, plus a bool indicating whether one was found. Shared
// helper used by reportCloudUsageBestEffort (cmd/root.go) and
// runTelemetryOutput (cmd/telemetry.go) — both need the same lookup with the
// same field selection.
func findCloudProviderProfile(cfg fs.ProjectConfigV2) (fs.ProjectProfile, bool) {
	for _, provider := range cfg.Providers {
		if strings.EqualFold(provider.Provider, loginProviderName) {
			return fs.ProjectProfile{
				Provider:    provider.Provider,
				TargetDir:   provider.TargetDir,
				CatalogPath: provider.CatalogPath,
			}, true
		}
	}
	return fs.ProjectProfile{}, false
}

type cloudProviderAttachment struct {
	config fs.ProjectConfigV2
	entry  fs.ProviderEntry
	added  bool
}

func (attachment cloudProviderAttachment) announcement() []string {
	if !attachment.added {
		return nil
	}
	return providerAddedLines(attachment.config, attachment.entry)
}

func ensureCloudProviderEntry() (cloudProviderAttachment, error) {
	cfg, err := fs.ReadProjectConfigV2()
	if err != nil {
		return cloudProviderAttachment{}, err
	}
	for _, provider := range cfg.Providers {
		if strings.EqualFold(provider.Provider, loginProviderName) {
			return cloudProviderAttachment{config: cfg, entry: provider}, nil
		}
	}

	entryID := loginDefaultProvID
	for suffix := 2; aliasIsDuplicate(entryID, cfg); suffix++ {
		entryID = fmt.Sprintf("%s-%d", loginDefaultProvID, suffix)
	}
	entry := fs.ProviderEntry{
		ID:        entryID,
		Provider:  loginProviderName,
		TargetDir: ".claude/skills",
	}
	cfg.Providers = append(cfg.Providers, entry)
	if _, writeErr := fs.WriteProjectConfigV2(cfg); writeErr != nil {
		return cloudProviderAttachment{}, writeErr
	}
	return cloudProviderAttachment{config: cfg, entry: entry, added: true}, nil
}

func loginHelpText() string {
	return strings.Join([]string{
		"Usage: sth login [<api-key>]",
		"",
		"Authenticate the CLI with STH Cloud.",
		"When no key is passed as an argument, the CLI prompts for it interactively and hides it in a TTY.",
		"The key is validated and then stored in the OS keychain.",
		"",
		"STH_CLOUD_API_KEY can be used as a fallback when no keychain is available, such as in CI.",
		"To point the CLI at a development backend, export STH_CLOUD_BASE_URL.",
	}, "\n")
}
