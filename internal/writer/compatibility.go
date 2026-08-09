package writer

type CompatibilityLevel string

const (
	CompatibilityNative      CompatibilityLevel = "native"
	CompatibilityAdapted     CompatibilityLevel = "adapted"
	CompatibilityPartial     CompatibilityLevel = "partial"
	CompatibilityUnsupported CompatibilityLevel = "unsupported"
)

type ArtifactCapability struct {
	TargetKind   string             `json:"target_kind"`
	ArtifactKind string             `json:"artifact_kind"`
	Level        CompatibilityLevel `json:"level"`
	Surface      string             `json:"surface"`
	Destination  string             `json:"destination,omitempty"`
	Invocation   string             `json:"invocation,omitempty"`
	Implemented  bool               `json:"implemented"`
	Reason       string             `json:"reason,omitempty"`
}

var artifactCapabilities = []ArtifactCapability{
	{TargetKind: KindClaude, ArtifactKind: "skill", Level: CompatibilityNative, Surface: "Claude Code", Destination: ".claude/skills/{name}/SKILL.md", Invocation: "/{name}", Implemented: true},
	{TargetKind: KindClaude, ArtifactKind: "agent", Level: CompatibilityNative, Surface: "Claude Code", Destination: ".claude/agents/{name}.md", Implemented: true},
	{TargetKind: KindClaude, ArtifactKind: "command", Level: CompatibilityNative, Surface: "Claude Code", Destination: ".claude/commands/{name}.md", Invocation: "/{name}", Implemented: true},
	{TargetKind: KindClaude, ArtifactKind: "instruction", Level: CompatibilityNative, Surface: "Claude Code", Destination: ".claude/rules/{name}.md", Implemented: true},
	{TargetKind: KindCodex, ArtifactKind: "skill", Level: CompatibilityNative, Surface: "Codex", Destination: ".agents/skills/{name}/SKILL.md", Invocation: "${name}", Implemented: true},
	{TargetKind: KindCodex, ArtifactKind: "agent", Level: CompatibilityNative, Surface: "Codex", Destination: ".codex/agents/{name}.toml", Implemented: true},
	{TargetKind: KindCodex, ArtifactKind: "command", Level: CompatibilityAdapted, Surface: "Codex", Destination: ".agents/skills/{name}/SKILL.md", Invocation: "${name}", Implemented: true},
	{TargetKind: KindCodex, ArtifactKind: "instruction", Level: CompatibilityAdapted, Surface: "Codex", Destination: ".codex/sth/instructions/{name}.md", Implemented: true},
	{TargetKind: KindCopilot, ArtifactKind: "skill", Level: CompatibilityNative, Surface: "Copilot", Destination: ".claude/skills/{name}/SKILL.md", Invocation: "/{name}", Implemented: true},
	{TargetKind: KindCopilot, ArtifactKind: "agent", Level: CompatibilityNative, Surface: "Copilot", Destination: ".github/agents/{name}.agent.md", Implemented: true},
	{TargetKind: KindCopilot, ArtifactKind: "command", Level: CompatibilityNative, Surface: "Copilot CLI et IDE", Destination: ".claude/commands/{name}.md + .github/prompts/{name}.prompt.md", Invocation: "/{name}", Implemented: true},
	{TargetKind: KindCopilot, ArtifactKind: "instruction", Level: CompatibilityNative, Surface: "Copilot", Destination: ".github/instructions/{name}.instructions.md", Implemented: true},
	{TargetKind: KindAntigravity, ArtifactKind: "skill", Level: CompatibilityNative, Surface: "Antigravity IDE", Destination: ".agents/skills/{name}/SKILL.md", Implemented: true},
	{TargetKind: KindAntigravity, ArtifactKind: "agent", Level: CompatibilityNative, Surface: "Antigravity IDE", Destination: ".agents/agents/{name}.md", Implemented: true},
	{TargetKind: KindAntigravity, ArtifactKind: "command", Level: CompatibilityAdapted, Surface: "Antigravity IDE", Destination: ".agents/workflows/{name}.md", Invocation: "/{name}", Implemented: true},
	{TargetKind: KindAntigravity, ArtifactKind: "instruction", Level: CompatibilityNative, Surface: "Antigravity IDE", Destination: ".agents/rules/{name}.md", Implemented: true},
	{TargetKind: KindGemini, ArtifactKind: "skill", Level: CompatibilityNative, Surface: "Gemini CLI", Destination: ".agents/skills/{name}/SKILL.md", Invocation: "/skills reload", Implemented: true},
	{TargetKind: KindGemini, ArtifactKind: "agent", Level: CompatibilityNative, Surface: "Gemini CLI", Destination: ".gemini/agents/{name}.md", Invocation: "@{name}", Implemented: true},
	{TargetKind: KindGemini, ArtifactKind: "command", Level: CompatibilityAdapted, Surface: "Gemini CLI", Destination: ".gemini/commands/{name}.toml", Invocation: "/{name}", Implemented: true},
	{TargetKind: KindGemini, ArtifactKind: "instruction", Level: CompatibilityAdapted, Surface: "Gemini CLI", Destination: ".gemini/sth/instructions/{name}.md", Implemented: true},
}

func ConsumerTargetKinds() []string {
	return []string{KindClaude, KindCodex, KindCopilot, KindAntigravity, KindGemini}
}

func ArtifactCapabilities() []ArtifactCapability {
	capabilities := make([]ArtifactCapability, len(artifactCapabilities))
	copy(capabilities, artifactCapabilities)
	return capabilities
}

func CapabilityFor(targetKind, artifactKind string) (ArtifactCapability, bool) {
	for _, capability := range artifactCapabilities {
		if capability.TargetKind == targetKind && capability.ArtifactKind == artifactKind {
			return capability, true
		}
	}
	return ArtifactCapability{}, false
}
