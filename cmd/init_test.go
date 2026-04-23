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

	result := runCLIWithInput(t, projectDir, homeDir, "\n\n\nowner/repo\n\n\n\n", "init")

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

	result := runCLIWithInput(t, projectDir, homeDir, "y\n\n\n\n\n\n\n", "init")

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

func TestRun_initGuidesUserThroughPublicGitHubSetup(t *testing.T) {
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

	result := runCLIWithInput(t, projectDir, homeDir, "github\nn\nowner-only\nowner/repo\n\n\n\n", "init")

	if result.err != nil {
		t.Fatalf("Run() error = %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}
	assertContains(t, result.stdout, "Assistant de configuration LoreSkills")
	assertContains(t, result.stdout, "Etape 1/5 - Choisir le provider")
	assertContains(t, result.stdout, "Etape 2/5 - Type d'acces au depot")
	assertContains(t, result.stdout, "Depot public")
	assertContains(t, result.stdout, "Format attendu pour repository_id : owner/repo")
	assertContains(t, result.stdout, "Repository ID invalide")
	assertContains(t, result.stdout, "Etape 5/5 - Validation de l'acces distant")
	assertContains(t, result.stdout, "Configuration projet enregistrée")
	assertContains(t, result.stdout, "Prochaine étape : utilisez `loreskills list`.")
	assertEmpty(t, result.stderr)
}

func TestValidateRepositoryIDRejectsEmptySegments(t *testing.T) {
	tests := []struct {
		name         string
		provider     string
		repositoryID string
	}{
		{name: "github double slash", provider: "github", repositoryID: "owner//repo"},
		{name: "github leading slash", provider: "github", repositoryID: "/owner/repo"},
		{name: "azure trailing slash", provider: "azure-devops", repositoryID: "org/project/repo/"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateRepositoryID(test.provider, test.repositoryID); err == nil {
				t.Fatalf("validateRepositoryID(%q, %q) error = nil, want error", test.provider, test.repositoryID)
			}
		})
	}
}

func TestRun_initShowsTokenGuidanceForPrivateRepo(t *testing.T) {
	providers := []struct {
		name           string
		wantURL        string
		wantEnvVar     string
		repoID         string
		serverPathFmt  string
		rawBaseURLEnv  string
	}{
		{
			name:          "github",
			wantURL:       "https://github.com/settings/tokens/new",
			wantEnvVar:    "GITHUB_TOKEN",
			repoID:        "owner/repo",
			serverPathFmt: "/owner/repo/main/SKILLS.md",
			rawBaseURLEnv: "LORESKILLS_GITHUB_RAW_BASE_URL",
		},
		{
			name:          "gitlab",
			wantURL:       "https://gitlab.com/-/profile/personal_access_tokens",
			wantEnvVar:    "GITLAB_TOKEN",
			repoID:        "owner/repo",
			serverPathFmt: "/api/v4/projects/owner%2Frepo/repository/files/SKILLS.md/raw",
			rawBaseURLEnv: "LORESKILLS_GITLAB_API_BASE_URL",
		},
		{
			name:          "bitbucket",
			wantURL:       "https://bitbucket.org/account/settings/app-passwords/new",
			wantEnvVar:    "BITBUCKET_TOKEN",
			repoID:        "owner/repo",
			serverPathFmt: "/2.0/repositories/owner/repo/src/main/SKILLS.md",
			rawBaseURLEnv: "LORESKILLS_BITBUCKET_API_BASE_URL",
		},
	}

	for _, p := range providers {
		t.Run(p.name, func(t *testing.T) {
			homeDir := t.TempDir()
			projectDir := t.TempDir()
			server := stdhttp.NewServeMux()
			server.HandleFunc("/", func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
				_, _ = w.Write([]byte(testCatalogMarkdown("| skill | alpha | skills | skills/alpha.md | Alpha |")))
			})
			ts := httptest.NewServer(server)
			defer ts.Close()

			baseURL := ts.URL
			if p.name == "gitlab" {
				baseURL = ts.URL + "/api/v4"
			} else if p.name == "bitbucket" {
				baseURL = ts.URL + "/2.0"
			}
			t.Setenv(p.rawBaseURLEnv, baseURL)
			// Set a dummy token so the credential fallback does not block waiting for git credentials.
			t.Setenv("LORESKILLS_TOKEN", "dummy-test-token")

			// Input: provider, private=y, skip token, repoID, ref, catalogPath, targetDir
			input := p.name + "\ny\n\n" + p.repoID + "\n\n\n\n"
			result := runCLIWithInput(t, projectDir, homeDir, input, "init")

			if result.err != nil {
				t.Fatalf("Run() error = %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
			}
			assertContains(t, result.stdout, "Depot prive - Authentification requise")
			assertContains(t, result.stdout, p.wantURL)
			assertContains(t, result.stdout, p.wantEnvVar)
			assertContains(t, result.stdout, "LORESKILLS_TOKEN")
			assertContains(t, result.stdout, "Token d'acces pour valider maintenant")
			assertContains(t, result.stdout, "Configuration projet enregistrée")
		})
	}
}

func TestRun_initPrivateGitHubWithTokenPassesValidation(t *testing.T) {
	homeDir := t.TempDir()
	projectDir := t.TempDir()
	const testToken = "ghp_test_token_abc123"

	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testToken {
			stdhttp.Error(w, "unauthorized", stdhttp.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(testCatalogMarkdown("| skill | alpha | skills | skills/alpha.md | Alpha |")))
	}))
	defer server.Close()
	t.Setenv("LORESKILLS_GITHUB_RAW_BASE_URL", server.URL)

	// Input: provider=github, private=y, enter token, repoID, ref, catalogPath, targetDir
	input := "github\ny\n" + testToken + "\nowner/repo\n\n\n\n"
	result := runCLIWithInput(t, projectDir, homeDir, input, "init")

	if result.err != nil {
		t.Fatalf("Run() error = %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}
	assertContains(t, result.stdout, "Accès au dépôt validé")
	assertContains(t, result.stdout, "Le token saisi a ete utilise pour la validation mais n'est pas sauvegarde")
	assertContains(t, result.stdout, "export GITHUB_TOKEN=<votre-token>")
}

func TestRun_initAzureDevOpsShowsTokenGuidance(t *testing.T) {
	homeDir := t.TempDir()
	projectDir := t.TempDir()

	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		_, _ = w.Write([]byte(testCatalogMarkdown("| skill | alpha | skills | skills/alpha.md | Alpha |")))
	}))
	defer server.Close()
	t.Setenv("LORESKILLS_AZURE_DEVOPS_BASE_URL", server.URL)
	// Set a dummy token so the credential fallback does not block waiting for git credentials.
	t.Setenv("LORESKILLS_TOKEN", "dummy-test-token")

	// Input: azure-devops, private=y, skip token, org/project/repo, ref, catalogPath, targetDir
	input := "azure-devops\ny\n\norg/project/repo\n\n\n\n"
	result := runCLIWithInput(t, projectDir, homeDir, input, "init")

	if result.err != nil {
		t.Fatalf("Run() error = %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}
	assertContains(t, result.stdout, "https://dev.azure.com/<votre-organisation>")
	assertContains(t, result.stdout, "AZURE_DEVOPS_EXT_PAT")
	assertContains(t, result.stdout, "Code (Read)")
}

func TestProviderTokenEnvVar(t *testing.T) {
	tests := []struct {
		provider string
		want     string
	}{
		{"github", "GITHUB_TOKEN"},
		{"gitlab", "GITLAB_TOKEN"},
		{"azure-devops", "AZURE_DEVOPS_EXT_PAT"},
		{"bitbucket", "BITBUCKET_TOKEN"},
		{"unknown", "LORESKILLS_TOKEN"},
	}
	for _, test := range tests {
		t.Run(test.provider, func(t *testing.T) {
			if got := providerTokenEnvVar(test.provider); got != test.want {
				t.Fatalf("providerTokenEnvVar(%q) = %q, want %q", test.provider, got, test.want)
			}
		})
	}
}

