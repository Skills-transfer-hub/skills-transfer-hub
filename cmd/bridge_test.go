package cmd

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sth/internal/fs"
)

func TestRun_bridgeNegotiatesAndReadsProjectWithoutMutation(t *testing.T) {
	projectDir := t.TempDir()
	writeProjectConfigV2ForTest(t, projectDir, fs.ProjectConfigV2{
		SchemaVersion: 2,
		Providers: []fs.ProviderEntry{{
			ID: "default", Provider: "github", RepositoryID: "owner/repo",
			CatalogRef: "main", CatalogPath: "SKILLS.md", TargetDir: ".claude/skills",
		}},
		Resources: []fs.ResourceEntry{},
	})
	before, err := os.ReadFile(filepath.Join(projectDir, ".sth", "project.json"))
	if err != nil {
		t.Fatal(err)
	}
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"handshake","params":{"protocol_version":1}}`,
		`{"jsonrpc":"2.0","id":2,"method":"project","params":{}}`,
		`{"jsonrpc":"2.0","id":3,"method":"status","params":{}}`,
	}, "\n") + "\n"

	result := runCLIWithInput(t, projectDir, t.TempDir(), input, "bridge", "--stdio")
	if result.err != nil {
		t.Fatalf("bridge: %v\nstderr=%s", result.err, result.stderr)
	}
	responses := decodeBridgeTestResponses(t, result.stdout)
	if len(responses) != 3 {
		t.Fatalf("responses=%d, want 3: %s", len(responses), result.stdout)
	}
	handshake := responses[0]["result"].(map[string]any)
	if handshake["protocol_version"] != float64(1) {
		t.Fatalf("handshake=%v", handshake)
	}
	methods := handshake["methods"].([]any)
	for _, expected := range []string{
		"catalogue", "update.preview", "update", "remove.preview", "remove",
		"library.list", "library.get", "library.save", "capture",
		"promote.preview", "promote",
	} {
		found := false
		for _, method := range methods {
			if method == expected {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("handshake methods=%v, missing %s", methods, expected)
		}
	}
	project := responses[1]["result"].(map[string]any)
	canonicalProjectDir, err := filepath.EvalSymlinks(projectDir)
	if err != nil {
		t.Fatal(err)
	}
	if project["configured"] != true || project["root"] != canonicalProjectDir {
		t.Fatalf("project=%v", project)
	}
	status := responses[2]["result"].(map[string]any)
	if len(status["resources"].([]any)) != 0 {
		t.Fatalf("status=%v", status)
	}
	after, err := os.ReadFile(filepath.Join(projectDir, ".sth", "project.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("bridge read mutated project.json")
	}
}

func TestRun_bridgeRejectsUnknownProtocolPerFrame(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"handshake","params":{"protocol_version":2}}` + "\n"
	result := runCLIWithInput(t, t.TempDir(), t.TempDir(), input, "bridge", "--stdio")
	if result.err != nil {
		t.Fatal(result.err)
	}
	responses := decodeBridgeTestResponses(t, result.stdout)
	errorValue := responses[0]["error"].(map[string]any)
	if errorValue["code"] != float64(-32001) {
		t.Fatalf("error=%v", errorValue)
	}
}

func TestRun_bridgeReadsProviderStateAndExposesMaterializedTargetKinds(t *testing.T) {
	projectDir := t.TempDir()
	writeProjectConfigV2ForTest(t, projectDir, fs.ProjectConfigV2{
		SchemaVersion: 2,
		Providers: []fs.ProviderEntry{{
			ID: "default", Provider: "github", RepositoryID: "owner/repo",
			CatalogRef: "main", CatalogPath: "SKILLS.md", TargetDir: ".sth",
		}},
		Targets: []fs.TargetEntry{
			{ID: "claude-local", Kind: "claude", RootDir: ".claude", Enabled: true},
			{ID: "codex-local", Kind: "codex", RootDir: ".agents", Enabled: true},
		},
		Resources: []fs.ResourceEntry{},
	})
	state := fs.InstalledResourceStateV2{
		SchemaVersion: 2,
		Entries: []fs.InstalledResourceRecordV2{{
			ResourceName:    "skill::skills::review-pr",
			ProviderID:      "default",
			ResolvedVersion: "abc123",
			ArtifactKind:    "skill",
			ManagedFiles: []fs.ManagedFile{
				{Path: ".claude/skills/review-pr/SKILL.md", SHA256: "sha-claude"},
				{Path: ".agents/skills/review-pr/SKILL.md", SHA256: "sha-codex"},
			},
		}},
	}
	stateBytes, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".sth", "sth-state-v2.json"), stateBytes, 0o600); err != nil {
		t.Fatal(err)
	}

	input := `{"jsonrpc":"2.0","id":1,"method":"status","params":{}}` + "\n"
	result := runCLIWithInput(t, projectDir, t.TempDir(), input, "bridge", "--stdio")
	if result.err != nil {
		t.Fatalf("bridge: %v\nstderr=%s", result.err, result.stderr)
	}
	response := decodeBridgeTestResponses(t, result.stdout)[0]["result"].(map[string]any)
	resources := response["resources"].([]any)
	if len(resources) != 1 {
		t.Fatalf("resources=%v, want one installed skill", resources)
	}
	resource := resources[0].(map[string]any)
	if resource["skill_name"] != "review-pr" {
		t.Fatalf("skill_name=%v, want review-pr", resource["skill_name"])
	}
	targets := resource["targets"].([]any)
	if len(targets) != 2 || targets[0] != "claude" || targets[1] != "codex" {
		t.Fatalf("targets=%v, want [claude codex]", targets)
	}
}

