package cmd

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

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

func TestRun_updateRefreshesTrackedBundleAndRemovesObsoleteFiles(t *testing.T) {
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/git/trees/main":
			_, _ = w.Write([]byte(`{"tree":[{"path":"react/form-wizard/SKILL.md","type":"blob"},{"path":"react/form-wizard/templates/prompt.md","type":"blob"}]}`))
		case "/owner/repo/main/react/form-wizard/SKILL.md":
			_, _ = w.Write([]byte("# Refreshed Bundle\n"))
		case "/owner/repo/main/react/form-wizard/templates/prompt.md":
			_, _ = w.Write([]byte("Updated prompt\n"))
		default:
			stdhttp.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("LORESKILLS_GITHUB_RAW_BASE_URL", server.URL)
	t.Setenv("LORESKILLS_GITHUB_API_BASE_URL", server.URL)

	writeProjectProfileForCommandTest(t, projectDir, fs.ProjectProfile{
		Provider:     "github",
		RepositoryID: "owner/repo",
		Ref:          "main",
		CatalogPath:  "SKILLS.md",
		TargetDir:    ".claude/resources",
	})
	writeInstalledStateForCommandTest(t, projectDir, ".claude/resources", fs.InstalledResourceState{Entries: []fs.InstalledResourceRecord{{
		CatalogID:     fs.CatalogID("bundle", "react", "form-wizard"),
		ResourceType:  "bundle",
		Name:          "form-wizard",
		FolderName:    "react",
		InstalledPath: ".claude/resources/react/form-wizard",
		SourcePath:    "react/form-wizard/SKILL.md",
		ManagedRoot:   ".claude/resources/react/form-wizard",
		SourceRoot:    "react/form-wizard",
		ManagedFiles: []fs.ManagedFileRecord{
			{RelativePath: "SKILL.md", InstalledPath: ".claude/resources/react/form-wizard/SKILL.md", SourcePath: "react/form-wizard/SKILL.md"},
			{RelativePath: "templates/obsolete.md", InstalledPath: ".claude/resources/react/form-wizard/templates/obsolete.md", SourcePath: "react/form-wizard/templates/obsolete.md"},
		},
		Status: "installed",
	}}})

	if err := os.MkdirAll(filepath.Join(projectDir, ".claude", "resources", "react", "form-wizard", "templates"), 0o755); err != nil {
		t.Fatalf("MkdirAll(bundle root): %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".claude", "resources", "react", "form-wizard", "SKILL.md"), []byte("stale"), 0o644); err != nil {
		t.Fatalf("WriteFile(SKILL.md): %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".claude", "resources", "react", "form-wizard", "templates", "obsolete.md"), []byte("obsolete"), 0o644); err != nil {
		t.Fatalf("WriteFile(obsolete.md): %v", err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, ".claude", "resources", "custom"), 0o755); err != nil {
		t.Fatalf("MkdirAll(custom): %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".claude", "resources", "custom", "keep.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatalf("WriteFile(keep.txt): %v", err)
	}

	result := runCLI(t, projectDir, t.TempDir(), "update")

	if result.err != nil {
		t.Fatalf("Run() error = %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}
	assertContains(t, result.stdout, "Ressource 'form-wizard' mise à jour")

	skillData, err := os.ReadFile(filepath.Join(projectDir, ".claude", "resources", "react", "form-wizard", "SKILL.md"))
	if err != nil {
		t.Fatalf("ReadFile(SKILL.md): %v", err)
	}
	assertContains(t, string(skillData), "Refreshed Bundle")
	if _, err := os.Stat(filepath.Join(projectDir, ".claude", "resources", "react", "form-wizard", "templates", "prompt.md")); err != nil {
		t.Fatalf("Stat(prompt.md): %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectDir, ".claude", "resources", "react", "form-wizard", "templates", "obsolete.md")); !os.IsNotExist(err) {
		t.Fatalf("obsolete managed file still exists, err=%v", err)
	}
	keepData, err := os.ReadFile(filepath.Join(projectDir, ".claude", "resources", "custom", "keep.txt"))
	if err != nil {
		t.Fatalf("ReadFile(keep.txt): %v", err)
	}
	if string(keepData) != "keep" {
		t.Fatalf("keep.txt = %q, want keep", keepData)
	}

	statePath := filepath.Join(projectDir, ".claude", "resources", "lore-state.json")
	stateData, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", statePath, err)
	}
	var state fs.InstalledResourceState
	if err := json.Unmarshal(stateData, &state); err != nil {
		t.Fatalf("json.Unmarshal(): %v", err)
	}
	if len(state.Entries) != 1 || len(state.Entries[0].ManagedFiles) != 2 {
		t.Fatalf("state after update = %+v, want one bundle entry with two managed files", state.Entries)
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

// T036
func TestRunUpdate_SetsPreviousVersion(t *testing.T) {
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
	writeInstalledStateForCommandTest(t, projectDir, ".claude/resources", fs.InstalledResourceState{
		Entries: []fs.InstalledResourceRecord{
			{CatalogID: fs.CatalogID("skill", "skills", "alpha"), ResourceType: "skill", Name: "alpha", FolderName: "skills", InstalledPath: ".claude/resources/skills/alpha.md", SourcePath: "skills/alpha.md", Status: "installed"},
		},
	})

	// Pre-write v2 state with resolved_version = "old-sha" so update can set previous_version.
	catalogID := fs.CatalogID("skill", "skills", "alpha")
	targetPath := filepath.Join(projectDir, ".claude", "resources")
	v2State := fs.InstalledResourceStateV2{
		SchemaVersion: 2,
		Entries: []fs.InstalledResourceRecordV2{
			{
				ResourceName:    catalogID,
				ProviderID:      "default",
				ResolvedVersion: "old-sha",
				ManagedFiles:    []fs.ManagedFile{},
			},
		},
	}
	v2Data, err := json.Marshal(v2State)
	if err != nil {
		t.Fatalf("json.Marshal(v2State): %v", err)
	}
	if err := os.WriteFile(filepath.Join(targetPath, "lore-state-v2.json"), v2Data, 0o644); err != nil {
		t.Fatalf("WriteFile(lore-state-v2.json): %v", err)
	}

	result := runCLI(t, projectDir, t.TempDir(), "update")
	if result.err != nil {
		t.Fatalf("Run() error = %v\nstderr=%s", result.err, result.stderr)
	}

	// Read v2 state and assert previous_version was set.
	v2Data2, err := os.ReadFile(filepath.Join(targetPath, "lore-state-v2.json"))
	if err != nil {
		t.Fatalf("ReadFile(lore-state-v2.json): %v", err)
	}
	var resultState fs.InstalledResourceStateV2
	if err := json.Unmarshal(v2Data2, &resultState); err != nil {
		t.Fatalf("json.Unmarshal(v2 state): %v", err)
	}
	if len(resultState.Entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(resultState.Entries))
	}
	if got := resultState.Entries[0].PreviousVersion; got != "old-sha" {
		t.Fatalf("PreviousVersion = %q, want %q", got, "old-sha")
	}
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
