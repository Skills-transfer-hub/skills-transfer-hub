package writer

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCopilotWriterKind(t *testing.T) {
	if actualKind := (CopilotWriter{}).Kind(); actualKind != KindCopilot {
		t.Fatalf("Kind() = %q, want %q", actualKind, KindCopilot)
	}
}

func TestCopilotWriterPlansNativeSingleFileArtifacts(t *testing.T) {
	testCases := []struct {
		artifactKind string
		artifactName string
		sourcePath   string
		expectedPath string
	}{
		{artifactKind: "agent", artifactName: "reviewer", sourcePath: "agents/reviewer.md", expectedPath: ".github/agents/reviewer.agent.md"},
		{artifactKind: "instruction", artifactName: "go", sourcePath: "instructions/go.md", expectedPath: ".github/sth/instructions/go.md"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.artifactKind, func(t *testing.T) {
			result := (CopilotWriter{}).Plan(testCase.artifactKind, testCase.artifactName, "go", []string{testCase.sourcePath})
			if result.Skipped {
				t.Fatalf("Plan() skipped %q: %s", testCase.artifactKind, result.Reason)
			}
			if len(result.Files) != 1 {
				t.Fatalf("Plan() files = %d, want 1", len(result.Files))
			}
			if result.Files[0].SourcePath != testCase.sourcePath {
				t.Fatalf("SourcePath = %q, want %q", result.Files[0].SourcePath, testCase.sourcePath)
			}
			if result.Files[0].RelativePath != testCase.expectedPath {
				t.Fatalf("RelativePath = %q, want %q", result.Files[0].RelativePath, testCase.expectedPath)
			}
		})
	}
}

func TestCopilotWriterPlansCommandPrompt(t *testing.T) {
	result := (CopilotWriter{}).Plan("command", "deploy", "", []string{"commands/deploy.md"})
	expectedFiles := []PlannedFile{
		{SourcePath: "commands/deploy.md", RelativePath: ".github/prompts/deploy.prompt.md"},
	}
	if !reflect.DeepEqual(result.Files, expectedFiles) {
		t.Fatalf("command files = %#v, want %#v", result.Files, expectedFiles)
	}
}

func TestCopilotWriterPlansNativeSkillBundle(t *testing.T) {
	sources := []string{
		"react/form-wizard/SKILL.md",
		"react/form-wizard/references/component-patterns.md",
		"react/form-wizard/scripts/check.sh",
	}
	expectedFiles := []PlannedFile{
		{SourcePath: sources[0], RelativePath: ".agents/skills/form-wizard/SKILL.md"},
		{SourcePath: sources[1], RelativePath: ".agents/skills/form-wizard/references/component-patterns.md"},
		{SourcePath: sources[2], RelativePath: ".agents/skills/form-wizard/scripts/check.sh"},
	}

	result := (CopilotWriter{}).Plan("skill", "form-wizard", "react", sources)
	if result.Skipped {
		t.Fatalf("Plan() skipped skill: %s", result.Reason)
	}
	if !reflect.DeepEqual(result.Files, expectedFiles) {
		t.Fatalf("Plan() files = %#v, want %#v", result.Files, expectedFiles)
	}
}

