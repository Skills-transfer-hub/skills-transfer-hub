package http

import (
	"context"
	"errors"
	"sth/internal/fs"
	"strings"
	"testing"

	stdhttp "net/http"
	"net/http/httptest"
)

func newCatalogServer(t *testing.T, handler stdhttp.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(handler)
}

func githubProfileForTest(targetDir string) fs.ProjectProfile {
	if strings.TrimSpace(targetDir) == "" {
		targetDir = ".claude/resources"
	}

	return fs.ProjectProfile{
		Provider:     "github",
		RepositoryID: "owner/repo",
		Ref:          "main",
		CatalogPath:  "SKILLS.md",
		TargetDir:    targetDir,
	}
}

func catalogMarkdown(rows ...string) string {
	lines := []string{
		"| Type | Name | Folder | Source | Description |",
		"| ---- | ---- | ------ | ------ | ----------- |",
	}
	lines = append(lines, rows...)
	return strings.Join(lines, "\n") + "\n"
}

func TestClient_ListCatalog_success(t *testing.T) {
	server := newCatalogServer(t, func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if r.URL.Path != "/owner/repo/main/SKILLS.md" {
			stdhttp.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(catalogMarkdown(
			"| skill | alpha | skills | skills/alpha.md | Alpha |",
			"| agent | bravo | agents | agents/bravo.md | Bravo |",
		)))
	})
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)

	client := NewClient(server.Client())
	entries, err := client.ListCatalog(context.Background(), githubProfileForTest(".claude/resources"))
	if err != nil {
		t.Fatalf("ListCatalog() error = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}
	if entries[0].CatalogID != fs.CatalogID("agent", "agents", "bravo") {
		t.Fatalf("entries[0].CatalogID = %q", entries[0].CatalogID)
	}
}

func TestClient_DownloadCatalogEntry_notFound(t *testing.T) {
	server := newCatalogServer(t, stdhttp.NotFoundHandler().ServeHTTP)
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)

	client := NewClient(server.Client())
	_, err := client.DownloadCatalogEntry(context.Background(), githubProfileForTest(".claude/resources"), fs.CatalogEntry{
		CatalogID:    fs.CatalogID("skill", "skills", "missing"),
		ResourceType: "skill",
		Name:         "missing",
		FolderName:   "skills",
		SourcePath:   "skills/missing.md",
	})
	if err == nil {
		t.Fatal("DownloadCatalogEntry() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Fatalf("error %q does not contain 404", err)
	}
}

func TestParseCatalogEntries_rejectsMissingTable(t *testing.T) {
	_, err := parseCatalogEntries("# SKILLS\n\nAucune table ici.\n")
	if err == nil {
		t.Fatal("parseCatalogEntries() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "table catalogue introuvable") {
		t.Fatalf("error %q does not contain missing-table detail", err)
	}
}

func TestExtractReleaseHighlights(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		max      int
		expected []string
	}{
		{
			name: "first three bullets only",
			body: "## What changed\n\n- First bullet\n- Second bullet\n- Third bullet\n- Fourth bullet\n",
			max:  3,
			expected: []string{"First bullet", "Second bullet", "Third bullet"},
		},
		{
			name: "skips full changelog section",
			body: "## Highlights\n\n- New telemetry command\n\n## Full Changelog\n\n- feat(x): noise\n- fix(y): more noise\n",
			max:  3,
			expected: []string{"New telemetry command"},
		},
		{
			name: "ignores headings and empty lines",
			body: "# Title\n\n## Sub\n\nSome prose paragraph.\n\n* Star bullet works too\n",
			max:  3,
			expected: []string{"Star bullet works too"},
		},
		{
			name:     "empty body returns nil",
			body:     "",
			max:      3,
			expected: nil,
		},
		{
			name:     "no bullets returns nil",
			body:     "Just prose, no list items.\nAnother line.\n",
			max:      3,
			expected: nil,
		},
		{
			name:     "max zero returns nil",
			body:     "- one\n- two\n",
			max:      0,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractReleaseHighlights(tt.body, tt.max)
			if len(got) != len(tt.expected) {
				t.Fatalf("extractReleaseHighlights() = %v, want %v", got, tt.expected)
			}
			for i, want := range tt.expected {
				if got[i] != want {
					t.Errorf("extractReleaseHighlights()[%d] = %q, want %q", i, got[i], want)
				}
			}
		})
	}
}

