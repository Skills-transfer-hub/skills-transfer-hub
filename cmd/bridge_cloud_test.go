package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"sth/internal/fs"
	"sth/internal/writer"
)

const bridgeTestAccessToken = "sth_at_abcdefghijklmnopqrstuvwxyzABCDEFGH012345678"

type bridgeCloudFixture struct {
	server   *httptest.Server
	latest   atomic.Value
	requests atomic.Int64
	corrupt  atomic.Bool
	files    map[string][]byte
}

func newBridgeCloudFixture(t *testing.T) *bridgeCloudFixture {
	t.Helper()
	fixture := &bridgeCloudFixture{files: map[string][]byte{
		"SKILL.md":                []byte("---\nname: review-pr\ndescription: Review pull requests\n---\n# Review PR\n"),
		"references/checklist.md": []byte("# Checklist\n"),
	}}
	fixture.latest.Store("version-1")
	fixture.server = httptest.NewServer(http.HandlerFunc(fixture.serveHTTP))
	t.Cleanup(fixture.server.Close)
	t.Setenv("STH_CLOUD_BASE_URL", fixture.server.URL)
	t.Setenv("STH_CLOUD_ACCESS_TOKEN", bridgeTestAccessToken)
	return fixture
}

func (fixture *bridgeCloudFixture) serveHTTP(writer http.ResponseWriter, request *http.Request) {
	fixture.requests.Add(1)
	if got := request.Header.Get("X-STH-Client"); got != "" {
		http.Error(writer, "MCP header forbidden", http.StatusBadRequest)
		return
	}
	if strings.HasPrefix(request.URL.Path, "/v2/") && request.Header.Get("Authorization") != "Bearer "+bridgeTestAccessToken {
		http.Error(writer, "unauthorized", http.StatusUnauthorized)
		return
	}
	assetID := "skill::skills::review-pr"
	versionID := fixture.latest.Load().(string)
	switch {
	case request.Method == http.MethodGet && request.URL.Path == "/v2/assets/"+assetID:
		writeBridgeCloudJSON(writer, map[string]any{
			"id": assetID, "family": "package", "kind": "skill", "name": "../../display-name-is-not-a-path",
			"description": "Review a pull request", "status": "available",
			"source_path": "skills/review-pr/SKILL.md", "latest_version_id": versionID, "latest_version": "1.0.0",
		})
	case request.Method == http.MethodGet && request.URL.Path == "/v2/packages/"+assetID+"/versions/"+versionID+"/manifest":
		files := make([]map[string]any, 0, len(fixture.files))
		for _, path := range sortedBridgeFixturePaths(fixture.files) {
			content := fixture.files[path]
			files = append(files, map[string]any{"path": path, "sha256": bridgeFixtureSHA(content), "size": len(content), "mime": "text/markdown"})
		}
		writeBridgeCloudJSON(writer, map[string]any{
			"schema_version": "2", "asset_id": assetID, "version_id": versionID, "version": "1.0.0", "kind": "skill",
			"origin":     map[string]any{"repository_url": "https://example.test/org/repo", "ref": "main", "commit": strings.Repeat("a", 40)},
			"entrypoint": "SKILL.md", "files": files,
			"compatibility": map[string]string{"claude": "supported", "codex": "supported"},
		})
	case request.Method == http.MethodPost && request.URL.Path == "/v2/packages/"+assetID+"/versions/"+versionID+"/downloads":
		downloads := make([]map[string]any, 0, len(fixture.files))
		for _, path := range sortedBridgeFixturePaths(fixture.files) {
			content := fixture.files[path]
			downloads = append(downloads, map[string]any{
				"path": path, "sha256": bridgeFixtureSHA(content), "size": len(content), "mime": "text/markdown",
				"download_url": fixture.server.URL + "/blob?path=" + url.QueryEscape(path),
				"expires_at":   time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
			})
		}
		writeBridgeCloudJSON(writer, map[string]any{"asset_id": assetID, "version_id": versionID, "downloads": downloads})
	case request.Method == http.MethodGet && request.URL.Path == "/blob":
		content, ok := fixture.files[request.URL.Query().Get("path")]
		if !ok {
			http.NotFound(writer, request)
			return
		}
		if fixture.corrupt.Load() {
			content = []byte("corrupt")
		}
		writer.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		_, _ = writer.Write(content)
	default:
		http.NotFound(writer, request)
	}
}

