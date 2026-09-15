package writer

import (
	"strings"
	"testing"
)

func TestValidateSkillConsumerMetadataContracts(t *testing.T) {
	codexMaximumDescription := strings.Repeat("c", 1024)
	copilotMaximumName := strings.Repeat("a", 64)
	copilotMaximumDescription := strings.Repeat("p", 1024)
	geminiMaximumDescription := strings.Repeat("g", 1024)
	antigravityMaximumDescription := strings.Repeat("a", 1024)
	testCases := []struct {
		name          string
		target        Target
		artifactName  string
		content       string
		expectedError string
	}{
		{name: "codex valid", target: CodexWriter{}, content: "---\nname: runtime-skill\ndescription: ready\n---\nbody\n"},
		{name: "codex maximum description", target: CodexWriter{}, content: "---\nname: runtime-skill\ndescription: " + codexMaximumDescription + "\n---\nbody\n"},
		{name: "codex missing description", target: CodexWriter{}, content: "---\nname: runtime-skill\n---\nbody\n", expectedError: "name and description are required"},
		{name: "codex mismatched name", target: CodexWriter{}, content: "---\nname: another-skill\ndescription: ready\n---\nbody\n", expectedError: "differs from directory"},
		{name: "codex oversized description", target: CodexWriter{}, content: "---\nname: runtime-skill\ndescription: " + codexMaximumDescription + "x\n---\nbody\n", expectedError: "between 1 and 1024"},
		{name: "copilot valid", target: CopilotWriter{}, artifactName: copilotMaximumName, content: "---\nname: " + copilotMaximumName + "\ndescription: " + copilotMaximumDescription + "\n---\nbody\n"},
		{name: "copilot missing name", target: CopilotWriter{}, content: "---\ndescription: ready\n---\nbody\n", expectedError: "name must be a non-empty string"},
		{name: "copilot non string description", target: CopilotWriter{}, content: "---\nname: runtime-skill\ndescription: 42\n---\nbody\n", expectedError: "description must be a string"},
		{name: "copilot invalid name format", target: CopilotWriter{}, content: "---\nname: Runtime_Skill\ndescription: ready\n---\nbody\n", expectedError: "from a-z, 0-9"},
		{name: "copilot consecutive hyphens", target: CopilotWriter{}, artifactName: "runtime--skill", content: "---\nname: runtime--skill\ndescription: ready\n---\nbody\n", expectedError: "leading, trailing, or consecutive hyphen"},
		{name: "copilot mismatched name", target: CopilotWriter{}, content: "---\nname: another-skill\ndescription: ready\n---\nbody\n", expectedError: "differs from directory"},
		{name: "copilot oversized name", target: CopilotWriter{}, content: "---\nname: " + copilotMaximumName + "a\ndescription: ready\n---\nbody\n", expectedError: "1 to 64"},
		{name: "copilot oversized description", target: CopilotWriter{}, content: "---\nname: runtime-skill\ndescription: " + copilotMaximumDescription + "x\n---\nbody\n", expectedError: "maximum 1024"},
		{name: "gemini valid strings", target: GeminiWriter{}, content: "---\nname: runtime-skill\ndescription: ready\n---\nbody\n"},
		{name: "gemini maximum description", target: GeminiWriter{}, content: "---\nname: runtime-skill\ndescription: " + geminiMaximumDescription + "\n---\nbody\n"},
		{name: "gemini non string name", target: GeminiWriter{}, content: "---\nname: 42\ndescription: ready\n---\nbody\n", expectedError: "name must be a string"},
		{name: "gemini invalid name format", target: GeminiWriter{}, artifactName: "Runtime_Skill", content: "---\nname: Runtime_Skill\ndescription: ready\n---\nbody\n", expectedError: "from a-z, 0-9"},
		{name: "gemini mismatched name", target: GeminiWriter{}, content: "---\nname: another-skill\ndescription: ready\n---\nbody\n", expectedError: "differs from directory"},
		{name: "gemini missing description", target: GeminiWriter{}, content: "---\nname: runtime-skill\n---\nbody\n", expectedError: "description must be a non-empty string"},
		{name: "gemini oversized description", target: GeminiWriter{}, content: "---\nname: runtime-skill\ndescription: " + geminiMaximumDescription + "x\n---\nbody\n", expectedError: "maximum 1024"},
		{name: "antigravity without name", target: AntigravityWriter{}, content: "---\ndescription: ready\n---\nbody\n"},
		{name: "antigravity with name", target: AntigravityWriter{}, content: "---\nname: runtime-skill\ndescription: ready\n---\nbody\n"},
		{name: "antigravity maximum description", target: AntigravityWriter{}, content: "---\nname: runtime-skill\ndescription: " + antigravityMaximumDescription + "\n---\nbody\n"},
		{name: "antigravity missing description", target: AntigravityWriter{}, content: "---\nname: runtime-skill\n---\nbody\n", expectedError: "description must be a non-empty string"},
		{name: "antigravity non string name", target: AntigravityWriter{}, content: "---\nname: 42\ndescription: ready\n---\nbody\n", expectedError: "name must be a string"},
		{name: "antigravity invalid default name", target: AntigravityWriter{}, artifactName: "Runtime_Skill", content: "---\ndescription: ready\n---\nbody\n", expectedError: "from a-z, 0-9"},
		{name: "antigravity mismatched name", target: AntigravityWriter{}, content: "---\nname: another-skill\ndescription: ready\n---\nbody\n", expectedError: "differs from directory"},
		{name: "antigravity oversized description", target: AntigravityWriter{}, content: "---\nname: runtime-skill\ndescription: " + antigravityMaximumDescription + "x\n---\nbody\n", expectedError: "maximum 1024"},
		{name: "cursor valid", target: CursorWriter{}, content: "---\nname: runtime-skill\ndescription: ready\n---\nbody\n"},
		{name: "cursor invalid name", target: CursorWriter{}, content: "---\nname: runtime_skill\ndescription: ready\n---\nbody\n", expectedError: "from a-z, 0-9"},
		{name: "claude unchanged", target: ClaudeWriter{}, content: "body without frontmatter\n"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			artifactName := testCase.artifactName
			if artifactName == "" {
				artifactName = "runtime-skill"
			}
			fetch := func(sourcePath string) ([]byte, error) {
				if sourcePath != "skills/runtime-skill/SKILL.md" {
					t.Fatalf("source path = %s", sourcePath)
				}
				return []byte(testCase.content), nil
			}
			err := ValidateSkillConsumer(t.TempDir(), testCase.target, artifactName, []string{"skills/runtime-skill/SKILL.md"}, fetch)
			if testCase.expectedError == "" {
				if err != nil {
					t.Fatalf("validation error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), testCase.expectedError) {
				t.Fatalf("validation error = %v, expected %q", err, testCase.expectedError)
			}
		})
	}
}

func TestValidateSkillConsumerAppliesMetadataOnlyToEntrypoint(t *testing.T) {
	contents := map[string][]byte{
		"skills/runtime-skill/SKILL.md":          []byte("---\nname: runtime-skill\ndescription: ready\n---\nbody\n"),
		"skills/runtime-skill/references/api.md": []byte("reference without frontmatter\n"),
	}
	fetch := func(sourcePath string) ([]byte, error) {
		return contents[sourcePath], nil
	}
	err := ValidateSkillConsumer(t.TempDir(), CopilotWriter{}, "runtime-skill", []string{
		"skills/runtime-skill/SKILL.md",
		"skills/runtime-skill/references/api.md",
	}, fetch)
	if err != nil {
		t.Fatalf("validation error = %v", err)
	}
}