func TestClient_CheckForStableUpdate_attachesHighlightsAndReleaseNotesURL(t *testing.T) {
	serverURL := ""
	server := newCatalogServer(t, func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if r.URL.Path != "/releases" {
			stdhttp.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`[
  {
    "tag_name": "v2.1.0",
    "prerelease": false,
    "html_url": "https://example.invalid/releases/v2.1.0",
    "body": "## What changed\n\n- 3-tier telemetry consent\n- Copilot writer\n- MultiStackScanner\n\n## Full Changelog\n\n- feat(x): noise",
    "assets": [
      {"name": "sth-windows-amd64.zip", "browser_download_url": "` + serverURL + `/asset"},
      {"name": "SHA256SUMS", "browser_download_url": "` + serverURL + `/checksum"}
    ]
  }
]`))
	})
	defer server.Close()
	serverURL = server.URL

	client := NewClient(server.Client())
	release, err := client.CheckForStableUpdate(context.Background(), ReleaseMetadata{
		CurrentVersion: "v1.0.0",
		ReleasesAPIURL: server.URL + "/releases",
		AssetName:      "sth-windows-amd64.zip",
		ChecksumName:   "SHA256SUMS",
	})
	if err != nil {
		t.Fatalf("CheckForStableUpdate() error = %v", err)
	}
	if release == nil {
		t.Fatal("CheckForStableUpdate() = nil, want stable release")
	}
	if release.ReleaseNotesURL != "https://example.invalid/releases/v2.1.0" {
		t.Errorf("release.ReleaseNotesURL = %q, want https://example.invalid/releases/v2.1.0", release.ReleaseNotesURL)
	}
	wantHighlights := []string{"3-tier telemetry consent", "Copilot writer", "MultiStackScanner"}
	if len(release.Highlights) != len(wantHighlights) {
		t.Fatalf("release.Highlights = %v, want %v", release.Highlights, wantHighlights)
	}
	for i, want := range wantHighlights {
		if release.Highlights[i] != want {
			t.Errorf("release.Highlights[%d] = %q, want %q", i, release.Highlights[i], want)
		}
	}
}

func TestClient_CheckForStableUpdate_selectsLatestStableRelease(t *testing.T) {
	serverURL := ""
	server := newCatalogServer(t, func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if r.URL.Path != "/releases" {
			stdhttp.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`[
  {
    "tag_name": "v3.0.0-beta.1",
    "prerelease": true,
    "assets": [
      {"name": "sth-windows-amd64.zip", "browser_download_url": "` + serverURL + `/beta"}
    ]
  },
  {
    "tag_name": "v2.0.0",
    "prerelease": false,
    "assets": [
      {"name": "sth-windows-amd64.zip", "browser_download_url": "` + serverURL + `/asset"},
      {"name": "SHA256SUMS", "browser_download_url": "` + serverURL + `/checksum"}
    ]
  }
]`))
	})
	defer server.Close()
	serverURL = server.URL

	client := NewClient(server.Client())
	release, err := client.CheckForStableUpdate(context.Background(), ReleaseMetadata{
		CurrentVersion: "v1.0.0",
		ReleasesAPIURL: server.URL + "/releases",
		AssetName:      "sth-windows-amd64.zip",
		ChecksumName:   "SHA256SUMS",
	})
	if err != nil {
		t.Fatalf("CheckForStableUpdate() error = %v", err)
	}
	if release == nil {
		t.Fatal("CheckForStableUpdate() = nil, want stable release")
	}
	if release.Version != "v2.0.0" {
		t.Fatalf("release.Version = %q, want v2.0.0", release.Version)
	}
	if !strings.Contains(release.DownloadURL, "/asset") {
		t.Fatalf("release.DownloadURL = %q, want /asset", release.DownloadURL)
	}
}

