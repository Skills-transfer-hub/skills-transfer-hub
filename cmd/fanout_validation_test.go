package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"sth/internal/fs"
)

func TestValidateFanoutTargetRootsAcceptsCanonicalRootForEveryTarget(t *testing.T) {
	testCases := []struct {
		targetKind    string
		canonicalRoot string
	}{
		{targetKind: "claude", canonicalRoot: ".claude"},
		{targetKind: "copilot", canonicalRoot: ".github"},
		{targetKind: "codex", canonicalRoot: ".codex"},
		{targetKind: "cursor", canonicalRoot: ".cursor"},
		{targetKind: "antigravity", canonicalRoot: ".agents"},
		{targetKind: "gemini", canonicalRoot: ".gemini"},
	}

	for _, testCase := range testCases {
		config := fanoutConfig(fanoutTarget(testCase.targetKind, testCase.canonicalRoot))
		if err := validateFanoutTargetRoots(config); err != nil {
			t.Errorf("canonical root rejected for %s: %v", testCase.targetKind, err)
		}
	}
}

func TestValidateFanoutTargetRootsRejectsCustomRootForEveryTarget(t *testing.T) {
	testCases := []struct {
		targetKind    string
		canonicalRoot string
	}{
		{targetKind: "claude", canonicalRoot: ".claude"},
		{targetKind: "copilot", canonicalRoot: ".github"},
		{targetKind: "codex", canonicalRoot: ".codex"},
		{targetKind: "cursor", canonicalRoot: ".cursor"},
		{targetKind: "antigravity", canonicalRoot: ".agents"},
		{targetKind: "gemini", canonicalRoot: ".gemini"},
	}

	for _, testCase := range testCases {
		config := fanoutConfig(fanoutTarget(testCase.targetKind, "custom/"+testCase.targetKind))
		err := validateFanoutTargetRoots(config)
		if err == nil {
			t.Errorf("custom root accepted for %s", testCase.targetKind)
			continue
		}
		if !strings.Contains(err.Error(), testCase.targetKind) || !strings.Contains(err.Error(), testCase.canonicalRoot) {
			t.Errorf("custom root error for %s = %v", testCase.targetKind, err)
		}
	}
}

func TestInstallFanoutRejectsCustomRootWithoutMutation(t *testing.T) {
	projectDirectory := t.TempDir()
	server := skillBundleServer(t)
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)
	t.Setenv("STH_GITHUB_API_BASE_URL", server.URL)
	writeProjectConfigV2ForTest(t, projectDirectory, fanoutConfig(fanoutTarget("codex", "custom/codex")))
	before := transactionalProjectFileSnapshot(t, projectDirectory)

	result := runCLI(t, projectDirectory, t.TempDir(), "install", "alpha")
	if result.err == nil || !strings.Contains(result.err.Error(), "root_dir") {
		t.Fatalf("install error = %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}

	after := transactionalProjectFileSnapshot(t, projectDirectory)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("custom root rejection mutated project files:\nbefore=%v\nafter=%v", before, after)
	}
	assertNoFile(t, filepath.Join(projectDirectory, ".agents", "skills", "alpha", "SKILL.md"))
	assertNoFile(t, filepath.Join(projectDirectory, "custom", "codex", "skills", "alpha", "SKILL.md"))
}

