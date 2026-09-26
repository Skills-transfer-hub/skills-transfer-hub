package cmd

import (
	"fmt"
	"strings"

	"sth/internal/artifact"
	"sth/internal/tui"
	"sth/internal/writer"
)

func (a *app) targetSelected(targetID, targetKind string) bool {
	if len(a.installTargets) == 0 {
		return true
	}
	return a.installTargets[targetID] || a.installTargets[targetKind]
}

func (a *app) allowInstallCapability(capability writer.ArtifactCapability, targetKind, sourcePath string, jsonOut bool) bool {
	plan := artifact.CompatibilityPlan{Level: capability.Level, TargetDialect: writer.DialectForTargetKind(targetKind, artifact.Kind(capability.ArtifactKind)), Transformer: "sth", TransformerVersion: "1", Report: artifact.CompatibilityReport{Preserved: []string{"name", "body", "files"}}}
	if capability.Level == artifact.CompatibilityLossy {
		plan.Report.Transformed = []string{"format", "frontmatter"}
		plan.Report.Warnings = []string{"target representation differs from the source dialect"}
	}
	return a.allowInstallPlan(plan, capability, targetKind, sourcePath, jsonOut)
}

func (a *app) allowInstallPlan(plan artifact.CompatibilityPlan, capability writer.ArtifactCapability, targetKind, sourcePath string, jsonOut bool) bool {
	if a.installAllowed == nil {
		switch plan.Level {
		case artifact.CompatibilityExact, artifact.CompatibilityLossy:
			return true
		case artifact.CompatibilityNativeOnly:
			return nativeSourceMatchesTarget(sourcePath, targetKind, capability.ArtifactKind)
		default:
			return false
		}
	}
	switch plan.Level {
	case artifact.CompatibilityExact:
		return a.installAllowed[artifact.CompatibilityExact]
	case artifact.CompatibilityLossy:
		a.writeCompatibilityReport(plan, capability, targetKind)
		if a.installAllowed[artifact.CompatibilityLossy] || a.installApprovedLossy[targetKind+"/"+capability.ArtifactKind] {
			return true
		}
		if jsonOut || !a.isInteractiveSession() {
			return false
		}
		session := tui.NewSession(a.stdin, a.stderr)
		confirmed, err := tui.Confirm(session, tui.ConfirmConfig{Question: "Install this lossy conversion?", DefaultYes: false})
		if err != nil || !confirmed {
			return false
		}
		a.installApprovedLossy[targetKind+"/"+capability.ArtifactKind] = true
		return true
	case artifact.CompatibilityNativeOnly:
		return nativeSourceMatchesTarget(sourcePath, targetKind, capability.ArtifactKind)
	case artifact.CompatibilityUnsupported:
		return false
	}
	return false
}

func (a *app) writeCompatibilityReport(plan artifact.CompatibilityPlan, capability writer.ArtifactCapability, targetKind string) {
	a.writeStderr(fmt.Sprintf("compatibility %s/%s: %s → %s", targetKind, capability.ArtifactKind, plan.Level, plan.TargetDialect))
	if capability.Destination != "" {
		a.writeStderr("  destination: " + capability.Destination)
	}
	for _, field := range plan.Report.Preserved {
		a.writeStderr("  preserved: " + field)
	}
	for _, field := range plan.Report.Transformed {
		a.writeStderr("  transformed: " + field)
	}
	for _, field := range plan.Report.Dropped {
		a.writeStderr("  dropped: " + field)
	}
	for _, warning := range plan.Report.Warnings {
		a.writeStderr("  warning: " + warning)
	}
}

func nativeSourceMatchesTarget(sourcePath, targetKind, artifactKind string) bool {
	normalizedPath := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(sourcePath), "\\", "/"))
	switch targetKind + "/" + artifactKind {
	case writer.KindClaude + "/agent":
		return strings.HasPrefix(normalizedPath, ".claude/agents/") && strings.HasSuffix(normalizedPath, ".md")
	case writer.KindClaude + "/instruction":
		return normalizedPath == "claude.md" || strings.HasSuffix(normalizedPath, "/claude.md")
	case writer.KindClaude + "/rule":
		return strings.HasPrefix(normalizedPath, ".claude/rules/") && strings.HasSuffix(normalizedPath, ".md")
	case writer.KindClaude + "/command":
		return strings.HasPrefix(normalizedPath, ".claude/commands/") && strings.HasSuffix(normalizedPath, ".md")
	case writer.KindClaude + "/workflow":
		return strings.HasPrefix(normalizedPath, ".claude/workflows/") && strings.HasSuffix(normalizedPath, ".js")
	case writer.KindCodex + "/agent":
		return strings.HasPrefix(normalizedPath, ".codex/agents/") && strings.HasSuffix(normalizedPath, ".toml")
	case writer.KindCodex + "/instruction", writer.KindCursor + "/instruction", writer.KindCopilot + "/instruction":
		return normalizedPath == "agents.md" || strings.HasSuffix(normalizedPath, "/agents.md")
	case writer.KindAntigravity + "/workflow":
		return strings.HasPrefix(normalizedPath, ".agents/workflows/") && strings.HasSuffix(normalizedPath, ".md")
	case writer.KindAntigravity + "/agent":
		return strings.HasPrefix(normalizedPath, ".agents/agents/") && strings.HasSuffix(normalizedPath, "/agent.md")
	case writer.KindAntigravity + "/rule", writer.KindAntigravity + "/instruction":
		return strings.HasPrefix(normalizedPath, ".agents/rules/") && strings.HasSuffix(normalizedPath, ".md")
	case writer.KindCodex + "/rule":
		return strings.HasPrefix(normalizedPath, ".codex/rules/") && strings.HasSuffix(normalizedPath, ".rules")
	case writer.KindGemini + "/agent":
		return strings.HasPrefix(normalizedPath, ".gemini/agents/") && strings.HasSuffix(normalizedPath, ".md")
	case writer.KindGemini + "/instruction":
		return normalizedPath == "gemini.md" || strings.HasSuffix(normalizedPath, "/gemini.md")
	case writer.KindGemini + "/command":
		return strings.HasPrefix(normalizedPath, ".gemini/commands/") && strings.HasSuffix(normalizedPath, ".toml")
	case writer.KindCopilot + "/agent":
		return strings.HasPrefix(normalizedPath, ".github/agents/") && strings.HasSuffix(normalizedPath, ".agent.md")
	case writer.KindCopilot + "/rule":
		return strings.HasPrefix(normalizedPath, ".github/instructions/") && strings.HasSuffix(normalizedPath, ".instructions.md")
	case writer.KindCopilot + "/command":
		return strings.HasPrefix(normalizedPath, ".github/prompts/") && strings.HasSuffix(normalizedPath, ".prompt.md")
	case writer.KindCursor + "/agent":
		return strings.HasPrefix(normalizedPath, ".cursor/agents/") && strings.HasSuffix(normalizedPath, ".md")
	case writer.KindCursor + "/rule":
		return strings.HasPrefix(normalizedPath, ".cursor/rules/") && strings.HasSuffix(normalizedPath, ".mdc")
	}
	return false
}
