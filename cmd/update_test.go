package cmd

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	stdhttp "net/http"
	"net/http/httptest"
	"sth/internal/fs"
	"sth/internal/writer"
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
	if !strings.Contains(result.stdout, "No resources to update.") {
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
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)

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
	if !strings.Contains(result.stdout, "Resource 'alpha' updated") {
		t.Fatalf("stdout %q does not contain alpha update", result.stdout)
	}
	if !strings.Contains(result.stdout, "Resource 'beta' updated") {
		t.Fatalf("stdout %q does not contain beta update", result.stdout)
	}
	assertContains(t, result.stderr, "[1/3] Preparing update")
	assertContains(t, result.stderr, "[2/3] Updating agent::agents::beta")
	assertContains(t, result.stderr, "[3/3] Updating skill::skills::alpha")

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
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)

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
	if !strings.Contains(result.stdout, "Resource 'alpha' updated") {
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
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)
	t.Setenv("STH_GITHUB_API_BASE_URL", server.URL)

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
	assertContains(t, result.stdout, "Resource 'form-wizard' updated")

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

	statePath := filepath.Join(projectDir, ".claude", "resources", "sth-state.json")
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
	t.Setenv("STH_EXECUTABLE_PATH", executablePath)

	assetName := defaultReleaseAssetName()
	archiveData := releaseArchiveForTest(t, assetName)
	manifestData := []byte("{\"schema_version\":1,\"engine_version\":\"9.9.9\",\"bridge_protocol\":1,\"contract\":\"sth-code-bridge-v1\"}\n")
	archiveChecksum := sha256.Sum256(archiveData)
	manifestChecksum := sha256.Sum256(manifestData)
	checksumBody := []byte(fmt.Sprintf("%s  %s\n%s  %s\n", hex.EncodeToString(archiveChecksum[:]), assetName, hex.EncodeToString(manifestChecksum[:]), fs.ReleaseEngineManifestName))

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey: %v", err)
	}
	t.Setenv("STH_RELEASE_SIGNING_PUBKEY", base64.StdEncoding.EncodeToString(publicKey))
	signatureBody := []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, checksumBody)) + "\n")

	serverURL := ""
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		switch r.URL.Path {
		case "/releases":
			_, _ = w.Write([]byte(fmt.Sprintf(`[{"tag_name":"v9.9.9","prerelease":false,"assets":[{"name":"%s","browser_download_url":"%s/asset"},{"name":"SHA256SUMS","browser_download_url":"%s/checksum"},{"name":"SHA256SUMS.sig","browser_download_url":"%s/signature"},{"name":"sth-code-engine.json","browser_download_url":"%s/manifest"}]},{"tag_name":"v10.0.0-beta.1","prerelease":true,"assets":[{"name":"%s","browser_download_url":"%s/beta"}]}]`, assetName, serverURL, serverURL, serverURL, serverURL, assetName, serverURL)))
		case "/asset":
			_, _ = w.Write(archiveData)
		case "/checksum":
			_, _ = w.Write(checksumBody)
		case "/signature":
			_, _ = w.Write(signatureBody)
		case "/manifest":
			_, _ = w.Write(manifestData)
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
	assertContains(t, result.stdout, "Stable version available: Alpha v9.9.9")
	assertContains(t, result.stdout, "Update applied")
	assertContains(t, result.stderr, "[1/4] Downloading binary")
	assertContains(t, result.stderr, "[2/4] Downloading checksum")
	assertContains(t, result.stderr, "[3/4] Downloading signature")
	assertContains(t, result.stderr, "[4/4] Downloading signed release manifest")

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
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)

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
	if err := os.WriteFile(filepath.Join(targetPath, "sth-state-v2.json"), v2Data, 0o644); err != nil {
		t.Fatalf("WriteFile(sth-state-v2.json): %v", err)
	}

	result := runCLI(t, projectDir, t.TempDir(), "update")
	if result.err != nil {
		t.Fatalf("Run() error = %v\nstderr=%s", result.err, result.stderr)
	}

	// Read v2 state and assert previous_version was set.
	v2Data2, err := os.ReadFile(filepath.Join(targetPath, "sth-state-v2.json"))
	if err != nil {
		t.Fatalf("ReadFile(sth-state-v2.json): %v", err)
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
		return "sth.exe"
	}
	return "sth"
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

