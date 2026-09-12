package http

import (
	"context"
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
	if err == nil {
		t.Fatal("expected rate limit error")
	}
	if _, authentication := IsAuthError(err); authentication {
		t.Fatal("quota failure must not prompt for a token")
	}
	if !strings.Contains(err.Error(), "quota resets") {
		t.Fatalf("missing next action: %v", err)
	}
}
