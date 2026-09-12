package cmd

import (
	"context"
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"sth/internal/fs"
	sthhttp "sth/internal/http"
)

func TestRepositoryUpdatesKeepReachableSourcesAndNeverWriteSkills(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(stdhttp.HandlerFunc(func(response stdhttp.ResponseWriter, request *stdhttp.Request) {
		requests.Add(1)
		if strings.Contains(request.URL.Path, "internal") {
			response.WriteHeader(stdhttp.StatusUnauthorized)
			return
		}
		_, _ = response.Write([]byte(`[{"sha":"new-version"}]`))
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_API_BASE_URL", server.URL)
	t.Setenv("GITHUB_TOKEN", "fixture-key")
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	t.Setenv("STH_USER_CONFIG_DIR", t.TempDir())
	config := multiProviderConfig()
	writeProjectConfigV2ForTest(t, projectDir, config)
	content := []byte("my personal skill content")
	filePath := filepath.Join(projectDir, "personal.md")
	if err := os.WriteFile(filePath, content, 0o600); err != nil {
		t.Fatal(err)
	}
	state := fs.InstalledResourceStateV2{SchemaVersion: 2}
	for _, provider := range []string{"public", "internal"} {
		state.Entries = append(state.Entries, fs.InstalledResourceRecordV2{ProviderID: provider, ResourceName: "bundle::skills::alpha", SourcePath: "skills/alpha/SKILL.md", ArtifactKind: "skill", ResolvedVersion: "old-version", ManagedFiles: []fs.ManagedFile{{Path: filePath, SHA256: fs.ComputeSHA256(content)}}})
	}
	writeV2StateForStatusTest(t, projectDir, ".sth", state)
	before := transactionalProjectFileSnapshot(t, projectDir)
	application := newApp(strings.NewReader("must not prompt"), nil, nil, sthhttp.NewClient(server.Client()))
	model := newGuidedMenuModel(application)
	check := model.Init()
	if check == nil {
		t.Fatal("expected first automatic check")
	}
	if requests.Load() != 0 {
		t.Fatal("menu construction blocked on network")
	}
	report := check().(repositoryUpdateReport)
	if len(report.Resources) != 2 || report.Resources[0].Status != "update-available" || report.Resources[1].Status != "unverified" {
		t.Fatalf("partial source result lost: %+v", report)
	}
	updated, _ := model.Update(report)
	assertContains(t, updated.(guidedMenuModel).View(), "1 mise(s) à jour disponible(s)")
	if _, repeated := application.prepareRepositoryUpdateCheck(false); repeated != nil {
		t.Fatal("menu refresh ignored five-minute cache")
	}
	if requests.Load() != 2 {
		t.Fatalf("unexpected requests: %d", requests.Load())
	}
	if after := transactionalProjectFileSnapshot(t, projectDir); !equalFileSnapshots(before, after) {
		t.Fatal("automatic check wrote project files")
	}
	report.CheckedAt = time.Now().Add(-6 * time.Minute)
	application.repositoryUpdates = &report
	if _, stale := application.prepareRepositoryUpdateCheck(false); stale == nil {
		t.Fatal("expired cache reused")
	}
	report.CheckedAt = time.Now()
	state.Entries[0].ResolvedVersion = "new-version"
	writeV2StateForStatusTest(t, projectDir, ".sth", state)
	if _, changed := application.prepareRepositoryUpdateCheck(false); changed == nil {
		t.Fatal("installed version change did not invalidate cache")
	}
}

func equalFileSnapshots(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for path, value := range left {
		if right[path] != value {
			return false
		}
	}
	return true
}

func TestCheckUpdatesTextAndJSONAgree(t *testing.T) {
	server := commitsListServer(t, "new-version")
	defer server.Close()
	t.Setenv("STH_GITHUB_API_BASE_URL", server.URL)
	projectDir, homeDir := t.TempDir(), t.TempDir()
	writeProjectConfigV2ForTest(t, projectDir, multiProviderConfig())
	writeV2StateForStatusTest(t, projectDir, ".sth", fs.InstalledResourceStateV2{SchemaVersion: 2, Entries: []fs.InstalledResourceRecordV2{{ProviderID: "public", ResourceName: "alpha", SourcePath: "skills/alpha/SKILL.md", ResolvedVersion: "old-version"}}})
	textResult := runCLI(t, projectDir, homeDir, "check-updates")
	jsonResult := runCLI(t, projectDir, homeDir, "check-updates", "--json")
	if textResult.err != nil || jsonResult.err != nil {
		t.Fatalf("%v / %v", textResult.err, jsonResult.err)
	}
	var report repositoryUpdateReport
	if err := json.Unmarshal([]byte(jsonResult.stdout), &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Resources) != 1 || report.Resources[0].Status != "update-available" {
		t.Fatalf("unexpected report: %+v", report)
	}
	assertContains(t, textResult.stdout, report.Resources[0].Name+" ["+report.Resources[0].Provider+"]")
	assertContains(t, textResult.stdout, report.Resources[0].NextAction)
}

func TestRepositoryCheckHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	application := newApp(nil, nil, nil, nil)
	state := fs.InstalledResourceStateV2{Entries: []fs.InstalledResourceRecordV2{{ProviderID: "public", ResourceName: "alpha", SourcePath: "skills/alpha/SKILL.md", ResolvedVersion: "old"}}}
	report := application.checkRepositoryUpdates(ctx, multiProviderConfig(), state, t.TempDir(), "test")
	if len(report.Resources) != 1 || report.Resources[0].Status != "unverified" {
		t.Fatalf("cancelled check reported success: %+v", report)
	}
}

func TestPrivateCreationGuideUsesReadOnlyScopedAccess(t *testing.T) {
	guide := strings.Join(privateCreationGuide("github"), "\n")
	for _, expected := range []string{"Only select repositories", "Read-only", "expiration", "GITHUB_TOKEN"} {
		assertContains(t, guide, expected)
	}
	assertNotContains(t, guide, "full access")
}
