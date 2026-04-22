package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"loreskills/internal/fs"
	stdhttp "net/http"
	"net/http/httptest"
)

func TestRun_updateWithNoStateShowsNoopMessage(t *testing.T) {
	homeDir := t.TempDir()
	projectDir := t.TempDir()

	writeConfigForCommandTest(t, homeDir, fs.Config{
		RepoRawURL: "https://example.invalid",
		TargetDir:  ".claude/skills",
	})

	result := runCLI(t, projectDir, homeDir, "update")

	if result.err != nil {
		t.Fatalf("Run() error = %v", result.err)
	}
	if !strings.Contains(result.stdout, asciiGuide) {
		t.Fatalf("stdout %q does not contain ascii guide", result.stdout)
	}
	if !strings.Contains(result.stdout, "Aucun skill à mettre à jour.") {
		t.Fatalf("stdout %q does not contain noop message", result.stdout)
	}
}

func TestRun_updateRefreshesEveryTrackedSkill(t *testing.T) {
	homeDir := t.TempDir()
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		switch r.URL.Path {
		case "/alpha.md":
			_, _ = w.Write([]byte("# Alpha refreshed\n"))
		case "/beta.md":
			_, _ = w.Write([]byte("# Beta refreshed\n"))
		default:
			stdhttp.NotFound(w, r)
		}
	}))
	defer server.Close()

	writeConfigForCommandTest(t, homeDir, fs.Config{
		RepoRawURL: server.URL,
		TargetDir:  ".claude/skills",
	})
	writeStateForCommandTest(t, projectDir, ".claude/skills", map[string]string{
		"alpha": "installed",
		"beta":  "installed",
	})

	result := runCLI(t, projectDir, homeDir, "update")

	if result.err != nil {
		t.Fatalf("Run() error = %v\nstderr=%s", result.err, result.stderr)
	}
	if !strings.Contains(result.stdout, "Skill 'alpha' mis à jour") {
		t.Fatalf("stdout %q does not contain alpha update", result.stdout)
	}
	if !strings.Contains(result.stdout, "Skill 'beta' mis à jour") {
		t.Fatalf("stdout %q does not contain beta update", result.stdout)
	}

	for _, skill := range []string{"alpha", "beta"} {
		skillPath := filepath.Join(projectDir, ".claude", "skills", skill+".md")
		data, err := os.ReadFile(skillPath)
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", skillPath, err)
		}
		if !strings.Contains(string(data), "refreshed") {
			t.Fatalf("skill %s was not refreshed: %q", skill, data)
		}
	}
}

func TestRun_updateReportsPartialFailures(t *testing.T) {
	homeDir := t.TempDir()
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if r.URL.Path == "/alpha.md" {
			_, _ = w.Write([]byte("# Alpha refreshed\n"))
			return
		}
		stdhttp.NotFound(w, r)
	}))
	defer server.Close()

	writeConfigForCommandTest(t, homeDir, fs.Config{
		RepoRawURL: server.URL,
		TargetDir:  ".claude/skills",
	})
	writeStateForCommandTest(t, projectDir, ".claude/skills", map[string]string{
		"alpha":   "installed",
		"missing": "installed",
	})

	result := runCLI(t, projectDir, homeDir, "update")

	if result.err == nil {
		t.Fatal("Run() error = nil, want error")
	}
	if !strings.Contains(result.stdout, "Skill 'alpha' mis à jour") {
		t.Fatalf("stdout %q does not contain successful update", result.stdout)
	}
	if !strings.Contains(result.stderr, "missing") {
		t.Fatalf("stderr %q does not mention the failing skill", result.stderr)
	}
}

func writeStateForCommandTest(t *testing.T, projectDir, targetDir string, state map[string]string) {
	t.Helper()

	targetPath := filepath.Join(projectDir, filepath.FromSlash(targetDir))
	if err := os.MkdirAll(targetPath, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", targetPath, err)
	}

	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("json.Marshal(): %v", err)
	}
	statePath := filepath.Join(targetPath, "lore-state.json")
	if err := os.WriteFile(statePath, data, 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", statePath, err)
	}
}
