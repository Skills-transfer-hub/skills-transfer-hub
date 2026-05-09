package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	sthhttp "sth/internal/http"
)

// withAuthRecovery runs fn, and on AuthError (HTTP 401/403) in an interactive
// session, prompts the user for a fresh token, sets STH_TOKEN, and retries
// once. FR-005/FR-006: at most one retry per command invocation.
//
// The new token is exposed via os.Setenv("STH_TOKEN", ...) so the next call
// to ResolveCredential picks it up without threading the value through every
// signature. Tests are serial (no t.Parallel), so the global mutation is safe.
func (a *app) withAuthRecovery(ctx context.Context, fn func() error) error {
	err := fn()
	if err == nil {
		return nil
	}
	ae, ok := sthhttp.IsAuthError(err)
	if !ok {
		return err
	}
	// Only prompt for HTTP 401/403 — "no credential available" (StatusCode==0)
	// and "OIDC exchange failed" surface as fatal in non-TTY too.
	if ae.StatusCode != 401 && ae.StatusCode != 403 {
		return err
	}
	if !a.isInteractiveSession() {
		return err
	}

	a.writeStderr(fmt.Sprintf("Authentication failed (HTTP %d) — your token may have expired.", ae.StatusCode))
	token, perr := a.promptSessionToken()
	if perr != nil {
		return fmt.Errorf("commande interrompue : %w", perr)
	}
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("commande interrompue : aucun token fourni")
	}

	if setErr := os.Setenv("STH_TOKEN", strings.TrimSpace(token)); setErr != nil {
		return fmt.Errorf("commande interrompue : impossible de définir STH_TOKEN : %w", setErr)
	}
	a.verbose.Decision("auth recovery: retrying with fresh STH_TOKEN")
	return fn()
}

// promptSessionToken reads a token line from stdin without echo masking
// (stdlib pur per spec 007 decision 1). The token is never written to disk.
func (a *app) promptSessionToken() (string, error) {
	if _, err := fmt.Fprintln(a.stderr, "Enter a fresh token (input not masked) and press Enter, or Ctrl+C to abort:"); err != nil {
		return "", err
	}
	r := bufio.NewReader(a.stdin)
	line, err := r.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimSpace(line), nil
}
