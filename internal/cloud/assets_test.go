package cloud

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"sth/internal/auth"
	"sth/internal/cloud/sthv2"
)

const testDesktopAccessToken = "sth_at_abcdefghijklmnopqrstuvwxyzABCDEFGH012345678"

func TestV2TransportPrefersShortDesktopTokenWithoutMCPHeader(t *testing.T) {
	t.Setenv(EnvCloudAccessToken, testDesktopAccessToken)
	t.Setenv(auth.EnvAPIKey, "legacy-key-must-not-be-used")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got := request.Header.Get("Authorization"); got != "Bearer "+testDesktopAccessToken {
			t.Errorf("Authorization = %q", got)
		}
		if got := request.Header.Get("X-STH-Client"); got != "" {
			t.Errorf("desktop v2 request leaked MCP header %q", got)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"id":"skill::skills::review-pr","family":"package","kind":"skill","name":"Review PR","status":"available","latest_version_id":"version-1","latest_version":"1.0.0"}`)
	}))
	defer server.Close()
	t.Setenv("STH_CLOUD_BASE_URL", server.URL)

	asset, err := GetAsset(context.Background(), nil, "skill::skills::review-pr")
	if err != nil {
		t.Fatal(err)
	}
	if asset.LatestVersionID != "version-1" {
		t.Fatalf("asset = %+v", asset)
	}
}

func TestV2TransportRejectsInvalidPresentDesktopTokenWithoutFallback(t *testing.T) {
	for _, invalid := range []string{"", " legacy", "sth_at_too short", "sth_at_bad\r\nheader"} {
		t.Run(fmt.Sprintf("%q", invalid), func(t *testing.T) {
			t.Setenv(EnvCloudAccessToken, invalid)
			t.Setenv(auth.EnvAPIKey, "legacy-key-must-not-be-used")
			var requests atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
			defer server.Close()
			t.Setenv("STH_CLOUD_BASE_URL", server.URL)
			_, err := GetAsset(context.Background(), nil, "skill::skills::review-pr")
			if err == nil || !strings.Contains(err.Error(), "jeton d’accès desktop invalide") {
				t.Fatalf("error = %v", err)
			}
			if requests.Load() != 0 {
				t.Fatal("invalid desktop token reached the network")
			}
		})
	}
}

func TestV2TransportRejectsRedirects(t *testing.T) {
	t.Setenv(EnvCloudAccessToken, testDesktopAccessToken)
	var redirected atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/redirected" {
			redirected.Add(1)
			return
		}
		http.Redirect(writer, request, "/redirected", http.StatusFound)
	}))
	defer server.Close()
	t.Setenv("STH_CLOUD_BASE_URL", server.URL)
	_, err := GetAsset(context.Background(), nil, "skill::skills::review-pr")
	if err == nil || !strings.Contains(err.Error(), "HTTP 302") {
		t.Fatalf("error = %v", err)
	}
	if redirected.Load() != 0 {
		t.Fatal("v2 client followed a redirect")
	}
}

func TestDownloadResourceVersionBundleVerifiesExactManifest(t *testing.T) {
	content := []byte("# Review PR\n")
	manifest := testPackageManifest(content)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		_, _ = writer.Write(content)
	}))
	defer server.Close()
	t.Setenv("STH_CLOUD_BASE_URL", server.URL)
	downloads := testPackageDownloads(server.URL+"/blob", manifest.Files[0])

	got, err := DownloadResourceVersionBundle(context.Background(), nil, manifest, downloads)
	if err != nil {
		t.Fatal(err)
	}
	if string(got[manifest.Entrypoint]) != string(content) {
		t.Fatalf("content = %q", got[manifest.Entrypoint])
	}
}

func TestDownloadResourceVersionBundleRejectsRedirectHashAndForeignOrigin(t *testing.T) {
	content := []byte("# Review PR\n")
	manifest := testPackageManifest(content)
	tests := []struct {
		name      string
		configure func(*testing.T, *httptest.Server, *PackageDownloads)
		want      string
	}{
		{
			name: "hash mismatch",
			configure: func(_ *testing.T, _ *httptest.Server, downloads *PackageDownloads) {
				downloads.Downloads[0].SHA256 = strings.Repeat("0", 64)
			},
			want: "métadonnées incohérentes",
		},
		{
			name: "foreign origin",
			configure: func(_ *testing.T, _ *httptest.Server, downloads *PackageDownloads) {
				downloads.Downloads[0].DownloadURL = "https://attacker.invalid/blob"
			},
			want: "URL signée invalide",
		},
		{
			name: "redirect",
			configure: func(_ *testing.T, server *httptest.Server, downloads *PackageDownloads) {
				downloads.Downloads[0].DownloadURL = server.URL + "/redirect"
			},
			want: "HTTP 302",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var followed atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path == "/redirect" {
					http.Redirect(writer, request, "/target", http.StatusFound)
					return
				}
				if request.URL.Path == "/target" {
					followed.Add(1)
				}
				writer.Header().Set("Content-Type", "text/markdown")
				_, _ = writer.Write(content)
			}))
			defer server.Close()
			t.Setenv("STH_CLOUD_BASE_URL", server.URL)
			downloads := testPackageDownloads(server.URL+"/blob", manifest.Files[0])
			test.configure(t, server, &downloads)
			_, err := DownloadResourceVersionBundle(context.Background(), nil, manifest, downloads)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
			if followed.Load() != 0 {
				t.Fatal("download client followed redirect")
			}
		})
	}
}

func TestValidateResourceVersionManifestRejectsTraversalAndBounds(t *testing.T) {
	content := []byte("# Review PR\n")
	manifest := testPackageManifest(content)
	manifest.Files[0].Path = "skills/review-pr/../../secret"
	if err := validateResourceVersionManifest(manifest, manifest.AssetID, manifest.VersionID); err == nil {
		t.Fatal("traversal manifest accepted")
	}
	manifest = testPackageManifest(content)
	manifest.Files[0].Size = maxCloudPackageFileBytes + 1
	if err := validateResourceVersionManifest(manifest, manifest.AssetID, manifest.VersionID); err == nil {
		t.Fatal("oversized manifest accepted")
	}
}

func TestPublicationUUIDValidationRequiresRFC4122VersionAndVariant(t *testing.T) {
	for _, valid := range []string{
		"11111111-1111-1111-8111-111111111111",
		"22222222-2222-5222-b222-222222222222",
	} {
		if !validCanonicalUUID(valid) {
			t.Fatalf("valid UUID rejected: %s", valid)
		}
	}
	for _, invalid := range []string{
		"11111111-1111-0111-8111-111111111111",
		"11111111-1111-4111-7111-111111111111",
		"11111111-1111-4111-C111-111111111111",
	} {
		if validCanonicalUUID(invalid) {
			t.Fatalf("invalid UUID accepted: %s", invalid)
		}
	}
}

func TestUploadPackagePublicationFilesNeverFollowsRedirect(t *testing.T) {
	content := []byte("# Review\n")
	manifest := PublicationManifestInput{
		Asset:   PublicationAssetInput{Kind: "skill", Slug: "review-pr", FolderName: "skills", Name: "Review PR"},
		Version: "1.0.0", Entrypoint: "SKILL.md",
		Files:         []PublicationFile{{Path: "SKILL.md", SHA256: sha256HexForTest(content), Size: int64(len(content)), MIME: "text/markdown"}},
		Compatibility: map[string]string{"claude": "supported", "codex": "supported"},
	}
	manifest.Origin.RepositoryURL = "https://example.test/org/repo"
	manifest.Origin.Ref = "main"
	manifest.Origin.Commit = strings.Repeat("a", 40)
	var followed atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/target" {
			followed.Add(1)
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		http.Redirect(writer, request, "/target", http.StatusFound)
	}))
	defer server.Close()
	t.Setenv("STH_CLOUD_BASE_URL", server.URL)
	uploads := []BlobUpload{{
		Path: "SKILL.md", SHA256: manifest.Files[0].SHA256, UploadURL: server.URL + "/redirect",
		ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	}}
	err := UploadPackagePublicationFiles(context.Background(), nil, manifest, uploads, map[string][]byte{"SKILL.md": content})
	if err == nil || !strings.Contains(err.Error(), "HTTP 302") {
		t.Fatalf("error = %v", err)
	}
	if followed.Load() != 0 {
		t.Fatal("signed upload followed redirect")
	}
}

func testPackageManifest(content []byte) ResourceVersionManifest {
	return ResourceVersionManifest{
		SchemaVersion: "2",
		AssetID:       "skill::skills::review-pr",
		VersionID:     "version-1",
		Version:       "1.0.0",
		Kind:          "skill",
		Origin: struct {
			RepositoryURL string `json:"repository_url"`
			Ref           string `json:"ref"`
			Commit        string `json:"commit"`
		}{RepositoryURL: "https://example.test/org/repo", Ref: "main", Commit: strings.Repeat("a", 40)},
		Entrypoint: "SKILL.md",
		Files: []sthv2.ManifestFile{{
			Path:   "SKILL.md",
			SHA256: sha256HexForTest(content),
			Size:   int64(len(content)),
			MIME:   "text/markdown",
		}},
		Compatibility: map[string]string{"claude": "supported", "codex": "supported"},
	}
}

func testPackageDownloads(downloadURL string, file sthv2.ManifestFile) PackageDownloads {
	return PackageDownloads{
		AssetID:   "skill::skills::review-pr",
		VersionID: "version-1",
		Downloads: []sthv2.PackageDownload{{
			Path: file.Path, SHA256: file.SHA256, Size: file.Size, MIME: file.MIME,
			DownloadURL: downloadURL, ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		}},
	}
}

func sha256HexForTest(content []byte) string {
	return fmt.Sprintf("%x", sha256SumForTest(content))
}

func sha256SumForTest(content []byte) [32]byte {
	return sha256.Sum256(content)
}
