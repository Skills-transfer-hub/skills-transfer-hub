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
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)

	// providerID(default), provider(default=github), private(default=yes), reviewAgain(no), skip token, repoID, ref, catalogPath, targetDir
	result := runCLIWithInput(t, projectDir, homeDir, "\n\n\n\n\nowner/repo\n\n\n\n", "init")

	if result.err != nil {
		t.Fatalf("Run() error = %v", result.err)
	}
	if !strings.Contains(result.stdout, "Configuration projet enregistrée") {
		t.Fatalf("stdout %q does not contain success message", result.stdout)
	}
	if !strings.Contains(result.stdout, "Accès au dépôt validé") {
		t.Fatalf("stdout %q does not contain reachability confirmation", result.stdout)
	}

	configPath := filepath.Join(projectDir, ".sth", "project.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", configPath, err)
	}

	var cfg fs.ProjectConfigV2
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("json.Unmarshal(): %v", err)
	}
	if cfg.SchemaVersion != 2 {
		t.Fatalf("schema_version = %d, want 2", cfg.SchemaVersion)
	}
	if len(cfg.Providers) != 1 {
		t.Fatalf("providers count = %d, want 1", len(cfg.Providers))
	}
	if cfg.Providers[0].Provider != "github" {
		t.Fatalf("provider = %q, want github", cfg.Providers[0].Provider)
	}
	if cfg.Providers[0].RepositoryID != "owner/repo" {
		t.Fatalf("repository_id = %q, want owner/repo", cfg.Providers[0].RepositoryID)
	}
	if cfg.Providers[0].CatalogPath != "SKILLS.md" {
		t.Fatalf("catalog_path = %q, want SKILLS.md", cfg.Providers[0].CatalogPath)
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
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)

	configPath := filepath.Join(homeDir, ".sthrc")
	original := []byte("{\"repo_raw_url\":\"https://raw.githubusercontent.com/owner/repo/main/skills\",\"target_dir\":\".claude/skills\"}\n")
	if err := os.WriteFile(configPath, original, 0o600); err != nil {
		t.Fatalf("WriteFile(%s): %v", configPath, err)
	}

	// import legacy=y, then providerID(default), provider(default), private(default=yes), reviewAgain(no), skip token,
	// repositoryID(default=owner/repo), ref(default), catalogPath(default), targetDir(default)
	result := runCLIWithInput(t, projectDir, homeDir, "y\n\n\n\n\n\n\n\n\n\n", "init")

	if result.err != nil {
		t.Fatalf("Run() error = %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}
	assertContains(t, result.stdout, "Configuration projet enregistrée")

	data, err := os.ReadFile(filepath.Join(projectDir, ".sth", "project.json"))
	if err != nil {
		t.Fatalf("ReadFile(project profile): %v", err)
	}
	var cfg fs.ProjectConfigV2
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("json.Unmarshal(): %v", err)
	}
	if len(cfg.Providers) != 1 {
		t.Fatalf("providers count = %d, want 1", len(cfg.Providers))
	}
	if cfg.Providers[0].CatalogPath != "skills/SKILLS.md" {
		t.Fatalf("catalog_path = %q, want skills/SKILLS.md", cfg.Providers[0].CatalogPath)
	}
	if cfg.Providers[0].RepositoryID != "owner/repo" {
		t.Fatalf("repository_id = %q, want owner/repo", cfg.Providers[0].RepositoryID)
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

	data, err := os.ReadFile(filepath.Join(projectDir, ".sth", "project.json"))
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
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)

	result := runCLIWithInput(t, projectDir, homeDir, "\ngithub\nn\n\nowner-only\nowner/repo\n\n\n\n", "init")

	if result.err != nil {
		t.Fatalf("Run() error = %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}
	assertContains(t, result.stdout, "Assistant de configuration STH")
	assertContains(t, result.stdout, "Etape 1/5 - Choisir le provider")
	assertContains(t, result.stdout, "Etape 2/5 - Type d'acces au depot")
	assertContains(t, result.stdout, "Depot public")
	assertContains(t, result.stdout, "Format attendu pour repository_id : owner/repo")
	assertContains(t, result.stdout, "Repository ID invalide")
	assertContains(t, result.stdout, "Etape 5/5 - Validation de l'acces distant")
	assertContains(t, result.stdout, "Configuration projet enregistrée")
	assertContains(t, result.stdout, "Prochaine étape : utilisez `sth list`.")
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
			wantURL:       "https://github.com/settings/personal-access-tokens/new",
			wantEnvVar:    "GITHUB_TOKEN",
			repoID:        "owner/repo",
			serverPathFmt: "/owner/repo/main/SKILLS.md",
			rawBaseURLEnv: "STH_GITHUB_RAW_BASE_URL",
		},
		{
			name:          "gitlab",
			wantURL:       "https://gitlab.com/-/user_settings/personal_access_tokens",
			wantEnvVar:    "GITLAB_TOKEN",
			repoID:        "owner/repo",
			serverPathFmt: "/api/v4/projects/owner%2Frepo/repository/files/SKILLS.md/raw",
			rawBaseURLEnv: "STH_GITLAB_API_BASE_URL",
		},
		{
			name:          "bitbucket",
			wantURL:       "https://support.atlassian.com/bitbucket-cloud/docs/create-an-api-token/",
			wantEnvVar:    "BITBUCKET_TOKEN",
			repoID:        "owner/repo",
			serverPathFmt: "/2.0/repositories/owner/repo/src/main/SKILLS.md",
			rawBaseURLEnv: "STH_BITBUCKET_API_BASE_URL",
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
			t.Setenv("STH_TOKEN", "dummy-test-token")

			// Input: providerID(default), provider, private=y, reviewAgain(no), skip token, repoID, ref, catalogPath, targetDir
			input := "\n" + p.name + "\ny\n\n\n" + p.repoID + "\n\n\n\n"
			result := runCLIWithInput(t, projectDir, homeDir, input, "init")

			if result.err != nil {
				t.Fatalf("Run() error = %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
			}
			assertContains(t, result.stdout, "Depot prive - Authentification requise")
			assertContains(t, result.stdout, p.wantURL)
			assertContains(t, result.stdout, p.wantEnvVar)
			assertContains(t, result.stdout, "STH_TOKEN")
			switch p.name {
			case "github":
				assertContains(t, result.stdout, "Contents: Read")
			case "gitlab":
				assertContains(t, result.stdout, "read_repository")
			case "bitbucket":
				assertContains(t, result.stdout, "Repositories: Read")
			}
			assertContains(t, result.stdout, "Token d'acces pour valider maintenant")
			assertContains(t, result.stdout, "Configuration projet enregistrée")
		})
	}
}

