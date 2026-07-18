package cmd

import (
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sth/internal/fs"
)

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
			_, _ = w.Write([]byte("# Alpha skill\n"))
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
	assertFileExists(t, filepath.Join(projectDir, ".claude", "skills", "alpha", "alpha.md"))
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
	))

	result := runCLI(t, projectDir, t.TempDir(), "install", "reviewer")
	if result.err != nil {
		t.Fatalf("Run() error = %v\nstderr=%s", result.err, result.stderr)
	}

	assertFileExists(t, filepath.Join(projectDir, ".claude", "agents", "reviewer.md"))
	assertFileExists(t, filepath.Join(projectDir, ".cursor", "rules", "agents", "reviewer.mdc"))
	assertFileExists(t, filepath.Join(projectDir, ".github", "agents", "reviewer.agent.md"))
	assertFileExists(t, filepath.Join(projectDir, ".agents", "agents", "reviewer.md"))

	state := readStateV2(t, projectDir)
	if len(state.Entries) != 4 {
		t.Fatalf("state entries = %d, want 4 (one per target)", len(state.Entries))
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

func TestInstallFanout_CommandSkipsCopilot(t *testing.T) {
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
		fanoutTarget("claude", ".claude"),
		fanoutTarget("copilot", ".github"),
	))

	result := runCLI(t, projectDir, t.TempDir(), "install", "deploy")
	if result.err != nil {
		t.Fatalf("Run() error = %v\nstderr=%s", result.err, result.stderr)
	}

	assertFileExists(t, filepath.Join(projectDir, ".claude", "commands", "deploy.md"))
	assertContains(t, result.stdout, "copilot")

	state := readStateV2(t, projectDir)
	if len(state.Entries) != 1 {
		t.Fatalf("state entries = %d, want 1 (copilot command skipped)", len(state.Entries))
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
