package writer

import "sth/internal/artifact"

type Target interface {
	TargetWriter
	TargetPlanner
}

const (
	KindClaude      = "claude"
	KindCopilot     = "copilot"
	KindCodex       = "codex"
	KindCursor      = "cursor"
	KindAntigravity = "antigravity"
	KindGemini      = "gemini"
)

type DetectMarker struct {
	Path     string `json:"path"`
	Evidence string `json:"evidence"`
}

type TargetSpec struct {
	Kind           string                                          `json:"kind"`
	DisplayName    string                                          `json:"display_name"`
	RootDir        string                                          `json:"root_dir"`
	DetectMarkers  []DetectMarker                                  `json:"detect_markers"`
	Dialects       map[artifact.Kind]artifact.Dialect              `json:"dialects"`
	NativePaths    map[artifact.Kind][]string                      `json:"native_paths"`
	ValidatorID    string                                          `json:"validator"`
	Validate       func(artifact.Definition) []artifact.Diagnostic `json:"-"`
	Maturity       string                                          `json:"maturity"`
	MinimumVersion string                                          `json:"minimum_version,omitempty"`
	Writer         Target                                          `json:"-"`
}

func AllTargetSpecs() []TargetSpec {
	return []TargetSpec{
		{Kind: KindClaude, DisplayName: "Claude Code", RootDir: ".claude", DetectMarkers: []DetectMarker{{Path: ".claude", Evidence: ".claude"}, {Path: "CLAUDE.md", Evidence: "CLAUDE.md"}}, Dialects: map[artifact.Kind]artifact.Dialect{artifact.KindSkill: artifact.DialectAgentSkills, artifact.KindAgent: artifact.DialectClaudeAgent, artifact.KindInstruction: artifact.DialectClaudeInstruction, artifact.KindRule: artifact.DialectClaudeRule, artifact.KindCommand: artifact.DialectClaudeCommand, artifact.KindWorkflow: artifact.DialectClaudeWorkflow}, NativePaths: map[artifact.Kind][]string{artifact.KindSkill: {".claude/skills/*/SKILL.md"}, artifact.KindAgent: {".claude/agents/**/*.md"}, artifact.KindInstruction: {"CLAUDE.md", "**/CLAUDE.md"}, artifact.KindRule: {".claude/rules/**/*.md"}, artifact.KindCommand: {".claude/commands/**/*.md"}, artifact.KindWorkflow: {".claude/workflows/**/*.js"}}, ValidatorID: "claude-v1", Validate: artifact.Validate, Maturity: "stable", MinimumVersion: "2.1.154", Writer: ClaudeWriter{}},
		{Kind: KindCodex, DisplayName: "OpenAI Codex", RootDir: ".codex", DetectMarkers: []DetectMarker{{Path: ".codex", Evidence: ".codex"}, {Path: "AGENTS.md", Evidence: "AGENTS.md"}}, Dialects: map[artifact.Kind]artifact.Dialect{artifact.KindSkill: artifact.DialectAgentSkills, artifact.KindAgent: artifact.DialectCodexAgent, artifact.KindInstruction: artifact.DialectSharedAgentsMD, artifact.KindRule: artifact.DialectCodexExecutionRule, artifact.KindCommand: artifact.DialectAgentSkills}, NativePaths: map[artifact.Kind][]string{artifact.KindSkill: {".agents/skills/*/SKILL.md"}, artifact.KindAgent: {".codex/agents/*.toml"}, artifact.KindInstruction: {"AGENTS.md", "**/AGENTS.md"}, artifact.KindRule: {".codex/rules/*.rules"}}, ValidatorID: "codex-v1", Validate: artifact.Validate, Maturity: "stable", Writer: CodexWriter{}},
		{Kind: KindGemini, DisplayName: "Gemini CLI", RootDir: ".gemini", DetectMarkers: []DetectMarker{{Path: ".gemini/agents", Evidence: ".gemini/agents/"}, {Path: ".gemini/commands", Evidence: ".gemini/commands/"}, {Path: "GEMINI.md", Evidence: "GEMINI.md"}}, Dialects: map[artifact.Kind]artifact.Dialect{artifact.KindSkill: artifact.DialectAgentSkills, artifact.KindAgent: artifact.DialectGeminiAgent, artifact.KindInstruction: artifact.DialectGeminiInstruction, artifact.KindCommand: artifact.DialectGeminiCommand}, NativePaths: map[artifact.Kind][]string{artifact.KindSkill: {".agents/skills/*/SKILL.md", ".gemini/skills/*/SKILL.md"}, artifact.KindAgent: {".gemini/agents/*.md"}, artifact.KindInstruction: {"GEMINI.md", "**/GEMINI.md"}, artifact.KindCommand: {".gemini/commands/**/*.toml"}}, ValidatorID: "gemini-v1", Validate: artifact.Validate, Maturity: "stable", Writer: GeminiWriter{}},
		{Kind: KindAntigravity, DisplayName: "Google Antigravity", RootDir: ".agents", DetectMarkers: []DetectMarker{{Path: ".agents/agents", Evidence: ".agents/agents/"}, {Path: ".agents/workflows", Evidence: ".agents/workflows/"}, {Path: ".agents/rules", Evidence: ".agents/rules/"}}, Dialects: map[artifact.Kind]artifact.Dialect{artifact.KindSkill: artifact.DialectAgentSkills, artifact.KindAgent: artifact.DialectAntigravityAgent, artifact.KindInstruction: artifact.DialectAntigravityRule, artifact.KindRule: artifact.DialectAntigravityRule, artifact.KindCommand: artifact.DialectAntigravityWorkflow, artifact.KindWorkflow: artifact.DialectAntigravityWorkflow}, NativePaths: map[artifact.Kind][]string{artifact.KindSkill: {".agents/skills/*/SKILL.md"}, artifact.KindAgent: {".agents/agents/*/agent.md", ".agents/agents.md"}, artifact.KindInstruction: {".agents/agents.md"}, artifact.KindRule: {".agents/rules/**/*.md"}, artifact.KindWorkflow: {".agents/workflows/**/*.md"}}, ValidatorID: "antigravity-v1", Validate: artifact.Validate, Maturity: "stable", Writer: AntigravityWriter{}},
		{Kind: KindCopilot, DisplayName: "GitHub Copilot", RootDir: ".github", DetectMarkers: []DetectMarker{{Path: ".github/copilot-instructions.md", Evidence: ".github/copilot-instructions.md"}, {Path: ".github/instructions", Evidence: ".github/instructions/"}}, Dialects: map[artifact.Kind]artifact.Dialect{artifact.KindSkill: artifact.DialectAgentSkills, artifact.KindAgent: artifact.DialectCopilotAgent, artifact.KindInstruction: artifact.DialectSharedAgentsMD, artifact.KindRule: artifact.DialectCopilotRule, artifact.KindCommand: artifact.DialectCopilotPrompt}, NativePaths: map[artifact.Kind][]string{artifact.KindSkill: {".agents/skills/*/SKILL.md", ".github/skills/*/SKILL.md"}, artifact.KindAgent: {".github/agents/*.agent.md"}, artifact.KindInstruction: {".github/copilot-instructions.md", "AGENTS.md"}, artifact.KindRule: {".github/instructions/*.instructions.md"}, artifact.KindCommand: {".github/prompts/*.prompt.md"}}, ValidatorID: "copilot-v1", Validate: artifact.Validate, Maturity: "stable", Writer: CopilotWriter{}},
		{Kind: KindCursor, DisplayName: "Cursor", RootDir: ".cursor", DetectMarkers: []DetectMarker{{Path: ".cursor", Evidence: ".cursor"}, {Path: "AGENTS.md", Evidence: "AGENTS.md"}}, Dialects: map[artifact.Kind]artifact.Dialect{artifact.KindSkill: artifact.DialectAgentSkills, artifact.KindAgent: artifact.DialectCursorAgent, artifact.KindInstruction: artifact.DialectSharedAgentsMD, artifact.KindRule: artifact.DialectCursorRule}, NativePaths: map[artifact.Kind][]string{artifact.KindSkill: {".agents/skills/*/SKILL.md", ".cursor/skills/*/SKILL.md"}, artifact.KindAgent: {".cursor/agents/*.md"}, artifact.KindInstruction: {"AGENTS.md", "**/AGENTS.md"}, artifact.KindRule: {".cursor/rules/*.mdc"}}, ValidatorID: "cursor-v1", Validate: artifact.Validate, Maturity: "stable", Writer: CursorWriter{}},
	}
}

func SpecForKind(kind string) (TargetSpec, bool) {
	for _, specification := range AllTargetSpecs() {
		if specification.Kind == kind {
			return specification, true
		}
	}
	return TargetSpec{}, false
}

func WriterForKind(kind string) (Target, bool) {
	specification, exists := SpecForKind(kind)
	if !exists {
		return nil, false
	}
	return specification.Writer, true
}

func ValidateForKind(kind string, definition artifact.Definition) []artifact.Diagnostic {
	specification, exists := SpecForKind(kind)
	if !exists || specification.Validate == nil {
		return artifact.Validate(definition)
	}
	return specification.Validate(definition)
}

func SupportedTargetKinds() []string {
	specifications := AllTargetSpecs()
	kinds := make([]string, 0, len(specifications))
	for _, specification := range specifications {
		kinds = append(kinds, specification.Kind)
	}
	return kinds
}
