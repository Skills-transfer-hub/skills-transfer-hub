package http

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	stdhttp "net/http"
)

func TestExchangeGitHubActionsOIDC_success(t *testing.T) {
	srv := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-actions-token" {
			t.Fatalf("Authorization=%q want %q", got, "Bearer test-actions-token")
		}
		if got := r.URL.Query().Get("audience"); got != "sth-cli" {
			t.Fatalf("audience=%q want %q", got, "sth-cli")
		}
		_, _ = w.Write([]byte(`{"value":"jwt-token-xyz"}`))
	}))
	defer srv.Close()

	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", srv.URL+"/token")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "test-actions-token")

	token, err := exchangeGitHubActionsOIDC(context.Background(), "sth-cli")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if token != "jwt-token-xyz" {
		t.Fatalf("token = %q want %q", token, "jwt-token-xyz")
	}
}

func TestExchangeGitHubActionsOIDC_missingEnv(t *testing.T) {
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", "")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "")
	if _, err := exchangeGitHubActionsOIDC(context.Background(), "sth-cli"); err == nil {
		t.Fatal("expected error for missing env vars")
	}
}

func TestExchangeGitHubActionsOIDC_serverError(t *testing.T) {
	srv := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		stdhttp.Error(w, "boom", stdhttp.StatusInternalServerError)
	}))
	defer srv.Close()

	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", srv.URL)
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "x")
	_, err := exchangeGitHubActionsOIDC(context.Background(), "sth-cli")
	if err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("err = %v, want HTTP 500", err)
	}
}

func TestExchangeGitHubActionsOIDC_appendsAudienceWhenQueryAlreadyPresent(t *testing.T) {
	called := false
	srv := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		called = true
		if got := r.URL.Query().Get("audience"); got != "sth-cli" {
			t.Fatalf("audience=%q want %q", got, "sth-cli")
		}
		if got := r.URL.Query().Get("foo"); got != "bar" {
			t.Fatalf("foo=%q want %q", got, "bar")
		}
		_, _ = w.Write([]byte(`{"value":"x"}`))
	}))
	defer srv.Close()

	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", srv.URL+"/token?foo=bar")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "y")
	if _, err := exchangeGitHubActionsOIDC(context.Background(), "sth-cli"); err != nil {
		t.Fatalf("err = %v", err)
	}
	if !called {
		t.Fatal("server not called")
	}
}
