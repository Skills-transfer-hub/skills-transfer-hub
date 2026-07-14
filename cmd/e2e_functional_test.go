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

func parseJSONArray(t *testing.T, payload string) []map[string]any {
	t.Helper()
	var parsed []map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(payload)), &parsed); err != nil {
		t.Fatalf("stdout is not a JSON array: %v\npayload=%q", err, payload)
	}
	return parsed
}

func assertJSONErrorObject(t *testing.T, payload string) {
	t.Helper()
	var parsed map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(payload)), &parsed); err != nil {
		t.Fatalf("stdout is not a JSON object: %v\npayload=%q", err, payload)
	}
	if _, ok := parsed["error"]; !ok {
		t.Fatalf("JSON error object missing \"error\" field: %q", payload)
	}
}

func writeSkillFile(t *testing.T, root, category, name, description string) {
	t.Helper()
	skillDir := filepath.Join(root, category, name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", skillDir, err)
	}
	body := "---\nname: " + name + "\ndescription: " + description + "\n---\n\n## Usage\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile(%s SKILL.md): %v", name, err)
	}
}

func TestE2EFunctional_InstallRoundTrip(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	server := skillBundleServer(t)
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)
	t.Setenv("STH_GITHUB_API_BASE_URL", server.URL)

	writeProjectConfigV2ForTest(t, projectDir, fanoutConfig(fanoutTarget("claude", ".claude")))

	installResult := runCLI(t, projectDir, homeDir, "install", "alpha")
	if installResult.err != nil {
		t.Fatalf("install: %v\nstderr=%s", installResult.err, installResult.stderr)
	}
	assertFileExists(t, filepath.Join(projectDir, ".claude", "skills", "alpha", "SKILL.md"))
	if state := readStateV2(t, projectDir); len(state.Entries) != 1 {
		t.Fatalf("state entries after install = %d, want 1", len(state.Entries))
	}

	statusText := runCLI(t, projectDir, homeDir, "status")
	if statusText.err != nil {
		t.Fatalf("status: %v\nstderr=%s", statusText.err, statusText.stderr)
	}
	assertContains(t, statusText.stdout, "alpha")

	statusJSON := runCLI(t, projectDir, homeDir, "status", "--json")
	if statusJSON.err != nil {
		t.Fatalf("status --json: %v\nstderr=%s", statusJSON.err, statusJSON.stderr)
	}
	statusEntries := parseJSONArray(t, statusJSON.stdout)
	if len(statusEntries) != 1 {
		t.Fatalf("status --json entries = %d, want 1\npayload=%s", len(statusEntries), statusJSON.stdout)
	}
	resourceName, _ := statusEntries[0]["resource_name"].(string)
	if !strings.Contains(resourceName, "alpha") {
		t.Fatalf("status --json resource_name = %q, want it to contain alpha", resourceName)
	}

	listJSON := runCLI(t, projectDir, homeDir, "list", "--json")
	if listJSON.err != nil {
		t.Fatalf("list --json: %v", listJSON.err)
	}
	if entries := parseJSONArray(t, listJSON.stdout); len(entries) != 1 {
		t.Fatalf("list --json entries = %d, want 1\npayload=%s", len(entries), listJSON.stdout)
	}

	dryRunJSON := runCLI(t, projectDir, homeDir, "update", "--dry-run", "--json")
	if dryRunJSON.err != nil {
		t.Fatalf("update --dry-run --json: %v\nstderr=%s", dryRunJSON.err, dryRunJSON.stderr)
	}
	parseJSONArray(t, dryRunJSON.stdout)

	updateJSON := runCLI(t, projectDir, homeDir, "update", "--json")
	if updateJSON.err != nil {
		t.Fatalf("update --json: %v\nstderr=%s", updateJSON.err, updateJSON.stderr)
	}
	updateEntries := parseJSONArray(t, updateJSON.stdout)
	if len(updateEntries) != 1 {
		t.Fatalf("update --json entries = %d, want 1\npayload=%s", len(updateEntries), updateJSON.stdout)
	}
	if updateEntries[0]["outcome"] != "updated" {
		t.Fatalf("update --json outcome = %v, want updated\npayload=%s", updateEntries[0]["outcome"], updateJSON.stdout)
	}

	removeResult := runCLI(t, projectDir, homeDir, "remove", resourceName, "--yes")
	if removeResult.err != nil {
		t.Fatalf("remove: %v\nstderr=%s", removeResult.err, removeResult.stderr)
	}
	assertNoFile(t, filepath.Join(projectDir, ".claude", "skills", "alpha", "SKILL.md"))

	finalStatus := runCLI(t, projectDir, homeDir, "status", "--json")
	if finalStatus.err != nil {
		t.Fatalf("final status --json: %v", finalStatus.err)
	}
	if strings.TrimSpace(finalStatus.stdout) != "[]" {
		t.Fatalf("after remove, status --json = %q, want []", finalStatus.stdout)
	}
}