func TestBridgeCloudPreviewHasNoFilesystemMutation(t *testing.T) {
	newBridgeCloudFixture(t)
	projectDir := t.TempDir()
	input := `{"jsonrpc":"2.0","id":1,"method":"cloud.preview","params":{"asset_id":"skill::skills::review-pr"}}` + "\n"
	result := runCLIWithInput(t, projectDir, t.TempDir(), input, "bridge", "--stdio")
	if result.err != nil {
		t.Fatalf("bridge: %v\nstderr=%s", result.err, result.stderr)
	}
	response := decodeBridgeTestResponses(t, result.stdout)[0]["result"].(map[string]any)
	if response["asset_id"] != "skill::skills::review-pr" || response["version_id"] != "version-1" {
		t.Fatalf("preview = %v", response)
	}
	if changes := response["changes"].([]any); len(changes) != 4 {
		t.Fatalf("changes = %v, want four bundle files across two native roots", changes)
	}
	for _, path := range []string{".claude", ".agents", ".sth"} {
		if _, err := os.Stat(filepath.Join(projectDir, path)); !os.IsNotExist(err) {
			t.Fatalf("preview mutated %s: %v", path, err)
		}
	}
}

func TestBridgeCloudInstallFansOutAndTracksImmutableVersion(t *testing.T) {
	newBridgeCloudFixture(t)
	projectDir := t.TempDir()
	input := `{"jsonrpc":"2.0","id":1,"method":"cloud.install","params":{"asset_id":"skill::skills::review-pr","expected_version_id":"version-1","confirmed":true}}` + "\n"
	result := runCLIWithInput(t, projectDir, t.TempDir(), input, "bridge", "--stdio")
	if result.err != nil {
		t.Fatalf("bridge: %v\nstderr=%s", result.err, result.stderr)
	}
	response := decodeBridgeTestResponses(t, result.stdout)[0]["result"].(map[string]any)
	if response["installed"] != true || response["version_id"] != "version-1" {
		t.Fatalf("install = %v", response)
	}
	claudeSkill := filepath.Join(projectDir, ".claude", "skills", "review-pr", "SKILL.md")
	agentsSkill := filepath.Join(projectDir, ".agents", "skills", "review-pr", "SKILL.md")
	for _, path := range []string{claudeSkill, agentsSkill, filepath.Join(projectDir, ".claude", "skills", "review-pr", "references", "checklist.md"), filepath.Join(projectDir, ".agents", "skills", "review-pr", "references", "checklist.md")} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("materialized file %s: %v", path, err)
		}
	}
	claudeContent, _ := os.ReadFile(claudeSkill)
	if !strings.Contains(string(claudeContent), writer.ClaudeManagedHeader) {
		t.Fatalf("Claude managed header missing: %q", claudeContent)
	}
	agentsContent, _ := os.ReadFile(agentsSkill)
	if !strings.Contains(string(agentsContent), "managed by sth") {
		t.Fatalf("agents managed marker missing: %q", agentsContent)
	}
	stateBytes, err := os.ReadFile(filepath.Join(projectDir, ".sth", "local", "code", "sth-state-v2.json"))
	if err != nil {
		t.Fatal(err)
	}
	var state fs.InstalledResourceStateV2
	if err := json.Unmarshal(stateBytes, &state); err != nil {
		t.Fatal(err)
	}
	if len(state.Entries) != 1 || state.Entries[0].ResourceName != "skill::skills::review-pr" || state.Entries[0].ResolvedVersion != "version-1" || len(state.Entries[0].ManagedFiles) != 4 {
		t.Fatalf("state = %+v", state)
	}

	status := runCLIWithInput(t, projectDir, t.TempDir(), `{"jsonrpc":"2.0","id":1,"method":"status","params":{}}`+"\n", "bridge", "--stdio")
	if status.err != nil {
		t.Fatal(status.err)
	}
	resources := decodeBridgeTestResponses(t, status.stdout)[0]["result"].(map[string]any)["resources"].([]any)
	resource := resources[0].(map[string]any)
	if resource["skill_name"] != "review-pr" {
		t.Fatalf("status resource = %v", resource)
	}
	targets := resource["targets"].([]any)
	if len(targets) != 2 || targets[0] != "claude" || targets[1] != "codex" {
		t.Fatalf("targets = %v", targets)
	}
}

