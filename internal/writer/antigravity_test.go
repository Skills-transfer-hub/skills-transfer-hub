package writer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAntigravityPlanPaths(t *testing.T) {
	target := NewAntigravityWriter()
	if target.Kind() != "antigravity" {
		t.Fatalf("Kind() = %q, want antigravity", target.Kind())
	}

	skillPlan := target.Plan("skill", "form-wizard", "", []string{
		"react/form-wizard/SKILL.md",
		"react/form-wizard/references/patterns.md",
	})
	expectedSkillPaths := map[string]string{
		"react/form-wizard/SKILL.md":               ".agents/skills/form-wizard/SKILL.md",
		"react/form-wizard/references/patterns.md": ".agents/skills/form-wizard/references/patterns.md",
	}
	if skillPlan.Skipped || skillPlan.Reason != "" {
		t.Fatalf("skill plan should be supported, got %+v", skillPlan)
	}
	if len(skillPlan.Files) != len(expectedSkillPaths) {
		t.Fatalf("skill: got %d files, want %d", len(skillPlan.Files), len(expectedSkillPaths))
	}
	for _, plannedFile := range skillPlan.Files {
		expectedPath, exists := expectedSkillPaths[plannedFile.SourcePath]
		if !exists {
			t.Errorf("unexpected skill source %q", plannedFile.SourcePath)
			continue
		}
		if expectedPath != plannedFile.RelativePath {
			t.Errorf("skill %q: got %q, want %q", plannedFile.SourcePath, plannedFile.RelativePath, expectedPath)
		}
	}

	planCases := []struct {
		name         string
		artifactKind string
		artifactName string
		sourcePath   string
		expectedPath string
	}{
		{name: "agent", artifactKind: "agent", artifactName: "code-reviewer", sourcePath: "src/agent.md", expectedPath: ".agents/agents/code-reviewer.md"},
		{name: "workflow", artifactKind: "command", artifactName: "deploy", sourcePath: "src/cmd.md", expectedPath: ".agents/workflows/deploy.md"},
		{name: "rule", artifactKind: "instruction", artifactName: "style", sourcePath: "src/style.md", expectedPath: ".agents/rules/style.md"},
	}
	for _, planCase := range planCases {
		t.Run(planCase.name, func(t *testing.T) {
			plan := target.Plan(planCase.artifactKind, planCase.artifactName, "", []string{planCase.sourcePath})
			if plan.Skipped || plan.Reason != "" {
				t.Fatalf("plan should be supported, got %+v", plan)
			}
			if len(plan.Files) != 1 {
				t.Fatalf("got %d files, want 1", len(plan.Files))
			}
			if plan.Files[0].SourcePath != planCase.sourcePath {
				t.Errorf("SourcePath = %q, want %q", plan.Files[0].SourcePath, planCase.sourcePath)
			}
			if plan.Files[0].RelativePath != planCase.expectedPath {
				t.Errorf("RelativePath = %q, want %q", plan.Files[0].RelativePath, planCase.expectedPath)
			}
		})
	}
}

func TestAntigravityPlanValidatesInput(t *testing.T) {
	target := NewAntigravityWriter()
	invalidPlans := []PlanResult{
		target.Plan("skill", "", "", []string{"src/SKILL.md"}),
		target.Plan("skill", "   ", "", []string{"src/SKILL.md"}),
		target.Plan("skill", "form-wizard", "", nil),
	}
	for _, invalidPlan := range invalidPlans {
		if invalidPlan.Skipped || invalidPlan.Reason != "" || len(invalidPlan.Files) != 0 {
			t.Errorf("incomplete input should return an empty plan, got %+v", invalidPlan)
		}
	}

	unknownPlan := target.Plan("unsupported", "form-wizard", "", []string{"src/SKILL.md"})
	if !unknownPlan.Skipped || unknownPlan.Reason == "" || len(unknownPlan.Files) != 0 {
		t.Errorf("unknown artifact should be skipped with a reason, got %+v", unknownPlan)
	}
}

func TestAntigravityWritePreservesFrontmatterAndIsIdempotent(t *testing.T) {
	temporaryDirectory := t.TempDir()
	installedPath := filepath.Join(temporaryDirectory, ".agents", "skills", "x", "SKILL.md")
	target := NewAntigravityWriter()
	sourceContent := []byte("---\nname: x\ndescription: X skill\n---\n# Title\n")

	firstResult, err := target.Write(WriteRequest{InstalledPath: installedPath, Content: sourceContent, AddManagedHeader: true})
	if err != nil {
		t.Fatal(err)
	}
	writtenContent, err := os.ReadFile(installedPath)
	if err != nil {
		t.Fatal(err)
	}
	expectedContent := "---\nname: x\ndescription: X skill\n---\n" + minimalManagedMarker + "# Title\n"
	if string(writtenContent) != expectedContent {
		t.Errorf("written content = %q, want %q", string(writtenContent), expectedContent)
	}
	expectedPath := filepath.ToSlash(filepath.Clean(installedPath))
	if firstResult.Path != expectedPath {
		t.Errorf("Path = %q, want %q", firstResult.Path, expectedPath)
	}
	if firstResult.SHA256 != computeSHA256(writtenContent) {
		t.Errorf("SHA256 = %q, want digest of written content", firstResult.SHA256)
	}

	secondResult, err := target.Write(WriteRequest{InstalledPath: installedPath, Content: writtenContent, AddManagedHeader: true})
	if err != nil {
		t.Fatal(err)
	}
	if firstResult.SHA256 != secondResult.SHA256 {
		t.Errorf("re-write changed content: %s vs %s", firstResult.SHA256, secondResult.SHA256)
	}
}

func TestAntigravityWriteValidatesPathAndLeavesAssetsUnmarked(t *testing.T) {
	target := NewAntigravityWriter()
	if _, err := target.Write(WriteRequest{InstalledPath: "   ", Content: []byte("ignored")}); err == nil {
		t.Fatal("blank installed path should fail")
	}

	temporaryDirectory := t.TempDir()
	installedPath := filepath.Join(temporaryDirectory, ".agents", "skills", "x", "references", "schema.json")
	sourceContent := []byte(`{"type":"object"}`)
	result, err := target.Write(WriteRequest{InstalledPath: installedPath, Content: sourceContent, AddManagedHeader: true})
	if err != nil {
		t.Fatal(err)
	}
	writtenContent, err := os.ReadFile(installedPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(writtenContent) != string(sourceContent) {
		t.Errorf("non-Markdown asset changed: got %q, want %q", string(writtenContent), string(sourceContent))
	}
	if strings.Contains(string(writtenContent), minimalManagedMarker) {
		t.Errorf("non-Markdown asset contains managed marker: %q", string(writtenContent))
	}
	if result.SHA256 != computeSHA256(sourceContent) {
		t.Errorf("SHA256 = %q, want digest of source content", result.SHA256)
	}
}
