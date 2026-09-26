package writer

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeWritersPreserveBytesWithoutManagedHeader(t *testing.T) {
	testCases := []struct {
		name         string
		target       Target
		artifactKind string
		sourcePath   string
		installed    string
		content      []byte
	}{
		{name: "claude workflow", target: ClaudeWriter{}, artifactKind: "workflow", sourcePath: ".claude/workflows/release.js", installed: ".claude/workflows/release.js", content: []byte("export const meta = { name: 'release' };\r\n")},
		{name: "codex agent", target: CodexWriter{}, artifactKind: "agent", sourcePath: ".codex/agents/reviewer.toml", installed: ".codex/agents/reviewer.toml", content: []byte("name = \"reviewer\"\r\ndescription = \"Review\"\r\ndeveloper_instructions = \"Keep bytes\"\r\nunknown = \"yes\"\r\n")},
		{name: "codex rule", target: CodexWriter{}, artifactKind: "rule", sourcePath: ".codex/rules/safe.rules", installed: ".codex/rules/safe.rules", content: []byte("prefix_rule(pattern = [\"git\", \"status\"], decision = \"allow\")\n")},
		{name: "gemini command", target: GeminiWriter{}, artifactKind: "command", sourcePath: ".gemini/commands/deploy.toml", installed: ".gemini/commands/deploy.toml", content: []byte("description = \"Deploy\"\r\nprompt = \"!{git status}\"\r\nunknown = \"yes\"\r\n")},
		{name: "antigravity workflow", target: AntigravityWriter{}, artifactKind: "workflow", sourcePath: ".agents/workflows/release.md", installed: ".agents/workflows/release.md", content: []byte("---\ndescription: Release\nunknown: retained\n---\nRelease.\n")},
		{name: "copilot prompt", target: CopilotWriter{}, artifactKind: "command", sourcePath: ".github/prompts/deploy.prompt.md", installed: ".github/prompts/deploy.prompt.md", content: []byte("---\ndescription: Deploy\nunknown: retained\n---\nDeploy.\r\n")},
		{name: "cursor rule", target: CursorWriter{}, artifactKind: "rule", sourcePath: ".cursor/rules/go.mdc", installed: ".cursor/rules/go.mdc", content: []byte("---\ndescription: Go\nglobs: '**/*.go'\nunknown: retained\n---\nGo.\r\n")},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			installedPath := filepath.Join(t.TempDir(), filepath.FromSlash(testCase.installed))
			result, err := testCase.target.Write(WriteRequest{InstalledPath: installedPath, SourcePath: testCase.sourcePath, Content: testCase.content, ArtifactKind: testCase.artifactKind, AddManagedHeader: false})
			if err != nil {
				t.Fatal(err)
			}
			written, err := os.ReadFile(installedPath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(written, testCase.content) {
				t.Fatalf("native bytes changed:\nwritten=%q\nsource=%q", written, testCase.content)
			}
			if result.SHA256 != computeSHA256(testCase.content) {
				t.Fatalf("sha256 = %s", result.SHA256)
			}
		})
	}
}
