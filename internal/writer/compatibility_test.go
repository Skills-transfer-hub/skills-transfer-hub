package writer

import (
	"testing"

	"sth/internal/artifact"
)

func TestArtifactCapabilitiesCoverSixConsumersAndSixArtifacts(t *testing.T) {
	capabilities := ArtifactCapabilities()
	if len(capabilities) != 36 {
		t.Fatalf("capabilities = %d, want 36", len(capabilities))
	}

	validLevels := map[CompatibilityLevel]bool{
		CompatibilityExact:       true,
		CompatibilityLossy:       true,
		CompatibilityNativeOnly:  true,
		CompatibilityUnsupported: true,
	}
	seen := map[string]bool{}
	for _, capability := range capabilities {
		key := capability.TargetKind + "/" + capability.ArtifactKind
		if seen[key] {
			t.Fatalf("duplicate capability %s", key)
		}
		seen[key] = true
		if !validLevels[capability.Level] {
			t.Errorf("%s has invalid level %q", key, capability.Level)
		}
		if capability.Surface == "" {
			t.Errorf("%s has no surface", key)
		}
		if capability.Implemented && capability.Destination == "" {
			t.Errorf("%s is implemented without destination", key)
		}
		if !capability.Implemented && capability.Reason == "" {
			t.Errorf("%s is not implemented without reason", key)
		}
	}

	for _, targetKind := range ConsumerTargetKinds() {
		for _, artifactKind := range []string{"skill", "agent", "instruction", "rule", "command", "workflow"} {
			key := targetKind + "/" + artifactKind
			if !seen[key] {
				t.Errorf("missing capability %s", key)
			}
		}
	}
}

func TestArtifactCapabilitiesReturnsCopy(t *testing.T) {
	first := ArtifactCapabilities()
	first[0].TargetKind = "changed"
	second := ArtifactCapabilities()
	if second[0].TargetKind == "changed" {
		t.Fatal("ArtifactCapabilities exposed mutable package state")
	}
}

func TestSharedInstructionCapabilitiesAreLossyAndImplemented(t *testing.T) {
	testCases := []struct {
		targetKind  string
		destination string
	}{
		{targetKind: KindCodex, destination: "AGENTS.md"},
		{targetKind: KindGemini, destination: "GEMINI.md"},
	}

	for _, testCase := range testCases {
		capability, exists := CapabilityFor(testCase.targetKind, "instruction")
		if !exists {
			t.Fatalf("missing capability %s/instruction", testCase.targetKind)
		}
		if !capability.Implemented || capability.Level != CompatibilityLossy || capability.Destination != testCase.destination || capability.Reason != "" {
			t.Errorf("%s/instruction capability = %+v", testCase.targetKind, capability)
		}
	}
}

func TestCompatibilityPlannerMakesLossAndNativeLimitsExplicit(t *testing.T) {
	testCases := []struct {
		name       string
		definition artifact.Definition
		targetKind string
		expected   artifact.CompatibilityLevel
	}{
		{name: "native codex rule", definition: artifact.Definition{Kind: artifact.KindRule, SourceDialect: artifact.DialectCodexExecutionRule}, targetKind: KindCodex, expected: artifact.CompatibilityExact},
		{name: "managed codex instruction region", definition: artifact.Definition{Kind: artifact.KindInstruction, SourceDialect: artifact.DialectSharedAgentsMD}, targetKind: KindCodex, expected: artifact.CompatibilityLossy},
		{name: "codex rule cross dialect", definition: artifact.Definition{Kind: artifact.KindRule, SourceDialect: artifact.DialectCodexExecutionRule}, targetKind: KindClaude, expected: artifact.CompatibilityNativeOnly},
		{name: "gemini interpolation", definition: artifact.Definition{Kind: artifact.KindCommand, SourceDialect: artifact.DialectGeminiCommand, Common: artifact.CommonFields{Body: "Run !{git status}"}}, targetKind: KindClaude, expected: artifact.CompatibilityNativeOnly},
		{name: "claude workflow cross dialect", definition: artifact.Definition{Kind: artifact.KindWorkflow, SourceDialect: artifact.DialectClaudeWorkflow}, targetKind: KindAntigravity, expected: artifact.CompatibilityNativeOnly},
		{name: "cursor command", definition: artifact.Definition{Kind: artifact.KindCommand, SourceDialect: artifact.DialectClaudeCommand}, targetKind: KindCursor, expected: artifact.CompatibilityUnsupported},
		{name: "agent conversion", definition: artifact.Definition{Kind: artifact.KindAgent, SourceDialect: artifact.DialectClaudeAgent, Native: artifact.NativePayload{Unmapped: []string{"permissionMode"}}}, targetKind: KindCopilot, expected: artifact.CompatibilityLossy},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			plan := PlanCompatibility(testCase.definition, testCase.targetKind)
			if plan.Level != testCase.expected {
				t.Fatalf("level = %s, want %s", plan.Level, testCase.expected)
			}
			if testCase.name == "agent conversion" && (len(plan.Report.Dropped) != 1 || len(plan.Report.Warnings) == 0) {
				t.Fatalf("loss report = %+v", plan.Report)
			}
		})
	}
}
