package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"sth/internal/auth"
	"sth/internal/cloud"
	"sth/internal/fs"

	"golang.org/x/term"
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
		return fmt.Errorf("usage : sth login [<api-key>]")
	}

	// Surface the mode transition up front when the user was running in
	// anonymous mode — they should know `sth login` is what flips them to
	// connected so their dashboard becomes populated.
	if fs.TelemetryModeActive() == fs.TelemetryAnonymous {
		a.writeStdout("Tu es actuellement en mode anonyme. `sth login` va passer en mode connecté.")
	}

	var key string
	if len(args) == 1 {
		key = strings.TrimSpace(args[0])
	} else {
		k, err := a.promptAPIKey(nil)
		if err != nil {
			return err
		}
		key = k
	}
	if key == "" {
		return fmt.Errorf("clé vide")
	}
	if !strings.HasPrefix(key, "lsk_live_") {
		return fmt.Errorf("clé invalide : doit commencer par lsk_live_")
	}

	ctx, cancel := context.WithTimeout(context.Background(), accessCheckTimeout)
	defer cancel()
	resp, err := a.client.AccessCheck(ctx, key)
	if err != nil {
		return fmt.Errorf("vérification de la clé échouée : %w", err)
	}
	if !resp.OK {
		return fmt.Errorf("clé refusée par le serveur")
	}

	if err := auth.SaveAPIKey(key); err != nil {
		return err
	}

	// Prime the entitlements cache so the next Pro-gated command runs
	// without an extra round-trip. Best-effort: a network blip here must
	// NOT fail the login (the key is already persisted and access-check
	// has already succeeded above).
	if primeErr := proGateMarkEntitled(ctx); primeErr != nil {
		a.writeStderr(fmt.Sprintf("Avertissement : cache entitlements non amorcé : %v", primeErr))
	}

	if err := ensureCloudProviderEntry(); err != nil {
		// Non-fatal: keychain is set, user can still call commands. Warn only.
		a.writeStderr(fmt.Sprintf("Avertissement : configuration projet non écrite : %v", err))
	}

	// Auto-promote to connected mode: a successful login is a strong opt-in
	// signal. If the user was anonymous they now get their dashboard; if they
	// were off and still ran `sth login` they probably want telemetry on.
	if _, modeErr := fs.SetTelemetryMode(fs.TelemetryConnected, valueOrDefault(version, "dev")); modeErr != nil {
		a.writeStderr(fmt.Sprintf("Avertissement : mode télémétrie non basculé : %v", modeErr))
	}

	a.writeStdout(
		fmt.Sprintf("Connecté à %s (plan %s, expire le %s).", resp.Org.Name, resp.Plan, resp.ExpiresAt),
		"Clé stockée dans le keychain de l'OS.",
	)
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
		"Tu appartiens à %d organisations. Utilise `sth org use <nom|id>` pour choisir l'organisation active.",
		len(orgs),
	))
}

// runLogout deletes the keychain entry. Idempotent.
func (a *app) runLogout(args []string) error {
	if len(args) > 0 && isHelpFlag(args[0]) {
		a.writeStdout("Usage : sth logout — supprime la clé STH Cloud du keychain.")
		return nil
	}
	if len(args) > 0 {
		return fmt.Errorf("usage : sth logout")
	}
	if err := auth.DeleteAPIKey(); err != nil {
		return err
	}
	a.writeStdout("Déconnecté. Clé supprimée du keychain.")
	return nil
}

// promptAPIKey reads the key from stdin. When stdin is a terminal we use
// term.ReadPassword so the key isn't echoed (no shell history, no shoulder
// surfing). Non-TTY stdin (CI, tests, piping) falls back to a line reader.
//
// Pass an already-buffered *bufio.Reader (from the enclosing wizard) so the
// fallback path shares one buffer with the rest of the wizard and lines
// stay in order. Pass nil to let promptAPIKey allocate its own reader (the
// historical behavior for the stand-alone `sth login` command).
func (a *app) promptAPIKey(reader *bufio.Reader) (string, error) {
	a.writeStdout("Colle ta clé STH Cloud (lsk_live_…) puis Entrée :")

	if f, ok := a.stdin.(*os.File); ok {
		fd := int(f.Fd())
		if term.IsTerminal(fd) {
			b, err := term.ReadPassword(fd)
			// ReadPassword swallows the trailing newline echo; emit one so the
			// next line of output isn't glued to the prompt.
			a.writeStdout("")
			if err != nil {
				return "", fmt.Errorf("lecture clé échouée : %w", err)
			}
			return strings.TrimSpace(string(b)), nil
		}
	}

	if reader == nil {
		reader = bufio.NewReader(a.stdin)
	}
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("lecture clé échouée : %w", err)
	}
	return strings.TrimSpace(line), nil
}

// findCloudProviderProfile returns the sth-cloud provider profile from the
// given project config, plus a bool indicating whether one was found. Shared
// helper used by reportCloudUsageBestEffort (cmd/root.go) and
// runTelemetryOutput (cmd/telemetry.go) — both need the same lookup with the
// same field selection.
func findCloudProviderProfile(cfg fs.ProjectConfigV2) (fs.ProjectProfile, bool) {
	for _, p := range cfg.Providers {
		if strings.EqualFold(p.Provider, loginProviderName) {
			return fs.ProjectProfile{
				Provider:    p.Provider,
				TargetDir:   p.TargetDir,
				CatalogPath: p.CatalogPath,
			}, true
		}
	}
	return fs.ProjectProfile{}, false
}

// ensureCloudProviderEntry guarantees .sth/project.json has a sth-cloud
// provider entry. Idempotent: if one already exists, we leave it alone.
func ensureCloudProviderEntry() error {
	cfg, err := fs.ReadProjectConfigV2()
	if err != nil {
		return err
	}
	for _, p := range cfg.Providers {
		if strings.EqualFold(p.Provider, loginProviderName) {
			return nil
		}
	}
	cfg.Providers = append(cfg.Providers, fs.ProviderEntry{
		ID:        loginDefaultProvID,
		Provider:  loginProviderName,
		TargetDir: ".claude/skills",
	})
	_, err = fs.WriteProjectConfigV2(cfg)
	return err
}

func loginHelpText() string {
	return strings.Join([]string{
		"Usage : sth login [<api-key>]",
		"",
		"Authentifie le CLI auprès de STH Cloud.",
		"Si aucune clé n'est passée en argument, elle est demandée interactivement (saisie masquée en TTY).",
		"La clé est validée puis stockée dans le keychain de l'OS.",
		"",
		"La variable STH_CLOUD_API_KEY peut servir de fallback en l'absence de keychain (CI).",
		"Pour basculer le CLI sur un backend de dev, exporte STH_CLOUD_BASE_URL.",
	}, "\n")
}
