package writer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAntigravity_PlanPaths(t *testing.T) {
	target := AntigravityWriter{}

	skill := target.Plan("skill", "form-wizard", "", []string{
		"react/form-wizard/SKILL.md",
		"react/form-wizard/references/patterns.md",
	})
	wantSkill := map[string]string{
		"react/form-wizard/SKILL.md":               ".agents/skills/form-wizard/SKILL.md",
		"react/form-wizard/references/patterns.md": ".agents/skills/form-wizard/references/patterns.md",
	}
	if len(skill.Files) != len(wantSkill) {
		t.Fatalf("skill: got %d files, want %d", len(skill.Files), len(wantSkill))
	}
	for _, planned := range skill.Files {
		if wantSkill[planned.SourcePath] != planned.RelativePath {
			t.Errorf("skill %q: got %q, want %q", planned.SourcePath, planned.RelativePath, wantSkill[planned.SourcePath])
		}
	}

	agent := target.Plan("agent", "code-reviewer", "", []string{"src/agent.md"})
	if len(agent.Files) != 1 || agent.Files[0].RelativePath != ".agents/agents/code-reviewer.md" {
		t.Errorf("agent: got %+v, want .agents/agents/code-reviewer.md", agent.Files)
	}

	command := target.Plan("command", "deploy", "", []string{"src/cmd.md"})
	if len(command.Files) != 1 || command.Files[0].RelativePath != ".agents/workflows/deploy.md" {
		t.Errorf("command: got %+v, want .agents/workflows/deploy.md", command.Files)
	}

	instruction := target.Plan("instruction", "style", "", []string{"src/style.md"})
	if !instruction.Skipped || instruction.Reason == "" {
		t.Errorf("instruction should be skipped with reason, got %+v", instruction)
	}
}

func TestAntigravity_WriteMarkerIdempotent(t *testing.T) {
	dir := t.TempDir()
	installedPath := filepath.Join(dir, ".agents", "skills", "x", "SKILL.md")
	target := AntigravityWriter{}

	first, err := target.Write(WriteRequest{InstalledPath: installedPath, Content: []byte("# Title\n"), AddManagedHeader: true})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(installedPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), minimalManagedMarker) {
		t.Errorf("expected minimal marker prefix, got %q", string(data))
	}

	second, err := target.Write(WriteRequest{InstalledPath: installedPath, Content: data, AddManagedHeader: true})
	if err != nil {
		t.Fatal(err)
	}
	if first.SHA256 != second.SHA256 {
		t.Errorf("re-write changed content: %s vs %s", first.SHA256, second.SHA256)
	}
}
