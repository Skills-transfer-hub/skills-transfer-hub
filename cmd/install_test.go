package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sth/internal/fs"
	stdhttp "net/http"
	"net/http/httptest"
)

func TestRun_installCreatesTargetFilesAndState(t *testing.T) {
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

	writeProjectProfileForCommandTest(t, projectDir, fs.ProjectProfile{
		Provider:     "github",
		RepositoryID: "owner/repo",
		Ref:          "main",
		CatalogPath:  "SKILLS.md",
		TargetDir:    ".claude/resources",
	})

	result := runCLI(t, projectDir, t.TempDir(), "install", "alpha")

	if result.err != nil {
		t.Fatalf("Run() error = %v\nstderr=%s", result.err, result.stderr)
	}
	if !strings.Contains(result.stdout, "Ressource 'alpha' installée") {
		t.Fatalf("stdout %q does not contain install confirmation", result.stdout)
	}

	skillPath := filepath.Join(projectDir, ".claude", "resources", "skills", "alpha.md")
	data, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", skillPath, err)
	}
	if !strings.HasPrefix(string(data), "> ⚠️ ATTENTION : Skill géré par STH. Ne pas modifier manuellement.\n\n") {
		t.Fatalf("skill file %q does not contain managed header", data)
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
	if len(state.Entries) != 1 {
		t.Fatalf("len(state.Entries) = %d, want 1", len(state.Entries))
	}
	if got := state.Entries[0].CatalogID; got != fs.CatalogID("skill", "skills", "alpha") {
		t.Fatalf("state.Entries[0].CatalogID = %q", got)
	}
}

func TestRun_installRejectsAmbiguousResourceName(t *testing.T) {
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if r.URL.Path != "/owner/repo/main/SKILLS.md" {
			stdhttp.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(testCatalogMarkdown(
			"| skill | alpha | skills | skills/alpha.md | Alpha |",
			"| agent | alpha | agents | agents/alpha.md | Agent Alpha |",
		)))
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

	result := runCLI(t, projectDir, t.TempDir(), "install", "alpha")

	if result.err == nil {
		t.Fatal("Run() error = nil, want error")
	}
	if !strings.Contains(result.stderr, "ambiguë") {
		t.Fatalf("stderr %q does not contain ambiguous-resource details", result.stderr)
	}
}
