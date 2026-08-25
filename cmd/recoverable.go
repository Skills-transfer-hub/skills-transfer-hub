package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	sthhttp "sth/internal/http"
	"sth/internal/tui"
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
	if !a.isInteractiveErrorSession() {
		return err
	}

	a.progress.suspend()
	a.writeStderr(fmt.Sprintf("Authentication failed (HTTP %d) — your token may have expired.", ae.StatusCode))
	token, perr := a.promptSessionToken()
	if perr != nil {
		return fmt.Errorf("command interrupted: %w", perr)
	}
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("command interrupted: no token provided")
	}

	if setErr := os.Setenv("STH_TOKEN", strings.TrimSpace(token)); setErr != nil {
		return fmt.Errorf("command interrupted: could not set STH_TOKEN: %w", setErr)
	}
	a.verbose.Decision("auth recovery: retrying with fresh STH_TOKEN")
	a.progress.resume()
	return fn()
}

func (a *app) promptSessionToken() (string, error) {
	session := tui.NewSession(a.stdin, a.stderr)
	defer session.Close()
	token, err := tui.Secret(session, tui.SecretConfig{
		Label: "Enter a fresh token",
		Hint:  "Input is hidden. Press Esc or Ctrl+C to abort.",
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(token), nil
}