func TestE2EFunctional_InstallJSONEmitsResults(t *testing.T) {
	configs := map[string]fs.ProjectConfigV2{
		"fanout": fanoutConfig(fanoutTarget("claude", ".claude")),
		"legacy": fanoutConfig(),
	}
	for label, cfg := range configs {
		t.Run(label, func(t *testing.T) {
			projectDir := t.TempDir()
			homeDir := t.TempDir()
			server := skillBundleServer(t)
			defer server.Close()
			t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)
			t.Setenv("STH_GITHUB_API_BASE_URL", server.URL)
			writeProjectConfigV2ForTest(t, projectDir, cfg)

			result := runCLI(t, projectDir, homeDir, "install", "alpha", "--json")
			if result.err != nil {
				t.Fatalf("install --json: %v\nstderr=%s", result.err, result.stderr)
			}
			entries := parseJSONArray(t, result.stdout)
			if len(entries) != 1 {
				t.Fatalf("install --json entries = %d, want 1\npayload=%s", len(entries), result.stdout)
			}
			if entries[0]["outcome"] != "installed" {
				t.Fatalf("install --json outcome = %v, want installed\npayload=%s", entries[0]["outcome"], result.stdout)
			}
		})
	}
}

func TestE2EFunctional_HelpLoginAndListWording(t *testing.T) {
	homeDir := t.TempDir()

	loginHelp := runCLI(t, "", homeDir, "help", "login")
	if loginHelp.err != nil {
		t.Fatalf("help login should not error, got: %v", loginHelp.err)
	}
	assertContains(t, loginHelp.stdout, "login")
	assertNotContains(t, loginHelp.stdout, "commande inconnue")

	listHelp := runCLI(t, "", homeDir, "help", "list")
	if listHelp.err != nil {
		t.Fatalf("help list: %v", listHelp.err)
	}
	assertContains(t, listHelp.stdout, "catalog init")
}

func TestE2EFunctional_JSONEmptyContract(t *testing.T) {
	configs := map[string]fs.ProjectConfigV2{
		"fanout":    fanoutConfig(fanoutTarget("claude", ".claude")),
		"nonfanout": fanoutConfig(),
	}
	invocations := [][]string{
		{"status", "--json"},
		{"list", "--json"},
		{"update", "--json"},
		{"update", "--dry-run", "--json"},
	}
	for label, cfg := range configs {
		t.Run(label, func(t *testing.T) {
			projectDir := t.TempDir()
			homeDir := t.TempDir()
			writeProjectConfigV2ForTest(t, projectDir, cfg)
			for _, args := range invocations {
				result := runCLI(t, projectDir, homeDir, args...)
				if result.err != nil {
					t.Fatalf("%v: unexpected err %v\nstderr=%s", args, result.err, result.stderr)
				}
				if strings.TrimSpace(result.stdout) != "[]" {
					t.Fatalf("%v stdout = %q, want []", args, result.stdout)
				}
			}
		})
	}
}

func TestE2EFunctional_UpdateJSONErrorIsJSON(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	configDir := filepath.Join(projectDir, ".sth")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "project.json"), []byte("{ not valid json"), 0o644); err != nil {
		t.Fatalf("WriteFile(project.json): %v", err)
	}

	result := runCLI(t, projectDir, homeDir, "update", "--json")
	if result.err == nil {
		t.Fatal("expected error updating with a corrupted project.json")
	}
	assertJSONErrorObject(t, result.stdout)
}

