package cmd

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"loreskills/internal/fs"
	stdhttp "net/http"
	"net/http/httptest"
)

func TestRun_updateWithNoStateShowsNoopMessage(t *testing.T) {
	projectDir := t.TempDir()

	writeProjectProfileForCommandTest(t, projectDir, fs.ProjectProfile{
		Provider:     "github",
		RepositoryID: "owner/repo",
		Ref:          "main",
		CatalogPath:  "SKILLS.md",
		TargetDir:    ".claude/resources",
	})

	result := runCLI(t, projectDir, t.TempDir(), "update")

	if result.err != nil {
		t.Fatalf("Run() error = %v", result.err)
	}
	if !strings.Contains(result.stdout, "Aucune ressource à mettre à jour.") {
		t.Fatalf("stdout %q does not contain noop message", result.stdout)
	}
}

func TestRun_updateRefreshesEveryTrackedResource(t *testing.T) {
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		switch r.URL.Path {
		case "/owner/repo/main/skills/alpha.md":
			_, _ = w.Write([]byte("# Alpha refreshed\n"))
		case "/owner/repo/main/agents/beta.md":
			_, _ = w.Write([]byte("# Beta refreshed\n"))
		default:
			stdhttp.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("LORESKILLS_GITHUB_RAW_BASE_URL", server.URL)

	writeProjectProfileForCommandTest(t, projectDir, fs.ProjectProfile{
		Provider:     "github",
		RepositoryID: "owner/repo",
		Ref:          "main",
		CatalogPath:  "SKILLS.md",
		TargetDir:    ".claude/resources",
	})
	writeInstalledStateForCommandTest(t, projectDir, ".claude/resources", fs.InstalledResourceState{Entries: []fs.InstalledResourceRecord{
		{CatalogID: fs.CatalogID("skill", "skills", "alpha"), ResourceType: "skill", Name: "alpha", FolderName: "skills", InstalledPath: ".claude/resources/skills/alpha.md", SourcePath: "skills/alpha.md", Status: "installed"},
		{CatalogID: fs.CatalogID("agent", "agents", "beta"), ResourceType: "agent", Name: "beta", FolderName: "agents", InstalledPath: ".claude/resources/agents/beta.md", SourcePath: "agents/beta.md", Status: "installed"},
	}})

	result := runCLI(t, projectDir, t.TempDir(), "update")

	if result.err != nil {
		t.Fatalf("Run() error = %v\nstderr=%s", result.err, result.stderr)
	}
	if !strings.Contains(result.stdout, "Ressource 'alpha' mise à jour") {
		t.Fatalf("stdout %q does not contain alpha update", result.stdout)
	}
	if !strings.Contains(result.stdout, "Ressource 'beta' mise à jour") {
		t.Fatalf("stdout %q does not contain beta update", result.stdout)
	}

	for _, relativePath := range []string{".claude/resources/skills/alpha.md", ".claude/resources/agents/beta.md"} {
		fullPath := filepath.Join(projectDir, filepath.FromSlash(relativePath))
		data, err := os.ReadFile(fullPath)
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", fullPath, err)
		}
		if !strings.Contains(string(data), "refreshed") {
			t.Fatalf("resource %s was not refreshed: %q", relativePath, data)
		}
	}
}

func TestRun_updateReportsPartialFailures(t *testing.T) {
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if r.URL.Path == "/owner/repo/main/skills/alpha.md" {
			_, _ = w.Write([]byte("# Alpha refreshed\n"))
			return
		}
		stdhttp.NotFound(w, r)
	}))
	defer server.Close()
	t.Setenv("LORESKILLS_GITHUB_RAW_BASE_URL", server.URL)

	writeProjectProfileForCommandTest(t, projectDir, fs.ProjectProfile{
		Provider:     "github",
		RepositoryID: "owner/repo",
		Ref:          "main",
		CatalogPath:  "SKILLS.md",
		TargetDir:    ".claude/resources",
	})
	writeInstalledStateForCommandTest(t, projectDir, ".claude/resources", fs.InstalledResourceState{Entries: []fs.InstalledResourceRecord{
		{CatalogID: fs.CatalogID("skill", "skills", "alpha"), ResourceType: "skill", Name: "alpha", FolderName: "skills", InstalledPath: ".claude/resources/skills/alpha.md", SourcePath: "skills/alpha.md", Status: "installed"},
		{CatalogID: fs.CatalogID("skill", "skills", "missing"), ResourceType: "skill", Name: "missing", FolderName: "skills", InstalledPath: ".claude/resources/skills/missing.md", SourcePath: "skills/missing.md", Status: "installed"},
	}})

	result := runCLI(t, projectDir, t.TempDir(), "update")

	if result.err == nil {
		t.Fatal("Run() error = nil, want error")
	}
	if !strings.Contains(result.stdout, "Ressource 'alpha' mise à jour") {
		t.Fatalf("stdout %q does not contain successful update", result.stdout)
	}
	if !strings.Contains(result.stderr, "missing") {
		t.Fatalf("stderr %q does not mention the failing resource", result.stderr)
	}
}

