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

func TestRun_installCreatesTargetFilesAndState(t *testing.T) {
	homeDir := t.TempDir()
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if r.URL.Path != "/alpha.md" {
			stdhttp.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("# Alpha\n"))
	}))
	defer server.Close()

	writeConfigForCommandTest(t, homeDir, fs.Config{
		RepoRawURL: server.URL,
		TargetDir:  ".claude/skills",
	})

	result := runCLI(t, projectDir, homeDir, "install", "alpha")

	if result.err != nil {
		t.Fatalf("Run() error = %v\nstderr=%s", result.err, result.stderr)
	}
	if !strings.Contains(result.stdout, "Skill 'alpha' installé") {
		t.Fatalf("stdout %q does not contain install confirmation", result.stdout)
	}

	skillPath := filepath.Join(projectDir, ".claude", "skills", "alpha.md")
	data, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", skillPath, err)
	}
	if !strings.HasPrefix(string(data), "> ⚠️ ATTENTION : Skill géré par LoreSkills. Ne pas modifier manuellement.\n\n") {
		t.Fatalf("skill file %q does not contain managed header", data)
	}

	statePath := filepath.Join(projectDir, ".claude", "skills", "lore-state.json")
	stateData, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", statePath, err)
	}

	var state map[string]string
	if err := json.Unmarshal(stateData, &state); err != nil {
		t.Fatalf("json.Unmarshal(): %v", err)
	}
	if got := state["alpha"]; got != "installed" {
		t.Fatalf("state[alpha] = %q, want installed", got)
	}
}

func TestRun_installReportsMissingRemoteSkill(t *testing.T) {
	homeDir := t.TempDir()
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.NotFoundHandler())
	defer server.Close()

	writeConfigForCommandTest(t, homeDir, fs.Config{
		RepoRawURL: server.URL,
		TargetDir:  ".claude/skills",
	})

	result := runCLI(t, projectDir, homeDir, "install", "missing")

	if result.err == nil {
		t.Fatal("Run() error = nil, want error")
	}
	if !strings.Contains(result.stderr, "404") {
		t.Fatalf("stderr %q does not contain 404 details", result.stderr)
	}

	skillPath := filepath.Join(projectDir, ".claude", "skills", "missing.md")
	if _, err := os.Stat(skillPath); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be absent, stat err = %v", skillPath, err)
	}
}

func writeConfigForCommandTest(t *testing.T, homeDir string, config fs.Config) {
	t.Helper()

	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("json.Marshal(): %v", err)
	}
	configPath := filepath.Join(homeDir, ".loreskillsrc")
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatalf("WriteFile(%s): %v", configPath, err)
	}
}