func TestRunUpdate_dryRunTextGroupsPerResource(t *testing.T) {
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		switch r.URL.Path {
		case "/owner/repo/main/skills/alpha.md":
			_, _ = w.Write([]byte("# Alpha updated\n"))
		case "/owner/repo/main/agents/beta.md":
			_, _ = w.Write([]byte("# Beta updated\n"))
		default:
			stdhttp.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)

	writeProjectProfileForCommandTest(t, projectDir, fs.ProjectProfile{
		Provider:     "github",
		RepositoryID: "owner/repo",
		Ref:          "main",
		CatalogPath:  "SKILLS.md",
		TargetDir:    ".claude/resources",
	})

	targetPath := filepath.Join(projectDir, ".claude", "resources")
	if err := os.MkdirAll(targetPath, 0o755); err != nil {
		t.Fatal(err)
	}
	v2State := fs.InstalledResourceStateV2{
		SchemaVersion: 2,
		Entries: []fs.InstalledResourceRecordV2{
			{ResourceName: "skills/alpha.md", ProviderID: "default", ResolvedVersion: "old", ManagedFiles: []fs.ManagedFile{{Path: ".claude/resources/skills/alpha.md", SHA256: "old-sha"}}},
			{ResourceName: "agents/beta.md", ProviderID: "default", ResolvedVersion: "old", ManagedFiles: []fs.ManagedFile{{Path: ".claude/resources/agents/beta.md", SHA256: "old-sha"}}},
		},
	}
	v2Data, _ := json.Marshal(v2State)
	if err := os.WriteFile(filepath.Join(targetPath, "sth-state-v2.json"), v2Data, 0o644); err != nil {
		t.Fatal(err)
	}

	result := runCLI(t, projectDir, t.TempDir(), "update", "--dry-run")
	if result.err != nil {
		t.Fatalf("Run() error = %v\nstderr=%s", result.err, result.stderr)
	}
	assertContains(t, result.stdout, "skills/alpha.md")
	assertContains(t, result.stdout, "agents/beta.md")
	assertContains(t, result.stdout, "[~]")
}

func TestRunUpdate_dryRunJSONIncludesResourceName(t *testing.T) {
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		switch r.URL.Path {
		case "/owner/repo/main/skills/alpha.md":
			_, _ = w.Write([]byte("# Alpha updated\n"))
		case "/owner/repo/main/agents/beta.md":
			_, _ = w.Write([]byte("# Beta updated\n"))
		default:
			stdhttp.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)

	writeProjectProfileForCommandTest(t, projectDir, fs.ProjectProfile{
		Provider:     "github",
		RepositoryID: "owner/repo",
		Ref:          "main",
		CatalogPath:  "SKILLS.md",
		TargetDir:    ".claude/resources",
	})

	targetPath := filepath.Join(projectDir, ".claude", "resources")
	if err := os.MkdirAll(targetPath, 0o755); err != nil {
		t.Fatal(err)
	}
	v2State := fs.InstalledResourceStateV2{
		SchemaVersion: 2,
		Entries: []fs.InstalledResourceRecordV2{
			{ResourceName: "skills/alpha.md", ProviderID: "default", ResolvedVersion: "old", ManagedFiles: []fs.ManagedFile{{Path: ".claude/resources/skills/alpha.md", SHA256: "old-sha"}}},
			{ResourceName: "agents/beta.md", ProviderID: "default", ResolvedVersion: "old", ManagedFiles: []fs.ManagedFile{{Path: ".claude/resources/agents/beta.md", SHA256: "old-sha"}}},
		},
	}
	v2Data, _ := json.Marshal(v2State)
	if err := os.WriteFile(filepath.Join(targetPath, "sth-state-v2.json"), v2Data, 0o644); err != nil {
		t.Fatal(err)
	}

	result := runCLI(t, projectDir, t.TempDir(), "update", "--dry-run", "--json")
	if result.err != nil {
		t.Fatalf("Run() error = %v\nstderr=%s", result.err, result.stderr)
	}
	var changes []fs.DryRunChange
	if err := json.Unmarshal([]byte(result.stdout), &changes); err != nil {
		t.Fatalf("json.Unmarshal: %v\nstdout=%s", err, result.stdout)
	}
	if len(changes) == 0 {
		t.Fatal("expected non-empty JSON changes array")
	}
	for _, c := range changes {
		if c.ResourceName == "" {
			t.Errorf("change %+v has empty resource_name", c)
		}
	}
}

