package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScaffoldPack_FlatLayoutWritesExpectedTree(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "my-pack")

	if err := scaffoldPack(target, "my-pack", packLayoutFlat); err != nil {
		t.Fatalf("scaffoldPack: %v", err)
	}

	mustExist(t, filepath.Join(target, "README.md"))
	mustExist(t, filepath.Join(target, ".sthignore"))
	mustExist(t, filepath.Join(target, "SKILLS.md"))
	mustExist(t, filepath.Join(target, "manifest.json"))
	mustExist(t, filepath.Join(target, "sample-skill", "SKILL.md"))

	// Flat layout must NOT scaffold the multi-stack containers.
	if _, err := os.Stat(filepath.Join(target, "agents")); err == nil {
		t.Error("flat layout should not create agents/")
	}
}

func TestScaffoldPack_MultiStackLayoutWritesExpectedTree(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "my-pack")

	if err := scaffoldPack(target, "my-pack", packLayoutMultiStack); err != nil {
		t.Fatalf("scaffoldPack: %v", err)
	}

	for _, rel := range []string{
		"README.md",
		"SKILLS.md",
		"manifest.json",
		"skills/sample/SKILL.md",
		"agents/sample.md",
		"commands/sample.md",
		"common/instructions/sample.instructions.md",
	} {
		mustExist(t, filepath.Join(target, rel))
	}
}

func TestScaffoldPack_ManifestDeclaresLayoutAndTargets(t *testing.T) {
	dir := t.TempDir()
	if err := scaffoldPack(dir, "my-pack", packLayoutMultiStack); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, `"layout": "multi-stack"`) {
		t.Errorf("manifest missing layout, got %s", s)
	}
	for _, targetKind := range []string{"claude", "copilot", "codex", "cursor", "antigravity", "gemini"} {
		if !strings.Contains(s, `"`+targetKind+`"`) {
			t.Errorf("manifest missing target %s, got %s", targetKind, s)
		}
	}
}

func TestScaffoldPack_SampleSkillCarriesFrontmatter(t *testing.T) {
	dir := t.TempDir()
	if err := scaffoldPack(dir, "my-pack", packLayoutFlat); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "sample-skill", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "---\nname: sample-skill\n") {
		t.Errorf("sample skill should start with valid YAML frontmatter, got %q", string(data[:80]))
	}
}

func TestScaffoldPack_SampleAgentUsesPortableFrontmatter(t *testing.T) {
	dir := t.TempDir()
	if err := scaffoldPack(dir, "my-pack", packLayoutMultiStack); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "agents", "sample.md"))
	s := string(data)
	if !strings.Contains(s, "name: sample") || !strings.Contains(s, "description:") {
		t.Errorf("sample agent should declare name and description, got %q", s)
	}
	if strings.Contains(s, "tools:") || strings.Contains(s, "model:") {
		t.Errorf("sample agent should not hard-code provider-specific tools or model, got %q", s)
	}
}

func TestScaffoldPack_SampleInstructionHasApplyTo(t *testing.T) {
	dir := t.TempDir()
	if err := scaffoldPack(dir, "my-pack", packLayoutMultiStack); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "common", "instructions", "sample.instructions.md"))
	if !strings.Contains(string(data), "applyTo:") {
		t.Errorf("sample instruction should carry applyTo, got %q", string(data))
	}
}

func TestValidatePack_PassesForFreshScaffold(t *testing.T) {
	dir := t.TempDir()
	if err := scaffoldPack(dir, "x", packLayoutMultiStack); err != nil {
		t.Fatal(err)
	}
	if err := validatePack(dir, packLayoutMultiStack); err != nil {
		t.Errorf("validatePack: %v", err)
	}
}

func TestScaffoldPack_RefusesNonEmptyTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "existing")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "README.md"), []byte("user file"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := scaffoldPack(target, "my-pack", packLayoutFlat); err == nil {
		t.Fatal("scaffoldPack must refuse to write into a non-empty existing directory")
	}
	data, _ := os.ReadFile(filepath.Join(target, "README.md"))
	if string(data) != "user file" {
		t.Fatalf("existing user file was overwritten, got %q", string(data))
	}
}

func TestScaffoldPack_AcceptsEmptyExistingTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "empty")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := scaffoldPack(target, "my-pack", packLayoutFlat); err != nil {
		t.Fatalf("scaffoldPack should accept an empty existing directory, got %v", err)
	}
	mustExist(t, filepath.Join(target, "manifest.json"))
}

func TestValidatePack_FailsWhenManifestMissing(t *testing.T) {
	dir := t.TempDir()
	if err := scaffoldPack(dir, "x", packLayoutFlat); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(filepath.Join(dir, "manifest.json"))
	if err := validatePack(dir, packLayoutFlat); err == nil {
		t.Error("validatePack should fail when manifest.json is missing")
	}
}

func TestIsValidPackName(t *testing.T) {
	good := []string{"a", "my-pack", "my_pack_2", "skills"}
	for _, name := range good {
		if !isValidPackName(name) {
			t.Errorf("%q should be valid", name)
		}
	}
	bad := []string{"", "My-Pack", "my pack", "../escape", "café"}
	for _, name := range bad {
		if isValidPackName(name) {
			t.Errorf("%q should be rejected", name)
		}
	}
}

func mustExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file %q to exist: %v", path, err)
	}
}