func TestBridgeCloudInstallCanBeRemovedWithoutProjectConfig(t *testing.T) {
	newBridgeCloudFixture(t)
	projectDir := t.TempDir()
	installInput := `{"jsonrpc":"2.0","id":1,"method":"cloud.install","params":{"asset_id":"skill::skills::review-pr","expected_version_id":"version-1","confirmed":true}}` + "\n"
	installResult := runCLIWithInput(t, projectDir, t.TempDir(), installInput, "bridge", "--stdio")
	if installResult.err != nil {
		t.Fatalf("install: %v\nstderr=%s", installResult.err, installResult.stderr)
	}

	previewInput := `{"jsonrpc":"2.0","id":1,"method":"remove.preview","params":{"resource_ref":"review-pr"}}` + "\n"
	previewResult := runCLIWithInput(t, projectDir, t.TempDir(), previewInput, "bridge", "--stdio")
	if previewResult.err != nil {
		t.Fatalf("preview removal: %v", previewResult.err)
	}
	previewResponse := decodeBridgeTestResponses(t, previewResult.stdout)[0]["result"].(map[string]any)
	previewResource := previewResponse["resource"].(map[string]any)
	if previewResource["resource_ref"] != "skill::skills::review-pr" {
		t.Fatalf("preview resource = %v", previewResource)
	}

	removeInput := `{"jsonrpc":"2.0","id":1,"method":"remove","params":{"resource_ref":"review-pr","confirmed":true}}` + "\n"
	removeResult := runCLIWithInput(t, projectDir, t.TempDir(), removeInput, "bridge", "--stdio")
	if removeResult.err != nil {
		t.Fatalf("remove: %v\nstderr=%s", removeResult.err, removeResult.stderr)
	}
	removeResponse := decodeBridgeTestResponses(t, removeResult.stdout)[0]["result"].(map[string]any)
	if removeResponse["removed"] != true {
		t.Fatalf("remove response = %v", removeResponse)
	}
	for _, path := range []string{
		filepath.Join(projectDir, ".claude", "skills", "review-pr", "SKILL.md"),
		filepath.Join(projectDir, ".agents", "skills", "review-pr", "SKILL.md"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("managed file still exists at %s: %v", path, err)
		}
	}
	stateData, err := os.ReadFile(filepath.Join(projectDir, ".sth", "local", "code", "sth-state-v2.json"))
	if err != nil {
		t.Fatal(err)
	}
	var state fs.InstalledResourceStateV2
	if err := json.Unmarshal(stateData, &state); err != nil {
		t.Fatal(err)
	}
	if len(state.Entries) != 0 {
		t.Fatalf("state after removal = %+v", state)
	}
}

func TestBridgeCloudInstallRequiresConfirmationAndExpectedVersionBeforeNetwork(t *testing.T) {
	fixture := newBridgeCloudFixture(t)
	projectDir := t.TempDir()
	input := `{"jsonrpc":"2.0","id":1,"method":"cloud.install","params":{"asset_id":"skill::skills::review-pr"}}` + "\n"
	result := runCLIWithInput(t, projectDir, t.TempDir(), input, "bridge", "--stdio")
	if result.err != nil {
		t.Fatal(result.err)
	}
	errorValue := decodeBridgeTestResponses(t, result.stdout)[0]["error"].(map[string]any)
	if errorValue["code"] != float64(-32602) || fixture.requests.Load() != 0 {
		t.Fatalf("error=%v requests=%d", errorValue, fixture.requests.Load())
	}
}