func TestRun_updateSelfAppliesStableRelease(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	executablePath := filepath.Join(projectDir, executableFileName())
	if err := os.WriteFile(executablePath, []byte("old-binary"), 0o755); err != nil {
		t.Fatalf("WriteFile(%s): %v", executablePath, err)
	}
	t.Setenv("LORESKILLS_EXECUTABLE_PATH", executablePath)

	assetName := defaultReleaseAssetName()
	archiveData := releaseArchiveForTest(t, assetName)
	checksum := sha256.Sum256(archiveData)
	checksumBody := []byte(fmt.Sprintf("%s  %s\n", hex.EncodeToString(checksum[:]), assetName))

	serverURL := ""
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		switch r.URL.Path {
		case "/releases":
			_, _ = w.Write([]byte(fmt.Sprintf(`[{"tag_name":"v9.9.9","prerelease":false,"assets":[{"name":"%s","browser_download_url":"%s/asset"},{"name":"SHA256SUMS","browser_download_url":"%s/checksum"}]},{"tag_name":"v10.0.0-beta.1","prerelease":true,"assets":[{"name":"%s","browser_download_url":"%s/beta"}]}]`, assetName, serverURL, serverURL, assetName, serverURL)))
		case "/asset":
			_, _ = w.Write(archiveData)
		case "/checksum":
			_, _ = w.Write(checksumBody)
		default:
			stdhttp.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	previousVersion := version
	previousReleaseAPIURL := releaseAPIURL
	previousReleaseAssetName := releaseAssetName
	version = "v1.0.0"
	releaseAPIURL = server.URL + "/releases"
	releaseAssetName = assetName
	t.Cleanup(func() {
		version = previousVersion
		releaseAPIURL = previousReleaseAPIURL
		releaseAssetName = previousReleaseAssetName
	})

	result := runCLI(t, projectDir, homeDir, "update", "self")

	if result.err != nil {
		t.Fatalf("Run() error = %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}
	assertContains(t, result.stdout, "Version stable détectée : v9.9.9")
	assertContains(t, result.stdout, "Mise à jour appliquée")

	data, err := os.ReadFile(executablePath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", executablePath, err)
	}
	assertContains(t, string(data), "new-binary")
}

func executableFileName() string {
	if runtime.GOOS == "windows" {
		return "loreskills.exe"
	}
	return "loreskills"
}

func releaseArchiveForTest(t *testing.T, assetName string) []byte {
	t.Helper()
	binaryName := executableFileName()
	if strings.HasSuffix(assetName, ".zip") {
		var buffer bytes.Buffer
		zipWriter := zip.NewWriter(&buffer)
		writer, err := zipWriter.Create(binaryName)
		if err != nil {
			t.Fatalf("Create(zip entry): %v", err)
		}
		if _, err := writer.Write([]byte("new-binary")); err != nil {
			t.Fatalf("Write(zip entry): %v", err)
		}
		if err := zipWriter.Close(); err != nil {
			t.Fatalf("Close(zip writer): %v", err)
		}
		return buffer.Bytes()
	}

	var tarBuffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&tarBuffer)
	tarWriter := tar.NewWriter(gzipWriter)
	data := []byte("new-binary")
	header := &tar.Header{Name: binaryName, Mode: 0o755, Size: int64(len(data))}
	if err := tarWriter.WriteHeader(header); err != nil {
		t.Fatalf("WriteHeader(tar): %v", err)
	}
	if _, err := tarWriter.Write(data); err != nil {
		t.Fatalf("Write(tar): %v", err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("Close(tar writer): %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("Close(gzip writer): %v", err)
	}
	return tarBuffer.Bytes()
}
