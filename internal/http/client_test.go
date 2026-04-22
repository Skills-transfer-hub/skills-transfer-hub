package http

import (
	"context"
	"loreskills/internal/fs"
	"strings"
	"testing"

	stdhttp "net/http"
	"net/http/httptest"
)

func TestClient_ListCatalog_success(t *testing.T) {
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if r.URL.Path != "/owner/repo/main/SKILLS.md" {
			stdhttp.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`| Type | Name | Folder | Source | Description |
| ---- | ---- | ------ | ------ | ----------- |
| skill | alpha | skills | skills/alpha.md | Alpha |
| agent | bravo | agents | agents/bravo.md | Bravo |
`))
	}))
	defer server.Close()
	t.Setenv("LORESKILLS_GITHUB_RAW_BASE_URL", server.URL)

	client := NewClient(server.Client())
	entries, err := client.ListCatalog(context.Background(), fs.ProjectProfile{
		Provider:     "github",
		RepositoryID: "owner/repo",
		Ref:          "main",
		CatalogPath:  "SKILLS.md",
		TargetDir:    ".claude/resources",
	})
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
	server := httptest.NewServer(stdhttp.NotFoundHandler())
	defer server.Close()
	t.Setenv("LORESKILLS_GITHUB_RAW_BASE_URL", server.URL)

	client := NewClient(server.Client())
	_, err := client.DownloadCatalogEntry(context.Background(), fs.ProjectProfile{
		Provider:     "github",
		RepositoryID: "owner/repo",
		Ref:          "main",
		CatalogPath:  "SKILLS.md",
		TargetDir:    ".claude/resources",
	}, fs.CatalogEntry{
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

func TestClient_CheckForStableUpdate_selectsLatestStableRelease(t *testing.T) {
	serverURL := ""
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if r.URL.Path != "/releases" {
			stdhttp.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`[
  {
    "tag_name": "v3.0.0-beta.1",
    "prerelease": true,
    "assets": [
      {"name": "loreskills-windows-amd64.zip", "browser_download_url": "` + serverURL + `/beta"}
    ]
  },
  {
    "tag_name": "v2.0.0",
    "prerelease": false,
    "assets": [
      {"name": "loreskills-windows-amd64.zip", "browser_download_url": "` + serverURL + `/asset"},
      {"name": "SHA256SUMS", "browser_download_url": "` + serverURL + `/checksum"}
    ]
  }
]`))
	}))
	defer server.Close()
	serverURL = server.URL

	client := NewClient(server.Client())
	release, err := client.CheckForStableUpdate(context.Background(), ReleaseMetadata{
		CurrentVersion: "v1.0.0",
		ReleasesAPIURL: server.URL + "/releases",
		AssetName:      "loreskills-windows-amd64.zip",
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
