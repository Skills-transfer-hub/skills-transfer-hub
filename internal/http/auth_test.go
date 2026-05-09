package http

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	stdhttp "net/http"

	"sth/internal/fs"
)

func TestAuthError_Error(t *testing.T) {
	cases := []struct {
		name string
		ae   *AuthError
		want string
	}{
		{name: "no creds", ae: &AuthError{}, want: "authentication required: no credential available"},
		{name: "no creds w/ reason", ae: &AuthError{Reason: "OIDC exchange failed: boom"}, want: "authentication required: OIDC exchange failed: boom"},
		{name: "401", ae: &AuthError{StatusCode: 401, URL: "https://x"}, want: "authentication failed (HTTP 401) for https://x"},
		{name: "403 w/ reason", ae: &AuthError{StatusCode: 403, URL: "https://x", Reason: "scope"}, want: "authentication failed (HTTP 403) for https://x: scope"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.ae.Error(); got != tc.want {
				t.Fatalf("Error()=%q want %q", got, tc.want)
			}
		})
	}
}

func TestIsAuthError_Wrapped(t *testing.T) {
	wrapped := fmt.Errorf("download failed: %w", &AuthError{StatusCode: 401, URL: "u"})
	ae, ok := IsAuthError(wrapped)
	if !ok || ae == nil || ae.StatusCode != 401 {
		t.Fatalf("IsAuthError did not extract: ae=%+v ok=%v", ae, ok)
	}
}

func TestIsAuthError_NonAuth(t *testing.T) {
	if _, ok := IsAuthError(errors.New("bare")); ok {
		t.Fatal("non-auth error matched")
	}
	if _, ok := IsAuthError(nil); ok {
		t.Fatal("nil err matched")
	}
}

func TestResolveCredentialChain_STH_TOKEN_takesPrecedenceOverOIDC(t *testing.T) {
	t.Setenv("STH_TOKEN", "from-env")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", "http://should-not-be-called.invalid")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "x")
	t.Setenv("GITHUB_TOKEN", "")

	headers, err := resolveCredentialHeadersWithPrompter(context.Background(), fs.ProjectProfile{Provider: "github", RepositoryID: "o/r"}, nil)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got := headers.headers["Authorization"]; got != "Bearer from-env" {
		t.Fatalf("Authorization=%q want %q", got, "Bearer from-env")
	}
	if headers.source.Hint != "STH_TOKEN" {
		t.Fatalf("source.Hint=%q want STH_TOKEN", headers.source.Hint)
	}
}

func TestResolveCredentialChain_OIDCWhenNoOtherCreds(t *testing.T) {
	srv := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		_, _ = w.Write([]byte(`{"value":"jwt-from-oidc"}`))
	}))
	defer srv.Close()

	t.Setenv("STH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", srv.URL)
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "actions-token")

	headers, err := resolveCredentialHeadersWithPrompter(context.Background(), fs.ProjectProfile{Provider: "github", RepositoryID: "o/r"}, nil)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got := headers.headers["Authorization"]; got != "Bearer jwt-from-oidc" {
		t.Fatalf("Authorization=%q want %q", got, "Bearer jwt-from-oidc")
	}
	if headers.source.Kind != "oidc" {
		t.Fatalf("source.Kind=%q want oidc", headers.source.Kind)
	}
}

func TestResolveCredentialChain_OIDCFailureReturnsAuthError(t *testing.T) {
	srv := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		stdhttp.Error(w, "boom", stdhttp.StatusInternalServerError)
	}))
	defer srv.Close()

	t.Setenv("STH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", srv.URL)
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "x")

	_, err := resolveCredentialHeadersWithPrompter(context.Background(), fs.ProjectProfile{Provider: "github", RepositoryID: "o/r"}, nil)
	ae, ok := IsAuthError(err)
	if !ok {
		t.Fatalf("expected AuthError, got %v", err)
	}
	if !strings.Contains(ae.Reason, "OIDC exchange failed") {
		t.Fatalf("Reason=%q want OIDC exchange failed", ae.Reason)
	}
}

func TestResolveCredentialChain_PrompterFallback(t *testing.T) {
	t.Setenv("STH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", "")
	t.Setenv("PATH", "") // make `git credential fill` fail

	called := false
	prompter := func(ctx context.Context) (string, error) {
		called = true
		return "from-prompt", nil
	}
	headers, err := resolveCredentialHeadersWithPrompter(context.Background(), fs.ProjectProfile{Provider: "github", RepositoryID: "o/r"}, prompter)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !called {
		t.Fatal("prompter not called")
	}
	if got := headers.headers["Authorization"]; got != "Bearer from-prompt" {
		t.Fatalf("Authorization=%q", got)
	}
}

func TestResolveCredentialChain_NoPrompterReturnsAuthError(t *testing.T) {
	t.Setenv("STH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", "")
	t.Setenv("PATH", "") // make `git credential fill` fail

	_, err := resolveCredentialHeadersWithPrompter(context.Background(), fs.ProjectProfile{Provider: "github", RepositoryID: "o/r"}, nil)
	if _, ok := IsAuthError(err); !ok {
		t.Fatalf("expected AuthError, got %v", err)
	}
}

func TestResolveCredentialChain_APITokenWinsOverEverything(t *testing.T) {
	t.Setenv("STH_TOKEN", "from-env")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", "x")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "y")

	profile := fs.ProjectProfile{Provider: "github", RepositoryID: "o/r", APIToken: "session-token"}
	headers, err := resolveCredentialHeadersWithPrompter(context.Background(), profile, nil)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got := headers.headers["Authorization"]; got != "Bearer session-token" {
		t.Fatalf("Authorization=%q want session-token", got)
	}
	// Note: github uses bearerHeaders (Kind=env, Hint=APIToken) for APIToken;
	// only gitlab uses Kind=session. Either way, the APIToken short-circuits
	// the env/OIDC chain — that's what we're verifying here.
	if headers.source.Hint != "APIToken" {
		t.Fatalf("source.Hint=%q want APIToken", headers.source.Hint)
	}
}

func TestDownloadURL_returnsAuthErrorOn401(t *testing.T) {
	srv := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		stdhttp.Error(w, "no", stdhttp.StatusUnauthorized)
	}))
	defer srv.Close()

	c := NewClient(nil)
	_, err := c.downloadURL(context.Background(), srv.URL, nil)
	ae, ok := IsAuthError(err)
	if !ok {
		t.Fatalf("err = %v, want AuthError", err)
	}
	if ae.StatusCode != 401 {
		t.Fatalf("StatusCode=%d want 401", ae.StatusCode)
	}
}

func TestDownloadURL_returnsAuthErrorOn403(t *testing.T) {
	srv := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		stdhttp.Error(w, "no", stdhttp.StatusForbidden)
	}))
	defer srv.Close()

	c := NewClient(nil)
	_, err := c.downloadURL(context.Background(), srv.URL, nil)
	ae, ok := IsAuthError(err)
	if !ok || ae.StatusCode != 403 {
		t.Fatalf("err = %v, want AuthError 403", err)
	}
}
