package cmd

import (
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"loreskills/internal/fs"
)

func TestRun_initCreatesProjectProfile(t *testing.T) {
	homeDir := t.TempDir()
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if r.URL.Path != "/owner/repo/main/SKILLS.md" {
			stdhttp.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(testCatalogMarkdown("| skill | alpha | skills | skills/alpha.md | Alpha |")))
	}))
	defer server.Close()
	t.Setenv("LORESKILLS_GITHUB_RAW_BASE_URL", server.URL)

	result := runCLIWithInput(t, projectDir, homeDir, "\nowner/repo\n\n\n\n", "init")

	if result.err != nil {
		t.Fatalf("Run() error = %v", result.err)
	}
	if !strings.Contains(result.stdout, "Configuration projet enregistrée") {
		t.Fatalf("stdout %q does not contain success message", result.stdout)
	}
	if !strings.Contains(result.stdout, "Accès au dépôt validé") {
		t.Fatalf("stdout %q does not contain reachability confirmation", result.stdout)
	}

	configPath := filepath.Join(projectDir, ".loreskills", "project.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", configPath, err)
	}

	var profile fs.ProjectProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		t.Fatalf("json.Unmarshal(): %v", err)
	}
	if profile.Provider != "github" {
		t.Fatalf("profile.Provider = %q, want github", profile.Provider)
	}
	if profile.RepositoryID != "owner/repo" {
		t.Fatalf("profile.RepositoryID = %q, want owner/repo", profile.RepositoryID)
	}
	if profile.CatalogPath != "SKILLS.md" {
		t.Fatalf("profile.CatalogPath = %q, want SKILLS.md", profile.CatalogPath)
	}
}

func TestRun_initImportsLegacyConfigWhenAccepted(t *testing.T) {
	homeDir := t.TempDir()
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if r.URL.Path != "/owner/repo/main/skills/SKILLS.md" {
			stdhttp.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(testCatalogMarkdown("| skill | alpha | skills | skills/alpha.md | Alpha |")))
	}))
	defer server.Close()
	t.Setenv("LORESKILLS_GITHUB_RAW_BASE_URL", server.URL)

	configPath := filepath.Join(homeDir, ".loreskillsrc")
	original := []byte("{\"repo_raw_url\":\"https://raw.githubusercontent.com/owner/repo/main/skills\",\"target_dir\":\".claude/skills\"}\n")
	if err := os.WriteFile(configPath, original, 0o600); err != nil {
		t.Fatalf("WriteFile(%s): %v", configPath, err)
	}

	result := runCLIWithInput(t, projectDir, homeDir, "y\n\n\n\n\n", "init")

	if result.err != nil {
		t.Fatalf("Run() error = %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}
	assertContains(t, result.stdout, "Configuration projet enregistrée")

	data, err := os.ReadFile(filepath.Join(projectDir, ".loreskills", "project.json"))
	if err != nil {
		t.Fatalf("ReadFile(project profile): %v", err)
	}
	var profile fs.ProjectProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		t.Fatalf("json.Unmarshal(): %v", err)
	}
	if !profile.MigratedFromLegacy {
		t.Fatal("expected migrated_from_legacy to be true")
	}
	if profile.CatalogPath != "skills/SKILLS.md" {
		t.Fatalf("profile.CatalogPath = %q, want skills/SKILLS.md", profile.CatalogPath)
	}
	if profile.RepositoryID != "owner/repo" {
		t.Fatalf("profile.RepositoryID = %q, want owner/repo", profile.RepositoryID)
	}
}

func TestRun_initKeepsExistingProjectProfileWhenUserDeclinesReplacement(t *testing.T) {
	homeDir := t.TempDir()
	projectDir := t.TempDir()
	writeProjectProfileForCommandTest(t, projectDir, fs.ProjectProfile{
		Provider:     "github",
		RepositoryID: "existing/repo",
		Ref:          "main",
		CatalogPath:  "SKILLS.md",
		TargetDir:    ".claude/skills",
	})

	result := runCLIWithInput(t, projectDir, homeDir, "n\n", "init")

	if result.err != nil {
		t.Fatalf("Run() error = %v", result.err)
	}
	assertContains(t, result.stdout, "Configuration projet conservée")

	data, err := os.ReadFile(filepath.Join(projectDir, ".loreskills", "project.json"))
	if err != nil {
		t.Fatalf("ReadFile(project profile): %v", err)
	}
	assertContains(t, string(data), "existing/repo")
	assertNotContains(t, string(data), "owner/repo")
	if strings.Contains(string(data), "raw.githubusercontent.com") {
		t.Fatalf("project profile unexpectedly changed: %s", data)
	}
}