func TestInstallFanoutRejectsInvalidCodexSkillWithoutMutation(t *testing.T) {
	projectDirectory := t.TempDir()
	fixture := newFanoutLifecycleFixture(t, "skill", "invalid-codex", "skills")
	fixture.content = "---\nname: invalid-codex\n---\n# Missing description\n"
	t.Setenv("STH_GITHUB_RAW_BASE_URL", fixture.server.URL)
	t.Setenv("STH_GITHUB_API_BASE_URL", fixture.server.URL)
	writeProjectConfigV2ForTest(t, projectDirectory, fanoutConfig(fanoutTarget("codex", ".codex")))
	before := transactionalProjectFileSnapshot(t, projectDirectory)

	result := runCLI(t, projectDirectory, t.TempDir(), "install", "invalid-codex")
	if result.err == nil || !strings.Contains(result.err.Error(), "frontmatter name and description are required") {
		t.Fatalf("install error = %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}

	after := transactionalProjectFileSnapshot(t, projectDirectory)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("invalid Codex skill mutated project files:\nbefore=%v\nafter=%v", before, after)
	}
	assertNoFile(t, filepath.Join(projectDirectory, ".agents", "skills", "invalid-codex", "SKILL.md"))
	assertNoFile(t, filepath.Join(projectDirectory, ".sth", "sth-state-v2.json"))
}

func TestUpdateFanoutRejectsInvalidCodexSkillAndPreservesInstalledVersion(t *testing.T) {
	projectDirectory := t.TempDir()
	fixture := newFanoutLifecycleFixture(t, "skill", "validated-codex", "skills")
	t.Setenv("STH_GITHUB_RAW_BASE_URL", fixture.server.URL)
	t.Setenv("STH_GITHUB_API_BASE_URL", fixture.server.URL)
	writeProjectConfigV2ForTest(t, projectDirectory, fanoutConfig(fanoutTarget("codex", ".codex")))

	installResult := runCLI(t, projectDirectory, t.TempDir(), "install", "validated-codex")
	if installResult.err != nil {
		t.Fatalf("install error = %v\nstderr=%s", installResult.err, installResult.stderr)
	}
	installedPath := filepath.Join(projectDirectory, ".agents", "skills", "validated-codex", "SKILL.md")
	validContent, err := os.ReadFile(installedPath)
	if err != nil || !strings.Contains(string(validContent), "name: validated-codex") || !strings.Contains(string(validContent), "description: Lifecycle skill") {
		t.Fatalf("valid Codex skill content = %q, error=%v", validContent, err)
	}
	before := transactionalProjectFileSnapshot(t, projectDirectory)
	fixture.content = "---\nname: validated-codex\n---\n# Missing description\n"
	fixture.commitSHA = "2222222222222222"

	updateResult := runCLI(t, projectDirectory, t.TempDir(), "update")
	if updateResult.err == nil || !strings.Contains(updateResult.err.Error()+updateResult.stderr, "frontmatter name and description are required") {
		t.Fatalf("update error = %v\nstdout=%s\nstderr=%s", updateResult.err, updateResult.stdout, updateResult.stderr)
	}

	after := transactionalProjectFileSnapshot(t, projectDirectory)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("invalid Codex update changed installed files:\nbefore=%v\nafter=%v", before, after)
	}
	installedContent, err := os.ReadFile(installedPath)
	if err != nil || !strings.Contains(string(installedContent), "SENTINEL_V1") {
		t.Fatalf("installed skill after rejected update = %q, error=%v", installedContent, err)
	}
}

