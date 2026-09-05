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

// Wizard input layout (first provider, no env-var token):
//   1. provider   (Enter = github)
//   2. repo       (owner/repo)
//   3. ref        (Enter = main)
//   4. catalog    (Enter = SKILLS.md)
//   5. targets    (Enter = detected / claude by default)
//   6. access     (Enter = privé / "n" = public)
//   7. details?   (only if private + no env token)
//   8. token      (only if private + no env token)
//   9. recap      (Enter = Y / "n" = abandon / "edit" = field edit loop)

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

	// provider, repo, ref, catalog, target, access(privé), details=N, token(skip), recap(Y)
	input := "\nowner/repo\n\n\n\n\n\n\n\n"
	result := runCLIWithInput(t, projectDir, homeDir, input, "init")

	if result.err != nil {
		t.Fatalf("Run() error = %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}
	assertContains(t, result.stdout, "✓ Configuration saved")
	assertContains(t, result.stdout, "All good! The catalog is reachable.")

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
	if cfg.Providers[0].ID != "default" {
		t.Fatalf("provider ID = %q, want default (auto-assigned)", cfg.Providers[0].ID)
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

	// import=y, provider, repo(default=owner/repo), ref, cat, target, access(privé), details=N, token(skip), recap
	input := "y\n\n\n\n\n\n\n\n\n\n"
	result := runCLIWithInput(t, projectDir, homeDir, input, "init")

	if result.err != nil {
		t.Fatalf("Run() error = %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}
	assertContains(t, result.stdout, "✓ Configuration saved")

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
	assertContains(t, result.stdout, "Project configuration kept.")

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

	// provider=github, bad-repo-then-good, ref/cat/target defaults, access=public(n), recap(Y)
	input := "github\nowner-only\nowner/repo\n\n\n\npublic\n\n"
	result := runCLIWithInput(t, projectDir, homeDir, input, "init")

	if result.err != nil {
		t.Fatalf("Run() error = %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}
	assertContains(t, result.stdout, "Where is your catalog hosted?")
	assertContains(t, result.stdout, "repository identifier")
	assertContains(t, result.stdout, "Expected format: owner/repo")
	assertContains(t, result.stdout, "Invalid repository identifier.")
	assertContains(t, result.stdout, "✓ Configuration saved")
	assertContains(t, result.stdout, "Next step: run `sth list`")
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

// When a provider env var is already set, the wizard skips the token prompt
// AND the "Détails complets ?" gate — the buddy just announces it found a
// token and continues. This test exercises that env-var fast path.
func TestRun_initSkipsTokenPromptWhenEnvVarSet(t *testing.T) {
	homeDir := t.TempDir()
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(responseWriter stdhttp.ResponseWriter, request *stdhttp.Request) {
		_, _ = responseWriter.Write([]byte(testCatalogMarkdown("| skill | alpha | skills | skills/alpha.md | Alpha |")))
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)
	t.Setenv("GITHUB_TOKEN", "ghp_test_env_token")

	// provider, repo, ref, cat, target, access(privé), recap (no token prompt!)
	input := "\nowner/repo\n\n\n\n\n\n"
	result := runCLIWithInput(t, projectDir, homeDir, input, "init")

	if result.err != nil {
		t.Fatalf("Run() error = %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}
	assertContains(t, result.stdout, "I found a token in $GITHUB_TOKEN")
	assertNotContains(t, result.stdout, "Token (optional)")
	assertNotContains(t, result.stdout, "Input is hidden")
	assertContains(t, result.stdout, "✓ Configuration saved")
}

// On user request, the wizard re-shows the full provider guidance via the
// "Détails complets ?" gate. This replaces the old re-read loop.
func TestRun_initShowsExpandedGuidanceOnRequest(t *testing.T) {
	homeDir := t.TempDir()
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		_, _ = w.Write([]byte(testCatalogMarkdown("| skill | alpha | react | react/alpha/SKILL.md | Alpha |")))
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)

	// provider, repo, ref/cat/target, access(privé), details=Y, token(skip), recap
	input := "github\nowner/repo\n\n\n\n\ny\n\n\n"
	result := runCLIWithInput(t, projectDir, homeDir, input, "init")

	if result.err != nil {
		t.Fatalf("Run() error = %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}
	// First the short teaser, then the full guidance after "y" is typed.
	assertContains(t, result.stdout, "https://github.com/settings/personal-access-tokens/new")
	assertContains(t, result.stdout, "Fine-grained token")
	assertContains(t, result.stdout, "✓ Configuration saved")
}

func TestRun_initRejectsUnsupportedProviderThenContinues(t *testing.T) {
	homeDir := t.TempDir()
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		_, _ = w.Write([]byte(testCatalogMarkdown("| skill | alpha | react | react/alpha/SKILL.md | Alpha |")))
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)

	// bad-provider, good-provider, repo, ref/cat/target, access=public(n), recap
	input := "gitea\ngithub\nowner/repo\n\n\n\npublic\n\n"
	result := runCLIWithInput(t, projectDir, homeDir, input, "init")

	if result.err != nil {
		t.Fatalf("Run() error = %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}
	assertContains(t, result.stdout, "invalid choice: gitea")
	assertContains(t, result.stdout, "✓ Configuration saved")
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

	// provider=gitlab, bad-url, good-id, ref/cat/target, access=public, recap
	input := "gitlab\nhttps://git.example.com/group/project\ngroup/project\n\n\n\npublic\n\n"
	result := runCLIWithInput(t, projectDir, homeDir, input, "init")

	if result.err != nil {
		t.Fatalf("Run() error = %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}
	assertContains(t, result.stdout, "Invalid repository identifier.")
	assertContains(t, result.stdout, "gitlab.com")
	assertContains(t, result.stdout, "✓ Configuration saved")
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

	// provider=github, repo, ref/cat/target, access(privé), details=N, manual token, recap
	input := "github\nowner/repo\n\n\n\n\nn\n" + testToken + "\n\n"
	result := runCLIWithInput(t, projectDir, homeDir, input, "init")

	if result.err != nil {
		t.Fatalf("Run() error = %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}
	assertContains(t, result.stdout, "All good! The catalog is reachable.")
	assertContains(t, result.stdout, "The token was used for validation but was not saved.")
	assertContains(t, result.stdout, "export GITHUB_TOKEN=<your-token>")
}

func TestRun_initAzureDevOpsShowsTokenGuidance(t *testing.T) {
	homeDir := t.TempDir()
	projectDir := t.TempDir()

	server := httptest.NewServer(stdhttp.HandlerFunc(func(responseWriter stdhttp.ResponseWriter, request *stdhttp.Request) {
		_, _ = responseWriter.Write([]byte(testCatalogMarkdown("| skill | alpha | skills | skills/alpha.md | Alpha |")))
	}))
	defer server.Close()
	t.Setenv("STH_AZURE_DEVOPS_BASE_URL", server.URL)
	// Note: do NOT set STH_TOKEN — we want the wizard to show guidance and prompt.

	// provider=azure-devops, repo, ref/cat/target, access(privé), details=Y, token(skip), recap
	input := "azure-devops\norg/project/repo\n\n\n\n\ny\n\n\n"
	result := runCLIWithInput(t, projectDir, homeDir, input, "init")

	if result.err != nil {
		t.Fatalf("Run() error = %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}
	assertContains(t, result.stdout, "https://dev.azure.com/<your-organization>")
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

// First-provider flow auto-assigns alias "default" — no prompt.
func TestRunInit_EmptyProject_CreatesV2WithOneProvider(t *testing.T) {
	homeDir := t.TempDir()
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(responseWriter stdhttp.ResponseWriter, request *stdhttp.Request) {
		_, _ = responseWriter.Write([]byte(testCatalogMarkdown("| skill | alpha | skills | skills/alpha.md | Alpha |")))
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)

	// provider, repo, ref, cat, targets(default), access=public(n), recap
	input := "github\norg/repo\nmain\nSKILLS.md\n\npublic\n\n"
	result := runCLIWithInput(t, projectDir, homeDir, input, "init")

	if result.err != nil {
		t.Fatalf("Run() error = %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}
	assertContains(t, result.stdout, "✓ Configuration saved")

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

// Adding a 2nd provider DOES prompt for an alias (progressive disclosure).
// STH_ALPHA_GATE=0 bypasses the Alpha-period 1-Git-provider cap so this test
// asserts the alias-prompt behaviour independently of the cap.
func TestRunInit_ExistingV2_AppendsSecondProviderWithAliasPrompt(t *testing.T) {
	t.Setenv("STH_ALPHA_GATE", "0")
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
			{ID: "default", Provider: "github", RepositoryID: "org/first", CatalogRef: "main", CatalogPath: "SKILLS.md", TargetDir: ".claude/skills"},
		},
		Resources: []fs.ResourceEntry{},
	})

	// add new=y, alias=second, provider=github, repo, ref/cat/targets(default), access=public, recap
	input := "y\nsecond\ngithub\norg/second\nmain\nSKILLS.md\n\npublic\n\n"
	result := runCLIWithInput(t, projectDir, homeDir, input, "init")

	if result.err != nil {
		t.Fatalf("Run() error = %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}
	assertContains(t, result.stdout, "Give it a short alias")
	assertContains(t, result.stdout, "✓ Source 'second' added: github org/second@main")
	assertContains(t, result.stdout, "2 sources are now configured")

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
	if cfg.Providers[0].ID != "default" {
		t.Fatalf("first provider ID = %q, want default", cfg.Providers[0].ID)
	}
	if cfg.Providers[0].RepositoryID != "org/first" {
		t.Fatalf("first provider repository_id = %q, want org/first (should be unchanged)", cfg.Providers[0].RepositoryID)
	}
	if cfg.Providers[1].ID != "second" {
		t.Fatalf("second provider ID = %q, want second", cfg.Providers[1].ID)
	}
}

func TestCountPrivateGitProviders(t *testing.T) {
	cfg := fs.ProjectConfigV2{Providers: []fs.ProviderEntry{
		{ID: "a", Provider: "github", Private: true},
		{ID: "b", Provider: "gitlab", Private: false},
		{ID: "c", Provider: "sth-cloud", Private: true},
	}}
	if got := countPrivateGitProviders(cfg); got != 1 {
		t.Fatalf("countPrivateGitProviders = %d, want 1 (public and cloud excluded)", got)
	}
}

func TestRunInit_AlphaGate_AllowsSecondPublicRepo(t *testing.T) {
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
			{ID: "default", Provider: "github", RepositoryID: "org/first", CatalogRef: "main", CatalogPath: "SKILLS.md", TargetDir: ".claude/skills", Private: true},
		},
		Resources: []fs.ResourceEntry{},
	})

	input := "y\nsecond\ngithub\norg/second\nmain\nSKILLS.md\n\npublic\n\n"
	result := runCLIWithInput(t, projectDir, homeDir, input, "init")

	if result.err != nil {
		t.Fatalf("Run() error = %v, want success for a public second repo\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
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
		t.Fatalf("providers count = %d, want 2 (public second repo allowed under Alpha)", len(cfg.Providers))
	}
}

// When the user picks an alias already used by an existing provider, the
// wizard re-prompts (does not abort, does not write a duplicate).
// STH_ALPHA_GATE=0 keeps the Alpha provider cap out of the way for this test.
func TestRunInit_DuplicateAlias_RepromptsAndKeepsConfig(t *testing.T) {
	t.Setenv("STH_ALPHA_GATE", "0")
	homeDir := t.TempDir()
	projectDir := t.TempDir()

	writeProjectConfigV2ForTest(t, projectDir, fs.ProjectConfigV2{
		SchemaVersion: 2,
		Providers: []fs.ProviderEntry{
			{ID: "default", Provider: "github", RepositoryID: "org/repo", CatalogRef: "main", CatalogPath: "SKILLS.md", TargetDir: ".claude/skills"},
		},
		Resources: []fs.ResourceEntry{},
	})

	// add=y, alias=default (duplicate) → re-prompt; input exhausts → io.EOF
	input := "y\ndefault\n"
	result := runCLIWithInput(t, projectDir, homeDir, input, "init")

	if result.err != nil {
		t.Fatalf("Run() error = %v, want clean abort when input exhausted after duplicate alias", result.err)
	}
	assertContains(t, result.stdout, "already in use")
	assertContains(t, result.stdout, "Setup cancelled.")

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

// When validation fails (server 404), the config is STILL written so the
// user can iterate. Diagnostic + remediation block appears, exit code is nil.
func TestRun_initWritesConfigEvenWhenValidationFails(t *testing.T) {
	homeDir := t.TempDir()
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		stdhttp.NotFound(w, r)
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)
	// API base too, otherwise ListRepositoryTree (404 fallback path) would hit real github.com.
	t.Setenv("STH_GITHUB_API_BASE_URL", server.URL)

	// provider, repo, ref/cat/target, access=public, recap
	input := "\nowner/repo\n\n\n\npublic\n\n"
	result := runCLIWithInput(t, projectDir, homeDir, input, "init")

	// Exit non-zero so CI scripts see the failure, BUT the config file must
	// still exist so the user can iterate without re-running the whole wizard.
	if result.err == nil {
		t.Fatalf("Run() error = nil, want non-nil on validation failure\nstdout=%s", result.stdout)
	}
	assertContains(t, result.err.Error(), "remote access validation failed")
	assertContains(t, result.stdout, "✓ Configuration saved")
	assertContains(t, result.stdout, "Config written but not validated.")
	assertContains(t, result.stdout, "sth doctor")

	configPath := filepath.Join(projectDir, ".sth", "project.json")
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("expected %s to exist after failed validation, got %v", configPath, err)
	}
}

// Server 401 → diagnostic categorises as auth_failed and references the
// provider-specific minimum permission.
func TestRun_initClassifiesAuthFailure(t *testing.T) {
	homeDir := t.TempDir()
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		stdhttp.Error(w, "unauthorized", stdhttp.StatusUnauthorized)
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)
	t.Setenv("STH_TOKEN", "wrong-token")

	// provider, repo, ref/cat/target, access=privé, recap (env token detected, no token prompt)
	input := "\nowner/repo\n\n\n\n\n\n"
	result := runCLIWithInput(t, projectDir, homeDir, input, "init")

	// Auth failure → non-zero exit so wrappers detect it. Config still written.
	if result.err == nil {
		t.Fatalf("Run() error = nil, want non-nil on auth failure\nstdout=%s", result.stdout)
	}
	assertContains(t, result.stdout, "auth_failed")
	assertContains(t, result.stdout, "Contents: Read")
	assertContains(t, result.stdout, "✓ Configuration saved")
}

// User confirms the recap → config is written. User selects "edit" → loops
// back, can change one field, then confirm. Validates the recap-edit-recap
// loop end-to-end.
func TestRun_initRecapAllowsEdit(t *testing.T) {
	homeDir := t.TempDir()
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		_, _ = w.Write([]byte(testCatalogMarkdown("| skill | alpha | skills | skills/alpha.md | Alpha |")))
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)

	// provider, repo=wrong/repo, ref/cat/target, access=public, recap=edit, field=depot (no accent variant), new repo, recap=Y
	input := "github\nwrong/repo\n\n\n\npublic\nedit\ndepot\ngood/repo\n\n"
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
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if len(cfg.Providers) != 1 {
		t.Fatalf("providers = %d, want 1", len(cfg.Providers))
	}
	if cfg.Providers[0].RepositoryID != "good/repo" {
		t.Fatalf("repo = %q, want good/repo (after edit)", cfg.Providers[0].RepositoryID)
	}
}

// User declines the recap → no config is written, exit code is nil.
func TestRun_initRecapDecline(t *testing.T) {
	homeDir := t.TempDir()
	projectDir := t.TempDir()

	// provider, repo, ref/cat/target, access=public, recap=n
	input := "github\nowner/repo\n\n\n\npublic\nn\n"
	result := runCLIWithInput(t, projectDir, homeDir, input, "init")

	if result.err != nil {
		t.Fatalf("Run() error = %v", result.err)
	}
	assertContains(t, result.stdout, "setup cancelled.")

	configPath := filepath.Join(projectDir, ".sth", "project.json")
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("expected %s to NOT exist after recap decline, got err=%v", configPath, err)
	}
}

// Recap-edit must NOT let the user smuggle in an alias that collides with an
// existing provider — askAlias enforces uniqueness up front, the edit path
// has to do the same or the final WriteProjectConfigV2 corrupts state.
func TestRun_initRecapEditRejectsDuplicateAlias(t *testing.T) {
	// Bypass the Alpha 1-Git-provider cap — this test asserts the alias
	// uniqueness check during the recap edit, orthogonal to the cap.
	t.Setenv("STH_ALPHA_GATE", "0")
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
			{ID: "default", Provider: "github", RepositoryID: "org/first", CatalogRef: "main", CatalogPath: "SKILLS.md", TargetDir: ".claude/skills"},
		},
		Resources: []fs.ResourceEntry{},
	})

	// add=y, alias=second, provider, repo, ref/cat/target, access=public,
	// recap=edit, field=alias, newAlias=default (duplicate!), recap=Y
	input := "y\nsecond\ngithub\norg/second\n\n\n\npublic\nedit\nalias\ndefault\n\n"
	result := runCLIWithInput(t, projectDir, homeDir, input, "init")

	if result.err != nil {
		t.Fatalf("Run() error = %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}
	assertContains(t, result.stdout, "already in use")

	// Final config must keep alias "second" (the original choice), NOT "default".
	configPath := filepath.Join(projectDir, ".sth", "project.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var cfg fs.ProjectConfigV2
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if len(cfg.Providers) != 2 {
		t.Fatalf("providers = %d, want 2", len(cfg.Providers))
	}
	ids := []string{cfg.Providers[0].ID, cfg.Providers[1].ID}
	if ids[0] != "default" || ids[1] != "second" {
		t.Fatalf("provider IDs = %v, want [default second] (duplicate edit should be rejected)", ids)
	}
}

// The repo prompt accepts a full URL; NormalizeRepositoryID strips the
// scheme/host and stores the canonical owner/repo.
func TestRun_initAcceptsRepositoryURL(t *testing.T) {
	homeDir := t.TempDir()
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		_, _ = w.Write([]byte(testCatalogMarkdown("| skill | alpha | skills | skills/alpha.md | Alpha |")))
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)

	// provider, repo=URL, ref/cat/target, access=public, recap
	input := "github\nhttps://github.com/Skills-transfer-hub/sth-dashboard\n\n\n\npublic\n\n"
	result := runCLIWithInput(t, projectDir, homeDir, input, "init")

	if result.err != nil {
		t.Fatalf("Run() error = %v\nstdout=%s", result.err, result.stdout)
	}
	configPath := filepath.Join(projectDir, ".sth", "project.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var cfg fs.ProjectConfigV2
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if cfg.Providers[0].RepositoryID != "Skills-transfer-hub/sth-dashboard" {
		t.Fatalf("repo = %q, want Skills-transfer-hub/sth-dashboard", cfg.Providers[0].RepositoryID)
	}
}

// The targets step persists the chosen models into ProjectConfigV2.Targets.
func TestRunInit_PersistsSelectedTargets(t *testing.T) {
	homeDir := t.TempDir()
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		_, _ = w.Write([]byte(testCatalogMarkdown("| skill | alpha | skills | skills/alpha.md | Alpha |")))
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)

	// provider, repo, ref, catalog, targets=claude,cursor, access=public(n), recap
	input := "github\nowner/repo\nmain\nSKILLS.md\nclaude,cursor\npublic\n\n"
	result := runCLIWithInput(t, projectDir, homeDir, input, "init")
	if result.err != nil {
		t.Fatalf("Run() error = %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}

	data, err := os.ReadFile(filepath.Join(projectDir, ".sth", "project.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var cfg fs.ProjectConfigV2
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	kinds := make([]string, 0, len(cfg.Targets))
	for _, entry := range cfg.Targets {
		kinds = append(kinds, entry.Kind)
		if !entry.Enabled {
			t.Errorf("target %q should be enabled", entry.Kind)
		}
	}
	if strings.Join(kinds, ",") != "claude,cursor" {
		t.Fatalf("targets = %v, want [claude cursor]", kinds)
	}
	if cfg.Targets[1].RootDir != ".cursor" {
		t.Fatalf("cursor RootDir = %q, want .cursor", cfg.Targets[1].RootDir)
	}
}

// An empty answer at the targets step keeps the detected targets (here Cursor,
// because the project already has a .cursor/ directory).
func TestRunInit_PreselectsDetectedTargets(t *testing.T) {
	homeDir := t.TempDir()
	projectDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectDir, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		_, _ = w.Write([]byte(testCatalogMarkdown("| skill | alpha | skills | skills/alpha.md | Alpha |")))
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)

	// provider, repo, ref, catalog, targets(empty=keep detected), access=public(n), recap
	input := "github\nowner/repo\nmain\nSKILLS.md\n\npublic\n\n"
	result := runCLIWithInput(t, projectDir, homeDir, input, "init")
	if result.err != nil {
		t.Fatalf("Run() error = %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}

	data, err := os.ReadFile(filepath.Join(projectDir, ".sth", "project.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var cfg fs.ProjectConfigV2
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if len(cfg.Targets) != 1 || cfg.Targets[0].Kind != "cursor" {
		t.Fatalf("targets = %+v, want a single detected cursor target", cfg.Targets)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Telemetry consent + Alpha provider gate
// ─────────────────────────────────────────────────────────────────────────────

func TestRunInit_FirstInit_PromptsTelemetryConsent_AcceptedAnonymous(t *testing.T) {
	homeDir := t.TempDir()
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		_, _ = w.Write([]byte(testCatalogMarkdown("| skill | alpha | skills | skills/alpha.md | Alpha |")))
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)

	// First input is the telemetry menu choice ("2" = anonymous). Mode 2 does
	// not trigger the cloud-login sub-prompt, so the rest of the input goes
	// straight into the wizard provider flow (defaults + owner/repo + ...).
	input := "2\n\nowner/repo\n\n\n\n\n\n\n\n"
	result := runCLIWithOptions(t, cliOptions{
		workDir:           projectDir,
		homeDir:           homeDir,
		input:             input,
		args:              []string{"init"},
		skipTelemetrySeed: true,
		interactive:       func() bool { return true },
	})
	if result.err != nil {
		t.Fatalf("Run() error = %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}
	if !strings.Contains(result.stdout, "How do you want to contribute") {
		t.Fatalf("expected telemetry menu in stdout, got: %s", result.stdout)
	}
	if !strings.Contains(result.stdout, "Anonymous telemetry enabled") {
		t.Fatalf("expected anonymous-confirmation in stdout, got: %s", result.stdout)
	}
	if fs.TelemetryModeActive() != fs.TelemetryAnonymous {
		t.Fatalf("mode = %q, want anonymous", fs.TelemetryModeActive())
	}
}

func TestRunInit_FirstInit_PromptsTelemetryConsent_RefusedOff(t *testing.T) {
	homeDir := t.TempDir()
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		_, _ = w.Write([]byte(testCatalogMarkdown("| skill | alpha | skills | skills/alpha.md | Alpha |")))
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)

	// "1" = off.
	input := "1\n\nowner/repo\n\n\n\n\n\n\n\n"
	result := runCLIWithOptions(t, cliOptions{
		workDir:           projectDir,
		homeDir:           homeDir,
		input:             input,
		args:              []string{"init"},
		skipTelemetrySeed: true,
		interactive:       func() bool { return true },
	})
	if result.err != nil {
		t.Fatalf("Run() error = %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}
	if !strings.Contains(result.stdout, "Telemetry disabled") {
		t.Fatalf("expected disabled-confirmation in stdout, got: %s", result.stdout)
	}
	if fs.TelemetryConsented() {
		t.Fatal("TelemetryConsented should be false after answering 1 (off)")
	}
	if !fs.HasTelemetryDecision() {
		t.Fatal("HasTelemetryDecision should be true after the prompt — refusal still counts as a decision")
	}
}

// --no-cloud-prompt must skip the cloud-login sub-prompt even when the user
// picks telemetry mode = connected. Mode stays "connected" — but no key is
// requested (the user can run `sth login` later).
func TestRunInit_NoCloudPromptFlag_SkipsLoginSubprompt(t *testing.T) {
	homeDir := t.TempDir()
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		_, _ = w.Write([]byte(testCatalogMarkdown("| skill | alpha | skills | skills/alpha.md | Alpha |")))
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)

	// "3\n" picks connected; without --no-cloud-prompt the next prompt would
	// be the y/N cloud-login. With the flag, the wizard jumps straight to
	// the provider questions.
	input := "3\n\nowner/repo\n\n\n\n\n\n\n\n"
	result := runCLIWithOptions(t, cliOptions{
		workDir:           projectDir,
		homeDir:           homeDir,
		input:             input,
		args:              []string{"init", "--no-cloud-prompt"},
		skipTelemetrySeed: true,
		interactive:       func() bool { return true },
	})
	if result.err != nil {
		t.Fatalf("Run() error = %v\nSTDOUT:\n%s\nSTDERR:\n%s", result.err, result.stdout, result.stderr)
	}
	if strings.Contains(result.stdout, "Connect to STH Cloud now") {
		t.Fatalf("--no-cloud-prompt must skip the login sub-prompt; got:\n%s", result.stdout)
	}
	if fs.TelemetryModeActive() != fs.TelemetryConnected {
		t.Fatalf("mode = %q, want connected", fs.TelemetryModeActive())
	}
}

func TestRunInit_FirstInit_DefaultIsOff(t *testing.T) {
	homeDir := t.TempDir()
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		_, _ = w.Write([]byte(testCatalogMarkdown("| skill | alpha | skills | skills/alpha.md | Alpha |")))
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)

	// Empty answer at the menu → default "1" (off). Alpha is opt-in.
	input := "\n\nowner/repo\n\n\n\n\n\n\n\n"
	result := runCLIWithOptions(t, cliOptions{
		workDir:           projectDir,
		homeDir:           homeDir,
		input:             input,
		args:              []string{"init"},
		skipTelemetrySeed: true,
		interactive:       func() bool { return true },
	})
	if result.err != nil {
		t.Fatalf("Run() error = %v\nSTDOUT:\n%s\nSTDERR:\n%s", result.err, result.stdout, result.stderr)
	}
	if fs.TelemetryConsented() {
		t.Fatal("Default answer should be off; got consented=true")
	}
}

func TestRunInit_SecondInit_DoesNotRePromptTelemetry(t *testing.T) {
	homeDir := t.TempDir()
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		_, _ = w.Write([]byte(testCatalogMarkdown("| skill | alpha | skills | skills/alpha.md | Alpha |")))
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)

	// Pre-seeded decision (default behavior of runCLIWithOptions) → no prompt.
	// Input feeds only the wizard, not a telemetry answer.
	input := "\nowner/repo\n\n\n\n\n\n\n\n"
	result := runCLIWithInput(t, projectDir, homeDir, input, "init")
	if result.err != nil {
		t.Fatalf("Run() error = %v\nstdout=%s", result.err, result.stdout)
	}
	if strings.Contains(result.stdout, "How do you want to contribute") {
		t.Fatalf("telemetry prompt should not appear when decision exists; stdout=%s", result.stdout)
	}
}

func TestRunInit_AlphaGate_LoopsBackOnSecondPrivateRepo(t *testing.T) {
	homeDir := t.TempDir()
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(responseWriter stdhttp.ResponseWriter, request *stdhttp.Request) {
		_, _ = responseWriter.Write([]byte(testCatalogMarkdown("| skill | alpha | skills | skills/alpha.md | Alpha |")))
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)

	writeProjectConfigV2ForTest(t, projectDir, fs.ProjectConfigV2{
		SchemaVersion: 2,
		Providers: []fs.ProviderEntry{
			{ID: "first", Provider: "github", RepositoryID: "org/first", CatalogRef: "main", CatalogPath: "SKILLS.md", TargetDir: ".claude/skills", Private: true},
		},
		Resources: []fs.ResourceEntry{},
	})

	input := "y\nsecond\ngithub\norg/second\nmain\nSKILLS.md\n\nprive\npublic\n\n"
	result := runCLIWithInput(t, projectDir, homeDir, input, "init")
	if result.err != nil {
		t.Fatalf("expected the gate to loop back, not abort — got: %v\nstdout=%s", result.err, result.stdout)
	}
	if !strings.Contains(result.stdout, "alphaLimits") {
		t.Fatalf("expected docs link in stdout, got: %s", result.stdout)
	}
	if !strings.Contains(strings.ReplaceAll(result.stdout, "\n", " "), "reference only one private") {
		t.Fatalf("expected Alpha-cap explanation in stdout, got: %s", result.stdout)
	}
	assertContains(t, result.stdout, "✓ Configuration saved")

	configPath := filepath.Join(projectDir, ".sth", "project.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var cfg fs.ProjectConfigV2
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	for _, provider := range cfg.Providers {
		if provider.ID == "second" && provider.Private {
			t.Fatal("second provider should have been saved as public after the loop-back")
		}
	}
}

func TestRunInit_AlphaGate_AllowsGitProviderAlongsideSthCloud(t *testing.T) {
	homeDir := t.TempDir()
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		_, _ = w.Write([]byte(testCatalogMarkdown("| skill | alpha | skills | skills/alpha.md | Alpha |")))
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)

	// Existing project has sth-cloud (added by `sth login`). Alpha rule must
	// still let the user add their first Git provider.
	writeProjectConfigV2ForTest(t, projectDir, fs.ProjectConfigV2{
		SchemaVersion: 2,
		Providers: []fs.ProviderEntry{
			{ID: "cloud", Provider: "sth-cloud", TargetDir: ".claude/skills"},
		},
		Resources: []fs.ResourceEntry{},
	})

	input := "y\nfirst\ngithub\norg/first\nmain\nSKILLS.md\n\npublic\n\n"
	result := runCLIWithInput(t, projectDir, homeDir, input, "init")
	if result.err != nil {
		t.Fatalf("expected success — sth-cloud does not count, got: %v\nstdout=%s", result.err, result.stdout)
	}
	configPath := filepath.Join(projectDir, ".sth", "project.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var cfg fs.ProjectConfigV2
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if len(cfg.Providers) != 2 {
		t.Fatalf("expected 2 providers (sth-cloud + first), got %d", len(cfg.Providers))
	}
}

func TestRunInit_AlphaGate_Bypassed(t *testing.T) {
	t.Setenv("STH_ALPHA_GATE", "0")
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

	// New wizard flow (same as the alias-prompt test above): addNew, alias,
	// provider, repo, ref, catalog, targets, access=n, recap accept, recap accept.
	input := "y\nsecond\ngithub\norg/second\nmain\nSKILLS.md\n\npublic\n\n"
	result := runCLIWithInput(t, projectDir, homeDir, input, "init")
	if result.err != nil {
		t.Fatalf("expected bypass to allow second Git provider, got: %v", result.err)
	}
}
