package writer

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestYAMLFrontmatterValueReadsDescriptionScalars(t *testing.T) {
	testCases := []struct {
		name        string
		frontmatter string
		expected    string
	}{
		{
			name:        "plain",
			frontmatter: "---\ndescription: Review API # displayed to users\n---\n",
			expected:    "Review API",
		},
		{
			name:        "single quoted",
			frontmatter: "---\ndescription: 'Review l''API' # displayed to users\n---\n",
			expected:    "Review l'API",
		},
		{
			name:        "double quoted",
			frontmatter: "---\ndescription: \"Review\\u0020API/CLI\\ncarefully\"\n---\n",
			expected:    "Review API/CLI\ncarefully",
		},
		{
			name:        "folded strip",
			frontmatter: "---\ndescription: >-\n  Review code\n  and tests.\n\n  Report risks.\nname: ignored\n---\n",
			expected:    "Review code and tests.\nReport risks.",
		},
		{
			name:        "literal clip",
			frontmatter: "---\ndescription: |\n  First line.\n  Second line.\n---\n",
			expected:    "First line.\nSecond line.\n",
		},
		{
			name:        "folded explicit indentation",
			frontmatter: "---\ndescription: >2-\n  Review:\n    exact output\n  Then report.\n---\n",
			expected:    "Review:\n  exact output\nThen report.",
		},
		{
			name:        "literal keep",
			frontmatter: "---\ndescription: |+2\n  First line.\n    Indented line.\n\n---\n",
			expected:    "First line.\n  Indented line.\n\n",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			actual := geminiFrontmatterValue(testCase.frontmatter, "description")
			if actual != testCase.expected {
				t.Fatalf("description = %q, want %q", actual, testCase.expected)
			}
		})
	}
}

func TestYAMLFrontmatterValueRejectsNonNativeDescriptionFields(t *testing.T) {
	testCases := []struct {
		name        string
		frontmatter string
	}{
		{name: "missing delimiters", frontmatter: "description: hidden\n"},
		{name: "nested", frontmatter: "---\nmetadata:\n  description: hidden\n---\n"},
		{name: "different key", frontmatter: "---\ndescription-extra: hidden\n---\n"},
		{name: "unterminated quote", frontmatter: "---\ndescription: \"hidden\n---\n"},
		{name: "invalid block header", frontmatter: "---\ndescription: >--\n  hidden\n---\n"},
		{name: "unindented block", frontmatter: "---\ndescription: >-\nother: value\n---\n"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if actual := geminiFrontmatterValue(testCase.frontmatter, "description"); actual != "" {
				t.Fatalf("description = %q, want empty", actual)
			}
		})
	}
}

func TestBlockDescriptionsReachNativeWriterFormats(t *testing.T) {
	t.Run("Codex agent", func(t *testing.T) {
		source := []byte("---\ndescription: >-\n  Review correctness\n  and security.\n---\nInspect the change.\n")
		rendered, err := (CodexWriter{}).Render(WriteRequest{
			InstalledPath: filepath.Join(t.TempDir(), ".codex", "agents", "reviewer.toml"),
			Content:       source,
			ArtifactName:  "reviewer",
		})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(rendered), `description = "Review correctness and security."`) {
			t.Fatalf("Codex agent = %s", rendered)
		}
	})

	t.Run("Codex command", func(t *testing.T) {
		source := []byte("---\ndescription: |-\n  Deploy safely.\n  Confirm completion.\n---\nDeploy now.\n")
		rendered, err := (CodexWriter{}).Render(WriteRequest{
			InstalledPath: filepath.Join(t.TempDir(), ".agents", "skills", "deploy", "SKILL.md"),
			Content:       source,
			ArtifactKind:  "command",
			ArtifactName:  "deploy",
		})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(rendered), `description: "Deploy safely.\nConfirm completion."`) {
			t.Fatalf("Codex command = %s", rendered)
		}
	})

	t.Run("Copilot prompt", func(t *testing.T) {
		source := []byte("---\ndescription: >-\n  Deploy safely\n  through Copilot.\n---\nDeploy now.\n")
		rendered, err := (CopilotWriter{}).Render(WriteRequest{
			InstalledPath: filepath.Join(t.TempDir(), ".github", "prompts", "deploy.prompt.md"),
			Content:       source,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(rendered), `description: "Deploy safely through Copilot."`) {
			t.Fatalf("Copilot prompt = %s", rendered)
		}
	})

	t.Run("Gemini command", func(t *testing.T) {
		source := []byte("---\ndescription: |\n  First line.\n  Second line.\n---\nRun the command.\n")
		rendered, err := RenderGeminiCommand(source)
		if err != nil {
			t.Fatal(err)
		}
		assignments := geminiRenderedAssignments(t, string(rendered))
		if assignments["description"] != "First line.\nSecond line.\n" {
			t.Fatalf("Gemini description = %q", assignments["description"])
		}
	})
}

func TestCodexNativeSkillUsesOnlyTopLevelDescription(t *testing.T) {
	installedPath := filepath.Join(t.TempDir(), ".agents", "skills", "review", "SKILL.md")
	validSource := []byte("---\nname: review\ndescription: >-\n  Review code\n  and tests.\n---\nReview the change.\n")
	if _, err := (CodexWriter{}).Render(WriteRequest{InstalledPath: installedPath, Content: validSource, ArtifactKind: "skill"}); err != nil {
		t.Fatalf("valid native skill rejected: %v", err)
	}

	invalidSource := []byte("---\nname: review\nmetadata:\n  description: hidden\n---\nReview the change.\n")
	_, err := (CodexWriter{}).Render(WriteRequest{InstalledPath: installedPath, Content: invalidSource, ArtifactKind: "skill"})
	if err == nil || !strings.Contains(err.Error(), "frontmatter name et description requis") {
		t.Fatalf("invalid native skill error = %v", err)
	}
}