func TestRunUpdate_dryRunNoChangesWhenAllCurrent(t *testing.T) {
	projectDir := t.TempDir()
	content := []byte("# Alpha\n")
	fullContent := []byte(writer.ClaudeManagedHeader + "# Alpha\n")
	sha := fs.ComputeSHA256(fullContent)

	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if r.URL.Path == "/owner/repo/main/skills/alpha.md" {
			_, _ = w.Write(content)
			return
		}
		stdhttp.NotFound(w, r)
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)

	writeProjectProfileForCommandTest(t, projectDir, fs.ProjectProfile{
		Provider:     "github",
		RepositoryID: "owner/repo",
		Ref:          "main",
		CatalogPath:  "SKILLS.md",
		TargetDir:    ".claude/resources",
	})

	targetPath := filepath.Join(projectDir, ".claude", "resources")
	if err := os.MkdirAll(targetPath, 0o755); err != nil {
		t.Fatal(err)
	}
	v2State := fs.InstalledResourceStateV2{
		SchemaVersion: 2,
		Entries: []fs.InstalledResourceRecordV2{
			{ResourceName: "skills/alpha.md", ProviderID: "default", ResolvedVersion: "current", ManagedFiles: []fs.ManagedFile{{Path: ".claude/resources/skills/alpha.md", SHA256: sha}}},
		},
	}
	v2Data, _ := json.Marshal(v2State)
	if err := os.WriteFile(filepath.Join(targetPath, "sth-state-v2.json"), v2Data, 0o644); err != nil {
		t.Fatal(err)
	}

	result := runCLI(t, projectDir, t.TempDir(), "update", "--dry-run")
	if result.err != nil {
		t.Fatalf("Run() error = %v\nstderr=%s", result.err, result.stderr)
	}
	assertContains(t, result.stdout, "No changes would be made.")
}

func TestRunUpdate_dryRunTextPrefixes(t *testing.T) {
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if r.URL.Path == "/owner/repo/main/skills/alpha.md" {
			_, _ = w.Write([]byte("# Alpha new\n"))
			return
		}
		stdhttp.NotFound(w, r)
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)

	writeProjectProfileForCommandTest(t, projectDir, fs.ProjectProfile{
		Provider:     "github",
		RepositoryID: "owner/repo",
		Ref:          "main",
		CatalogPath:  "SKILLS.md",
		TargetDir:    ".claude/resources",
	})

	targetPath := filepath.Join(projectDir, ".claude", "resources")
	if err := os.MkdirAll(targetPath, 0o755); err != nil {
		t.Fatal(err)
	}
	alphaCatalogID := fs.CatalogID("skill", "skills", "alpha")
	v2State := fs.InstalledResourceStateV2{
		SchemaVersion: 2,
		Entries: []fs.InstalledResourceRecordV2{
			{ResourceName: alphaCatalogID, ProviderID: "default", ResolvedVersion: "old", ManagedFiles: []fs.ManagedFile{{Path: ".claude/resources/skills/alpha.md", SHA256: "old-sha"}}},
		},
	}
	v2Data, _ := json.Marshal(v2State)
	if err := os.WriteFile(filepath.Join(targetPath, "sth-state-v2.json"), v2Data, 0o644); err != nil {
		t.Fatal(err)
	}

	result := runCLI(t, projectDir, t.TempDir(), "update", "--dry-run")
	if result.err != nil {
		t.Fatalf("Run() error = %v\nstderr=%s", result.err, result.stderr)
	}
	assertContains(t, result.stdout, "[~]")
}

