package http

import (
	"context"
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGlobalSourceParsing(t *testing.T) {
	for _, input := range []string{"https://github.com/DietrichGebert/ponytail", "github:DietrichGebert/ponytail", "https://github.com/DietrichGebert/ponytail.git/"} {
		source, err := ParseGlobalSource(input)
		if err != nil {
			t.Fatal(err)
		}
		if source.Provider != "github" || source.Host != "github.com" || source.Repository != "dietrichgebert/ponytail" {
			t.Fatalf("unexpected source: %#v", source)
		}
		if source.CanonicalID("skills/ponytail") != "github:github.com/dietrichgebert/ponytail#skills/ponytail" {
			t.Fatal(source.CanonicalID("skills/ponytail"))
		}
	}
	for _, input := range []string{"ponytail", "http://github.com/owner/repo", "https://token@github.com/owner/repo", "https://github.com/owner/repo/tree/main", "https://github.com/owner/repo?token=secret", "https://github.com/owner/repo#main", "github:owner/../repo", "github:owner/repo/", "https://gitlab.com/owner/repo", "https://github.com:443/owner/repo", "https://github.com/owner/%2e%2e"} {
		_, err := ParseGlobalSource(input)
		if err == nil {
			t.Errorf("accepted invalid input %q", input)
		}
		if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "token@") {
			t.Errorf("unsafe error: %v", err)
		}
	}
}

const globalTestRevision = "0123456789abcdef0123456789abcdef01234567"

func globalTestClient(t *testing.T, handler stdhttp.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	t.Setenv("STH_GITHUB_API_BASE_URL", server.URL)
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL+"/raw")
	t.Setenv("STH_TOKEN", "test-token")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("STH_USER_CONFIG_DIR", t.TempDir())
	return NewClient(server.Client())
}

func TestGlobalResolveFreezesDefaultBranchAndExplicitReference(t *testing.T) {
	for _, reference := range []string{"", "feature/skills", globalTestRevision} {
		t.Run(reference, func(t *testing.T) {
			requests := []string{}
			client := globalTestClient(t, func(response stdhttp.ResponseWriter, request *stdhttp.Request) {
				requests = append(requests, request.URL.Path)
				if request.Header.Get("Authorization") != "Bearer test-token" {
					t.Error("existing credential resolver was not used")
				}
				switch request.URL.Path {
				case "/repos/owner/repo":
					json.NewEncoder(response).Encode(map[string]string{"default_branch": "main"})
				case "/repos/owner/repo/commits/main", "/repos/owner/repo/commits/feature/skills", "/repos/owner/repo/commits/" + globalTestRevision:
					json.NewEncoder(response).Encode(map[string]string{"sha": globalTestRevision})
				default:
					t.Errorf("unexpected request: %s", request.URL.Path)
					response.WriteHeader(404)
				}
			})
			source, _ := ParseGlobalSource("github:owner/repo")
			resolution, err := client.ResolveGlobalSource(context.Background(), source, reference)
			if err != nil {
				t.Fatal(err)
			}
			expectedRef := reference
			if expectedRef == "" {
				expectedRef = "main"
			}
			if resolution.Revision != globalTestRevision || resolution.RequestedRef != expectedRef || resolution.Source != source {
				t.Fatalf("wrong resolution: %#v", resolution)
			}
			expectedRequests := 1
			if reference == "" {
				expectedRequests = 2
			}
			if len(requests) != expectedRequests {
				t.Fatalf("requests: %v", requests)
			}
		})
	}
}

func TestGlobalResolveRejectsMissingOrInvalidSHAAndAuthenticationErrors(t *testing.T) {
	for _, responseBody := range []string{`{"sha":"main"}`, `{"sha":"zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"}`, `{}`, `invalid`} {
		client := globalTestClient(t, func(response stdhttp.ResponseWriter, request *stdhttp.Request) { response.Write([]byte(responseBody)) })
		source, _ := ParseGlobalSource("github:owner/repo")
		if _, err := client.ResolveGlobalSource(context.Background(), source, "main"); err == nil {
			t.Fatalf("accepted %s", responseBody)
		}
	}
	client := globalTestClient(t, func(response stdhttp.ResponseWriter, request *stdhttp.Request) {
		response.WriteHeader(stdhttp.StatusForbidden)
	})
	source, _ := ParseGlobalSource("github:owner/repo")
	if _, err := client.ResolveGlobalSource(context.Background(), source, "main"); err == nil {
		t.Fatal("accepted forbidden source")
	}
}

func TestGlobalSourceAcceptsDotPrefixedRepositoryAndRejectsNavigationReferences(t *testing.T) {
	source, err := ParseGlobalSource("https://github.com/owner/.github")
	if err != nil || source.Repository != "owner/.github" {
		t.Fatalf("valid repository rejected: %#v %v", source, err)
	}
	client := globalTestClient(t, func(response stdhttp.ResponseWriter, request *stdhttp.Request) {
		t.Error("invalid reference reached network")
		response.WriteHeader(500)
	})
	for _, reference := range []string{"..", "../main", "refs/../main", "feature//skill", "/main", "main/", "main.lock", "main@{1}", "main\tsecret"} {
		if _, err := client.ResolveGlobalSource(context.Background(), source, reference); err == nil {
			t.Errorf("accepted reference %q", reference)
		}
	}
}

func TestGlobalResolveRejectsDifferentCommitForExplicitSHA(t *testing.T) {
	client := globalTestClient(t, func(response stdhttp.ResponseWriter, request *stdhttp.Request) {
		json.NewEncoder(response).Encode(map[string]string{"sha": strings.Repeat("f", 40)})
	})
	source, _ := ParseGlobalSource("github:owner/repo")
	if _, err := client.ResolveGlobalSource(context.Background(), source, globalTestRevision); err == nil {
		t.Fatal("explicit commit changed during resolution")
	}
}