func TestRun_initRejectsUnsupportedProviderThenContinues(t *testing.T) {
	homeDir := t.TempDir()
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		_, _ = w.Write([]byte(testCatalogMarkdown("| skill | alpha | react | react/alpha/SKILL.md | Alpha |")))
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)

	result := runCLIWithInput(t, projectDir, homeDir, "\ngitea\ngithub\nn\nn\nowner/repo\n\n\n\n", "init")

	if result.err != nil {
		t.Fatalf("Run() error = %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}
	assertContains(t, result.stdout, "Provider invalide")
	assertContains(t, result.stdout, "Choisissez azure-devops, bitbucket, github, gitlab")
	assertContains(t, result.stdout, "Configuration projet enregistrée")
}

func TestRun_initRejectsSelfHostedRepositoryURLThenAcceptsHostedIdentifier(t *testing.T) {
	homeDir := t.TempDir()
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		_, _ = w.Write([]byte(testCatalogMarkdown("| skill | alpha | react | react/alpha/SKILL.md | Alpha |")))
	}))
	defer server.Close()
	t.Setenv("STH_GITLAB_API_BASE_URL", server.URL+"/api/v4")
	t.Setenv("STH_TOKEN", "dummy-test-token")

	input := "\ngitlab\nn\nn\nhttps://git.example.com/group/project\ngroup/project\n\n\n\n"
	result := runCLIWithInput(t, projectDir, homeDir, input, "init")

	if result.err != nil {
		t.Fatalf("Run() error = %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}
	assertContains(t, result.stdout, "Repository ID invalide")
	assertContains(t, result.stdout, "gitlab.com")
	assertContains(t, result.stdout, "Configuration projet enregistrée")
}

func TestRun_initAllowsReviewingProviderGuidanceAgain(t *testing.T) {
	homeDir := t.TempDir()
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		_, _ = w.Write([]byte(testCatalogMarkdown("| skill | alpha | react | react/alpha/SKILL.md | Alpha |")))
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)
	t.Setenv("STH_TOKEN", "dummy-test-token")

	input := "\ngithub\ny\ny\nn\n\nowner/repo\n\n\n\n"
	result := runCLIWithInput(t, projectDir, homeDir, input, "init")

	if result.err != nil {
		t.Fatalf("Run() error = %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}
	if strings.Count(result.stdout, "https://github.com/settings/personal-access-tokens/new") < 2 {
		t.Fatalf("stdout %q does not show the provider guidance twice", result.stdout)
	}
	assertContains(t, result.stdout, "Configuration projet enregistrée")
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
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)

	// Input: providerID(default), provider=github, private=y, reviewAgain(no), enter token, repoID, ref, catalogPath, targetDir
	input := "\ngithub\ny\n\n" + testToken + "\nowner/repo\n\n\n\n"
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
	t.Setenv("STH_AZURE_DEVOPS_BASE_URL", server.URL)
	// Set a dummy token so the credential fallback does not block waiting for git credentials.
	t.Setenv("STH_TOKEN", "dummy-test-token")

	// Input: providerID(default), azure-devops, private=y, reviewAgain(no), skip token, org/project/repo, ref, catalogPath, targetDir
	input := "\nazure-devops\ny\n\n\norg/project/repo\n\n\n\n"
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
		{"unknown", "STH_TOKEN"},
	}
	for _, test := range tests {
		t.Run(test.provider, func(t *testing.T) {
			if got := providerTokenEnvVar(test.provider); got != test.want {
				t.Fatalf("providerTokenEnvVar(%q) = %q, want %q", test.provider, got, test.want)
			}
		})
	}
}

