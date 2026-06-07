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

func TestInstallFanout_CodexConfigEntry(t *testing.T) {
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

	assertFileExists(t, filepath.Join(projectDir, ".claude", "skills", "alpha", "SKILL.md"))
	configData, err := os.ReadFile(filepath.Join(projectDir, ".codex", "config.toml"))
	if err != nil {
		t.Fatalf("ReadFile(.codex/config.toml): %v", err)
	}
	content := string(configData)
	if !strings.Contains(content, "[[skills.config]]") || !strings.Contains(content, `path = ".claude/skills/alpha/SKILL.md"`) {
		t.Fatalf("codex config missing skill entry: %q", content)
	}
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
