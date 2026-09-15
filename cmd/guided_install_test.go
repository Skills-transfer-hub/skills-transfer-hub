package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	stdhttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"sth/internal/fs"
	sthhttp "sth/internal/http"
)

func TestLargeCatalogKeepsDescriptionsWhenOneFileIsMissing(t *testing.T) {
	server := httptest.NewServer(stdhttp.HandlerFunc(func(response stdhttp.ResponseWriter, request *stdhttp.Request) {
		if strings.Contains(request.URL.Path, "skill-00/") {
			stdhttp.NotFound(response, request)
			return
		}
		_, _ = response.Write([]byte("---\ndescription: Description utile\n---\nSkill\n"))
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)
	entries := make([]fs.CatalogEntry, 32)
	for index := range entries {
		name := fmt.Sprintf("skill-%02d", index)
		entries[index] = fs.CatalogEntry{ProviderID: "public", ResourceType: "bundle", ArtifactKind: "skill", Name: name, FolderName: "skills", SourcePath: "skills/" + name + "/SKILL.md"}
	}
	application := newApp(nil, nil, nil, sthhttp.NewClient(server.Client()))
	application.describeCatalogSkills(context.Background(), multiProviderConfig(), entries)
	for index, entry := range entries {
		if index == 0 {
			assertContains(t, entry.Description, "inaccessible")
		} else if entry.Description != "Description utile" {
			t.Fatalf("description lost for %s: %s", entry.Name, entry.Description)
		}
	}
}

func TestScannedSkillDescriptionAppearsInMenuTextAndJSON(t *testing.T) {
	description := "Créer des graphiques clairs à partir de données."
	server := httptest.NewServer(stdhttp.HandlerFunc(func(response stdhttp.ResponseWriter, request *stdhttp.Request) {
		switch request.URL.Path {
		case "/repos/owner/repo/git/trees/main":
			_, _ = response.Write([]byte(`{"tree":[{"path":"skills/alpha/SKILL.md","type":"blob"}]}`))
		case "/owner/repo/main/skills/alpha/SKILL.md":
			_, _ = response.Write([]byte("---\nname: alpha\ndescription: >-\n  Créer des graphiques clairs\n  à partir de données.\n---\n# Alpha\n"))
		case "/repos/owner/repo/commits":
			_, _ = response.Write([]byte(`[{"sha":"` + fanoutCommitSHA + `"}]`))
		default:
			stdhttp.NotFound(response, request)
		}
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)
	t.Setenv("STH_GITHUB_API_BASE_URL", server.URL)
	projectDir, homeDir := t.TempDir(), t.TempDir()
	result := runCLIWithOptions(t, cliOptions{workDir: projectDir, homeDir: homeDir, input: "install\nowner/repo\nalpha\nclaude\nn\nquit\n", interactive: func() bool { return true }})
	if result.err != nil {
		t.Fatalf("%v\n%s", result.err, result.stdout)
	}
	assertContains(t, result.stdout, description)
	for _, arguments := range [][]string{{"list"}, {"list", "--json"}} {
		result := runCLI(t, projectDir, homeDir, arguments...)
		if result.err != nil {
			t.Fatal(result.err)
		}
		assertContains(t, result.stdout, description)
	}
	assertNoFile(t, filepath.Join(projectDir, ".claude/skills/alpha/SKILL.md"))
}

func TestFirstSkillMenuFromEmptyFolder(t *testing.T) {
	server := skillBundleServer(t)
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)
	t.Setenv("STH_GITHUB_API_BASE_URL", server.URL)
	projectDir := t.TempDir()
	result := runCLIWithOptions(t, cliOptions{workDir: projectDir, homeDir: t.TempDir(), input: "install\nowner/repo\nalpha\nclaude\ny\nquit\n", interactive: func() bool { return true }})
	if result.err != nil {
		t.Fatalf("%v\n%s\n%s", result.err, result.stdout, result.stderr)
	}
	for _, expected := range []string{"Alpha", "Choisir son assistant", "Fichiers concernés", ".claude/skills/alpha/SKILL.md", "Pour essayer", "/alpha", "sth remove"} {
		assertContains(t, result.stdout, expected)
	}
	assertFileExists(t, filepath.Join(projectDir, ".claude/skills/alpha/SKILL.md"))
	if strings.Index(result.stdout, "Fichiers concernés") > strings.Index(result.stdout, "Installer ce skill") {
		t.Fatal("confirmation happened before file preview")
	}
}

func TestFirstSkillCancelWritesNoSkillFiles(t *testing.T) {
	server := skillBundleServer(t)
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)
	t.Setenv("STH_GITHUB_API_BASE_URL", server.URL)
	projectDir := t.TempDir()
	result := runCLIWithOptions(t, cliOptions{workDir: projectDir, homeDir: t.TempDir(), input: "install\nowner/repo\nalpha\nclaude\nn\nquit\n", interactive: func() bool { return true }})
	if result.err != nil {
		t.Fatalf("%v\n%s", result.err, result.stdout)
	}
	assertNoFile(t, filepath.Join(projectDir, ".claude/skills/alpha/SKILL.md"))
	assertContains(t, result.stdout, "Installation annulée")
}

func TestCatalogTextAndJSONDescribeSameRemoteSkills(t *testing.T) {
	server := skillBundleServer(t)
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)
	t.Setenv("STH_GITHUB_API_BASE_URL", server.URL)
	projectDir, homeDir := t.TempDir(), t.TempDir()
	writeProjectConfigV2ForTest(t, projectDir, fanoutConfig(fanoutTarget("claude", ".claude")))
	textResult := runCLI(t, projectDir, homeDir, "list")
	jsonResult := runCLI(t, projectDir, homeDir, "list", "--json")
	if textResult.err != nil || jsonResult.err != nil {
		t.Fatalf("%v / %v", textResult.err, jsonResult.err)
	}
	var entries []fs.CatalogEntry
	if err := json.Unmarshal([]byte(jsonResult.stdout), &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected available uninstalled skill: %s", jsonResult.stdout)
	}
	assertContains(t, textResult.stdout, entries[0].Name)
	assertContains(t, textResult.stdout, entries[0].Description)
}

func TestUnchangedSkillPreservesFilesStateAndChosenAssistant(t *testing.T) {
	server := skillBundleServer(t)
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)
	t.Setenv("STH_GITHUB_API_BASE_URL", server.URL)
	projectDir, homeDir := t.TempDir(), t.TempDir()
	writeProjectConfigV2ForTest(t, projectDir, fanoutConfig(fanoutTarget("claude", ".claude"), fanoutTarget("codex", ".codex")))
	if result := runCLI(t, projectDir, homeDir, "install", "alpha", "--target", "claude"); result.err != nil {
		t.Fatal(result.err)
	}
	before := transactionalProjectFileSnapshot(t, projectDir)
	for _, args := range [][]string{{"install", "alpha", "--target", "claude"}, {"update"}} {
		result := runCLI(t, projectDir, homeDir, args...)
		if result.err != nil {
			t.Fatal(result.err)
		}
		assertContains(t, result.stdout, "Déjà à jour")
		if after := transactionalProjectFileSnapshot(t, projectDir); !reflect.DeepEqual(before, after) {
			t.Fatal("unchanged operation altered project files or state")
		}
	}
	assertNoFile(t, filepath.Join(projectDir, ".agents/skills/alpha/SKILL.md"))
	filePath := filepath.Join(projectDir, ".claude/skills/alpha/SKILL.md")
	if err := os.WriteFile(filePath, []byte("personal edit"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"update"}, {"remove", "alpha", "--yes"}} {
		result := runCLI(t, projectDir, homeDir, args...)
		if result.err == nil {
			t.Fatalf("%v should preserve edits", args)
		}
		assertContains(t, result.err.Error(), "copy your edits")
	}
	if content, err := os.ReadFile(filePath); err != nil || string(content) != "personal edit" {
		t.Fatalf("edit lost: %q %v", content, err)
	}
}
