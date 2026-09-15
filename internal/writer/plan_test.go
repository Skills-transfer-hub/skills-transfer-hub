package writer

import "testing"

type planner interface {
	Kind() string
	Plan(artifactKind, artifactName, lang string, sources []string) PlanResult
}

// Compile-time guards: every concrete writer implements both interfaces.
var (
	_ TargetWriter   = ClaudeWriter{}
	_ TargetWriter   = CopilotWriter{}
	_ TargetWriter   = CodexWriter{}
	_ TargetWriter   = CursorWriter{}
	_ TargetWriter   = AntigravityWriter{}
	_ TargetWriter   = GeminiWriter{}
	_ TargetPlanner  = ClaudeWriter{}
	_ TargetPlanner  = CopilotWriter{}
	_ TargetPlanner  = CodexWriter{}
	_ TargetPlanner  = CursorWriter{}
	_ TargetPlanner  = AntigravityWriter{}
	_ TargetPlanner  = GeminiWriter{}
	_ TargetRenderer = CodexWriter{}
	_ TargetRenderer = GeminiWriter{}
)

func TestPlan_MappingMatrix(t *testing.T) {
	cases := []struct {
		writer      planner
		kind        string
		source      string
		want        string
		wantSkipped bool
	}{
		{ClaudeWriter{}, "skill", "src/SKILL.md", ".claude/skills/code-reviewer/SKILL.md", false},
		{ClaudeWriter{}, "agent", "src/foo.md", ".claude/agents/code-reviewer.md", false},
		{ClaudeWriter{}, "command", "src/foo.md", ".claude/commands/code-reviewer.md", false},
		{ClaudeWriter{}, "instruction", "src/foo.md", ".claude/rules/code-reviewer.md", false},
		{CopilotWriter{}, "skill", "src/SKILL.md", ".agents/skills/code-reviewer/SKILL.md", false},
		{CopilotWriter{}, "agent", "src/foo.md", ".github/agents/code-reviewer.agent.md", false},
		{CopilotWriter{}, "instruction", "src/foo.md", ".github/sth/instructions/code-reviewer.md", false},
		{CodexWriter{}, "skill", "src/SKILL.md", ".agents/skills/code-reviewer/SKILL.md", false},
		{CodexWriter{}, "agent", "src/foo.md", ".codex/agents/code-reviewer.toml", false},
		{CodexWriter{}, "command", "src/foo.md", ".agents/skills/code-reviewer/SKILL.md", false},
		{CodexWriter{}, "instruction", "src/foo.md", ".codex/sth/instructions/code-reviewer.md", false},
		{CursorWriter{}, "skill", "src/foo.md", ".agents/skills/code-reviewer/SKILL.md", false},
		{CursorWriter{}, "agent", "src/foo.md", ".cursor/agents/code-reviewer.md", false},
		{CursorWriter{}, "command", "src/foo.md", "", true},
		{CursorWriter{}, "instruction", "src/foo.md", ".cursor/sth/instructions/code-reviewer.md", false},
		{AntigravityWriter{}, "skill", "src/SKILL.md", ".agents/skills/code-reviewer/SKILL.md", false},
		{AntigravityWriter{}, "agent", "src/foo.md", ".agents/agents/code-reviewer/agent.md", false},
		{AntigravityWriter{}, "command", "src/foo.md", ".agents/workflows/code-reviewer.md", false},
		{AntigravityWriter{}, "instruction", "src/foo.md", ".agents/rules/code-reviewer.md", false},
		{GeminiWriter{}, "skill", "src/SKILL.md", ".agents/skills/code-reviewer/SKILL.md", false},
		{GeminiWriter{}, "agent", "src/foo.md", ".gemini/agents/code-reviewer.md", false},
		{GeminiWriter{}, "command", "src/foo.md", ".gemini/commands/code-reviewer.toml", false},
		{GeminiWriter{}, "instruction", "src/foo.md", ".gemini/sth/instructions/code-reviewer.md", false},
	}

	for _, testCase := range cases {
		got := testCase.writer.Plan(testCase.kind, "code-reviewer", "", []string{testCase.source})
		if got.Skipped != testCase.wantSkipped {
			t.Errorf("%s/%s: Skipped = %v, want %v (reason: %q)", testCase.writer.Kind(), testCase.kind, got.Skipped, testCase.wantSkipped, got.Reason)
			continue
		}
		if testCase.wantSkipped {
			if got.Reason == "" {
				t.Errorf("%s/%s: skipped result must carry a Reason", testCase.writer.Kind(), testCase.kind)
			}
			continue
		}
		if len(got.Files) != 1 {
			t.Errorf("%s/%s: got %d files, want 1", testCase.writer.Kind(), testCase.kind, len(got.Files))
			continue
		}
		if got.Files[0].RelativePath != testCase.want {
			t.Errorf("%s/%s: RelativePath = %q, want %q", testCase.writer.Kind(), testCase.kind, got.Files[0].RelativePath, testCase.want)
		}
	}
}

func TestPlan_UnknownKindSkipsWithReason(t *testing.T) {
	for _, targetPlanner := range []planner{ClaudeWriter{}, CopilotWriter{}, CodexWriter{}, CursorWriter{}, AntigravityWriter{}, GeminiWriter{}} {
		got := targetPlanner.Plan("nonsense", "x", "", []string{"src.md"})
		if !got.Skipped || got.Reason == "" {
			t.Errorf("%s: unknown kind should be skipped with reason, got %+v", targetPlanner.Kind(), got)
		}
	}
}

func TestPlan_EmptyInputReturnsEmptyResult(t *testing.T) {
	got := ClaudeWriter{}.Plan("skill", "", "", nil)
	if got.Skipped || len(got.Files) != 0 {
		t.Errorf("empty name/sources should yield empty (non-skip) result, got %+v", got)
	}
}

func TestClaude_SkillKeepsAssetsUnderSkillDir(t *testing.T) {
	got := ClaudeWriter{}.Plan("skill", "form-wizard", "", []string{
		"react/form-wizard/SKILL.md",
		"react/form-wizard/references/component-patterns.md",
		"react/form-wizard/references/mcp-tools.md",
	})
	if len(got.Files) != 3 {
		t.Fatalf("want 3 planned files, got %d", len(got.Files))
	}
	// The subtree under the skill folder (here: references/) MUST be
	// preserved verbatim — flattening would collide assets sharing a
	// basename and break relative links inside SKILL.md.
	wantByRel := map[string]string{
		"react/form-wizard/SKILL.md":                         ".claude/skills/form-wizard/SKILL.md",
		"react/form-wizard/references/component-patterns.md": ".claude/skills/form-wizard/references/component-patterns.md",
		"react/form-wizard/references/mcp-tools.md":          ".claude/skills/form-wizard/references/mcp-tools.md",
	}
	for _, f := range got.Files {
		want, ok := wantByRel[f.SourcePath]
		if !ok {
			t.Errorf("unexpected source path %q", f.SourcePath)
			continue
		}
		if f.RelativePath != want {
			t.Errorf("source %q: got %q, want %q", f.SourcePath, f.RelativePath, want)
		}
	}
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
