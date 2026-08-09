package cmd

import (
	"encoding/json"
	"fmt"
	stdhttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"sth/internal/fs"
	"sth/internal/writer"
)

func singleArtifactServer(t *testing.T, resourceType, artifactName, folderName, sourcePath, description, content string, sourceRequestCount *atomic.Int32) *httptest.Server {
	t.Helper()
	row := fmt.Sprintf("| %s | %s | %s | %s | %s |", resourceType, artifactName, folderName, sourcePath, description)
	return httptest.NewServer(stdhttp.HandlerFunc(func(responseWriter stdhttp.ResponseWriter, request *stdhttp.Request) {
		switch request.URL.Path {
		case "/owner/repo/main/SKILLS.md":
			_, _ = responseWriter.Write([]byte(testCatalogMarkdown(row)))
		case "/owner/repo/main/" + sourcePath:
			if sourceRequestCount != nil {
				sourceRequestCount.Add(1)
			}
			_, _ = responseWriter.Write([]byte(content))
		default:
			stdhttp.NotFound(responseWriter, request)
		}
	}))
}

func transactionalProjectFileSnapshot(t *testing.T, projectDir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	walkErr := filepath.Walk(projectDir, func(path string, fileInfo os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fileInfo.IsDir() {
			return nil
		}
		relativePath, relativeErr := filepath.Rel(projectDir, path)
		if relativeErr != nil {
			return relativeErr
		}
		if filepath.ToSlash(relativePath) == ".sth/last-operation.json" {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		files[relativePath] = string(content)
		return nil
	})
	if walkErr != nil {
		t.Fatal(walkErr)
	}
	return files
}

func fanoutConfig(targets ...fs.TargetEntry) fs.ProjectConfigV2 {
	return fs.ProjectConfigV2{
		SchemaVersion: 2,
		Providers: []fs.ProviderEntry{
			{ID: "default", Provider: "github", RepositoryID: "owner/repo", CatalogRef: "main", CatalogPath: "SKILLS.md", TargetDir: ".sth"},
		},
		Resources: []fs.ResourceEntry{},
		Targets:   targets,
	}
}

func fanoutTarget(kind, rootDir string) fs.TargetEntry {
	return fs.TargetEntry{ID: kind, Kind: kind, RootDir: rootDir, Enabled: true}
}

const fanoutCommitSHA = "deadbeefcafe0000"

func skillBundleServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		switch r.URL.Path {
		case "/owner/repo/main/SKILLS.md":
			_, _ = w.Write([]byte(testCatalogMarkdown("| skill | alpha | skills | skills/alpha/SKILL.md | Alpha |")))
		case "/repos/owner/repo/git/trees/main":
			_, _ = w.Write([]byte(`{"tree":[{"path":"skills/alpha/SKILL.md","type":"blob"}]}`))
		case "/owner/repo/main/skills/alpha/SKILL.md":
			_, _ = w.Write([]byte("---\nname: alpha\ndescription: Alpha skill\n---\n# Alpha skill\n"))
		case "/repos/owner/repo/commits":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"sha":"` + fanoutCommitSHA + `"}]`))
		default:
			stdhttp.NotFound(w, r)
		}
	}))
}

func assertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file %s to exist: %v", path, err)
	}
}

func assertNoFile(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Errorf("expected file %s to NOT exist", path)
	}
}

func readStateV2(t *testing.T, projectDir string) fs.InstalledResourceStateV2 {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(projectDir, ".sth", "sth-state-v2.json"))
	if err != nil {
		t.Fatalf("ReadFile(state): %v", err)
	}
	var state fs.InstalledResourceStateV2
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("json.Unmarshal(state): %v", err)
	}
	return state
}

func TestInstallFanout_SkillDedupToClaudeHub(t *testing.T) {
	projectDir := t.TempDir()
	server := skillBundleServer(t)
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)
	t.Setenv("STH_GITHUB_API_BASE_URL", server.URL)

	writeProjectConfigV2ForTest(t, projectDir, fanoutConfig(
		fanoutTarget("claude", ".claude"),
		fanoutTarget("cursor", ".cursor"),
		fanoutTarget("copilot", ".github"),
	))

	result := runCLI(t, projectDir, t.TempDir(), "install", "alpha")
	if result.err != nil {
		t.Fatalf("Run() error = %v\nstderr=%s", result.err, result.stderr)
	}

	assertFileExists(t, filepath.Join(projectDir, ".claude", "skills", "alpha", "SKILL.md"))
	assertNoFile(t, filepath.Join(projectDir, ".cursor", "skills", "alpha", "SKILL.md"))
	assertNoFile(t, filepath.Join(projectDir, ".github", "skills", "alpha", "SKILL.md"))
	assertContains(t, result.stdout, ".claude/skills/alpha")

	state := readStateV2(t, projectDir)
	if len(state.Entries) != 1 {
		t.Fatalf("state entries = %d, want 1", len(state.Entries))
	}
	if len(state.Entries[0].ManagedFiles) != 1 {
		t.Fatalf("managed files = %d, want 1", len(state.Entries[0].ManagedFiles))
	}
	if got := state.Entries[0].ManagedFiles[0].Path; got != ".claude/skills/alpha/SKILL.md" {
		t.Fatalf("managed path = %q, want .claude/skills/alpha/SKILL.md", got)
	}
}

func TestInstallFanout_SkillClaudeAndAntigravity(t *testing.T) {
	projectDir := t.TempDir()
	server := skillBundleServer(t)
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)
	t.Setenv("STH_GITHUB_API_BASE_URL", server.URL)

	writeProjectConfigV2ForTest(t, projectDir, fanoutConfig(
		fanoutTarget("claude", ".claude"),
		fanoutTarget("antigravity", ".agents"),
	))

	result := runCLI(t, projectDir, t.TempDir(), "install", "alpha")
	if result.err != nil {
		t.Fatalf("Run() error = %v\nstderr=%s", result.err, result.stderr)
	}

	assertFileExists(t, filepath.Join(projectDir, ".claude", "skills", "alpha", "SKILL.md"))
	assertFileExists(t, filepath.Join(projectDir, ".agents", "skills", "alpha", "SKILL.md"))

	state := readStateV2(t, projectDir)
	if len(state.Entries) != 1 {
		t.Fatalf("state entries = %d, want 1", len(state.Entries))
	}
	if len(state.Entries[0].ManagedFiles) != 2 {
		t.Fatalf("managed files = %d, want 2 (claude + agents)", len(state.Entries[0].ManagedFiles))
	}
}

func TestInstallFanout_SkillRollsBackFirstHubWhenSecondHubFails(t *testing.T) {
	projectDir := t.TempDir()
	server := skillBundleServer(t)
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)
	t.Setenv("STH_GITHUB_API_BASE_URL", server.URL)

	writeProjectConfigV2ForTest(t, projectDir, fanoutConfig(
		fanoutTarget("claude", ".claude"),
		fanoutTarget("antigravity", ".agents"),
	))
	blockerPath := filepath.Join(projectDir, ".agents", "skills", "alpha", "SKILL.md")
	if err := os.MkdirAll(blockerPath, 0o750); err != nil {
		t.Fatal(err)
	}
	before := transactionalProjectFileSnapshot(t, projectDir)

	result := runCLI(t, projectDir, t.TempDir(), "install", "alpha")
	if result.err == nil {
		t.Fatalf("skill install succeeded despite blocked second hub\nstdout=%s", result.stdout)
	}

	after := transactionalProjectFileSnapshot(t, projectDir)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("failed skill fan-out mutated project files:\nbefore=%v\nafter=%v", before, after)
	}
	assertNoFile(t, filepath.Join(projectDir, ".claude", "skills", "alpha", "SKILL.md"))
	assertNoFile(t, filepath.Join(projectDir, ".sth", "sth-state-v2.json"))
	restoredInfo, err := os.Stat(blockerPath)
	if err != nil {
		t.Fatal(err)
	}
	if !restoredInfo.IsDir() {
		t.Fatalf("second-hub blocker is no longer a directory: %s", blockerPath)
	}
}

func TestInstallFanout_FlatSkillSingleFile(t *testing.T) {
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		switch r.URL.Path {
		case "/owner/repo/main/SKILLS.md":
			_, _ = w.Write([]byte(testCatalogMarkdown("| skill | alpha | skills | skills/alpha.md | Alpha |")))
		case "/owner/repo/main/skills/alpha.md":
			_, _ = w.Write([]byte("# Alpha\n"))
		default:
			stdhttp.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)

	writeProjectConfigV2ForTest(t, projectDir, fanoutConfig(fanoutTarget("claude", ".claude")))

	result := runCLI(t, projectDir, t.TempDir(), "install", "alpha")
	if result.err != nil {
		t.Fatalf("Run() error = %v\nstderr=%s", result.err, result.stderr)
	}
	assertFileExists(t, filepath.Join(projectDir, ".claude", "skills", "alpha", "SKILL.md"))
}

func TestInstallFanout_CodexUsesNativeAgentsSkillsDiscovery(t *testing.T) {
	projectDir := t.TempDir()
	server := skillBundleServer(t)
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)
	t.Setenv("STH_GITHUB_API_BASE_URL", server.URL)

	writeProjectConfigV2ForTest(t, projectDir, fanoutConfig(fanoutTarget("codex", ".codex")))

	result := runCLI(t, projectDir, t.TempDir(), "install", "alpha")
	if result.err != nil {
		t.Fatalf("Run() error = %v\nstderr=%s", result.err, result.stderr)
	}

	assertFileExists(t, filepath.Join(projectDir, ".agents", "skills", "alpha", "SKILL.md"))
	assertNoFile(t, filepath.Join(projectDir, ".claude", "skills", "alpha", "SKILL.md"))
	assertNoFile(t, filepath.Join(projectDir, ".codex", "config.toml"))
}

func TestInstallFanout_AgentAcrossTargets(t *testing.T) {
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		switch r.URL.Path {
		case "/owner/repo/main/SKILLS.md":
			_, _ = w.Write([]byte(testCatalogMarkdown("| agent | reviewer | agents | agents/reviewer.md | Reviewer |")))
		case "/owner/repo/main/agents/reviewer.md":
			_, _ = w.Write([]byte("# Reviewer\n"))
		default:
			stdhttp.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)

	writeProjectConfigV2ForTest(t, projectDir, fanoutConfig(
		fanoutTarget("claude", ".claude"),
		fanoutTarget("cursor", ".cursor"),
		fanoutTarget("copilot", ".github"),
		fanoutTarget("antigravity", ".agents"),
		fanoutTarget("gemini", ".gemini"),
	))

	result := runCLI(t, projectDir, t.TempDir(), "install", "reviewer")
	if result.err != nil {
		t.Fatalf("Run() error = %v\nstderr=%s", result.err, result.stderr)
	}

	assertFileExists(t, filepath.Join(projectDir, ".claude", "agents", "reviewer.md"))
	assertFileExists(t, filepath.Join(projectDir, ".cursor", "rules", "agents", "reviewer.mdc"))
	assertFileExists(t, filepath.Join(projectDir, ".github", "agents", "reviewer.agent.md"))
	assertFileExists(t, filepath.Join(projectDir, ".agents", "agents", "reviewer.md"))
	assertFileExists(t, filepath.Join(projectDir, ".gemini", "agents", "reviewer.md"))

	state := readStateV2(t, projectDir)
	if len(state.Entries) != 5 {
		t.Fatalf("state entries = %d, want 5 (one per target)", len(state.Entries))
	}
}

func TestInstallFanout_ForcedDryRunDoesNotTransferOwnership(t *testing.T) {
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		switch r.URL.Path {
		case "/owner/repo/main/SKILLS.md":
			_, _ = w.Write([]byte(testCatalogMarkdown("| agent | reviewer | agents | agents/reviewer.md | Reviewer |")))
		case "/owner/repo/main/agents/reviewer.md":
			_, _ = w.Write([]byte("# Reviewer\n"))
		default:
			stdhttp.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)

	writeProjectConfigV2ForTest(t, projectDir, fanoutConfig(fanoutTarget("claude", ".claude")))
	state := fs.InstalledResourceStateV2{
		SchemaVersion: 2,
		Entries: []fs.InstalledResourceRecordV2{{
			ResourceName: "agent::agents::other",
			ProviderID:   "default",
			TargetID:     "claude",
			ArtifactKind: "agent",
			ManagedFiles: []fs.ManagedFile{{
				Path: ".claude/agents/reviewer.md", SHA256: "existing-owner",
			}},
		}},
	}
	stateBytes, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(projectDir, ".sth", "sth-state-v2.json")
	if err := os.WriteFile(statePath, stateBytes, 0o600); err != nil {
		t.Fatal(err)
	}

	result := runCLI(t, projectDir, t.TempDir(), "install", "reviewer", "--dry-run", "--json", "--force")
	if result.err != nil {
		t.Fatalf("forced dry-run: %v\nstderr=%s", result.err, result.stderr)
	}
	after, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(stateBytes) {
		t.Fatalf("forced dry-run mutated ownership state:\nbefore=%s\nafter=%s", stateBytes, after)
	}
}

func TestInstallFanout_RollsBackFilesAndOwnershipWhenSecondTargetFails(t *testing.T) {
	projectDir := t.TempDir()
	server := singleArtifactServer(t, "agent", "reviewer", "agents", "agents/reviewer.md", "Reviewer", "# Updated reviewer\n", nil)
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)

	writeProjectConfigV2ForTest(t, projectDir, fanoutConfig(
		fanoutTarget("claude", ".claude"),
		fanoutTarget("copilot", ".github"),
	))
	existingPath := filepath.Join(projectDir, ".claude", "agents", "reviewer.md")
	if err := os.MkdirAll(filepath.Dir(existingPath), 0o755); err != nil {
		t.Fatal(err)
	}
	existingContent := []byte("# Existing reviewer\n")
	if err := os.WriteFile(existingPath, existingContent, 0o600); err != nil {
		t.Fatal(err)
	}
	blockerPath := filepath.Join(projectDir, ".github", "agents", "reviewer.agent.md")
	if err := os.MkdirAll(blockerPath, 0o750); err != nil {
		t.Fatal(err)
	}
	state := fs.InstalledResourceStateV2{
		SchemaVersion: 2,
		Entries: []fs.InstalledResourceRecordV2{{
			ResourceName: "agent::agents::other",
			ProviderID:   "default",
			TargetID:     "claude",
			ArtifactKind: "agent",
			ManagedFiles: []fs.ManagedFile{{Path: ".claude/agents/reviewer.md", SHA256: "existing-owner"}},
		}},
	}
	stateBytes, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(projectDir, ".sth", "sth-state-v2.json")
	if err := os.WriteFile(statePath, stateBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	before := transactionalProjectFileSnapshot(t, projectDir)

	result := runCLI(t, projectDir, t.TempDir(), "install", "reviewer", "--force")
	if result.err == nil {
		t.Fatalf("install succeeded despite blocked second target\nstdout=%s", result.stdout)
	}

	after := transactionalProjectFileSnapshot(t, projectDir)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("failed fan-out mutated project files:\nbefore=%v\nafter=%v", before, after)
	}
	restoredInfo, err := os.Stat(blockerPath)
	if err != nil {
		t.Fatal(err)
	}
	if !restoredInfo.IsDir() {
		t.Fatalf("second-target blocker is no longer a directory: %s", blockerPath)
	}
	restoredInfo, err = os.Stat(existingPath)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && restoredInfo.Mode().Perm() != 0o600 {
		t.Fatalf("restored mode = %o, want 600", restoredInfo.Mode().Perm())
	}
	if strings.Contains(result.stdout, "ownership") {
		t.Fatalf("failed transaction reported a completed ownership transfer: %s", result.stdout)
	}
}

func TestFanoutProjectPathRejectsParentSymlinkOutsideProject(t *testing.T) {
	projectDir := t.TempDir()
	outsideDir := t.TempDir()
	victimPath := filepath.Join(outsideDir, "victim.md")
	if err := os.WriteFile(victimPath, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(projectDir, "escape")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	if _, err := fanoutProjectPath(projectDir, "escape/victim.md"); err == nil {
		t.Fatal("expected redirected transaction path error")
	}
	content, err := os.ReadFile(victimPath)
	if err != nil {
		t.Fatalf("outside victim was removed: %v", err)
	}
	if string(content) != "outside" {
		t.Fatalf("outside victim changed: %q", content)
	}
}

func TestFanoutRecordMigrationReplacesLegacyTarget(t *testing.T) {
	resourceName := "agent::agents::reviewer"
	legacyRecord := fs.InstalledResourceRecordV2{
		ResourceName: resourceName,
		ManagedFiles: []fs.ManagedFile{{Path: ".claude/agents/reviewer.md", SHA256: "legacy"}},
	}
	state := fs.InstalledResourceStateV2{SchemaVersion: 2, Entries: []fs.InstalledResourceRecordV2{legacyRecord}}
	previous, found := fanoutInstalledRecord(state, resourceName, "claude")
	if !found || previous.TargetID != "" || previous.ManagedFiles[0].SHA256 != "legacy" {
		t.Fatalf("legacy record not found: %+v, found=%v", previous, found)
	}

	replacement := legacyRecord
	replacement.TargetID = "claude"
	replacement.ManagedFiles[0].SHA256 = "current"
	upsertFanoutRecord(&state, replacement)
	if len(state.Entries) != 1 || state.Entries[0].TargetID != "claude" || state.Entries[0].ManagedFiles[0].SHA256 != "current" {
		t.Fatalf("legacy record was not migrated: %+v", state.Entries)
	}
}

func TestTransferAllFanoutOwnershipRemovesEveryClaimant(t *testing.T) {
	sharedPath := ".claude/commands/deploy.md"
	state := fs.InstalledResourceStateV2{SchemaVersion: 2, Entries: []fs.InstalledResourceRecordV2{
		{ResourceName: "command::commands::old", TargetID: "claude", ManagedFiles: []fs.ManagedFile{{Path: sharedPath}}},
		{ResourceName: "command::commands::old", TargetID: "copilot", ManagedFiles: []fs.ManagedFile{{Path: sharedPath}}},
	}}
	messages := transferAllFanoutOwnership(&state, "command::commands::new", []fs.ManagedFile{{Path: sharedPath}})
	if len(messages) != 1 {
		t.Fatalf("ownership messages = %v, want one deduplicated warning", messages)
	}
	for _, record := range state.Entries {
		if len(record.ManagedFiles) != 0 {
			t.Fatalf("old claimant remains: %+v", state.Entries)
		}
	}
}

func TestTransferAllFanoutOwnershipRemovesOrphanedManagedRegions(t *testing.T) {
	sharedPath := ".codex/sth/instructions/deploy.md"
	state := fs.InstalledResourceStateV2{SchemaVersion: 2, Entries: []fs.InstalledResourceRecordV2{{
		ResourceName:   "instruction::old::deploy",
		TargetID:       "codex",
		ManagedFiles:   []fs.ManagedFile{{Path: sharedPath}},
		ManagedRegions: []fs.ManagedRegion{{Path: "AGENTS.md", ID: codexInstructionRegionID, SHA256: "old"}},
	}}}
	messages := transferAllFanoutOwnership(&state, "instruction::new::deploy", []fs.ManagedFile{{Path: sharedPath}})
	if len(messages) != 1 {
		t.Fatalf("ownership messages = %v, want one", messages)
	}
	if len(state.Entries[0].ManagedFiles) != 0 || len(state.Entries[0].ManagedRegions) != 0 {
		t.Fatalf("old instruction claims remain: %+v", state.Entries[0])
	}
}

func TestInstallFanout_SkillOwnershipRequiresForce(t *testing.T) {
	projectDir := t.TempDir()
	server := skillBundleServer(t)
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)
	t.Setenv("STH_GITHUB_API_BASE_URL", server.URL)
	writeProjectConfigV2ForTest(t, projectDir, fanoutConfig(fanoutTarget("claude", ".claude")))

	installedPath := filepath.Join(projectDir, ".claude", "skills", "alpha", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(installedPath), 0o755); err != nil {
		t.Fatal(err)
	}
	existingContent := []byte("existing owner")
	if err := os.WriteFile(installedPath, existingContent, 0o600); err != nil {
		t.Fatal(err)
	}
	state := fs.InstalledResourceStateV2{SchemaVersion: 2, Entries: []fs.InstalledResourceRecordV2{{
		ResourceName: "skill::other::alpha",
		ProviderID:   "default",
		ManagedFiles: []fs.ManagedFile{{Path: ".claude/skills/alpha/SKILL.md", SHA256: fs.ComputeSHA256(existingContent)}},
	}}}
	stateBytes, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".sth", "sth-state-v2.json"), stateBytes, 0o600); err != nil {
		t.Fatal(err)
	}

	blockedResult := runCLI(t, projectDir, t.TempDir(), "install", "alpha")
	if blockedResult.err == nil {
		t.Fatal("skill ownership collision was not rejected")
	}
	unchangedContent, err := os.ReadFile(installedPath)
	if err != nil || string(unchangedContent) != string(existingContent) {
		t.Fatalf("blocked install changed file: %q, error=%v", unchangedContent, err)
	}

	forcedResult := runCLI(t, projectDir, t.TempDir(), "install", "alpha", "--force")
	if forcedResult.err != nil {
		t.Fatalf("forced skill install error = %v\nstderr=%s", forcedResult.err, forcedResult.stderr)
	}
	installedState := readStateV2(t, projectDir)
	newOwnerFound := false
	for _, record := range installedState.Entries {
		if record.ResourceName == "skill::other::alpha" && len(record.ManagedFiles) != 0 {
			t.Fatalf("old skill owner still claims destination: %+v", installedState.Entries)
		}
		if record.ResourceName == "skill::skills::alpha" && len(record.ManagedFiles) == 1 {
			newOwnerFound = true
		}
	}
	if !newOwnerFound {
		t.Fatalf("new skill owner missing: %+v", installedState.Entries)
	}
}

func TestRemoveFanoutStaleFilesKeepsCaseAlias(t *testing.T) {
	projectDir := t.TempDir()
	keptRelativePath := ".github/agents/reviewer.md"
	previousRelativePath := ".github/Agents/reviewer.md"
	keptPath := filepath.Join(projectDir, filepath.FromSlash(keptRelativePath))
	if err := os.MkdirAll(filepath.Dir(keptPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keptPath, []byte("reviewer"), 0o600); err != nil {
		t.Fatal(err)
	}
	previousPath := filepath.Join(projectDir, filepath.FromSlash(previousRelativePath))
	keptInfo, keptErr := os.Stat(keptPath)
	previousInfo, previousErr := os.Stat(previousPath)
	if keptErr != nil || previousErr != nil || !os.SameFile(keptInfo, previousInfo) {
		t.Skip("filesystem is case-sensitive")
	}
	plans := []fanoutRecordPlan{{
		record:   fs.InstalledResourceRecordV2{ManagedFiles: []fs.ManagedFile{{Path: keptRelativePath}}},
		previous: fs.InstalledResourceRecordV2{ManagedFiles: []fs.ManagedFile{{Path: previousRelativePath}}},
	}}
	if err := removeFanoutStaleFiles(projectDir, plans); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(keptPath); err != nil {
		t.Fatalf("kept case alias was removed: %v", err)
	}
}

func TestAppendFanoutManagedFileRejectsCaseCollision(t *testing.T) {
	indexes := map[string]int{}
	files, err := appendFanoutManagedFile(nil, indexes, fs.ManagedFile{Path: ".agents/skills/example/README.md", SHA256: "one"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := appendFanoutManagedFile(files, indexes, fs.ManagedFile{Path: ".agents/skills/example/readme.md", SHA256: "two"}); err == nil {
		t.Fatal("expected case-insensitive path collision")
	}
}

func TestInstallFanout_CommandSupportsCopilotCLI(t *testing.T) {
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		switch r.URL.Path {
		case "/owner/repo/main/SKILLS.md":
			_, _ = w.Write([]byte(testCatalogMarkdown("| command | deploy | commands | commands/deploy.md | Deploy |")))
		case "/owner/repo/main/commands/deploy.md":
			_, _ = w.Write([]byte("# Deploy\n"))
		default:
			stdhttp.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)

	writeProjectConfigV2ForTest(t, projectDir, fanoutConfig(
		fanoutTarget("copilot", ".github"),
	))

	result := runCLI(t, projectDir, t.TempDir(), "install", "deploy")
	if result.err != nil {
		t.Fatalf("Run() error = %v\nstderr=%s", result.err, result.stderr)
	}

	assertFileExists(t, filepath.Join(projectDir, ".claude", "commands", "deploy.md"))

	state := readStateV2(t, projectDir)
	if len(state.Entries) != 1 {
		t.Fatalf("state entries = %d, want 1", len(state.Entries))
	}
	if state.Entries[0].TargetID != "copilot" {
		t.Fatalf("target id = %q, want copilot", state.Entries[0].TargetID)
	}
}

func TestInstallFanout_ConsumerArtifactContracts(t *testing.T) {
	testCases := []struct {
		name          string
		targetKind    string
		targetRoot    string
		artifactKind  string
		artifactName  string
		folderName    string
		sourcePath    string
		content       string
		expectedPath  string
		expectedText  string
		forbiddenText string
	}{
		{
			name:       "claude instruction",
			targetKind: "claude", targetRoot: ".claude", artifactKind: "instruction", artifactName: "go-style", folderName: "instructions", sourcePath: "instructions/go-style.md",
			content: "---\ndescription: Go style\n---\nUse descriptive names.\n", expectedPath: ".claude/rules/go-style.md", expectedText: "Use descriptive names.",
		},
		{
			name:       "copilot agent",
			targetKind: "copilot", targetRoot: ".github", artifactKind: "agent", artifactName: "reviewer", folderName: "agents", sourcePath: "agents/reviewer.md",
			content: "---\nname: reviewer\ndescription: Reviews changes\n---\nReview the diff.\n", expectedPath: ".github/agents/reviewer.agent.md", expectedText: "description: Reviews changes",
		},
		{
			name:       "copilot instruction",
			targetKind: "copilot", targetRoot: ".github", artifactKind: "instruction", artifactName: "typescript", folderName: "instructions", sourcePath: "instructions/typescript.md",
			content: "---\ndescription: TypeScript rules\napplyTo: '**/*.ts'\n---\nUse strict mode.\n", expectedPath: ".github/instructions/typescript.instructions.md", expectedText: "applyTo: '**/*.ts'",
		},
		{
			name:       "antigravity instruction",
			targetKind: "antigravity", targetRoot: ".agents", artifactKind: "instruction", artifactName: "markdown", folderName: "instructions", sourcePath: "instructions/markdown.md",
			content: "---\ndescription: Markdown rules\n---\nKeep headings concise.\n", expectedPath: ".agents/rules/markdown.md", expectedText: "Keep headings concise.",
		},
		{
			name:       "gemini agent",
			targetKind: "gemini", targetRoot: ".gemini", artifactKind: "agent", artifactName: "reviewer", folderName: "agents", sourcePath: "agents/reviewer.md",
			content: "---\nname: reviewer\ndescription: Reviews Gemini changes\n---\nReview the diff.\n", expectedPath: ".gemini/agents/reviewer.md", expectedText: "description: Reviews Gemini changes",
		},
		{
			name:       "gemini command",
			targetKind: "gemini", targetRoot: ".gemini", artifactKind: "command", artifactName: "deploy", folderName: "commands", sourcePath: "commands/deploy.md",
			content: "---\ndescription: Deploy safely\n---\nDeploy {{args}} safely.\n", expectedPath: ".gemini/commands/deploy.toml", expectedText: "prompt = \"Deploy {{args}} safely.\"", forbiddenText: "---",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			projectDir := t.TempDir()
			server := singleArtifactServer(t, testCase.artifactKind, testCase.artifactName, testCase.folderName, testCase.sourcePath, testCase.name, testCase.content, nil)
			defer server.Close()
			t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)
			writeProjectConfigV2ForTest(t, projectDir, fanoutConfig(fanoutTarget(testCase.targetKind, testCase.targetRoot)))

			result := runCLI(t, projectDir, t.TempDir(), "install", testCase.artifactName)
			if result.err != nil {
				t.Fatalf("install error = %v\nstderr=%s", result.err, result.stderr)
			}

			installedPath := filepath.Join(projectDir, filepath.FromSlash(testCase.expectedPath))
			installedContent, err := os.ReadFile(installedPath)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(installedContent), testCase.expectedText) {
				t.Errorf("%s does not contain %q: %s", testCase.expectedPath, testCase.expectedText, installedContent)
			}
			if testCase.forbiddenText != "" && strings.Contains(string(installedContent), testCase.forbiddenText) {
				t.Errorf("%s contains forbidden text %q: %s", testCase.expectedPath, testCase.forbiddenText, installedContent)
			}

			state := readStateV2(t, projectDir)
			if len(state.Entries) != 1 || state.Entries[0].TargetID != testCase.targetKind || state.Entries[0].ArtifactKind != testCase.artifactKind {
				t.Fatalf("unexpected state: %+v", state.Entries)
			}
		})
	}
}

func TestInstallFanout_AllConsumerContractsImplemented(t *testing.T) {
	consumerKinds := make(map[string]struct{})
	for _, targetKind := range writer.ConsumerTargetKinds() {
		consumerKinds[targetKind] = struct{}{}
	}
	for _, capability := range writer.ArtifactCapabilities() {
		if _, consumer := consumerKinds[capability.TargetKind]; consumer && !capability.Implemented {
			t.Errorf("contract %s/%s is not implemented: %s", capability.TargetKind, capability.ArtifactKind, capability.Reason)
		}
	}
}

func TestGeminiCommandLifecycle(t *testing.T) {
	projectDir := t.TempDir()
	commandContent := "---\ndescription: Deploy safely\n---\nReturn SENTINEL_V1.\n"
	commitSHA := "1111111111111111"
	server := httptest.NewServer(stdhttp.HandlerFunc(func(responseWriter stdhttp.ResponseWriter, request *stdhttp.Request) {
		switch request.URL.Path {
		case "/owner/repo/main/SKILLS.md":
			_, _ = responseWriter.Write([]byte(testCatalogMarkdown("| command | deploy | commands | commands/deploy.md | Deploy |")))
		case "/owner/repo/main/commands/deploy.md":
			_, _ = responseWriter.Write([]byte(commandContent))
		case "/repos/owner/repo/commits":
			responseWriter.Header().Set("Content-Type", "application/json")
			_, _ = responseWriter.Write([]byte(`[{"sha":"` + commitSHA + `"}]`))
		default:
			stdhttp.NotFound(responseWriter, request)
		}
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)
	t.Setenv("STH_GITHUB_API_BASE_URL", server.URL)
	writeProjectConfigV2ForTest(t, projectDir, fanoutConfig(fanoutTarget("gemini", ".gemini")))

	installResult := runCLI(t, projectDir, t.TempDir(), "install", "deploy")
	if installResult.err != nil {
		t.Fatalf("install error = %v\nstderr=%s", installResult.err, installResult.stderr)
	}
	installedPath := filepath.Join(projectDir, ".gemini", "commands", "deploy.toml")
	versionOne, err := os.ReadFile(installedPath)
	if err != nil || !strings.Contains(string(versionOne), "SENTINEL_V1") {
		t.Fatalf("version one = %q, error=%v", versionOne, err)
	}

	commandContent = "---\ndescription: Deploy safely\n---\nReturn SENTINEL_V2.\n"
	commitSHA = "2222222222222222"
	dryRunResult := runCLI(t, projectDir, t.TempDir(), "update", "--dry-run", "--json")
	if dryRunResult.err != nil {
		t.Fatalf("dry-run error = %v\nstderr=%s", dryRunResult.err, dryRunResult.stderr)
	}
	if !strings.Contains(dryRunResult.stdout, `"action": "modify"`) {
		t.Fatalf("dry-run output = %s", dryRunResult.stdout)
	}
	stillVersionOne, err := os.ReadFile(installedPath)
	if err != nil || string(stillVersionOne) != string(versionOne) {
		t.Fatalf("dry-run mutated file: %q, error=%v", stillVersionOne, err)
	}

	updateResult := runCLI(t, projectDir, t.TempDir(), "update")
	if updateResult.err != nil {
		t.Fatalf("update error = %v\nstderr=%s", updateResult.err, updateResult.stderr)
	}
	versionTwo, err := os.ReadFile(installedPath)
	if err != nil || !strings.Contains(string(versionTwo), "SENTINEL_V2") || strings.Contains(string(versionTwo), "SENTINEL_V1") {
		t.Fatalf("version two = %q, error=%v", versionTwo, err)
	}
	state := readStateV2(t, projectDir)
	if len(state.Entries) != 1 || state.Entries[0].ResolvedVersion != commitSHA || state.Entries[0].PreviousVersion != "1111111111111111" {
		t.Fatalf("state after update = %+v", state.Entries)
	}

	removeResult := runCLI(t, projectDir, t.TempDir(), "remove", "command::commands::deploy", "--yes")
	if removeResult.err != nil {
		t.Fatalf("remove error = %v\nstderr=%s", removeResult.err, removeResult.stderr)
	}
	assertNoFile(t, installedPath)
	if entries := readStateV2(t, projectDir).Entries; len(entries) != 0 {
		t.Fatalf("state entries after remove = %+v", entries)
	}
}

type fanoutLifecycleFixture struct {
	server       *httptest.Server
	artifactKind string
	artifactName string
	folderName   string
	sourcePath   string
	content      string
	commitSHA    string
}

func newFanoutLifecycleFixture(t *testing.T, artifactKind, artifactName, folderName string) *fanoutLifecycleFixture {
	t.Helper()
	sourcePath := folderName + "/" + artifactName + ".md"
	if artifactKind == "skill" {
		sourcePath = folderName + "/" + artifactName + "/SKILL.md"
	}
	fixture := &fanoutLifecycleFixture{
		artifactKind: artifactKind,
		artifactName: artifactName,
		folderName:   folderName,
		sourcePath:   sourcePath,
		content:      lifecycleArtifactContent(artifactKind, artifactName, "SENTINEL_V1"),
		commitSHA:    "1111111111111111",
	}
	fixture.server = httptest.NewServer(stdhttp.HandlerFunc(fixture.serveHTTP))
	t.Cleanup(fixture.server.Close)
	return fixture
}

func (fixture *fanoutLifecycleFixture) serveHTTP(responseWriter stdhttp.ResponseWriter, request *stdhttp.Request) {
	row := fmt.Sprintf("| %s | %s | %s | %s | Lifecycle |", fixture.artifactKind, fixture.artifactName, fixture.folderName, fixture.sourcePath)
	switch request.URL.Path {
	case "/owner/repo/main/SKILLS.md":
		_, _ = responseWriter.Write([]byte(testCatalogMarkdown(row)))
	case "/owner/repo/main/" + fixture.sourcePath:
		_, _ = responseWriter.Write([]byte(fixture.content))
	case "/repos/owner/repo/git/trees/main":
		_, _ = responseWriter.Write([]byte(`{"tree":[{"path":"` + fixture.sourcePath + `","type":"blob"}]}`))
	case "/repos/owner/repo/commits":
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`[{"sha":"` + fixture.commitSHA + `"}]`))
	default:
		stdhttp.NotFound(responseWriter, request)
	}
}

func (fixture *fanoutLifecycleFixture) setVersion(version, commitSHA string) {
	fixture.content = lifecycleArtifactContent(fixture.artifactKind, fixture.artifactName, version)
	fixture.commitSHA = commitSHA
}

func lifecycleArtifactContent(artifactKind, artifactName, sentinel string) string {
	switch artifactKind {
	case "skill":
		return fmt.Sprintf("---\nname: %s\ndescription: Lifecycle skill\n---\nReturn %s.\n", artifactName, sentinel)
	case "agent":
		return fmt.Sprintf("---\nname: %s\ndescription: Lifecycle agent\n---\nReturn %s.\n", artifactName, sentinel)
	case "command":
		return fmt.Sprintf("---\ndescription: Lifecycle command\n---\nReturn %s.\n", sentinel)
	case "instruction":
		return fmt.Sprintf("---\ndescription: Lifecycle instruction\napplyTo: '**/*'\n---\nReturn %s.\n", sentinel)
	}
	return sentinel
}

func TestInstallUpdateStatusRemoveMatrix(t *testing.T) {
	testCases := []struct {
		name            string
		targetKind      string
		targetRoot      string
		artifactKind    string
		artifactName    string
		folderName      string
		expectedPath    string
		additionalPaths []string
		rootPath        string
		rootExpected    string
	}{
		{name: "claude skill", targetKind: "claude", targetRoot: ".claude", artifactKind: "skill", artifactName: "lifecycle-skill", folderName: "skills", expectedPath: ".claude/skills/lifecycle-skill/SKILL.md"},
		{name: "claude agent", targetKind: "claude", targetRoot: ".claude", artifactKind: "agent", artifactName: "lifecycle-agent", folderName: "agents", expectedPath: ".claude/agents/lifecycle-agent.md"},
		{name: "claude command", targetKind: "claude", targetRoot: ".claude", artifactKind: "command", artifactName: "lifecycle-command", folderName: "commands", expectedPath: ".claude/commands/lifecycle-command.md"},
		{name: "claude instruction", targetKind: "claude", targetRoot: ".claude", artifactKind: "instruction", artifactName: "lifecycle-instruction", folderName: "instructions", expectedPath: ".claude/rules/lifecycle-instruction.md"},
		{name: "codex skill", targetKind: "codex", targetRoot: ".codex", artifactKind: "skill", artifactName: "lifecycle-skill", folderName: "skills", expectedPath: ".agents/skills/lifecycle-skill/SKILL.md"},
		{name: "codex agent", targetKind: "codex", targetRoot: ".codex", artifactKind: "agent", artifactName: "lifecycle-agent", folderName: "agents", expectedPath: ".codex/agents/lifecycle-agent.toml"},
		{name: "codex command", targetKind: "codex", targetRoot: ".codex", artifactKind: "command", artifactName: "lifecycle-command", folderName: "commands", expectedPath: ".agents/skills/lifecycle-command/SKILL.md"},
		{name: "codex instruction", targetKind: "codex", targetRoot: ".codex", artifactKind: "instruction", artifactName: "lifecycle-instruction", folderName: "instructions", expectedPath: ".codex/sth/instructions/lifecycle-instruction.md", rootPath: "AGENTS.md", rootExpected: "$SENTINEL"},
		{name: "copilot skill", targetKind: "copilot", targetRoot: ".github", artifactKind: "skill", artifactName: "lifecycle-skill", folderName: "skills", expectedPath: ".claude/skills/lifecycle-skill/SKILL.md"},
		{name: "copilot agent", targetKind: "copilot", targetRoot: ".github", artifactKind: "agent", artifactName: "lifecycle-agent", folderName: "agents", expectedPath: ".github/agents/lifecycle-agent.agent.md"},
		{name: "copilot command", targetKind: "copilot", targetRoot: ".github", artifactKind: "command", artifactName: "lifecycle-command", folderName: "commands", expectedPath: ".claude/commands/lifecycle-command.md", additionalPaths: []string{".github/prompts/lifecycle-command.prompt.md"}},
		{name: "copilot instruction", targetKind: "copilot", targetRoot: ".github", artifactKind: "instruction", artifactName: "lifecycle-instruction", folderName: "instructions", expectedPath: ".github/instructions/lifecycle-instruction.instructions.md"},
		{name: "antigravity skill", targetKind: "antigravity", targetRoot: ".agents", artifactKind: "skill", artifactName: "lifecycle-skill", folderName: "skills", expectedPath: ".agents/skills/lifecycle-skill/SKILL.md"},
		{name: "antigravity agent", targetKind: "antigravity", targetRoot: ".agents", artifactKind: "agent", artifactName: "lifecycle-agent", folderName: "agents", expectedPath: ".agents/agents/lifecycle-agent.md"},
		{name: "antigravity command", targetKind: "antigravity", targetRoot: ".agents", artifactKind: "command", artifactName: "lifecycle-command", folderName: "commands", expectedPath: ".agents/workflows/lifecycle-command.md"},
		{name: "antigravity instruction", targetKind: "antigravity", targetRoot: ".agents", artifactKind: "instruction", artifactName: "lifecycle-instruction", folderName: "instructions", expectedPath: ".agents/rules/lifecycle-instruction.md"},
		{name: "gemini skill", targetKind: "gemini", targetRoot: ".gemini", artifactKind: "skill", artifactName: "lifecycle-skill", folderName: "skills", expectedPath: ".agents/skills/lifecycle-skill/SKILL.md"},
		{name: "gemini agent", targetKind: "gemini", targetRoot: ".gemini", artifactKind: "agent", artifactName: "lifecycle-agent", folderName: "agents", expectedPath: ".gemini/agents/lifecycle-agent.md"},
		{name: "gemini command", targetKind: "gemini", targetRoot: ".gemini", artifactKind: "command", artifactName: "lifecycle-command", folderName: "commands", expectedPath: ".gemini/commands/lifecycle-command.toml"},
		{name: "gemini instruction", targetKind: "gemini", targetRoot: ".gemini", artifactKind: "instruction", artifactName: "lifecycle-instruction", folderName: "instructions", expectedPath: ".gemini/sth/instructions/lifecycle-instruction.md", rootPath: "GEMINI.md", rootExpected: "@./.gemini/sth/instructions/lifecycle-instruction.md"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			projectDir := t.TempDir()
			fixture := newFanoutLifecycleFixture(t, testCase.artifactKind, testCase.artifactName, testCase.folderName)
			t.Setenv("STH_GITHUB_RAW_BASE_URL", fixture.server.URL)
			t.Setenv("STH_GITHUB_API_BASE_URL", fixture.server.URL)
			writeProjectConfigV2ForTest(t, projectDir, fanoutConfig(fanoutTarget(testCase.targetKind, testCase.targetRoot)))

			installResult := runCLI(t, projectDir, t.TempDir(), "install", testCase.artifactName)
			if installResult.err != nil {
				t.Fatalf("install error = %v\nstderr=%s", installResult.err, installResult.stderr)
			}
			expectedPaths := append([]string{testCase.expectedPath}, testCase.additionalPaths...)
			versionOneByPath := make(map[string][]byte, len(expectedPaths))
			for _, expectedPath := range expectedPaths {
				installedPath := filepath.Join(projectDir, filepath.FromSlash(expectedPath))
				versionOne, err := os.ReadFile(installedPath)
				if err != nil || !strings.Contains(string(versionOne), "SENTINEL_V1") {
					t.Fatalf("version one for %s = %q, error=%v", expectedPath, versionOne, err)
				}
				versionOneByPath[expectedPath] = versionOne
			}
			if testCase.rootPath != "" {
				rootContent, err := os.ReadFile(filepath.Join(projectDir, filepath.FromSlash(testCase.rootPath)))
				expectedRootContent := testCase.rootExpected
				if expectedRootContent == "$SENTINEL" {
					expectedRootContent = "SENTINEL_V1"
				}
				if err != nil || !strings.Contains(string(rootContent), expectedRootContent) {
					t.Fatalf("root %s = %q, expected %q, error=%v", testCase.rootPath, rootContent, expectedRootContent, err)
				}
			}
			installedState := readStateV2(t, projectDir)
			if len(installedState.Entries) != 1 || installedState.Entries[0].ResolvedVersion != "1111111111111111" {
				t.Fatalf("installed state = %+v", installedState.Entries)
			}

			fixture.setVersion("SENTINEL_V2", "2222222222222222")
			dryRunResult := runCLI(t, projectDir, t.TempDir(), "update", "--dry-run", "--json")
			if dryRunResult.err != nil || !strings.Contains(dryRunResult.stdout, `"action": "modify"`) {
				t.Fatalf("dry-run error=%v stdout=%s stderr=%s", dryRunResult.err, dryRunResult.stdout, dryRunResult.stderr)
			}
			for _, expectedPath := range expectedPaths {
				unchanged, err := os.ReadFile(filepath.Join(projectDir, filepath.FromSlash(expectedPath)))
				if err != nil || string(unchanged) != string(versionOneByPath[expectedPath]) {
					t.Fatalf("dry-run mutated %s: %q, error=%v", expectedPath, unchanged, err)
				}
			}
			if testCase.rootPath != "" {
				rootContent, err := os.ReadFile(filepath.Join(projectDir, filepath.FromSlash(testCase.rootPath)))
				expectedRootContent := testCase.rootExpected
				if expectedRootContent == "$SENTINEL" {
					expectedRootContent = "SENTINEL_V1"
				}
				if err != nil || !strings.Contains(string(rootContent), expectedRootContent) {
					t.Fatalf("dry-run mutated root %s = %q, expected %q, error=%v", testCase.rootPath, rootContent, expectedRootContent, err)
				}
			}

			updateResult := runCLI(t, projectDir, t.TempDir(), "update")
			if updateResult.err != nil {
				t.Fatalf("update error = %v\nstderr=%s", updateResult.err, updateResult.stderr)
			}
			for _, expectedPath := range expectedPaths {
				versionTwo, err := os.ReadFile(filepath.Join(projectDir, filepath.FromSlash(expectedPath)))
				if err != nil || !strings.Contains(string(versionTwo), "SENTINEL_V2") || strings.Contains(string(versionTwo), "SENTINEL_V1") {
					t.Fatalf("version two for %s = %q, error=%v", expectedPath, versionTwo, err)
				}
			}
			if testCase.rootPath != "" {
				rootContent, err := os.ReadFile(filepath.Join(projectDir, filepath.FromSlash(testCase.rootPath)))
				expectedRootContent := testCase.rootExpected
				if expectedRootContent == "$SENTINEL" {
					expectedRootContent = "SENTINEL_V2"
				}
				if err != nil || !strings.Contains(string(rootContent), expectedRootContent) {
					t.Fatalf("updated root %s = %q, expected %q, error=%v", testCase.rootPath, rootContent, expectedRootContent, err)
				}
				if testCase.rootExpected == "$SENTINEL" && strings.Contains(string(rootContent), "SENTINEL_V1") {
					t.Fatalf("updated root %s still contains SENTINEL_V1: %s", testCase.rootPath, rootContent)
				}
			}
			updatedState := readStateV2(t, projectDir)
			if len(updatedState.Entries) != 1 || updatedState.Entries[0].ResolvedVersion != "2222222222222222" || updatedState.Entries[0].PreviousVersion != "1111111111111111" {
				t.Fatalf("updated state = %+v", updatedState.Entries)
			}

			statusResult := runCLI(t, projectDir, t.TempDir(), "status", "--json")
			if statusResult.err != nil || !strings.Contains(statusResult.stdout, `"status": "up-to-date"`) {
				t.Fatalf("status error=%v stdout=%s stderr=%s", statusResult.err, statusResult.stdout, statusResult.stderr)
			}

			resourceName := fs.CatalogID(testCase.artifactKind, testCase.folderName, testCase.artifactName)
			removeResult := runCLI(t, projectDir, t.TempDir(), "remove", resourceName, "--yes")
			if removeResult.err != nil {
				t.Fatalf("remove error = %v\nstderr=%s", removeResult.err, removeResult.stderr)
			}
			for _, expectedPath := range expectedPaths {
				assertNoFile(t, filepath.Join(projectDir, filepath.FromSlash(expectedPath)))
			}
			if testCase.rootPath != "" {
				assertNoFile(t, filepath.Join(projectDir, filepath.FromSlash(testCase.rootPath)))
			}
			if entries := readStateV2(t, projectDir).Entries; len(entries) != 0 {
				t.Fatalf("state entries after remove = %+v", entries)
			}
		})
	}
}

func TestSkillSynchronizationAcrossAllConsumers(t *testing.T) {
	projectDir := t.TempDir()
	fixture := newFanoutLifecycleFixture(t, "skill", "shared-skill", "skills")
	t.Setenv("STH_GITHUB_RAW_BASE_URL", fixture.server.URL)
	t.Setenv("STH_GITHUB_API_BASE_URL", fixture.server.URL)
	writeProjectConfigV2ForTest(t, projectDir, fanoutConfig(
		fanoutTarget("claude", ".claude"),
		fanoutTarget("codex", ".codex"),
		fanoutTarget("copilot", ".github"),
		fanoutTarget("antigravity", ".agents"),
		fanoutTarget("gemini", ".gemini"),
	))

	installResult := runCLI(t, projectDir, t.TempDir(), "install", "shared-skill")
	if installResult.err != nil {
		t.Fatalf("install error = %v\nstderr=%s", installResult.err, installResult.stderr)
	}
	claudePath := filepath.Join(projectDir, ".claude", "skills", "shared-skill", "SKILL.md")
	agentsPath := filepath.Join(projectDir, ".agents", "skills", "shared-skill", "SKILL.md")
	for _, installedPath := range []string{claudePath, agentsPath} {
		content, err := os.ReadFile(installedPath)
		if err != nil || !strings.Contains(string(content), "SENTINEL_V1") {
			t.Fatalf("installed path %s content=%q error=%v", installedPath, content, err)
		}
	}
	state := readStateV2(t, projectDir)
	if len(state.Entries) != 1 || len(state.Entries[0].ManagedFiles) != 2 {
		t.Fatalf("shared skill state = %+v", state.Entries)
	}
	for _, targetKind := range []string{"claude", "codex", "copilot", "antigravity", "gemini"} {
		if !strings.Contains(installResult.stdout, targetKind) {
			t.Errorf("install output does not name consumer %s: %s", targetKind, installResult.stdout)
		}
	}

	fixture.setVersion("SENTINEL_V2", "2222222222222222")
	updateResult := runCLI(t, projectDir, t.TempDir(), "update")
	if updateResult.err != nil {
		t.Fatalf("update error = %v\nstderr=%s", updateResult.err, updateResult.stderr)
	}
	for _, installedPath := range []string{claudePath, agentsPath} {
		content, err := os.ReadFile(installedPath)
		if err != nil || !strings.Contains(string(content), "SENTINEL_V2") || strings.Contains(string(content), "SENTINEL_V1") {
			t.Fatalf("updated path %s content=%q error=%v", installedPath, content, err)
		}
	}

	removeResult := runCLI(t, projectDir, t.TempDir(), "remove", "skill::skills::shared-skill", "--yes")
	if removeResult.err != nil {
		t.Fatalf("remove error = %v\nstderr=%s", removeResult.err, removeResult.stderr)
	}
	assertNoFile(t, claudePath)
	assertNoFile(t, agentsPath)
	if entries := readStateV2(t, projectDir).Entries; len(entries) != 0 {
		t.Fatalf("state entries after remove = %+v", entries)
	}
}

func TestRemoveFanout_DeletesAllCopies(t *testing.T) {
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		switch r.URL.Path {
		case "/owner/repo/main/SKILLS.md":
			_, _ = w.Write([]byte(testCatalogMarkdown("| agent | reviewer | agents | agents/reviewer.md | Reviewer |")))
		case "/owner/repo/main/agents/reviewer.md":
			_, _ = w.Write([]byte("# Reviewer\n"))
		default:
			stdhttp.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)

	writeProjectConfigV2ForTest(t, projectDir, fanoutConfig(
		fanoutTarget("claude", ".claude"),
		fanoutTarget("cursor", ".cursor"),
		fanoutTarget("antigravity", ".agents"),
	))

	install := runCLI(t, projectDir, t.TempDir(), "install", "reviewer")
	if install.err != nil {
		t.Fatalf("install error = %v\nstderr=%s", install.err, install.stderr)
	}

	remove := runCLI(t, projectDir, t.TempDir(), "remove", "agent::agents::reviewer", "--yes")
	if remove.err != nil {
		t.Fatalf("remove error = %v\nstderr=%s", remove.err, remove.stderr)
	}

	assertNoFile(t, filepath.Join(projectDir, ".claude", "agents", "reviewer.md"))
	assertNoFile(t, filepath.Join(projectDir, ".cursor", "rules", "agents", "reviewer.mdc"))
	assertNoFile(t, filepath.Join(projectDir, ".agents", "agents", "reviewer.md"))

	state := readStateV2(t, projectDir)
	if len(state.Entries) != 0 {
		t.Fatalf("state entries = %d, want 0 after remove", len(state.Entries))
	}
}

func TestInstallFanout_RecordsResolvedVersion(t *testing.T) {
	projectDir := t.TempDir()
	server := skillBundleServer(t)
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)
	t.Setenv("STH_GITHUB_API_BASE_URL", server.URL)

	writeProjectConfigV2ForTest(t, projectDir, fanoutConfig(
		fanoutTarget("claude", ".claude"),
	))

	result := runCLI(t, projectDir, t.TempDir(), "install", "alpha")
	if result.err != nil {
		t.Fatalf("Run() error = %v\nstderr=%s", result.err, result.stderr)
	}

	state := readStateV2(t, projectDir)
	if len(state.Entries) == 0 {
		t.Fatal("no entries recorded")
	}
	for _, entry := range state.Entries {
		if entry.ResolvedVersion != fanoutCommitSHA {
			t.Errorf("ResolvedVersion = %q, want %q", entry.ResolvedVersion, fanoutCommitSHA)
		}
		if entry.SourcePath != "skills/alpha/SKILL.md" {
			t.Errorf("SourcePath = %q, want %q", entry.SourcePath, "skills/alpha/SKILL.md")
		}
	}

	statusResult := runCLI(t, projectDir, t.TempDir(), "status")
	if statusResult.err != nil {
		t.Fatalf("status error = %v\nstderr=%s", statusResult.err, statusResult.stderr)
	}
	if strings.Contains(statusResult.stdout, "outdated") {
		t.Errorf("status should not report outdated:\n%s", statusResult.stdout)
	}
}

func TestUpdateFanout_PreservesResolvedVersionOnResolveFailure(t *testing.T) {
	projectDir := t.TempDir()
	commitsSHA := fanoutCommitSHA
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		switch r.URL.Path {
		case "/owner/repo/main/SKILLS.md":
			_, _ = w.Write([]byte(testCatalogMarkdown("| skill | alpha | skills | skills/alpha/SKILL.md | Alpha |")))
		case "/repos/owner/repo/git/trees/main":
			_, _ = w.Write([]byte(`{"tree":[{"path":"skills/alpha/SKILL.md","type":"blob"}]}`))
		case "/owner/repo/main/skills/alpha/SKILL.md":
			_, _ = w.Write([]byte("# Alpha skill\n"))
		case "/repos/owner/repo/commits":
			if commitsSHA == "" {
				stdhttp.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"sha":"` + commitsSHA + `"}]`))
		default:
			stdhttp.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)
	t.Setenv("STH_GITHUB_API_BASE_URL", server.URL)

	writeProjectConfigV2ForTest(t, projectDir, fanoutConfig(fanoutTarget("claude", ".claude")))

	if result := runCLI(t, projectDir, t.TempDir(), "install", "alpha"); result.err != nil {
		t.Fatalf("install error = %v\nstderr=%s", result.err, result.stderr)
	}
	if got := readStateV2(t, projectDir).Entries[0].ResolvedVersion; got != fanoutCommitSHA {
		t.Fatalf("after install ResolvedVersion = %q, want %q", got, fanoutCommitSHA)
	}

	commitsSHA = ""
	if result := runCLI(t, projectDir, t.TempDir(), "update"); result.err != nil {
		t.Fatalf("update error = %v\nstderr=%s", result.err, result.stderr)
	}
	if got := readStateV2(t, projectDir).Entries[0].ResolvedVersion; got != fanoutCommitSHA {
		t.Errorf("after failed-resolve update ResolvedVersion = %q, want preserved %q", got, fanoutCommitSHA)
	}
}