// writeV2StateForUpdatePinTest sets up v2 state for pinning update tests.
func writeV2StateForUpdatePinTest(t *testing.T, projectDir, targetDir string, state fs.InstalledResourceStateV2) {
	t.Helper()
	targetPath := filepath.Join(projectDir, filepath.FromSlash(targetDir))
	if err := os.MkdirAll(targetPath, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	data, _ := json.Marshal(state)
	if err := os.WriteFile(filepath.Join(targetPath, "sth-state-v2.json"), data, 0o644); err != nil {
		t.Fatalf("WriteFile(v2 state): %v", err)
	}
}

// TestRunUpdate_pinnedResourceShowsTag verifies a pinned resource is skipped with tag.
func TestRunUpdate_pinnedResourceShowsTag(t *testing.T) {
	projectDir := t.TempDir()
	rawCalled := false
	rawServer := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		rawCalled = true
		stdhttp.NotFound(w, r)
	}))
	defer rawServer.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", rawServer.URL)

	writeProjectProfileForCommandTest(t, projectDir, fs.ProjectProfile{
		Provider:     "github",
		RepositoryID: "owner/repo",
		Ref:          "main",
		CatalogPath:  "SKILLS.md",
		TargetDir:    ".claude/resources",
	})
	alphaCatalogIDPin := fs.CatalogID("skill", "skills", "alpha")
	writeProjectConfigV2ForTest(t, projectDir, fs.ProjectConfigV2{
		SchemaVersion: 2,
		Providers: []fs.ProviderEntry{
			{ID: "default", Provider: "github", RepositoryID: "owner/repo", CatalogRef: "main", CatalogPath: "SKILLS.md", TargetDir: ".claude/resources"},
		},
		Resources: []fs.ResourceEntry{
			{ResourceName: alphaCatalogIDPin, ProviderID: "default", PinnedVersion: "v1.0.0"},
		},
	})
	writeInstalledStateForCommandTest(t, projectDir, ".claude/resources", fs.InstalledResourceState{
		Entries: []fs.InstalledResourceRecord{
			{CatalogID: alphaCatalogIDPin, ResourceType: "skill", Name: "alpha", FolderName: "skills", InstalledPath: ".claude/resources/skills/alpha.md", SourcePath: "skills/alpha.md", Status: "installed"},
		},
	})
	writeV2StateForUpdatePinTest(t, projectDir, ".claude/resources", fs.InstalledResourceStateV2{
		SchemaVersion: 2,
		Entries: []fs.InstalledResourceRecordV2{
			{ResourceName: alphaCatalogIDPin, ProviderID: "default", ResolvedVersion: "old-pinned-sha"},
		},
	})

	result := runCLI(t, projectDir, t.TempDir(), "update")
	if result.err != nil {
		t.Fatalf("Run() error = %v\nstderr=%s", result.err, result.stderr)
	}
	assertContains(t, result.stdout, "[pinned at v1.0.0]")
	if rawCalled {
		t.Error("raw content server was called for pinned resource — should be skipped")
	}

	targetPath := filepath.Join(projectDir, ".claude", "resources")
	v2Data, err := os.ReadFile(filepath.Join(targetPath, "sth-state-v2.json"))
	if err != nil {
		t.Fatalf("ReadFile(v2 state): %v", err)
	}
	var resultState fs.InstalledResourceStateV2
	if err := json.Unmarshal(v2Data, &resultState); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if len(resultState.Entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(resultState.Entries))
	}
	if got := resultState.Entries[0].ResolvedVersion; got != "old-pinned-sha" {
		t.Fatalf("ResolvedVersion = %q, want %q (should be unchanged)", got, "old-pinned-sha")
	}
}

