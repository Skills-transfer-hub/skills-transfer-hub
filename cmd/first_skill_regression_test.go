package cmd

import (
	"errors"
	stdhttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoveAcceptsInstallationReferences(t *testing.T) {
	server := skillBundleServer(t)
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)
	t.Setenv("STH_GITHUB_API_BASE_URL", server.URL)
	for _, reference := range []string{"alpha", "skills/alpha", "default:skills/alpha", "skill::skills::alpha"} {
		t.Run(reference, func(t *testing.T) {
			projectDir, homeDir := t.TempDir(), t.TempDir()
			writeProjectConfigV2ForTest(t, projectDir, fanoutConfig(fanoutTarget("claude", ".claude")))
			result := runCLI(t, projectDir, homeDir, "install", "skills/alpha")
			if result.err != nil {
				t.Fatal(result.err)
			}
			result = runCLI(t, projectDir, homeDir, "remove", reference, "--yes")
			if result.err != nil {
				t.Fatal(result.err)
			}
			assertNoFile(t, filepath.Join(projectDir, ".claude/skills/alpha/SKILL.md"))
			if entries := readStateV2(t, projectDir).Entries; len(entries) != 0 {
				t.Fatalf("remaining records: %+v", entries)
			}
		})
	}
}

func TestRemoveAmbiguousReferencePreservesAllCandidates(t *testing.T) {
	selections := []installedSelection{{providerID: "public", resourceName: "bundle::skills::alpha"}, {providerID: "private", resourceName: "bundle::skills::alpha"}}
	if _, err := resolveInstalledSelection(selections, "alpha", ""); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("expected actionable ambiguity, got %v", err)
	}
	selected, err := resolveInstalledSelection(selections, "private:skills/alpha", "")
	if err != nil || selected.providerID != "private" {
		t.Fatalf("wrong source: %+v, %v", selected, err)
	}
}

func TestFailedInitLeavesConnectionUnverified(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	t.Setenv("STH_USER_CONFIG_DIR", t.TempDir())
	application := newApp(nil, nil, nil, nil)
	draft := wizardProfile{Alias: "public", Provider: "github", RepositoryID: "owner/repo", Ref: "main", CatalogPath: "SKILLS.md", TargetDir: ".sth", Targets: []string{"claude"}}
	if _, err := application.collectInitDraftOutput(draft, errors.New("connection refused")); err == nil {
		t.Fatal("expected validation failure")
	}
	state := application.loadGuidedStartupState()
	if state.ProjectState != projectStateConnectionUnverified {
		t.Fatalf("unexpected state: %+v", state)
	}
	if label := projectStateLabel(state.ProjectState); label != "connexion non vérifiée" {
		t.Fatal(label)
	}
	if _, err := os.Stat(filepath.Join(projectDir, ".sth/project.json")); err != nil {
		t.Fatal(err)
	}
}

func TestRemoveScannedBundleUsesSameReference(t *testing.T) {
	server := httptest.NewServer(stdhttp.HandlerFunc(func(response stdhttp.ResponseWriter, request *stdhttp.Request) {
		switch request.URL.Path {
		case "/repos/owner/repo/git/trees/main":
			response.Write([]byte(`{"tree":[{"path":"skills/alpha/SKILL.md","type":"blob"}]}`))
		case "/owner/repo/main/skills/alpha/SKILL.md":
			response.Write([]byte("---\nname: alpha\ndescription: Alpha skill\n---\nUse alpha.\n"))
		case "/repos/owner/repo/commits":
			response.Write([]byte(`[{"sha":"abc123"}]`))
		default:
			stdhttp.NotFound(response, request)
		}
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)
	t.Setenv("STH_GITHUB_API_BASE_URL", server.URL)
	projectDir, homeDir := t.TempDir(), t.TempDir()
	config := fanoutConfig(fanoutTarget("codex", ".codex"))
	config.Providers[0].TargetDir = ".sth/secondary"
	writeProjectConfigV2ForTest(t, projectDir, config)
	for _, args := range [][]string{{"install", "skills/alpha"}, {"remove", "skills/alpha", "--yes"}} {
		result := runCLI(t, projectDir, homeDir, args...)
		if result.err != nil {
			t.Fatalf("%v: %v %s", args, result.err, result.stderr)
		}
	}
	assertNoFile(t, filepath.Join(projectDir, ".agents/skills/alpha/SKILL.md"))
}
