package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
)

type globalReinstallFixture struct {
	home          string
	configuration string
	project       string
	revision      atomic.Int32
}

func newGlobalReinstallFixture(t *testing.T) *globalReinstallFixture {
	t.Helper()
	fixture := &globalReinstallFixture{home: t.TempDir(), configuration: t.TempDir(), project: t.TempDir()}
	fixture.revision.Store(1)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch {
		case request.URL.Path == "/repos/example/changing":
			fmt.Fprint(response, `{"default_branch":"main"}`)
		case strings.HasPrefix(request.URL.Path, "/repos/example/changing/commits/"):
			reference := strings.TrimPrefix(request.URL.Path, "/repos/example/changing/commits/")
			if len(reference) != 40 {
				reference = strings.Repeat(fmt.Sprint(fixture.revision.Load()), 40)
			}
			fmt.Fprintf(response, `{"sha":%q}`, reference)
		case strings.HasPrefix(request.URL.Path, "/repos/example/changing/git/trees/"):
			revision := strings.TrimPrefix(request.URL.Path, "/repos/example/changing/git/trees/")
			name := globalReinstallSkillForRevision(revision)
			if name == "" {
				fmt.Fprint(response, `{"tree":[{"path":"README.md","mode":"100644","type":"blob"}],"truncated":false}`)
				return
			}
			fmt.Fprintf(response, `{"tree":[{"path":%q,"mode":"100644","type":"blob"}],"truncated":false}`, "skills/"+name+"/SKILL.md")
		case strings.HasPrefix(request.URL.Path, "/example/changing/"):
			parts := strings.SplitN(strings.TrimPrefix(request.URL.Path, "/example/changing/"), "/", 2)
			if len(parts) != 2 || len(parts[0]) != 40 {
				t.Errorf("download not pinned: %s", request.URL.Path)
				http.Error(response, "invalid revision", 400)
				return
			}
			name := globalReinstallSkillForRevision(parts[0])
			if name == "" || parts[1] != "skills/"+name+"/SKILL.md" {
				http.NotFound(response, request)
				return
			}
			fmt.Fprintf(response, "---\nname: %s\ndescription: Revision-specific skill\n---\n%s\n", name, parts[0])
		default:
			http.NotFound(response, request)
		}
	}))
	t.Cleanup(server.Close)
	t.Setenv("STH_GITHUB_API_BASE_URL", server.URL)
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)
	t.Setenv("STH_TOKEN", "fixture-token")
	return fixture
}

func globalReinstallSkillForRevision(revision string) string {
	switch revision {
	case strings.Repeat("1", 40):
		return "alpha"
	case strings.Repeat("2", 40):
		return "beta"
	case strings.Repeat("3", 40):
		return "gamma"
	default:
		return ""
	}
}

func (fixture *globalReinstallFixture) run(t *testing.T, arguments ...string) (globalResult, error) {
	t.Helper()
	result := runCLIWithOptions(t, cliOptions{workDir: fixture.project, homeDir: fixture.home, userConfigDir: fixture.configuration, skipTelemetrySeed: true, args: append(arguments, "--global", "--json")})
	var envelope globalResult
	if err := json.Unmarshal([]byte(result.stdout), &envelope); err != nil {
		t.Fatalf("invalid envelope %q: %v (%v)", result.stdout, err, result.err)
	}
	return envelope, result.err
}

func TestGlobalReinstallSkillsAtDifferentRevisionsPreservesEachRevision(t *testing.T) {
	fixture := newGlobalReinstallFixture(t)
	if _, err := fixture.run(t, "install", "github:example/changing", "--skill", "alpha", "--agent", "codex"); err != nil {
		t.Fatal(err)
	}
	fixture.revision.Store(2)
	if _, err := fixture.run(t, "install", "github:example/changing", "--skill", "beta", "--agent", "codex"); err != nil {
		t.Fatal(err)
	}
	fixture.revision.Store(4)
	result, err := fixture.run(t, "install", "github:example/changing", "--skill", "alpha", "--skill", "beta", "--agent", "claude")
	if err != nil {
		t.Fatalf("adding an agent must reuse each installed revision even when the selected skills no longer coexist upstream: %v", err)
	}
	assertGlobalReinstalledRevisions(t, fixture, result, map[string]string{"alpha": strings.Repeat("1", 40), "beta": strings.Repeat("2", 40)})
}

func TestGlobalReinstallSingleInstalledSkillSurvivesRemovalFromHead(t *testing.T) {
	fixture := newGlobalReinstallFixture(t)
	if _, err := fixture.run(t, "install", "github:example/changing", "--agent", "codex"); err != nil {
		t.Fatal(err)
	}
	fixture.revision.Store(4)
	result, err := fixture.run(t, "install", "github:example/changing", "--agent", "claude")
	if err != nil {
		t.Fatalf("reinstall without a ref must preserve the installed skill and revision after upstream deletion: %v", err)
	}
	assertGlobalReinstalledRevisions(t, fixture, result, map[string]string{"alpha": strings.Repeat("1", 40)})
}

func TestGlobalReinstallWildcardRetainsInstalledSkillsAndAddsNewSkills(t *testing.T) {
	fixture := newGlobalReinstallFixture(t)
	if _, err := fixture.run(t, "install", "github:example/changing", "--skill", "alpha", "--agent", "codex"); err != nil {
		t.Fatal(err)
	}
	fixture.revision.Store(3)
	result, err := fixture.run(t, "install", "github:example/changing", "--skill", "*", "--agent", "claude")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Resources) != 2 {
		t.Fatalf("wildcard lost the installed skill while adding new upstream skills: %#v", result.Resources)
	}
	for _, resource := range result.Resources {
		if resource.SkillName == "alpha" && (resource.ResolvedRevision != strings.Repeat("1", 40) || !slices.Contains(resource.RequestedConsumers, "codex") || !slices.Contains(resource.RequestedConsumers, "claude")) {
			t.Fatalf("existing wildcard skill changed or lost consumers: %#v", resource)
		}
		if resource.SkillName == "gamma" && resource.ResolvedRevision != strings.Repeat("3", 40) {
			t.Fatalf("new wildcard skill has wrong revision: %#v", resource)
		}
	}
}

func assertGlobalReinstalledRevisions(t *testing.T, fixture *globalReinstallFixture, result globalResult, expected map[string]string) {
	t.Helper()
	if len(result.Resources) != len(expected) {
		t.Fatalf("unexpected resources %#v", result.Resources)
	}
	for _, resource := range result.Resources {
		revision, exists := expected[resource.SkillName]
		if !exists || resource.ResolvedRevision != revision || !slices.Contains(resource.RequestedConsumers, "codex") || !slices.Contains(resource.RequestedConsumers, "claude") {
			t.Fatalf("reinstallation changed revision or lost consumers: %#v", resource)
		}
		for _, root := range []string{".agents", ".claude"} {
			content, err := os.ReadFile(filepath.Join(fixture.home, root, "skills", resource.SkillName, "SKILL.md"))
			if err != nil || !strings.Contains(string(content), revision) {
				t.Fatalf("wrong %s files for %s: %s %v", root, resource.SkillName, content, err)
			}
		}
	}
}
