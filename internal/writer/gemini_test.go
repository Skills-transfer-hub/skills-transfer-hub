package writer

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

var (
	_ TargetWriter  = GeminiWriter{}
	_ TargetPlanner = GeminiWriter{}
)

func TestGeminiPlanPaths(t *testing.T) {
	writer := GeminiWriter{}
	if writer.Kind() != "gemini" {
		t.Fatalf("kind = %q, want gemini", writer.Kind())
	}

	skill := writer.Plan("skill", "form-wizard", "", []string{
		"react/form-wizard/SKILL.md",
		"react/form-wizard/references/patterns.md",
	})
	expectedSkillPaths := map[string]string{
		"react/form-wizard/SKILL.md":               ".agents/skills/form-wizard/SKILL.md",
		"react/form-wizard/references/patterns.md": ".agents/skills/form-wizard/references/patterns.md",
	}
	if len(skill.Files) != len(expectedSkillPaths) {
		t.Fatalf("skill files = %d, want %d", len(skill.Files), len(expectedSkillPaths))
	}
	for _, plannedFile := range skill.Files {
		if plannedFile.RelativePath != expectedSkillPaths[plannedFile.SourcePath] {
			t.Errorf("skill %q path = %q, want %q", plannedFile.SourcePath, plannedFile.RelativePath, expectedSkillPaths[plannedFile.SourcePath])
		}
	}

	testCases := []struct {
		artifactKind string
		expectedPath string
	}{
		{artifactKind: "agent", expectedPath: ".gemini/agents/reviewer.md"},
		{artifactKind: "command", expectedPath: ".gemini/commands/reviewer.toml"},
		{artifactKind: "instruction", expectedPath: ".gemini/sth/instructions/reviewer.md"},
	}
	for _, testCase := range testCases {
		result := writer.Plan(testCase.artifactKind, "reviewer", "", []string{"source/reviewer.md"})
		if result.Skipped || len(result.Files) != 1 {
			t.Fatalf("%s plan = %+v", testCase.artifactKind, result)
		}
		if result.Files[0].RelativePath != testCase.expectedPath {
			t.Errorf("%s path = %q, want %q", testCase.artifactKind, result.Files[0].RelativePath, testCase.expectedPath)
		}
	}

	unknown := writer.Plan("unknown", "reviewer", "", []string{"source/reviewer.md"})
	if !unknown.Skipped || unknown.Reason == "" {
		t.Fatalf("unknown plan = %+v, want skipped with reason", unknown)
	}
}

func TestGeminiCommandRenderingProducesDecodableTOMLStrings(t *testing.T) {
	source := []byte("---\r\nname: review\r\ndescription: 'Review l''API \\\"publique\\\"'\r\n---\r\n\r\nUse \\\\server and \\\"quoted\\\" text.\nKeep \"\"\"triple quotes\"\"\" intact.\n")
	rendered, err := RenderGeminiCommand(source)
	if err != nil {
		t.Fatal(err)
	}

	values := geminiRenderedAssignments(t, string(rendered))
	if values["description"] != "Review l'API \\\"publique\\\"" {
		t.Fatalf("description = %q", values["description"])
	}
	expectedPrompt := "Use \\\\server and \\\"quoted\\\" text.\nKeep \"\"\"triple quotes\"\"\" intact."
	if values["prompt"] != expectedPrompt {
		t.Fatalf("prompt = %q, want %q", values["prompt"], expectedPrompt)
	}
}

func TestGeminiWriteCommandAddsTOMLMarkerAndIsIdempotent(t *testing.T) {
	directory := t.TempDir()
	installedPath := filepath.Join(directory, ".gemini", "commands", "deploy.toml")
	writer := GeminiWriter{}
	request := WriteRequest{
		InstalledPath:    installedPath,
		Content:          []byte("---\ndescription: Deploy safely\n---\nRun the deployment.\n"),
		AddManagedHeader: true,
	}

	first, err := writer.Write(request)
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(installedPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(content), geminiManagedTOMLMarker) {
		t.Fatalf("TOML marker missing: %q", string(content))
	}
	values := geminiRenderedAssignments(t, strings.TrimPrefix(string(content), geminiManagedTOMLMarker))
	if values["description"] != "Deploy safely" || values["prompt"] != "Run the deployment." {
		t.Fatalf("rendered assignments = %+v", values)
	}

	second, err := writer.Write(WriteRequest{InstalledPath: installedPath, Content: content, AddManagedHeader: true})
	if err != nil {
		t.Fatal(err)
	}
	if first.SHA256 != second.SHA256 {
		t.Fatalf("rewrite changed SHA-256: %s vs %s", first.SHA256, second.SHA256)
	}
}

func TestGeminiWriteMarkdownPreservesFrontmatter(t *testing.T) {
	directory := t.TempDir()
	installedPath := filepath.Join(directory, ".gemini", "agents", "reviewer.md")
	source := []byte("---\nname: reviewer\ndescription: Reviews code\n---\nBody\n")

	_, err := (GeminiWriter{}).Write(WriteRequest{
		InstalledPath:    installedPath,
		Content:          source,
		AddManagedHeader: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(installedPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(content), "---\nname: reviewer\ndescription: Reviews code\n---\n") {
		t.Fatalf("frontmatter moved: %q", string(content))
	}
	if !strings.Contains(string(content), minimalManagedMarker) {
		t.Fatalf("managed marker missing: %q", string(content))
	}
}

func TestGeminiCommandRenderingRejectsInvalidUTF8(t *testing.T) {
	_, err := RenderGeminiCommand([]byte{0xff, 0xfe})
	if err == nil {
		t.Fatal("invalid UTF-8 should fail")
	}
}

func geminiRenderedAssignments(t *testing.T, content string) map[string]string {
	t.Helper()
	assignments := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(content), "\n") {
		parts := strings.SplitN(line, " = ", 2)
		if len(parts) != 2 {
			t.Fatalf("invalid TOML assignment %q", line)
		}
		value, err := strconv.Unquote(parts[1])
		if err != nil {
			t.Fatalf("invalid TOML string %q: %v", parts[1], err)
		}
		assignments[parts[0]] = value
	}
	return assignments
}