// TestRunUpdate_pinnedNewerVersionNotice verifies "Newer version available" is shown when
// upstream HEAD differs from installed resolved_version for a pinned resource.
func TestRunUpdate_pinnedNewerVersionNotice(t *testing.T) {
	projectDir := t.TempDir()

	latestSHA := "new-head-sha-12345"
	apiServer := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if strings.Contains(r.URL.Path, "/commits") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"sha":"` + latestSHA + `"}]`))
			return
		}
		stdhttp.NotFound(w, r)
	}))
	defer apiServer.Close()
	t.Setenv("STH_GITHUB_API_BASE_URL", apiServer.URL)

	writeProjectProfileForCommandTest(t, projectDir, fs.ProjectProfile{
		Provider:     "github",
		RepositoryID: "owner/repo",
		Ref:          "main",
		CatalogPath:  "SKILLS.md",
		TargetDir:    ".claude/resources",
	})
	alphaCatalogIDNotice := fs.CatalogID("skill", "skills", "alpha")
	writeProjectConfigV2ForTest(t, projectDir, fs.ProjectConfigV2{
		SchemaVersion: 2,
		Providers: []fs.ProviderEntry{
			{ID: "default", Provider: "github", RepositoryID: "owner/repo", CatalogRef: "main", CatalogPath: "SKILLS.md", TargetDir: ".claude/resources"},
		},
		Resources: []fs.ResourceEntry{
			{ResourceName: alphaCatalogIDNotice, ProviderID: "default", PinnedVersion: "v1.0.0"},
		},
	})
	writeInstalledStateForCommandTest(t, projectDir, ".claude/resources", fs.InstalledResourceState{
		Entries: []fs.InstalledResourceRecord{
			{CatalogID: alphaCatalogIDNotice, ResourceType: "skill", Name: "alpha", FolderName: "skills", InstalledPath: ".claude/resources/skills/alpha.md", SourcePath: "skills/alpha.md", Status: "installed"},
		},
	})
	writeV2StateForUpdatePinTest(t, projectDir, ".claude/resources", fs.InstalledResourceStateV2{
		SchemaVersion: 2,
		Entries: []fs.InstalledResourceRecordV2{
			{ResourceName: alphaCatalogIDNotice, ProviderID: "default", ResolvedVersion: "old-pinned-sha"},
		},
	})

	result := runCLI(t, projectDir, t.TempDir(), "update")
	if result.err != nil {
		t.Fatalf("Run() error = %v\nstderr=%s", result.err, result.stderr)
	}
	assertContains(t, result.stdout, "[pinned at v1.0.0]")
	assertContains(t, result.stdout, "Newer version available")
	assertContains(t, result.stdout, alphaCatalogIDNotice)
}

// TestRunUpdate_ownershipConflictSkipsResourceContinuesOthers verifies that when
// a resource's path is already owned by a different resource in v2 state, the
// update for the conflicting resource is skipped but continues for other resources.
func TestRunUpdate_ownershipConflictSkipsResourceContinuesOthers(t *testing.T) {
	projectDir := t.TempDir()

	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		switch r.URL.Path {
		case "/owner/repo/main/skills/alpha.md":
			_, _ = w.Write([]byte("# Alpha updated\n"))
		case "/owner/repo/main/agents/beta.md":
			_, _ = w.Write([]byte("# Beta updated\n"))
		default:
			stdhttp.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)

	writeProjectProfileForCommandTest(t, projectDir, fs.ProjectProfile{
		Provider:     "github",
		RepositoryID: "owner/repo",
		Ref:          "main",
		CatalogPath:  "SKILLS.md",
		TargetDir:    ".claude/resources",
	})

	writeInstalledStateForCommandTest(t, projectDir, ".claude/resources", fs.InstalledResourceState{
		Entries: []fs.InstalledResourceRecord{
			{CatalogID: fs.CatalogID("skill", "skills", "alpha"), ResourceType: "skill", Name: "alpha", FolderName: "skills",
				InstalledPath: ".claude/resources/skills/alpha.md", SourcePath: "skills/alpha.md", Status: "installed"},
			{CatalogID: fs.CatalogID("skill", "agents", "beta"), ResourceType: "skill", Name: "beta", FolderName: "agents",
				InstalledPath: ".claude/resources/agents/beta.md", SourcePath: "agents/beta.md", Status: "installed"},
		},
	})

	// gamma (different resource) owns alpha's path → conflict for alpha; beta is uncontested.
	writeV2StateForUpdatePinTest(t, projectDir, ".claude/resources", fs.InstalledResourceStateV2{
		SchemaVersion: 2,
		Entries: []fs.InstalledResourceRecordV2{
			{
				ResourceName: fs.CatalogID("skill", "skills", "gamma"),
				ProviderID:   "default",
				ManagedFiles: []fs.ManagedFile{{Path: ".claude/resources/skills/alpha.md", SHA256: "abc"}},
			},
			{
				ResourceName: fs.CatalogID("skill", "agents", "beta"),
				ProviderID:   "default",
				ManagedFiles: []fs.ManagedFile{{Path: ".claude/resources/agents/beta.md", SHA256: "def"}},
			},
		},
	})

	betaDir := filepath.Join(projectDir, ".claude", "resources", "agents")
	if err := os.MkdirAll(betaDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	betaPath := filepath.Join(betaDir, "beta.md")
	if err := os.WriteFile(betaPath, []byte("# Beta old\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(beta old): %v", err)
	}

	result := runCLI(t, projectDir, t.TempDir(), "update")

	if !strings.Contains(result.stderr, "conflict") && !strings.Contains(result.stderr, "Conflit") {
		t.Fatalf("stderr %q does not contain conflict message for alpha", result.stderr)
	}

	betaData, err := os.ReadFile(betaPath)
	if err != nil {
		t.Fatalf("ReadFile(beta): %v", err)
	}
	if !strings.Contains(string(betaData), "Beta updated") {
		t.Fatalf("beta.md was not updated; content=%q", string(betaData))
	}

	if result.err == nil {
		t.Fatal("expected non-zero exit due to alpha conflict, got nil error")
	}
}