func TestE2EFunctional_Provider404IsGraceful(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(response stdhttp.ResponseWriter, request *stdhttp.Request) {
		stdhttp.NotFound(response, request)
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)
	t.Setenv("STH_GITHUB_API_BASE_URL", server.URL)

	writeProjectConfigV2ForTest(t, projectDir, fanoutConfig(fanoutTarget("claude", ".claude")))

	result := runCLI(t, projectDir, homeDir, "install", "alpha")
	if result.err == nil {
		t.Fatal("expected an error installing from a 404 provider")
	}
	assertContains(t, result.stderr, "404")
}

func TestE2EFunctional_CatalogInitScan(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	writeSkillFile(t, projectDir, "analysis", "risk", "Analyse de risque projet")
	writeSkillFile(t, projectDir, "writing", "blog-post", "Redige un article de blog")
	if err := os.MkdirAll(filepath.Join(projectDir, "writing", "empty-no-skill"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	generated := runCLI(t, projectDir, homeDir, "catalog", "init")
	if generated.err != nil {
		t.Fatalf("catalog init: %v\nstderr=%s", generated.err, generated.stderr)
	}
	skillsPath := filepath.Join(projectDir, "SKILLS.md")
	assertFileExists(t, skillsPath)
	data, err := os.ReadFile(skillsPath)
	if err != nil {
		t.Fatalf("ReadFile(SKILLS.md): %v", err)
	}
	catalog := string(data)
	assertContains(t, catalog, "risk")
	assertContains(t, catalog, "blog-post")
	assertContains(t, catalog, "Analyse de risque projet")
	assertNotContains(t, catalog, "empty-no-skill")

	blocked := runCLI(t, projectDir, homeDir, "catalog", "init")
	if blocked.err == nil {
		t.Fatal("expected catalog init to refuse overwriting SKILLS.md without --force")
	}

	forced := runCLI(t, projectDir, homeDir, "catalog", "init", "--force")
	if forced.err != nil {
		t.Fatalf("catalog init --force: %v\nstderr=%s", forced.err, forced.stderr)
	}
}

func TestE2EFunctional_NewScaffold(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	created := runCLI(t, projectDir, homeDir, "new", "prompts/code-review")
	if created.err != nil {
		t.Fatalf("new: %v\nstderr=%s", created.err, created.stderr)
	}
	assertFileExists(t, filepath.Join(projectDir, "prompts", "code-review", "SKILL.md"))

	duplicate := runCLI(t, projectDir, homeDir, "new", "prompts/code-review")
	if duplicate.err == nil {
		t.Fatal("expected new to refuse overwriting an existing skill")
	}

	invalid := runCLI(t, projectDir, homeDir, "new", "bad segment/x")
	if invalid.err == nil {
		t.Fatal("expected new to reject an invalid segment")
	}
}

func TestE2EFunctional_PackInitLayouts(t *testing.T) {
	t.Setenv("STH_PRO_GATE", "0")
	cases := map[string]struct {
		input    string
		expected []string
	}{
		"flat": {
			input:    "mypack\nflat\n\ny\n",
			expected: []string{"manifest.json", "SKILLS.md"},
		},
		"multistack": {
			input: "mypack\nmulti-stack\n\ny\n",
			expected: []string{
				"manifest.json",
				filepath.Join("skills", "sample", "SKILL.md"),
				filepath.Join("agents", "sample.md"),
			},
		},
	}
	for label, scenario := range cases {
		t.Run(label, func(t *testing.T) {
			projectDir := t.TempDir()
			homeDir := t.TempDir()
			result := runCLIWithInput(t, projectDir, homeDir, scenario.input, "pack", "init")
			if result.err != nil {
				t.Fatalf("pack init: %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
			}
			for _, relative := range scenario.expected {
				assertFileExists(t, filepath.Join(projectDir, "mypack", relative))
			}
		})
	}
}

func TestE2EFunctional_InitThenDoctor(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(response stdhttp.ResponseWriter, request *stdhttp.Request) {
		_, _ = response.Write([]byte(testCatalogMarkdown("| skill | alpha | skills | skills/alpha.md | Alpha |")))
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)
	t.Setenv("STH_GITHUB_API_BASE_URL", server.URL)

	if err := os.MkdirAll(filepath.Join(projectDir, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git): %v", err)
	}

	initInput := "github\norg/repo\nmain\nSKILLS.md\n\nn\n\n"
	initResult := runCLIWithInput(t, projectDir, homeDir, initInput, "init")
	if initResult.err != nil {
		t.Fatalf("init: %v\nstdout=%s\nstderr=%s", initResult.err, initResult.stdout, initResult.stderr)
	}
	assertContains(t, initResult.stdout, "Configuration enregistrée")
	assertFileExists(t, filepath.Join(projectDir, ".sth", "project.json"))
	assertFileExists(t, filepath.Join(projectDir, ".gitignore"))

	doctorResult := runCLI(t, projectDir, homeDir, "doctor")
	assertContains(t, doctorResult.stdout, "Project configuration")
}

func TestE2EFunctional_TelemetryTransitions(t *testing.T) {
	configDir := t.TempDir()
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	run := func(args ...string) cliResult {
		return runCLIWithOptions(t, cliOptions{
			workDir:           projectDir,
			homeDir:           homeDir,
			userConfigDir:     configDir,
			skipTelemetrySeed: true,
			args:              args,
		})
	}

	if anon := run("telemetry", "anon"); anon.err != nil {
		t.Fatalf("telemetry anon: %v\nstderr=%s", anon.err, anon.stderr)
	}
	afterAnon := run("telemetry", "status")
	if afterAnon.err != nil {
		t.Fatalf("telemetry status: %v", afterAnon.err)
	}
	assertContains(t, afterAnon.stdout, "anonymous")

	if off := run("telemetry", "off"); off.err != nil {
		t.Fatalf("telemetry off: %v\nstderr=%s", off.err, off.stderr)
	}
	afterOff := run("telemetry", "status")
	if afterOff.err != nil {
		t.Fatalf("telemetry status: %v", afterOff.err)
	}
	assertContains(t, afterOff.stdout, "off")
}

func TestE2EFunctional_CompletionAndHelp(t *testing.T) {
	homeDir := t.TempDir()

	shells := map[string]string{
		"bash":       "bash completion for sth",
		"zsh":        "#compdef sth",
		"powershell": "PowerShell completion",
	}
	for shell, marker := range shells {
		result := runCLI(t, "", homeDir, "completion", shell)
		if result.err != nil {
			t.Fatalf("completion %s: %v", shell, result.err)
		}
		assertContains(t, result.stdout, marker)
	}

	if bad := runCLI(t, "", homeDir, "completion", "fish"); bad.err == nil {
		t.Fatal("expected completion to reject an unsupported shell")
	}

	for _, command := range []string{
		"init", "list", "install", "update", "status", "remove", "buddy",
		"version", "doctor", "telemetry", "completion", "pack", "catalog",
		"new", "alpha", "mcp",
	} {
		result := runCLI(t, "", homeDir, "help", command)
		if result.err != nil {
			t.Fatalf("help %s: %v", command, result.err)
		}
		if strings.TrimSpace(result.stdout) == "" {
			t.Fatalf("help %s produced no output", command)
		}
	}

	if unknown := runCLI(t, "", homeDir, "does-not-exist"); unknown.err == nil {
		t.Fatal("expected an unknown command to error")
	}
}

func TestE2EFunctional_BuddyAndMCPInstall(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	writeProjectConfigV2ForTest(t, projectDir, fanoutConfig(fanoutTarget("claude", ".claude")))

	buddyResult := runCLI(t, projectDir, homeDir, "buddy")
	if buddyResult.err != nil {
		t.Fatalf("buddy: %v\nstderr=%s", buddyResult.err, buddyResult.stderr)
	}
	if strings.TrimSpace(buddyResult.stdout) == "" {
		t.Fatal("buddy produced no output")
	}

	mcpResult := runCLI(t, projectDir, homeDir, "mcp", "install")
	if mcpResult.err != nil {
		t.Fatalf("mcp install: %v\nstderr=%s", mcpResult.err, mcpResult.stderr)
	}
	mcpPath := filepath.Join(projectDir, ".mcp.json")
	assertFileExists(t, mcpPath)
	data, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatalf("ReadFile(.mcp.json): %v", err)
	}
	assertContains(t, string(data), "mcpServers")
}
