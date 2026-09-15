package http

import (
	"context"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sth/internal/fs"
)

func TestPrivateProvidersUseTheirOwnCredentials(t *testing.T) {
	t.Setenv("STH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "github-fixture-key")
	t.Setenv("GITLAB_TOKEN", "gitlab-fixture-key")
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	server := httptest.NewServer(stdhttp.HandlerFunc(func(response stdhttp.ResponseWriter, request *stdhttp.Request) {
		if strings.Contains(request.URL.Path, "/projects/") {
			if request.Header.Get("PRIVATE-TOKEN") != "gitlab-fixture-key" || request.Header.Get("Authorization") != "" {
				response.WriteHeader(stdhttp.StatusUnauthorized)
				return
			}
		} else if request.Header.Get("Authorization") != "Bearer github-fixture-key" || request.Header.Get("PRIVATE-TOKEN") != "" {
			response.WriteHeader(stdhttp.StatusUnauthorized)
			return
		}
		_, _ = response.Write([]byte("catalog accessible"))
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)
	t.Setenv("STH_GITLAB_API_BASE_URL", server.URL)
	client := NewClient(server.Client())
	for _, provider := range []string{"github", "gitlab"} {
		profile := fs.ProjectProfile{Provider: provider, RepositoryID: "owner/private", Ref: "main", CatalogPath: "SKILLS.md", TargetDir: ".sth"}
		if err := client.CheckProjectAccess(context.Background(), profile); err != nil {
			t.Fatalf("%s private access failed: %v", provider, err)
		}
	}
	t.Setenv("GITLAB_TOKEN", "wrong-key")
	profile := fs.ProjectProfile{Provider: "gitlab", RepositoryID: "owner/private", Ref: "main", CatalogPath: "SKILLS.md", TargetDir: ".sth"}
	if _, authentication := IsAuthError(client.CheckProjectAccess(context.Background(), profile)); !authentication {
		t.Fatal("invalid private credential accepted")
	}
}
