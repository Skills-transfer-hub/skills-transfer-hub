package writer

import "testing"

type planner interface {
	Kind() string
	Plan(artifactKind, artifactName, lang string, sources []string) PlanResult
}

// Compile-time guards: every concrete writer implements both interfaces.
var (
	_ TargetWriter  = ClaudeWriter{}
	_ TargetWriter  = CopilotWriter{}
	_ TargetWriter  = CodexWriter{}
	_ TargetWriter  = CursorWriter{}
	_ TargetWriter  = AntigravityWriter{}
	_ TargetPlanner = ClaudeWriter{}
	_ TargetPlanner = CopilotWriter{}
	_ TargetPlanner = CodexWriter{}
	_ TargetPlanner = CursorWriter{}
	_ TargetPlanner = AntigravityWriter{}
)

func TestPlan_MappingMatrix(t *testing.T) {
	cases := []struct {
		writer       planner
		kind         string
		want         string
		wantSkipped  bool
	}{
		// Claude — historical conventions.
		{ClaudeWriter{}, "agent", ".claude/agents/code-reviewer.md", false},
		{ClaudeWriter{}, "command", ".claude/commands/code-reviewer.md", false},
		{ClaudeWriter{}, "instruction", ".claude/skills/instructions/code-reviewer.md", false},

		// Copilot — agents renamed from chatmode, commands skipped.
		{CopilotWriter{}, "skill", ".github/instructions/code-reviewer.instructions.md", false},
		{CopilotWriter{}, "agent", ".github/agents/code-reviewer.agent.md", false},
		{CopilotWriter{}, "command", "", true},
		{CopilotWriter{}, "instruction", ".github/instructions/code-reviewer.instructions.md", false},

		// Codex — single tree under .codex/.
		{CodexWriter{}, "skill", ".codex/prompts/code-reviewer.md", false},
		{CodexWriter{}, "agent", ".codex/agents/code-reviewer.md", false},
		{CodexWriter{}, "command", ".codex/commands/code-reviewer.md", false},
		{CodexWriter{}, "instruction", ".codex/prompts/code-reviewer.md", false},

		// Cursor — .mdc for rules, .md for commands.
		{CursorWriter{}, "skill", ".cursor/rules/code-reviewer.mdc", false},
		{CursorWriter{}, "agent", ".cursor/rules/agents/code-reviewer.mdc", false},
		{CursorWriter{}, "command", ".cursor/commands/code-reviewer.md", false},
		{CursorWriter{}, "instruction", ".cursor/rules/code-reviewer.mdc", false},
	}

	for _, tc := range cases {
		got := tc.writer.Plan(tc.kind, "code-reviewer", "", []string{"src/foo.md"})
		if got.Skipped != tc.wantSkipped {
			t.Errorf("%s/%s: Skipped = %v, want %v (reason: %q)", tc.writer.Kind(), tc.kind, got.Skipped, tc.wantSkipped, got.Reason)
			continue
		}
		if tc.wantSkipped {
			if got.Reason == "" {
				t.Errorf("%s/%s: skipped result must carry a Reason", tc.writer.Kind(), tc.kind)
			}
			continue
		}
		if len(got.Files) != 1 {
			t.Errorf("%s/%s: got %d files, want 1", tc.writer.Kind(), tc.kind, len(got.Files))
			continue
		}
		if got.Files[0].RelativePath != tc.want {
			t.Errorf("%s/%s: RelativePath = %q, want %q", tc.writer.Kind(), tc.kind, got.Files[0].RelativePath, tc.want)
		}
	}
}

func TestPlan_UnknownKindSkipsWithReason(t *testing.T) {
	for _, w := range []planner{ClaudeWriter{}, CopilotWriter{}, CodexWriter{}, CursorWriter{}} {
		got := w.Plan("nonsense", "x", "", []string{"src.md"})
		if !got.Skipped || got.Reason == "" {
			t.Errorf("%s: unknown kind should be skipped with reason, got %+v", w.Kind(), got)
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
		"react/form-wizard/SKILL.md":                          ".claude/skills/form-wizard/SKILL.md",
		"react/form-wizard/references/component-patterns.md":  ".claude/skills/form-wizard/references/component-patterns.md",
		"react/form-wizard/references/mcp-tools.md":           ".claude/skills/form-wizard/references/mcp-tools.md",
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
