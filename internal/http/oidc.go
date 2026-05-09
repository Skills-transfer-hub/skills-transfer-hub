package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	stdhttp "net/http"
	"os"
	"strings"
	"time"
)

// oidcAudience is hardcoded by spec 007 — the JWT issued by GitHub Actions
// is requested with this audience claim. Note: api.github.com will NOT
// accept a JWT with audience "sth-cli" — the OIDC leg is useful only when
// the target service explicitly trusts this audience (e.g. sth-cloud).
const oidcAudience = "sth-cli"

const oidcExchangeTimeout = 5 * time.Second

// exchangeGitHubActionsOIDC asks the in-runner OIDC service for a JWT with
// the given audience claim and returns the JWT string. The two env vars
// ACTIONS_ID_TOKEN_REQUEST_URL and ACTIONS_ID_TOKEN_REQUEST_TOKEN are
// injected automatically by GitHub Actions when `permissions: id-token: write`
// is set on the job. Both are required.
func exchangeGitHubActionsOIDC(ctx context.Context, audience string) (string, error) {
	requestURL := strings.TrimSpace(os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL"))
	requestToken := strings.TrimSpace(os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN"))
	if requestURL == "" {
		return "", fmt.Errorf("ACTIONS_ID_TOKEN_REQUEST_URL is empty")
	}
	if requestToken == "" {
		return "", fmt.Errorf("ACTIONS_ID_TOKEN_REQUEST_TOKEN is empty")
	}

	separator := "?"
	if strings.Contains(requestURL, "?") {
		separator = "&"
	}
	url := fmt.Sprintf("%s%saudience=%s", requestURL, separator, audience)

	cctx, cancel := context.WithTimeout(ctx, oidcExchangeTimeout)
	defer cancel()
	req, err := stdhttp.NewRequestWithContext(cctx, stdhttp.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("OIDC request build: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+requestToken)
	req.Header.Set("Accept", "application/json")

	client := &stdhttp.Client{Timeout: oidcExchangeTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("OIDC request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != stdhttp.StatusOK {
		return "", fmt.Errorf("OIDC server returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return "", fmt.Errorf("OIDC body read: %w", err)
	}
	var parsed struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("OIDC body parse: %w", err)
	}
	if strings.TrimSpace(parsed.Value) == "" {
		return "", fmt.Errorf("OIDC response missing 'value'")
	}
	return parsed.Value, nil
}