func TestRun_bridgeRequiresExplicitConfirmationBeforeInstall(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"install","params":{"resource_ref":"review-pr"}}` + "\n"
	result := runCLIWithInput(t, t.TempDir(), t.TempDir(), input, "bridge", "--stdio")
	if result.err != nil {
		t.Fatal(result.err)
	}
	responses := decodeBridgeTestResponses(t, result.stdout)
	errorValue := responses[0]["error"].(map[string]any)
	if errorValue["code"] != float64(-32602) {
		t.Fatalf("error=%v", errorValue)
	}
}

func TestRun_bridgeRequiresExplicitConfirmationBeforeUpdateAndRemove(t *testing.T) {
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"update","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"remove","params":{"resource_ref":"review-pr"}}`,
	}, "\n") + "\n"
	result := runCLIWithInput(t, t.TempDir(), t.TempDir(), input, "bridge", "--stdio")
	if result.err != nil {
		t.Fatal(result.err)
	}
	responses := decodeBridgeTestResponses(t, result.stdout)
	for _, response := range responses {
		errorValue := response["error"].(map[string]any)
		if errorValue["code"] != float64(-32602) {
			t.Fatalf("error=%v", errorValue)
		}
	}
}

func TestRun_bridgeCaptureIsExplicitAndUsesLocalCodeState(t *testing.T) {
	projectDir := t.TempDir()
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"capture","params":{"kind":"prompt","title":"Review","body":"Review this diff"}}`,
		`{"jsonrpc":"2.0","id":2,"method":"capture","params":{"kind":"prompt","title":"Review","body":"Review this diff","tags":["git"],"confirmed":true}}`,
		`{"jsonrpc":"2.0","id":3,"method":"library.list","params":{}}`,
	}, "\n") + "\n"
	result := runCLIWithInput(t, projectDir, t.TempDir(), input, "bridge", "--stdio")
	if result.err != nil {
		t.Fatal(result.err)
	}
	responses := decodeBridgeTestResponses(t, result.stdout)
	if errorValue := responses[0]["error"].(map[string]any); errorValue["code"] != float64(-32602) {
		t.Fatalf("capture without confirmation error=%v", errorValue)
	}
	saved := responses[1]["result"].(map[string]any)
	if saved["title"] != "Review" || saved["source"] != "local" {
		t.Fatalf("saved=%v", saved)
	}
	listed := responses[2]["result"].(map[string]any)["items"].([]any)
	if len(listed) != 1 {
		t.Fatalf("listed=%v", listed)
	}
	if _, err := os.Stat(filepath.Join(projectDir, ".sth", "local", "code", "library.json")); err != nil {
		t.Fatalf("local library missing: %v", err)
	}
}

func TestBridgeInstalledSkillNameUsesOnlyMaterializedAgentsSkill(t *testing.T) {
	record := fs.InstalledResourceRecordV2{
		ResourceName: "skill::catalog::review-pr",
		ManagedFiles: []fs.ManagedFile{
			{Path: ".agents/skills/review-pr/SKILL.md"},
			{Path: ".agents/skills/review-pr/references/checklist.md"},
		},
	}
	if got := bridgeInstalledSkillName(record, "skill"); got != "review-pr" {
		t.Fatalf("skill name = %q, want review-pr", got)
	}
	if got := bridgeInstalledSkillName(record, "agent"); got != "" {
		t.Fatalf("agent unexpectedly exposed as skill %q", got)
	}
	if got := bridgeInstalledSkillName(fs.InstalledResourceRecordV2{
		ManagedFiles: []fs.ManagedFile{{Path: ".claude/skills/review-pr/SKILL.md"}},
	}, "skill"); got != "" {
		t.Fatalf("non-Codex skill unexpectedly exposed as %q", got)
	}
}

func decodeBridgeTestResponses(t *testing.T, output string) []map[string]any {
	t.Helper()
	responses := []map[string]any{}
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		var response map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &response); err != nil {
			t.Fatalf("response JSON: %v (%q)", err, scanner.Text())
		}
		responses = append(responses, response)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return responses
}