// T020 — fresh project creates a v2 config with one provider.
func TestRunInit_EmptyProject_CreatesV2WithOneProvider(t *testing.T) {
	homeDir := t.TempDir()
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		_, _ = w.Write([]byte(testCatalogMarkdown("| skill | alpha | skills | skills/alpha.md | Alpha |")))
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)

	// providerID=default, provider=github, private=no, reviewAgain(no), repoID=org/repo, ref=main, catalogPath=SKILLS.md, targetDir=.claude/skills
	input := "default\ngithub\nn\n\norg/repo\nmain\nSKILLS.md\n.claude/skills\n"
	result := runCLIWithInput(t, projectDir, homeDir, input, "init")

	if result.err != nil {
		t.Fatalf("Run() error = %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}
	assertContains(t, result.stdout, "Configuration projet enregistrée")

	configPath := filepath.Join(projectDir, ".sth", "project.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", configPath, err)
	}

	var cfg fs.ProjectConfigV2
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("json.Unmarshal(): %v", err)
	}
	if cfg.SchemaVersion != 2 {
		t.Fatalf("schema_version = %d, want 2", cfg.SchemaVersion)
	}
	if len(cfg.Providers) != 1 {
		t.Fatalf("providers count = %d, want 1", len(cfg.Providers))
	}
	p := cfg.Providers[0]
	if p.ID != "default" {
		t.Fatalf("provider ID = %q, want default", p.ID)
	}
	if p.Provider != "github" {
		t.Fatalf("provider = %q, want github", p.Provider)
	}
	if p.RepositoryID != "org/repo" {
		t.Fatalf("repository_id = %q, want org/repo", p.RepositoryID)
	}
	if len(cfg.Resources) != 0 {
		t.Fatalf("resources = %v, want empty array", cfg.Resources)
	}
}

