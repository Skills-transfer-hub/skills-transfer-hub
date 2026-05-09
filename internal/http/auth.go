package http

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"sth/internal/auth"
	"sth/internal/fs"
)

// AuthError signals that the remote refused the credential (or none was
// provided). cmd/recoverable.go inspects this via IsAuthError to decide
// whether to prompt for a fresh token. StatusCode == 0 means no network call
// was made (no credential available).
type AuthError struct {
	StatusCode int
	URL        string
	Reason     string // optional context (e.g. "OIDC exchange failed: ...")
}

// Error returns a stable English-leaning summary that auth-recovery code
// can rely on. Callers that want a French-language wrapper can decorate.
func (e *AuthError) Error() string {
	if e == nil {
		return ""
	}
	if e.StatusCode == 0 {
		if e.Reason != "" {
			return fmt.Sprintf("authentication required: %s", e.Reason)
		}
		return "authentication required: no credential available"
	}
	if e.Reason != "" {
		return fmt.Sprintf("authentication failed (HTTP %d) for %s: %s", e.StatusCode, e.URL, e.Reason)
	}
	return fmt.Sprintf("authentication failed (HTTP %d) for %s", e.StatusCode, e.URL)
}

// IsAuthError extracts an *AuthError from an error chain.
func IsAuthError(err error) (*AuthError, bool) {
	if err == nil {
		return nil, false
	}
	var ae *AuthError
	if errors.As(err, &ae) {
		return ae, true
	}
	return nil, false
}

// Prompter is invoked by the credential resolver when no env-based source
// produces a token AND the caller (in cmd/) decided a TTY-based prompt is
// allowed. Returning empty + nil means "user dismissed".
type Prompter func(ctx context.Context) (string, error)

// resolveCredentialHeadersWithPrompter runs the spec-007 chain:
//   - profile.APIToken (session token from init)
//   - sth-cloud keychain (unchanged)
//   - STH_TOKEN env
//   - OIDC (when ACTIONS_ID_TOKEN_REQUEST_URL is set)
//   - provider env vars
//   - git-credential
//   - prompter (when supplied)
//   - AuthError otherwise
func resolveCredentialHeadersWithPrompter(ctx context.Context, profile fs.ProjectProfile, prompter Prompter) (credentialHeaders, error) {
	if profile.APIToken != "" {
		return providerAPITokenHeaders(profile.APIToken, profile.Provider), nil
	}

	if strings.EqualFold(strings.TrimSpace(profile.Provider), "sth-cloud") {
		return resolveCloudCredential(profile)
	}

	if headers, ok := resolveEnvironmentHeaders(profile); ok {
		return headers, nil
	}

	if strings.TrimSpace(os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL")) != "" {
		token, err := exchangeGitHubActionsOIDC(ctx, oidcAudience)
		if err != nil {
			return credentialHeaders{
					source: AccessSource{Kind: "oidc", Hint: "github-actions", Resolved: false},
				}, &AuthError{
					Reason: fmt.Sprintf("OIDC exchange failed: %v", err),
				}
		}
		return credentialHeaders{
			headers: map[string]string{"Authorization": "Bearer " + token},
			source:  AccessSource{Kind: "oidc", Hint: "github-actions", Resolved: true},
		}, nil
	}

	host, credentialPath, err := providerCredentialTarget(profile)
	if err != nil {
		return credentialHeaders{}, err
	}

	username, password, gcErr := gitCredentialFill(ctx, host, credentialPath)
	if gcErr == nil && (strings.TrimSpace(username) != "" || strings.TrimSpace(password) != "") {
		return credentialHeaders{
			headers: map[string]string{
				"Authorization": basicAuthorization(username, password),
			},
			source: AccessSource{Kind: "git-credential", Hint: host, Resolved: true},
		}, nil
	}

	if prompter != nil {
		token, perr := prompter(ctx)
		if perr == nil && strings.TrimSpace(token) != "" {
			return bearerHeaders(strings.TrimSpace(token), "session"), nil
		}
	}

	return credentialHeaders{
			source: AccessSource{Kind: "none", Hint: host, Resolved: false},
		}, &AuthError{
			Reason: fmt.Sprintf("no credential available for %s — set STH_TOKEN, configure git-credential, or run `sth init`", host),
		}
}

func resolveCloudCredential(profile fs.ProjectProfile) (credentialHeaders, error) {
	key, err := auth.LoadAPIKey()
	if err != nil {
		return credentialHeaders{
			source: AccessSource{Kind: "none", Hint: "sth-cloud", Resolved: false},
		}, err
	}
	return credentialHeaders{
		headers: map[string]string{"Authorization": "Bearer " + key},
		source:  AccessSource{Kind: "keychain", Hint: auth.KeyringService, Resolved: true},
	}, nil
}