func TestInstallFanoutRejectsAntigravitySkillWithoutDescription(t *testing.T) {
	projectDirectory := t.TempDir()
	fixture := newFanoutLifecycleFixture(t, "skill", "antigravity-only", "skills")
	fixture.content = "---\nname: antigravity-only\n---\n# Antigravity skill\n"
	t.Setenv("STH_GITHUB_RAW_BASE_URL", fixture.server.URL)
	t.Setenv("STH_GITHUB_API_BASE_URL", fixture.server.URL)
	writeProjectConfigV2ForTest(t, projectDirectory, fanoutConfig(fanoutTarget("antigravity", ".agents")))
	before := transactionalProjectFileSnapshot(t, projectDirectory)

	result := runCLI(t, projectDirectory, t.TempDir(), "install", "antigravity-only")
	if result.err == nil || !strings.Contains(result.err.Error()+result.stderr, "description must be a non-empty string") {
		t.Fatalf("install error = %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}

	after := transactionalProjectFileSnapshot(t, projectDirectory)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("invalid Antigravity skill mutated project files:\nbefore=%v\nafter=%v", before, after)
	}
	assertNoFile(t, filepath.Join(projectDirectory, ".agents", "skills", "antigravity-only", "SKILL.md"))
	assertNoFile(t, filepath.Join(projectDirectory, ".sth", "sth-state-v2.json"))
}

func TestInstallFanoutAcceptsAntigravitySkillWithoutName(t *testing.T) {
	projectDirectory := t.TempDir()
	fixture := newFanoutLifecycleFixture(t, "skill", "antigravity-no-name", "skills")
	fixture.content = "---\ndescription: Antigravity skill without explicit name\n---\n# Antigravity skill\n"
	t.Setenv("STH_GITHUB_RAW_BASE_URL", fixture.server.URL)
	t.Setenv("STH_GITHUB_API_BASE_URL", fixture.server.URL)
	writeProjectConfigV2ForTest(t, projectDirectory, fanoutConfig(fanoutTarget("antigravity", ".agents")))

	result := runCLI(t, projectDirectory, t.TempDir(), "install", "antigravity-no-name")
	if result.err != nil {
		t.Fatalf("install error = %v\nstderr=%s", result.err, result.stderr)
	}

	installedPath := filepath.Join(projectDirectory, ".agents", "skills", "antigravity-no-name", "SKILL.md")
	installedContent, err := os.ReadFile(installedPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(installedContent), "description: Antigravity skill without explicit name") || !strings.Contains(string(installedContent), "# Antigravity skill") {
		t.Fatalf("Antigravity skill content = %s", installedContent)
	}
	state := readStateV2(t, projectDirectory)
	if len(state.Entries) != 1 || len(state.Entries[0].ManagedFiles) != 1 || state.Entries[0].ManagedFiles[0].Path != ".agents/skills/antigravity-no-name/SKILL.md" {
		t.Fatalf("Antigravity skill state = %+v", state.Entries)
	}
}

func TestValidateFanoutTargetRootsIgnoresUnknownTargetKind(t *testing.T) {
	config := fanoutConfig(fs.TargetEntry{ID: "future", Kind: "future", RootDir: "custom/future", Enabled: true})
	if err := validateFanoutTargetRoots(config); err != nil {
		t.Fatalf("unknown target root validation error = %v", err)
	}
}

func TestValidateFanoutTargetRootsIgnoresDisabledCustomRoot(t *testing.T) {
	config := fanoutConfig(fs.TargetEntry{ID: "old-codex", Kind: "codex", RootDir: ".agents", Enabled: false})
	if err := validateFanoutTargetRoots(config); err != nil {
		t.Fatalf("disabled target root validation error = %v", err)
	}
}

func TestInstallCursorSkillValidatesActualSharedHub(t *testing.T) {
	projectDirectory := t.TempDir()
	fixture := newFanoutLifecycleFixture(t, "skill", "cursor-shared", "skills")
	t.Setenv("STH_GITHUB_RAW_BASE_URL", fixture.server.URL)
	t.Setenv("STH_GITHUB_API_BASE_URL", fixture.server.URL)
	writeProjectConfigV2ForTest(t, projectDirectory, fanoutConfig(fanoutTarget("cursor", ".cursor")))
	if err := os.Symlink(t.TempDir(), filepath.Join(projectDirectory, ".cursor")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	result := runCLI(t, projectDirectory, t.TempDir(), "install", "cursor-shared")
	if result.err != nil {
		t.Fatalf("install error = %v\nstderr=%s", result.err, result.stderr)
	}
	installedContent, err := os.ReadFile(filepath.Join(projectDirectory, ".agents", "skills", "cursor-shared", "SKILL.md"))
	if err != nil || !strings.Contains(string(installedContent), "SENTINEL_V1") {
		t.Fatalf("installed content = %q, error = %v", installedContent, err)
	}
}