func TestClient_CheckForStableUpdate_ignoresPrereleaseOnlyFeed(t *testing.T) {
	serverURL := ""
	server := newCatalogServer(t, func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if r.URL.Path != "/releases" {
			stdhttp.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`[
  {
    "tag_name": "v3.0.0-beta.1",
    "prerelease": true,
    "assets": [
      {"name": "sth-windows-amd64.zip", "browser_download_url": "` + serverURL + `/beta"},
      {"name": "SHA256SUMS", "browser_download_url": "` + serverURL + `/checksum"}
    ]
  }
]`))
	})
	defer server.Close()
	serverURL = server.URL

	client := NewClient(server.Client())
	release, err := client.CheckForStableUpdate(context.Background(), ReleaseMetadata{
		CurrentVersion: "v1.0.0",
		ReleasesAPIURL: server.URL + "/releases",
		AssetName:      "sth-windows-amd64.zip",
		ChecksumName:   "SHA256SUMS",
	})
	if err != nil {
		t.Fatalf("CheckForStableUpdate() error = %v", err)
	}
	if release != nil {
		t.Fatalf("CheckForStableUpdate() = %+v, want nil when only prereleases exist", release)
	}
}

func TestClient_CheckProjectAccess_acceptsStructureScanWhenCatalogMissing(t *testing.T) {
	server := newCatalogServer(t, func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		switch r.URL.Path {
		case "/owner/repo/main/SKILLS.md":
			stdhttp.NotFound(w, r)
		case "/repos/owner/repo/git/trees/main":
			if r.URL.Query().Get("recursive") != "1" {
				stdhttp.Error(w, "missing recursive flag", stdhttp.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"tree":[{"path":"react/form-wizard/SKILL.md","type":"blob"}]}`))
		default:
			stdhttp.NotFound(w, r)
		}
	})
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)
	t.Setenv("STH_GITHUB_API_BASE_URL", server.URL)

	client := NewClient(server.Client())
	err := client.CheckProjectAccess(context.Background(), githubProfileForTest(".claude/resources"))
	if err != nil {
		t.Fatalf("CheckProjectAccess() error = %v", err)
	}
}

func commitLookupConfigForTest(providerName string) fs.ProjectConfigV2 {
	return fs.ProjectConfigV2{
		SchemaVersion: 2,
		Providers: []fs.ProviderEntry{
			{ID: "default", Provider: providerName, RepositoryID: "owner/repo", CatalogRef: "main"},
		},
	}
}

func TestClient_GetLatestCommitSHAForPath_success(t *testing.T) {
	server := newCatalogServer(t, func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if r.URL.Path == "/repos/owner/repo/commits" && r.URL.Query().Get("path") == "skills/alpha" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"sha":"deadbeef"}]`))
			return
		}
		stdhttp.NotFound(w, r)
	})
	defer server.Close()
	t.Setenv("STH_GITHUB_API_BASE_URL", server.URL)

	client := NewClient(server.Client())
	sha, err := client.GetLatestCommitSHAForPath(context.Background(), commitLookupConfigForTest("github"), "default", "skills/alpha")
	if err != nil {
		t.Fatalf("GetLatestCommitSHAForPath() error = %v", err)
	}
	if sha != "deadbeef" {
		t.Fatalf("sha = %q, want %q", sha, "deadbeef")
	}
}

func TestClient_GetLatestCommitSHAForPath_emptyArray(t *testing.T) {
	server := newCatalogServer(t, func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	})
	defer server.Close()
	t.Setenv("STH_GITHUB_API_BASE_URL", server.URL)

	client := NewClient(server.Client())
	sha, err := client.GetLatestCommitSHAForPath(context.Background(), commitLookupConfigForTest("github"), "default", "skills/alpha")
	if err != nil {
		t.Fatalf("GetLatestCommitSHAForPath() error = %v", err)
	}
	if sha != "" {
		t.Fatalf("sha = %q, want empty", sha)
	}
}

func TestClient_GetLatestCommitSHAForPath_nonGithubIsConfigError(t *testing.T) {
	client := NewClient(stdhttp.DefaultClient)
	_, err := client.GetLatestCommitSHAForPath(context.Background(), commitLookupConfigForTest("gitlab"), "default", "skills/alpha")
	var configErr *ProviderConfigError
	if !errors.As(err, &configErr) {
		t.Fatalf("error = %v, want *ProviderConfigError", err)
	}
}

func TestClient_GetLatestCommitSHAForPath_emptyPathIsConfigError(t *testing.T) {
	client := NewClient(stdhttp.DefaultClient)
	_, err := client.GetLatestCommitSHAForPath(context.Background(), commitLookupConfigForTest("github"), "default", "")
	var configErr *ProviderConfigError
	if !errors.As(err, &configErr) {
		t.Fatalf("error = %v, want *ProviderConfigError", err)
	}
}
