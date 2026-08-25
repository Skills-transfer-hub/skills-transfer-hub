package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"sth/internal/cloud"
)

const (
	bridgePublicationID = "11111111-1111-4111-8111-111111111111"
	bridgeVersionID     = "22222222-2222-4222-8222-222222222222"
)

type bridgePublicationFixture struct {
	server      *httptest.Server
	requests    atomic.Int64
	apiRequests atomic.Int64
	quota       atomic.Bool
	mu          sync.Mutex
	manifest    cloud.PublicationManifestInput
	uploaded    map[string][]byte
	errors      []string
}

func newBridgePublicationFixture(t *testing.T) *bridgePublicationFixture {
	t.Helper()
	fixture := &bridgePublicationFixture{uploaded: map[string][]byte{}}
	fixture.server = httptest.NewServer(http.HandlerFunc(fixture.serveHTTP))
	t.Cleanup(fixture.server.Close)
	t.Setenv("STH_CLOUD_BASE_URL", fixture.server.URL)
	t.Setenv("STH_CLOUD_ACCESS_TOKEN", bridgeTestAccessToken)
	return fixture
}

func (fixture *bridgePublicationFixture) serveHTTP(writer http.ResponseWriter, request *http.Request) {
	fixture.requests.Add(1)
	if strings.HasPrefix(request.URL.Path, "/v2/") {
		fixture.apiRequests.Add(1)
		if request.Header.Get("Authorization") != "Bearer "+bridgeTestAccessToken {
			fixture.recordError("API publication sans token court exact")
		}
		if request.Header.Get("X-STH-Client") != "" {
			fixture.recordError("API publication contient le header MCP")
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	switch {
	case request.Method == http.MethodPost && request.URL.Path == "/v2/publications":
		if fixture.quota.Load() {
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(writer, `{"error":"quota_exceeded","detail":"hosted package quota reached"}`)
			return
		}
		var payload struct {
			Manifest cloud.PublicationManifestInput `json:"manifest"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			http.Error(writer, "bad manifest", http.StatusBadRequest)
			return
		}
		fixture.mu.Lock()
		fixture.manifest = payload.Manifest
		fixture.mu.Unlock()
		writeBridgeCloudJSON(writer, map[string]any{
			"id": bridgePublicationID, "status": "draft", "asset_id": bridgePublicationAssetID(payload.Manifest),
			"version_id": nil, "manifest": payload.Manifest, "created_at": now, "finalized_at": nil,
		})
	case request.Method == http.MethodPost && request.URL.Path == "/v2/publications/"+bridgePublicationID+"/uploads":
		manifest := fixture.currentManifest()
		uploads := make([]map[string]any, 0, len(manifest.Files))
		for _, file := range manifest.Files {
			uploads = append(uploads, map[string]any{
				"path": file.Path, "sha256": file.SHA256,
				"upload_url": fixture.server.URL + "/signed-upload?path=" + url.QueryEscape(file.Path),
				"expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
			})
		}
		writeBridgeCloudJSON(writer, map[string]any{"uploads": uploads})
	case request.Method == http.MethodPut && request.URL.Path == "/signed-upload":
		if request.Header.Get("Authorization") != "" {
			fixture.recordError("token Bearer envoyé au stockage signé")
		}
		logicalPath := request.URL.Query().Get("path")
		file, ok := fixture.publicationFile(logicalPath)
		if !ok {
			http.Error(writer, "unknown upload", http.StatusBadRequest)
			return
		}
		if request.Header.Get("Content-Type") != file.MIME {
			fixture.recordError(fmt.Sprintf("MIME %s invalide", logicalPath))
		}
		content, _ := io.ReadAll(io.LimitReader(request.Body, bridgePublicationMaxFileBytes+1))
		fixture.mu.Lock()
		fixture.uploaded[logicalPath] = content
		fixture.mu.Unlock()
		writer.WriteHeader(http.StatusNoContent)
	case request.Method == http.MethodPost && request.URL.Path == "/v2/publications/"+bridgePublicationID+"/finalize":
		manifest := fixture.currentManifest()
		writeBridgeCloudJSON(writer, map[string]any{
			"id": bridgePublicationID, "status": "published", "asset_id": bridgePublicationAssetID(manifest),
			"version_id": bridgeVersionID, "manifest": manifest, "created_at": now, "finalized_at": now,
		})
	default:
		http.NotFound(writer, request)
	}
}

func (fixture *bridgePublicationFixture) currentManifest() cloud.PublicationManifestInput {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	return fixture.manifest
}

func (fixture *bridgePublicationFixture) publicationFile(logicalPath string) (cloud.PublicationFile, bool) {
	manifest := fixture.currentManifest()
	for _, file := range manifest.Files {
		if file.Path == logicalPath {
			return file, true
		}
	}
	return cloud.PublicationFile{}, false
}

func (fixture *bridgePublicationFixture) recordError(message string) {
	fixture.mu.Lock()
	fixture.errors = append(fixture.errors, message)
	fixture.mu.Unlock()
}

func TestBridgeCloudPublicationUsesExactHEADAndReturnsConcreteAsset(t *testing.T) {
	fixture := newBridgePublicationFixture(t)
	repository := newBridgePublicationRepository(t, "git@github.com:org/repo.git")
	committedSkill := []byte("---\nname: Review PR\ndescription: Review pull requests safely\n---\n# Committed\n")
	writeBridgePublicationFile(t, repository, "Shared Skills/review-pr/SKILL.md", committedSkill)
	writeBridgePublicationFile(t, repository, "Shared Skills/review-pr/references/checklist.md", []byte("# Checklist\n"))
	bridgeTestGit(t, repository, "add", "Shared Skills/review-pr")
	bridgeTestGit(t, repository, "commit", "-m", "add skill")
	writeBridgePublicationFile(t, repository, "Shared Skills/review-pr/SKILL.md", []byte("# DIRTY CONTENT MUST NEVER LEAVE\n"))
	statusBefore := bridgeTestGitOutput(t, repository, "status", "--porcelain=v1")

	previewInput := `{"jsonrpc":"2.0","id":1,"method":"cloud.publish.preview","params":{"source_path":"Shared Skills/review-pr","folder_name":"skills","version":"1.0.0"}}` + "\n"
	previewResult := runCLIWithInput(t, repository, t.TempDir(), previewInput, "bridge", "--stdio")
	if previewResult.err != nil {
		t.Fatalf("preview: %v\nstderr=%s", previewResult.err, previewResult.stderr)
	}
	if fixture.requests.Load() != 0 {
		t.Fatalf("preview touched network: %d requests", fixture.requests.Load())
	}
	preview := decodeBridgeTestResponses(t, previewResult.stdout)[0]["result"].(map[string]any)
	commit := preview["commit"].(string)
	manifestSHA := preview["manifest_sha256"].(string)
	if !validBridgeGitOID(commit) || len(manifestSHA) != 64 || len(preview["changes"].([]any)) != 2 || len(preview["warnings"].([]any)) != 1 {
		t.Fatalf("preview = %v", preview)
	}

	publishInput := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"cloud.publish","params":{"source_path":"Shared Skills/review-pr","folder_name":"skills","version":"1.0.0","expected_commit":%q,"expected_manifest_sha256":%q,"confirmed":true}}`, commit, manifestSHA) + "\n"
	publishResult := runCLIWithInput(t, repository, t.TempDir(), publishInput, "bridge", "--stdio")
	if publishResult.err != nil {
		t.Fatalf("publish: %v\nstderr=%s", publishResult.err, publishResult.stderr)
	}
	response := decodeBridgeTestResponses(t, publishResult.stdout)[0]["result"].(map[string]any)
	if len(response) != 5 || response["published"] != true || response["asset_id"] != "skill::skills::review-pr" || response["version_id"] != bridgeVersionID || response["commit"] != commit || response["manifest_sha256"] != manifestSHA {
		t.Fatalf("publish result = %v", response)
	}
	manifest := fixture.currentManifest()
	if manifest.Origin.RepositoryURL != "ssh://git@github.com/org/repo.git" || manifest.Origin.Commit != commit || manifest.Entrypoint != "SKILL.md" || manifest.Asset.Name != "Review PR" || manifest.Asset.Description == nil || *manifest.Asset.Description != "Review pull requests safely" {
		t.Fatalf("manifest = %+v", manifest)
	}
	if len(manifest.Files) != 2 || manifest.Files[0].Path != "SKILL.md" || manifest.Files[1].Path != "references/checklist.md" {
		t.Fatalf("manifest files = %+v", manifest.Files)
	}
	for _, targetKind := range []string{"antigravity", "claude", "codex", "copilot", "gemini"} {
		if manifest.Compatibility[targetKind] != "supported" {
			t.Fatalf("manifest compatibility = %+v, missing %s", manifest.Compatibility, targetKind)
		}
	}
	fixture.mu.Lock()
	uploadedSkill := append([]byte(nil), fixture.uploaded["SKILL.md"]...)
	errors := append([]string(nil), fixture.errors...)
	fixture.mu.Unlock()
	if string(uploadedSkill) != string(committedSkill) {
		t.Fatalf("uploaded bytes are not HEAD: %q", uploadedSkill)
	}
	if len(errors) != 0 {
		t.Fatalf("server validation errors = %v", errors)
	}
	if statusAfter := bridgeTestGitOutput(t, repository, "status", "--porcelain=v1"); statusAfter != statusBefore {
		t.Fatalf("publication mutated worktree:\nbefore=%q\nafter=%q", statusBefore, statusAfter)
	}
}

func TestBridgeCloudPublicationRejectsHeadTOCTOUBeforeServerMutation(t *testing.T) {
	fixture := newBridgePublicationFixture(t)
	repository := newBridgePublicationRepository(t, "https://github.com/org/repo.git")
	writeBridgePublicationFile(t, repository, "skills/review-pr/SKILL.md", []byte("# Version one\n"))
	bridgeTestGit(t, repository, "add", ".")
	bridgeTestGit(t, repository, "commit", "-m", "v1")
	preview := bridgePublicationPreviewForTest(t, repository)
	writeBridgePublicationFile(t, repository, "skills/review-pr/SKILL.md", []byte("# Version two\n"))
	bridgeTestGit(t, repository, "add", ".")
	bridgeTestGit(t, repository, "commit", "-m", "v2")

	publishInput := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"cloud.publish","params":{"source_path":"skills/review-pr","folder_name":"skills","version":"1.0.0","expected_commit":%q,"expected_manifest_sha256":%q,"confirmed":true}}`, preview["commit"], preview["manifest_sha256"]) + "\n"
	result := runCLIWithInput(t, repository, t.TempDir(), publishInput, "bridge", "--stdio")
	if result.err != nil {
		t.Fatal(result.err)
	}
	errorValue := decodeBridgeTestResponses(t, result.stdout)[0]["error"].(map[string]any)
	if errorValue["code"] != float64(-32033) || !strings.Contains(errorValue["message"].(string), "changed") {
		t.Fatalf("error = %v", errorValue)
	}
	if fixture.requests.Load() != 0 {
		t.Fatalf("TOCTOU rejection mutated server: %d requests", fixture.requests.Load())
	}
}

func TestBridgeCloudPublicationSurfacesQuotaWithoutUploading(t *testing.T) {
	fixture := newBridgePublicationFixture(t)
	fixture.quota.Store(true)
	repository := newBridgePublicationRepository(t, "https://github.com/org/repo.git")
	writeBridgePublicationFile(t, repository, "skills/review-pr/SKILL.md", []byte("# Review\n"))
	bridgeTestGit(t, repository, "add", ".")
	bridgeTestGit(t, repository, "commit", "-m", "skill")
	preview := bridgePublicationPreviewForTest(t, repository)

	publishInput := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"cloud.publish","params":{"source_path":"skills/review-pr","folder_name":"skills","version":"1.0.0","expected_commit":%q,"expected_manifest_sha256":%q,"confirmed":true}}`, preview["commit"], preview["manifest_sha256"]) + "\n"
	result := runCLIWithInput(t, repository, t.TempDir(), publishInput, "bridge", "--stdio")
	if result.err != nil {
		t.Fatal(result.err)
	}
	errorValue := decodeBridgeTestResponses(t, result.stdout)[0]["error"].(map[string]any)
	if !strings.Contains(errorValue["message"].(string), "hosted package quota reached") {
		t.Fatalf("error = %v", errorValue)
	}
	if fixture.apiRequests.Load() != 1 || fixture.requests.Load() != 1 {
		t.Fatalf("quota path continued after create: api=%d all=%d", fixture.apiRequests.Load(), fixture.requests.Load())
	}
}

func TestBridgeCloudPublicationRejectsSymlinkFromHEAD(t *testing.T) {
	repository := newBridgePublicationRepository(t, "https://github.com/org/repo.git")
	writeBridgePublicationFile(t, repository, "skills/review-pr/SKILL.md", []byte("# Review\n"))
	if err := os.Symlink("../../outside", filepath.Join(repository, "skills", "review-pr", "link")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	bridgeTestGit(t, repository, "add", ".")
	bridgeTestGit(t, repository, "commit", "-m", "malicious")
	result := runCLIWithInput(t, repository, t.TempDir(), `{"jsonrpc":"2.0","id":1,"method":"cloud.publish.preview","params":{"source_path":"skills/review-pr","folder_name":"skills","version":"1.0.0"}}`+"\n", "bridge", "--stdio")
	if result.err != nil {
		t.Fatal(result.err)
	}
	errorValue := decodeBridgeTestResponses(t, result.stdout)[0]["error"].(map[string]any)
	if !strings.Contains(errorValue["message"].(string), "symbolic link") {
		t.Fatalf("error = %v", errorValue)
	}
}

func TestBridgeCloudPublicationRejectsUnsafeSourcePaths(t *testing.T) {
	for _, sourcePath := range []string{"skills/.git/review-pr", "skills/.Git/review-pr"} {
		t.Run(sourcePath, func(t *testing.T) {
			if _, err := normalizeBridgePublicationSourcePath(sourcePath); err == nil {
				t.Fatalf("normalizeBridgePublicationSourcePath(%q) error = nil", sourcePath)
			}
		})
	}
}

func TestBridgeGitEnvironmentDropsCallerGitOverrides(t *testing.T) {
	t.Setenv("GIT_CONFIG_PARAMETERS", "'remote.origin.url=https://attacker.invalid/repo'")
	t.Setenv("GIT_CONFIG", "/tmp/attacker-config")
	t.Setenv("GIT_EXEC_PATH", "/tmp/attacker-bin")
	t.Setenv("GIT_DIR", "/tmp/attacker-repo")
	environment := bridgeGitEnvironment()
	joined := "\n" + strings.Join(environment, "\n") + "\n"
	for _, forbidden := range []string{"GIT_CONFIG_PARAMETERS=", "GIT_CONFIG=/tmp", "GIT_EXEC_PATH=", "GIT_DIR="} {
		if strings.Contains(joined, "\n"+forbidden) {
			t.Fatalf("caller Git override survived: %s", forbidden)
		}
	}
	for _, required := range []string{"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + os.DevNull, "GIT_NO_REPLACE_OBJECTS=1", "GIT_OPTIONAL_LOCKS=0"} {
		if !strings.Contains(joined, "\n"+required+"\n") {
			t.Fatalf("safe Git environment missing %s", required)
		}
	}
}

func bridgePublicationPreviewForTest(t *testing.T, repository string) map[string]any {
	t.Helper()
	result := runCLIWithInput(t, repository, t.TempDir(), `{"jsonrpc":"2.0","id":1,"method":"cloud.publish.preview","params":{"source_path":"skills/review-pr","folder_name":"skills","version":"1.0.0"}}`+"\n", "bridge", "--stdio")
	if result.err != nil {
		t.Fatalf("preview: %v\nstderr=%s", result.err, result.stderr)
	}
	return decodeBridgeTestResponses(t, result.stdout)[0]["result"].(map[string]any)
}

func newBridgePublicationRepository(t *testing.T, remote string) string {
	t.Helper()
	repository := t.TempDir()
	bridgeTestGit(t, repository, "init")
	bridgeTestGit(t, repository, "config", "user.name", "STH Test")
	bridgeTestGit(t, repository, "config", "user.email", "sth@example.test")
	bridgeTestGit(t, repository, "remote", "add", "origin", remote)
	return repository
}

func writeBridgePublicationFile(t *testing.T, repository, relativePath string, content []byte) {
	t.Helper()
	filePath := filepath.Join(repository, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, content, 0o644); err != nil {
		t.Fatal(err)
	}
}

func bridgeTestGit(t *testing.T, repository string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = repository
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", arguments, err, output)
	}
}

func bridgeTestGitOutput(t *testing.T, repository string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = repository
	output, err := command.Output()
	if err != nil {
		t.Fatalf("git %v: %v", arguments, err)
	}
	return string(output)
}

func bridgePublicationAssetID(manifest cloud.PublicationManifestInput) string {
	return manifest.Asset.Kind + "::" + manifest.Asset.FolderName + "::" + manifest.Asset.Slug
}

func TestBridgePublicationMetadataUsesParsedYAMLDescription(t *testing.T) {
	content := []byte("---\nname: review-pr\ndescription: >-\n  Review pull requests\n  safely.\n---\n# Review\n")
	name, description := bridgePublicationMetadata(content, "fallback")
	if name != "review-pr" || description == nil || *description != "Review pull requests safely." {
		t.Fatalf("name = %q, description = %v", name, description)
	}
}
