package writer

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
)

type DetectMarker struct {
	Path     string
	Evidence string
}

type TargetSpec struct {
	Kind          string
	DisplayName   string
	RootDir       string
	DetectMarkers []DetectMarker
	Writer        Target
}

func AllTargetSpecs() []TargetSpec {
	return []TargetSpec{
		{
			Kind:        KindClaude,
			DisplayName: "Claude Code",
			RootDir:     ".claude",
			DetectMarkers: []DetectMarker{
				{Path: ".claude", Evidence: ".claude"},
				{Path: "CLAUDE.md", Evidence: "CLAUDE.md"},
			},
			Writer: ClaudeWriter{},
		},
		{
			Kind:        KindCopilot,
			DisplayName: "GitHub Copilot",
			RootDir:     ".github",
			DetectMarkers: []DetectMarker{
				{Path: ".github/copilot-instructions.md", Evidence: ".github/copilot-instructions.md"},
				{Path: ".github/instructions", Evidence: ".github/instructions/"},
			},
			Writer: CopilotWriter{},
		},
		{
			Kind:        KindCodex,
			DisplayName: "OpenAI Codex",
			RootDir:     ".codex",
			DetectMarkers: []DetectMarker{
				{Path: ".codex", Evidence: ".codex"},
				{Path: "AGENTS.md", Evidence: "AGENTS.md"},
			},
			Writer: CodexWriter{},
		},
		{
			Kind:        KindCursor,
			DisplayName: "Cursor",
			RootDir:     ".cursor",
			DetectMarkers: []DetectMarker{
				{Path: ".cursor", Evidence: ".cursor"},
				{Path: ".cursorrules", Evidence: ".cursorrules"},
			},
			Writer: CursorWriter{},
		},
		{
			Kind:        KindAntigravity,
			DisplayName: "Google Antigravity (Gemini)",
			RootDir:     ".agents",
			DetectMarkers: []DetectMarker{
				{Path: ".agents", Evidence: ".agents"},
				{Path: ".agent", Evidence: ".agent"},
				{Path: "GEMINI.md", Evidence: "GEMINI.md"},
			},
			Writer: AntigravityWriter{},
		},
	}
}

func SpecForKind(kind string) (TargetSpec, bool) {
	for _, spec := range AllTargetSpecs() {
		if spec.Kind == kind {
			return spec, true
		}
	}
	return TargetSpec{}, false
}

func WriterForKind(kind string) (Target, bool) {
	spec, ok := SpecForKind(kind)
	if !ok {
		return nil, false
	}
	return spec.Writer, true
}

func SupportedTargetKinds() []string {
	specs := AllTargetSpecs()
	kinds := make([]string, 0, len(specs))
	for _, spec := range specs {
		kinds = append(kinds, spec.Kind)
	}
	return kinds
}