func TestBridgeCloudInstallRejectsVersionChangedAfterPreview(t *testing.T) {
	fixture := newBridgeCloudFixture(t)
	projectDir := t.TempDir()
	preview := runCLIWithInput(t, projectDir, t.TempDir(), `{"jsonrpc":"2.0","id":1,"method":"cloud.preview","params":{"asset_id":"skill::skills::review-pr"}}`+"\n", "bridge", "--stdio")
	if preview.err != nil {
		t.Fatal(preview.err)
	}
	fixture.latest.Store("version-2")
	install := runCLIWithInput(t, projectDir, t.TempDir(), `{"jsonrpc":"2.0","id":1,"method":"cloud.install","params":{"asset_id":"skill::skills::review-pr","expected_version_id":"version-1","confirmed":true}}`+"\n", "bridge", "--stdio")
	if install.err != nil {
		t.Fatal(install.err)
	}
	errorValue := decodeBridgeTestResponses(t, install.stdout)[0]["error"].(map[string]any)
	if errorValue["code"] != float64(-32031) || !strings.Contains(errorValue["message"].(string), "changed") {
		t.Fatalf("error = %v", errorValue)
	}
	for _, path := range []string{".claude", ".agents", ".sth"} {
		if _, err := os.Stat(filepath.Join(projectDir, path)); !os.IsNotExist(err) {
			t.Fatalf("rejected install mutated %s", path)
		}
	}
}

func TestBridgeCloudHashFailureNeverMutatesProject(t *testing.T) {
	fixture := newBridgeCloudFixture(t)
	fixture.corrupt.Store(true)
	projectDir := t.TempDir()
	result := runCLIWithInput(t, projectDir, t.TempDir(), `{"jsonrpc":"2.0","id":1,"method":"cloud.preview","params":{"asset_id":"skill::skills::review-pr"}}`+"\n", "bridge", "--stdio")
	if result.err != nil {
		t.Fatal(result.err)
	}
	errorValue := decodeBridgeTestResponses(t, result.stdout)[0]["error"].(map[string]any)
	if !strings.Contains(errorValue["message"].(string), "invalid HTTP size") && !strings.Contains(errorValue["message"].(string), "invalid SHA-256") {
		t.Fatalf("error = %v", errorValue)
	}
	for _, path := range []string{".claude", ".agents", ".sth"} {
		if _, err := os.Stat(filepath.Join(projectDir, path)); !os.IsNotExist(err) {
			t.Fatalf("failed preview mutated %s", path)
		}
	}
}

func TestBridgeCloudPackageComponentRejectsTrailingDot(t *testing.T) {
	for _, invalid := range []string{"review-pr.", "review-pr-", "review-pr_", ".review-pr", "Review-pr"} {
		if validBridgeCloudIDSegment(invalid) {
			t.Fatalf("unsafe package component accepted: %q", invalid)
		}
	}
	for _, valid := range []string{"r", "review-pr", "review_pr", "review.pr"} {
		if !validBridgeCloudIDSegment(valid) {
			t.Fatalf("safe package component rejected: %q", valid)
		}
	}
}

func TestBridgeCloudRemovalFindsSlugWithoutCodexPlacement(t *testing.T) {
	state := fs.InstalledResourceStateV2{Entries: []fs.InstalledResourceRecordV2{{
		ResourceName: "skill::skills::review-pr",
		ProviderID:   "sth-cloud",
		ArtifactKind: "skill",
		ManagedFiles: []fs.ManagedFile{{Path: ".claude/skills/review-pr/SKILL.md"}},
	}}}
	recordIndex, found, err := bridgeCloudRemovalRecord(state, "review-pr")
	if err != nil || !found || recordIndex != 0 {
		t.Fatalf("record index=%d found=%t err=%v", recordIndex, found, err)
	}
}

func writeBridgeCloudJSON(writer http.ResponseWriter, value any) {
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(value)
}

func sortedBridgeFixturePaths(files map[string][]byte) []string {
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func bridgeFixtureSHA(content []byte) string {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}
