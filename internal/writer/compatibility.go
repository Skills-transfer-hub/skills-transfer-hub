package writer

import (
	"sort"
	"strings"

	"sth/internal/artifact"
)

type CompatibilityLevel = artifact.CompatibilityLevel

const (
	CompatibilityExact       = artifact.CompatibilityExact
	CompatibilityLossy       = artifact.CompatibilityLossy
	CompatibilityNativeOnly  = artifact.CompatibilityNativeOnly
	CompatibilityUnsupported = artifact.CompatibilityUnsupported
)

type ArtifactCapability struct {
	TargetKind     string             `json:"target_kind"`
	ArtifactKind   string             `json:"artifact_kind"`
	Level          CompatibilityLevel `json:"level"`
	Surface        string             `json:"surface"`
	Destination    string             `json:"destination,omitempty"`
	Invocation     string             `json:"invocation,omitempty"`
	Implemented    bool               `json:"implemented"`
	Reason         string             `json:"reason,omitempty"`
	Maturity       string             `json:"maturity"`
	MinimumVersion string             `json:"minimum_version,omitempty"`
}

var artifactCapabilities = []ArtifactCapability{
	{TargetKind: KindClaude, ArtifactKind: "skill", Level: CompatibilityExact, Surface: "Claude Code", Destination: ".claude/skills/{name}/SKILL.md", Invocation: "/{name}", Implemented: true, Maturity: "stable"},
	{TargetKind: KindClaude, ArtifactKind: "agent", Level: CompatibilityLossy, Surface: "Claude Code", Destination: ".claude/agents/{name}.md", Implemented: true, Maturity: "stable"},
	{TargetKind: KindClaude, ArtifactKind: "instruction", Level: CompatibilityLossy, Surface: "Claude Code", Destination: ".claude/rules/{name}.md", Implemented: true, Maturity: "stable"},
	{TargetKind: KindClaude, ArtifactKind: "rule", Level: CompatibilityLossy, Surface: "Claude Code", Destination: ".claude/rules/{name}.md", Implemented: true, Maturity: "stable"},
	{TargetKind: KindClaude, ArtifactKind: "command", Level: CompatibilityLossy, Surface: "Claude Code", Destination: ".claude/commands/{name}.md", Invocation: "/{name}", Implemented: true, Maturity: "legacy"},
	{TargetKind: KindClaude, ArtifactKind: "workflow", Level: CompatibilityNativeOnly, Surface: "Claude Code", Destination: ".claude/workflows/{name}.js", Invocation: "/{name}", Implemented: true, Maturity: "experimental", MinimumVersion: "2.1.154"},
	{TargetKind: KindCodex, ArtifactKind: "skill", Level: CompatibilityExact, Surface: "Codex", Destination: ".agents/skills/{name}/SKILL.md", Invocation: "${name}", Implemented: true, Maturity: "stable"},
	{TargetKind: KindCodex, ArtifactKind: "agent", Level: CompatibilityLossy, Surface: "Codex", Destination: ".codex/agents/{name}.toml", Implemented: true, Maturity: "stable"},
	{TargetKind: KindCodex, ArtifactKind: "instruction", Level: CompatibilityLossy, Surface: "Codex", Destination: "AGENTS.md", Implemented: true, Maturity: "stable"},
	{TargetKind: KindCodex, ArtifactKind: "rule", Level: CompatibilityNativeOnly, Surface: "Codex", Destination: ".codex/rules/{name}.rules", Implemented: true, Maturity: "experimental"},
	{TargetKind: KindCodex, ArtifactKind: "command", Level: CompatibilityLossy, Surface: "Codex", Destination: ".agents/skills/{name}/SKILL.md", Invocation: "${name}", Implemented: true, Maturity: "stable"},
	{TargetKind: KindCodex, ArtifactKind: "workflow", Level: CompatibilityUnsupported, Surface: "Codex", Implemented: false, Reason: "Codex has no project workflow file contract", Maturity: "unsupported"},
	{TargetKind: KindGemini, ArtifactKind: "skill", Level: CompatibilityExact, Surface: "Gemini CLI", Destination: ".agents/skills/{name}/SKILL.md", Invocation: "/skills reload", Implemented: true, Maturity: "stable"},
	{TargetKind: KindGemini, ArtifactKind: "agent", Level: CompatibilityLossy, Surface: "Gemini CLI", Destination: ".gemini/agents/{name}.md", Invocation: "@{name}", Implemented: true, Maturity: "stable"},
	{TargetKind: KindGemini, ArtifactKind: "instruction", Level: CompatibilityLossy, Surface: "Gemini CLI", Destination: "GEMINI.md", Implemented: true, Maturity: "stable"},
	{TargetKind: KindGemini, ArtifactKind: "rule", Level: CompatibilityUnsupported, Surface: "Gemini CLI", Implemented: false, Reason: "Gemini CLI has no project rule contract distinct from GEMINI.md", Maturity: "unsupported"},
	{TargetKind: KindGemini, ArtifactKind: "command", Level: CompatibilityLossy, Surface: "Gemini CLI", Destination: ".gemini/commands/{name}.toml", Invocation: "/{name}", Implemented: true, Maturity: "stable"},
	{TargetKind: KindGemini, ArtifactKind: "workflow", Level: CompatibilityUnsupported, Surface: "Gemini CLI", Implemented: false, Reason: "Gemini CLI has no project workflow file contract", Maturity: "unsupported"},
	{TargetKind: KindAntigravity, ArtifactKind: "skill", Level: CompatibilityExact, Surface: "Antigravity", Destination: ".agents/skills/{name}/SKILL.md", Invocation: "/{name}", Implemented: true, Maturity: "stable"},
	{TargetKind: KindAntigravity, ArtifactKind: "agent", Level: CompatibilityLossy, Surface: "Antigravity", Destination: ".agents/agents/{name}/agent.md", Implemented: true, Maturity: "stable"},
	{TargetKind: KindAntigravity, ArtifactKind: "instruction", Level: CompatibilityLossy, Surface: "Antigravity", Destination: ".agents/rules/{name}.md", Implemented: true, Maturity: "stable"},
	{TargetKind: KindAntigravity, ArtifactKind: "rule", Level: CompatibilityLossy, Surface: "Antigravity", Destination: ".agents/rules/{name}.md", Implemented: true, Maturity: "stable"},
	{TargetKind: KindAntigravity, ArtifactKind: "command", Level: CompatibilityLossy, Surface: "Antigravity", Destination: ".agents/workflows/{name}.md", Invocation: "/{name}", Implemented: true, Maturity: "stable"},
	{TargetKind: KindAntigravity, ArtifactKind: "workflow", Level: CompatibilityNativeOnly, Surface: "Antigravity", Destination: ".agents/workflows/{name}.md", Invocation: "/{name}", Implemented: true, Maturity: "stable"},
	{TargetKind: KindCopilot, ArtifactKind: "skill", Level: CompatibilityExact, Surface: "Copilot", Destination: ".agents/skills/{name}/SKILL.md", Invocation: "/{name}", Implemented: true, Maturity: "stable"},
	{TargetKind: KindCopilot, ArtifactKind: "agent", Level: CompatibilityLossy, Surface: "Copilot", Destination: ".github/agents/{name}.agent.md", Implemented: true, Maturity: "stable"},
	{TargetKind: KindCopilot, ArtifactKind: "instruction", Level: CompatibilityLossy, Surface: "Copilot", Destination: "AGENTS.md + .github/sth/instructions/{name}.md", Implemented: true, Maturity: "stable"},
	{TargetKind: KindCopilot, ArtifactKind: "rule", Level: CompatibilityLossy, Surface: "Copilot", Destination: ".github/instructions/{name}.instructions.md", Implemented: true, Maturity: "stable"},
	{TargetKind: KindCopilot, ArtifactKind: "command", Level: CompatibilityLossy, Surface: "Copilot", Destination: ".github/prompts/{name}.prompt.md", Invocation: "/{name}", Implemented: true, Maturity: "stable"},
	{TargetKind: KindCopilot, ArtifactKind: "workflow", Level: CompatibilityUnsupported, Surface: "Copilot", Implemented: false, Reason: "Copilot workflows are outside the V1 artifact contract", Maturity: "unsupported"},
	{TargetKind: KindCursor, ArtifactKind: "skill", Level: CompatibilityExact, Surface: "Cursor", Destination: ".agents/skills/{name}/SKILL.md", Invocation: "/{name}", Implemented: true, Maturity: "stable"},
	{TargetKind: KindCursor, ArtifactKind: "agent", Level: CompatibilityLossy, Surface: "Cursor", Destination: ".cursor/agents/{name}.md", Implemented: true, Maturity: "stable"},
	{TargetKind: KindCursor, ArtifactKind: "instruction", Level: CompatibilityLossy, Surface: "Cursor", Destination: "AGENTS.md", Implemented: true, Maturity: "stable"},
	{TargetKind: KindCursor, ArtifactKind: "rule", Level: CompatibilityLossy, Surface: "Cursor", Destination: ".cursor/rules/{name}.mdc", Implemented: true, Maturity: "stable"},
	{TargetKind: KindCursor, ArtifactKind: "command", Level: CompatibilityUnsupported, Surface: "Cursor", Implemented: false, Reason: "Cursor commands are not advertised without a current certified contract", Maturity: "unsupported"},
	{TargetKind: KindCursor, ArtifactKind: "workflow", Level: CompatibilityUnsupported, Surface: "Cursor", Implemented: false, Reason: "Cursor workflows are outside the V1 artifact contract", Maturity: "unsupported"},
}