func TestCopilotWriterPreservesFrontmatterAndContent(t *testing.T) {
	installedPath := filepath.Join(t.TempDir(), ".github", "agents", "reviewer.agent.md")
	content := []byte("---\nname: Reviewer\ndescription: Reviews code\ntools: ['read']\n---\n\nReturn the review.\n")
	expectedContent := []byte("---\nname: Reviewer\ndescription: Reviews code\ntools: ['read']\n---\n<!-- managed by sth -->\n\nReturn the review.\n")

	result, err := (CopilotWriter{}).Write(WriteRequest{
		InstalledPath:    installedPath,
		Content:          content,
		AddManagedHeader: true,
	})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	writtenContent, err := os.ReadFile(installedPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Equal(writtenContent, expectedContent) {
		t.Fatalf("written content = %q, want %q", writtenContent, expectedContent)
	}
	if result.Path != filepath.ToSlash(filepath.Clean(installedPath)) {
		t.Fatalf("result path = %q, want %q", result.Path, filepath.ToSlash(filepath.Clean(installedPath)))
	}
	if result.SHA256 != computeSHA256(expectedContent) {
		t.Fatalf("result SHA256 = %q, want %q", result.SHA256, computeSHA256(expectedContent))
	}
}

func TestCopilotWriterPreservesBytesWithoutManagedMarker(t *testing.T) {
	installedPath := filepath.Join(t.TempDir(), ".claude", "commands", "deploy.md")
	content := []byte("---\ndescription: Deploy safely\nargument-hint: '[environment]'\n---\n\nDeploy the requested environment.\n")

	_, err := (CopilotWriter{}).Write(WriteRequest{
		InstalledPath:    installedPath,
		Content:          content,
		AddManagedHeader: false,
	})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	writtenContent, err := os.ReadFile(installedPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Equal(writtenContent, content) {
		t.Fatalf("written content = %q, want %q", writtenContent, content)
	}
}

func TestCopilotCommandRenderingMatchesClaude(t *testing.T) {
	content := []byte("---\ndescription: Deploy safely\n---\n\nDeploy safely.\n")
	claudePath := filepath.Join(t.TempDir(), ".claude", "commands", "deploy.md")
	copilotPath := filepath.Join(t.TempDir(), ".claude", "commands", "deploy.md")

	if _, err := (ClaudeWriter{}).Write(WriteRequest{InstalledPath: claudePath, Content: content, AddManagedHeader: true}); err != nil {
		t.Fatalf("Claude Write() error = %v", err)
	}
	if _, err := (CopilotWriter{}).Write(WriteRequest{InstalledPath: copilotPath, Content: content, AddManagedHeader: true}); err != nil {
		t.Fatalf("Copilot Write() error = %v", err)
	}
	claudeContent, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatal(err)
	}
	copilotContent, err := os.ReadFile(copilotPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(copilotContent, claudeContent) {
		t.Fatalf("Copilot command = %q, Claude command = %q", copilotContent, claudeContent)
	}
}

func TestCopilotPromptRenderingKeepsDescriptionAndBody(t *testing.T) {
	installedPath := filepath.Join(t.TempDir(), ".github", "prompts", "deploy.prompt.md")
	source := []byte("---\ndescription: Deploy safely\nargument-hint: '[environment]'\n---\nDeploy the requested environment.\n")

	first, err := (CopilotWriter{}).Write(WriteRequest{InstalledPath: installedPath, Content: source, AddManagedHeader: true})
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(installedPath)
	if err != nil {
		t.Fatal(err)
	}
	expected := "---\ndescription: \"Deploy safely\"\n---\n" + minimalManagedMarker + "Deploy the requested environment.\n"
	if string(content) != expected {
		t.Fatalf("prompt = %q, want %q", content, expected)
	}
	second, err := (CopilotWriter{}).Write(WriteRequest{InstalledPath: installedPath, Content: content, AddManagedHeader: true})
	if err != nil {
		t.Fatal(err)
	}
	if first.SHA256 != second.SHA256 {
		t.Fatalf("rewrite changed SHA-256: %s vs %s", first.SHA256, second.SHA256)
	}
}

func TestCopilotWriterPlanContractsForEmptyAndUnknownArtifacts(t *testing.T) {
	emptyResult := (CopilotWriter{}).Plan("skill", "", "", nil)
	if emptyResult.Skipped || len(emptyResult.Files) != 0 {
		t.Fatalf("empty Plan() = %#v, want empty result", emptyResult)
	}

	unknownResult := (CopilotWriter{}).Plan("unsupported", "example", "", []string{"example.md"})
	if !unknownResult.Skipped || unknownResult.Reason == "" || len(unknownResult.Files) != 0 {
		t.Fatalf("unknown Plan() = %#v, want skipped result with reason", unknownResult)
	}
}
