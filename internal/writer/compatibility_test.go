package writer

import "testing"

func TestArtifactCapabilitiesCoverFiveConsumersAndFourArtifacts(t *testing.T) {
	capabilities := ArtifactCapabilities()
	if len(capabilities) != 20 {
		t.Fatalf("capabilities = %d, want 20", len(capabilities))
	}

	validLevels := map[CompatibilityLevel]bool{
		CompatibilityNative:      true,
		CompatibilityAdapted:     true,
		CompatibilityPartial:     true,
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
		for _, artifactKind := range []string{"skill", "agent", "command", "instruction"} {
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

func TestInstructionCapabilitiesAreAdaptedAndImplemented(t *testing.T) {
	testCases := []struct {
		targetKind  string
		destination string
	}{
		{targetKind: KindCodex, destination: ".codex/sth/instructions/{name}.md"},
		{targetKind: KindGemini, destination: ".gemini/sth/instructions/{name}.md"},
	}

	for _, testCase := range testCases {
		capability, exists := CapabilityFor(testCase.targetKind, "instruction")
		if !exists {
			t.Fatalf("missing capability %s/instruction", testCase.targetKind)
		}
		if !capability.Implemented || capability.Level != CompatibilityAdapted || capability.Destination != testCase.destination || capability.Reason != "" {
			t.Errorf("%s/instruction capability = %+v", testCase.targetKind, capability)
		}
	}
}
