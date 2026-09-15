package artifact

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeFormFixtureMatchesScannerRegistry(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "specs", "012-multi-artifact", "fixtures", "native-forms.json")
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	fixture := struct {
		Forms []struct {
			Path    string  `json:"path"`
			Kind    Kind    `json:"kind"`
			Dialect Dialect `json:"dialect"`
		} `json:"forms"`
	}{}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, form := range fixture.Forms {
		candidate, matched := classifyPath(form.Path)
		if !matched || candidate.kind != form.Kind || candidate.dialect != form.Dialect {
			t.Errorf("form %s = matched:%v %s/%s, want %s/%s", form.Path, matched, candidate.kind, candidate.dialect, form.Kind, form.Dialect)
		}
	}
}

func TestScanProjectRecognizesV1NativeForms(t *testing.T) {
	projectRoot := t.TempDir()
	fixtures := map[string]string{
		".claude/skills/review/SKILL.md":          "---\nname: review\ndescription: Review code\n---\nClaude skill\n",
		".claude/agents/team/reviewer.md":         "---\nname: reviewer\ndescription: Review code\n---\nClaude agent\n",
		"CLAUDE.md":                               "Claude instruction\n",
		".claude/rules/team/go.md":                "Claude rule\n",
		".claude/commands/team/deploy.md":         "Claude command\n",
		".claude/workflows/release.js":            "export const meta = { name: 'release' }\n",
		".codex/agents/reviewer.toml":             "name = \"reviewer\"\ndescription = \"Review code\"\ndeveloper_instructions = \"Codex agent\"\nunknown = \"preserved\"\n",
		".codex/rules/safe.rules":                 "prefix_rule(pattern = [\"git\", \"status\"], decision = \"allow\")\n",
		"GEMINI.md":                               "Gemini instruction\n",
		".gemini/agents/reviewer.md":              "---\nname: reviewer\ndescription: Review code\n---\nGemini agent\n",
		".gemini/commands/team/deploy.toml":       "description = \"Deploy\"\nprompt = \"Deploy {{args}}\"\n",
		".agents/agents/reviewer/agent.md":        "---\nname: reviewer\ndescription: Review code\n---\nAntigravity agent\n",
		".agents/rules/team/go.md":                "Antigravity rule\n",
		".agents/workflows/team/release.md":       "Antigravity workflow\n",
		".github/copilot-instructions.md":         "Copilot instruction\n",
		".github/instructions/go.instructions.md": "---\napplyTo: '**/*.go'\n---\nCopilot rule\n",
		".github/agents/reviewer.agent.md":        "---\nname: reviewer\ndescription: Review code\n---\nCopilot agent\n",
		".github/prompts/deploy.prompt.md":        "---\ndescription: Deploy\n---\nCopilot command\n",
		"AGENTS.md":                               "Shared instruction\n",
		".cursor/rules/go.mdc":                    "---\ndescription: Go rule\nglobs: '**/*.go'\n---\nCursor rule\n",
		".cursor/agents/reviewer.md":              "---\nname: reviewer\ndescription: Review code\n---\nCursor agent\n",
	}
	for relativePath, content := range fixtures {
		writeArtifactFixture(t, projectRoot, relativePath, []byte(content))
	}
	writeArtifactFixture(t, projectRoot, ".claude/skills/review/references/check.md", []byte("Reference bytes\r\n"))

	definitions, err := ScanProject(projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(definitions) != 21 {
		t.Fatalf("definitions = %d, want 21: %+v", len(definitions), definitions)
	}
	assertScannedDialect(t, definitions, ".codex/rules/safe.rules", KindRule, DialectCodexExecutionRule)
	assertScannedDialect(t, definitions, ".github/instructions/go.instructions.md", KindRule, DialectCopilotRule)
	assertScannedDialect(t, definitions, ".claude/workflows/release.js", KindWorkflow, DialectClaudeWorkflow)
	assertScannedDialect(t, definitions, ".agents/workflows/team/release.md", KindWorkflow, DialectAntigravityWorkflow)
	codexAgent := findDefinitionByPath(t, definitions, ".codex/agents/reviewer.toml")
	if len(codexAgent.Native.Unmapped) != 1 || codexAgent.Native.Unmapped[0] != "unknown" {
		t.Fatalf("unmapped fields = %v", codexAgent.Native.Unmapped)
	}
	skill := findDefinitionByPath(t, definitions, ".claude/skills/review/SKILL.md")
	if len(skill.Files) != 2 || !bytes.Equal(skill.Files[1].Content, []byte("Reference bytes\r\n")) {
		t.Fatalf("skill raw files were not preserved: %+v", skill.Files)
	}
}

func TestScanProjectPreservesNestedScopeAndSkipsSymlinks(t *testing.T) {
	projectRoot := t.TempDir()
	writeArtifactFixture(t, projectRoot, "services/api/AGENTS.md", []byte("Nested instructions\n"))
	outsideRoot := t.TempDir()
	writeArtifactFixture(t, outsideRoot, ".cursor/agents/outside.md", []byte("---\ndescription: Outside\n---\nOutside\n"))
	if err := os.Symlink(outsideRoot, filepath.Join(projectRoot, "linked")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	definitions, err := ScanProject(projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(definitions) != 1 {
		t.Fatalf("definitions = %+v", definitions)
	}
	if definitions[0].Scope.Path != "services/api" || definitions[0].Scope.Level != "nested" {
		t.Fatalf("scope = %+v", definitions[0].Scope)
	}
}

func TestValidateReportsStructuredParseErrors(t *testing.T) {
	projectRoot := t.TempDir()
	writeArtifactFixture(t, projectRoot, ".gemini/commands/broken.toml", []byte("prompt = \"\"\"missing\n"))
	definitions, err := ScanProject(projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	diagnostics := Validate(definitions[0])
	if len(diagnostics) == 0 || diagnostics[0].Code != "parse_error" || diagnostics[0].Severity != SeverityError || diagnostics[0].Path != ".gemini/commands/broken.toml" {
		t.Fatalf("diagnostics = %+v", diagnostics)
	}
}

func writeArtifactFixture(t *testing.T, projectRoot, relativePath string, content []byte) {
	t.Helper()
	absolutePath := filepath.Join(projectRoot, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(absolutePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absolutePath, content, 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertScannedDialect(t *testing.T, definitions []Definition, sourcePath string, kind Kind, dialect Dialect) {
	t.Helper()
	definition := findDefinitionByPath(t, definitions, sourcePath)
	if definition.Kind != kind || definition.SourceDialect != dialect {
		t.Fatalf("%s = %s/%s, want %s/%s", sourcePath, definition.Kind, definition.SourceDialect, kind, dialect)
	}
}

func findDefinitionByPath(t *testing.T, definitions []Definition, sourcePath string) Definition {
	t.Helper()
	for _, definition := range definitions {
		if definition.SourcePath == sourcePath {
			return definition
		}
	}
	t.Fatalf("definition not found: %s", sourcePath)
	return Definition{}
}
