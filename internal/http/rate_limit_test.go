package http

import (
	"context"
	"errors"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAnonymousRateLimitDoesNotTriggerAuthenticationRecovery(t *testing.T) {
	server := httptest.NewServer(stdhttp.HandlerFunc(func(response stdhttp.ResponseWriter, request *stdhttp.Request) {
		response.Header().Set("X-RateLimit-Remaining", "0")
		response.WriteHeader(stdhttp.StatusForbidden)
	}))
	defer server.Close()
	client := NewClient(server.Client())
	_, err := client.downloadURL(context.Background(), server.URL, nil)
	err = decorateAccessError(err, AccessSource{}, "github")
	if err == nil {
		t.Fatal("expected rate limit error")
	}
	if _, authentication := IsAuthError(err); authentication {
		t.Fatal("quota failure must not prompt for a token")
	}
	if !strings.Contains(err.Error(), "quota resets") {
		t.Fatalf("missing next action: %v", err)
	}
	if strings.Contains(err.Error(), "authenticate") {
		t.Fatalf("quota error suggests authentication: %v", err)
	}
}

func TestAccessAdviceDistinguishesNetworkAndAuthenticationFailures(t *testing.T) {
	networkFailure := errors.New("connection refused; check your network connection and retry the command")
	if decorated := decorateAccessError(networkFailure, AccessSource{}, "github"); decorated != networkFailure {
		t.Fatalf("network advice changed: %v", decorated)
	}
	authenticationFailure := &AuthError{StatusCode: 401, URL: "https://example.test/private"}
	decorated := decorateAccessError(authenticationFailure, AccessSource{}, "github")
	if !strings.Contains(decorated.Error(), "authenticate using") || !errors.Is(decorated, authenticationFailure) {
		t.Fatalf("authentication advice missing: %v", decorated)
	}
}
