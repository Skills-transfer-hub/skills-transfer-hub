package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateWithoutTargetChecksStructureOnly(t *testing.T) {
	projectRoot := chdirToTemp(t)
	workflowPath := filepath.Join(projectRoot, ".claude", "workflows", "release.js")
	if err := os.MkdirAll(filepath.Dir(workflowPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(workflowPath, []byte("export const meta = { name: 'release' }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	application := newApp(nil, io.Discard, io.Discard, nil)
	if err := application.runValidate(nil); err != nil {
		t.Fatalf("structurally valid native workflow failed: %v", err)
	}
}

func TestInspectReportsNativeOnlyBeforeWriting(t *testing.T) {
	projectRoot := chdirToTemp(t)
	rulePath := filepath.Join(projectRoot, ".codex", "rules", "safe.rules")
	if err := os.MkdirAll(filepath.Dir(rulePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rulePath, []byte("prefix_rule(pattern = [\"git\", \"status\"], decision = \"allow\")\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	application := newApp(nil, &stdout, io.Discard, nil)
	if err := application.runInspect([]string{rulePath, "--target", "claude", "--json"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), `"level": "native-only"`) || !strings.Contains(stdout.String(), `"code": "native_only"`) {
		t.Fatalf("inspect output = %s", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(projectRoot, ".claude", "rules", "safe.md")); !os.IsNotExist(err) {
		t.Fatalf("inspect mutated project: %v", err)
	}
}

func TestValidateStrictFailsOnUnmappedNativeFields(t *testing.T) {
	projectRoot := chdirToTemp(t)
	agentPath := filepath.Join(projectRoot, ".codex", "agents", "reviewer.toml")
	if err := os.MkdirAll(filepath.Dir(agentPath), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "name = \"reviewer\"\ndescription = \"Review\"\ndeveloper_instructions = \"Review\"\nunknown = \"retained\"\n"
	if err := os.WriteFile(agentPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	application := newApp(nil, io.Discard, io.Discard, nil)
	if err := application.runValidate([]string{"--strict"}); err == nil {
		t.Fatal("strict validation accepted an unmapped native field warning")
	}
}

func TestInspectPlansManagedInstructionRegionOutput(t *testing.T) {
	projectRoot := chdirToTemp(t)
	instructionPath := filepath.Join(projectRoot, "AGENTS.md")
	if err := os.WriteFile(instructionPath, []byte("Project instructions\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	application := newApp(nil, &stdout, io.Discard, nil)
	if err := application.runInspect([]string{instructionPath, "--target", "cursor", "--json"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), `"path": "AGENTS.md"`) || !strings.Contains(stdout.String(), `"level": "lossy"`) {
		t.Fatalf("inspect output = %s", stdout.String())
	}
}