func ConsumerTargetKinds() []string {
	return []string{KindClaude, KindCodex, KindGemini, KindAntigravity, KindCopilot, KindCursor}
}

func ArtifactCapabilities() []ArtifactCapability {
	capabilities := append([]ArtifactCapability(nil), artifactCapabilities...)
	sort.Slice(capabilities, func(leftIndex, rightIndex int) bool {
		if capabilities[leftIndex].TargetKind != capabilities[rightIndex].TargetKind {
			return capabilities[leftIndex].TargetKind < capabilities[rightIndex].TargetKind
		}
		return capabilities[leftIndex].ArtifactKind < capabilities[rightIndex].ArtifactKind
	})
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

func PlanCompatibility(definition artifact.Definition, targetKind string) artifact.CompatibilityPlan {
	capability, exists := CapabilityFor(targetKind, string(definition.Kind))
	if !exists || !capability.Implemented {
		reason := "target does not support artifact kind"
		if exists && capability.Reason != "" {
			reason = capability.Reason
		}
		return artifact.CompatibilityPlan{SourceDialect: definition.SourceDialect, Level: artifact.CompatibilityUnsupported, Transformer: "sth", TransformerVersion: "1", Report: artifact.CompatibilityReport{Warnings: []string{reason}}}
	}
	targetDialect := dialectForTargetKind(targetKind, definition.Kind)
	level := capability.Level
	report := artifact.CompatibilityReport{Preserved: []string{"name", "body", "files"}}
	if definition.Kind == artifact.KindSkill || definition.Kind != artifact.KindInstruction && dialectBelongsToTarget(definition.SourceDialect, targetKind) {
		level = artifact.CompatibilityExact
	}
	if definition.Kind == artifact.KindWorkflow && !dialectBelongsToTarget(definition.SourceDialect, targetKind) {
		level = artifact.CompatibilityNativeOnly
		report.Warnings = append(report.Warnings, "workflow execution semantics are not converted")
	}
	if definition.SourceDialect == artifact.DialectCodexExecutionRule && targetKind != KindCodex {
		level = artifact.CompatibilityNativeOnly
		report.Warnings = append(report.Warnings, "Codex execution rules are not prompt guidance")
	}
	if definition.SourceDialect == artifact.DialectGeminiCommand && targetKind != KindGemini && (strings.Contains(definition.Common.Body, "!{") || strings.Contains(definition.Common.Body, "@{")) {
		level = artifact.CompatibilityNativeOnly
		report.Warnings = append(report.Warnings, "Gemini command interpolation cannot be preserved safely")
	}
	if len(definition.Native.Unmapped) > 0 && level == artifact.CompatibilityLossy {
		report.Dropped = append(report.Dropped, definition.Native.Unmapped...)
		report.Warnings = append(report.Warnings, "native fields are not representable in the target dialect")
	}
	if level == artifact.CompatibilityLossy {
		report.Transformed = append(report.Transformed, "format", "frontmatter")
		report.Warnings = append(report.Warnings, "target representation differs from the source dialect")
	}
	return artifact.CompatibilityPlan{SourceDialect: definition.SourceDialect, TargetDialect: targetDialect, Level: level, Transformer: "sth", TransformerVersion: "1", Report: report}
}

func dialectBelongsToTarget(dialect artifact.Dialect, targetKind string) bool {
	prefix := strings.SplitN(string(dialect), ".", 2)[0]
	if dialect == artifact.DialectAgentSkills {
		return true
	}
	if dialect == artifact.DialectSharedAgentsMD {
		return targetKind == KindCodex || targetKind == KindCursor || targetKind == KindCopilot
	}
	return prefix == targetKind
}

func dialectForTargetKind(targetKind string, artifactKind artifact.Kind) artifact.Dialect {
	specification, exists := SpecForKind(targetKind)
	if !exists {
		return ""
	}
	dialect, exists := specification.Dialects[artifactKind]
	if !exists {
		return ""
	}
	return dialect
}

func DialectForTargetKind(targetKind string, artifactKind artifact.Kind) artifact.Dialect {
	return dialectForTargetKind(targetKind, artifactKind)
}