// T021 — existing v2 config: a second provider is appended, first unchanged.
func TestRunInit_ExistingV2_AppendsSecondProvider(t *testing.T) {
	homeDir := t.TempDir()
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		_, _ = w.Write([]byte(testCatalogMarkdown("| skill | alpha | skills | skills/alpha.md | Alpha |")))
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)

	writeProjectConfigV2ForTest(t, projectDir, fs.ProjectConfigV2{
		SchemaVersion: 2,
		Providers: []fs.ProviderEntry{
			{ID: "first", Provider: "github", RepositoryID: "org/first", CatalogRef: "main", CatalogPath: "SKILLS.md", TargetDir: ".claude/skills"},
		},
		Resources: []fs.ResourceEntry{},
	})

	// yes to add provider, providerID=second, provider=github, private=no, reviewAgain(no), repoID, ref, catalogPath, targetDir
	input := "y\nsecond\ngithub\nn\n\norg/second\nmain\nSKILLS.md\n.claude/skills\n"
	result := runCLIWithInput(t, projectDir, homeDir, input, "init")

	if result.err != nil {
		t.Fatalf("Run() error = %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}

	configPath := filepath.Join(projectDir, ".sth", "project.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", configPath, err)
	}
	var cfg fs.ProjectConfigV2
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("json.Unmarshal(): %v", err)
	}
	if len(cfg.Providers) != 2 {
		t.Fatalf("providers count = %d, want 2", len(cfg.Providers))
	}
	if cfg.Providers[0].ID != "first" {
		t.Fatalf("first provider ID = %q, want first", cfg.Providers[0].ID)
	}
	if cfg.Providers[0].RepositoryID != "org/first" {
		t.Fatalf("first provider repository_id = %q, want org/first (should be unchanged)", cfg.Providers[0].RepositoryID)
	}
	if cfg.Providers[1].ID != "second" {
		t.Fatalf("second provider ID = %q, want second", cfg.Providers[1].ID)
	}
}

// T022 — duplicate provider ID shows a warning and loops; file is untouched.
func TestRunInit_DuplicateProviderID_ReturnsError(t *testing.T) {
	homeDir := t.TempDir()
	projectDir := t.TempDir()

	writeProjectConfigV2ForTest(t, projectDir, fs.ProjectConfigV2{
		SchemaVersion: 2,
		Providers: []fs.ProviderEntry{
			{ID: "default", Provider: "github", RepositoryID: "org/repo", CatalogRef: "main", CatalogPath: "SKILLS.md", TargetDir: ".claude/skills"},
		},
		Resources: []fs.ResourceEntry{},
	})

	// yes to add provider, then duplicate providerID; input exhausted → wizard terminates with io.EOF
	input := "y\ndefault\n"
	result := runCLIWithInput(t, projectDir, homeDir, input, "init")

	if result.err == nil {
		t.Fatal("Run() error = nil, want error when input exhausted after duplicate provider ID")
	}
	if !strings.Contains(result.stdout, "default") {
		t.Fatalf("expected stdout to mention 'default', got: stdout=%s", result.stdout)
	}

	configPath := filepath.Join(projectDir, ".sth", "project.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", configPath, err)
	}
	var cfg fs.ProjectConfigV2
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("json.Unmarshal(): %v", err)
	}
	if len(cfg.Providers) != 1 {
		t.Fatalf("providers count = %d, want 1 after error", len(cfg.Providers))
	}
}

// T023 — written v2 config has all correct provider fields and empty resources array.
func TestRunInit_V2WrittenCorrectly_ProvidersArray(t *testing.T) {
	homeDir := t.TempDir()
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		_, _ = w.Write([]byte(testCatalogMarkdown("| skill | alpha | skills | skills/alpha.md | Alpha |")))
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)

	input := "myprovider\ngithub\nn\n\norg/repo\nmain\nSKILLS.md\n.claude/skills\n"
	result := runCLIWithInput(t, projectDir, homeDir, input, "init")

	if result.err != nil {
		t.Fatalf("Run() error = %v\nstdout=%s", result.err, result.stdout)
	}

	configPath := filepath.Join(projectDir, ".sth", "project.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", configPath, err)
	}
	var cfg fs.ProjectConfigV2
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("json.Unmarshal(): %v", err)
	}
	if len(cfg.Providers) != 1 {
		t.Fatalf("providers count = %d, want 1", len(cfg.Providers))
	}
	p := cfg.Providers[0]
	if p.ID != "myprovider" {
		t.Fatalf("id = %q, want myprovider", p.ID)
	}
	if p.Provider != "github" {
		t.Fatalf("provider = %q, want github", p.Provider)
	}
	if p.RepositoryID != "org/repo" {
		t.Fatalf("repository_id = %q, want org/repo", p.RepositoryID)
	}
	if p.CatalogRef != "main" {
		t.Fatalf("catalog_ref = %q, want main", p.CatalogRef)
	}
	if p.CatalogPath != "SKILLS.md" {
		t.Fatalf("catalog_path = %q, want SKILLS.md", p.CatalogPath)
	}
	if p.TargetDir != ".claude/skills" {
		t.Fatalf("target_dir = %q, want .claude/skills", p.TargetDir)
	}
	if len(cfg.Resources) != 0 {
		t.Fatalf("resources = %v, want empty array", cfg.Resources)
	}
}
