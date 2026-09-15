package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"sth/internal/artifact"
	"sth/internal/writer"
)

func TestCapabilitiesManifestIsDeterministicAndComplete(t *testing.T) {
	first := buildCapabilitiesManifest()
	second := buildCapabilitiesManifest()
	firstJSON, firstErr := json.Marshal(first)
	if firstErr != nil {
		t.Fatal(firstErr)
	}
	secondJSON, secondErr := json.Marshal(second)
	if secondErr != nil {
		t.Fatal(secondErr)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("manifest is not deterministic:\n%+v\n%+v", first, second)
	}
	if first.SchemaVersion != 1 || len(first.Targets) != 6 || len(first.ArtifactKinds) != 6 || len(first.TargetSpecs) != 6 || len(first.Compatibility) != 36 {
		t.Fatalf("manifest = %+v", first)
	}
	for targetIndex := 1; targetIndex < len(first.Targets); targetIndex++ {
		if first.Targets[targetIndex-1] >= first.Targets[targetIndex] {
			t.Fatalf("targets are not strictly sorted: %v", first.Targets)
		}
	}
	for specificationIndex := range first.TargetSpecs {
		specification := first.TargetSpecs[specificationIndex]
		if specification.Writer == nil || specification.Validate == nil || specification.ValidatorID == "" || len(specification.NativePaths) == 0 || len(specification.Dialects) == 0 {
			t.Fatalf("target spec is incomplete: %+v", specification)
		}
	}
}

func TestCapabilitiesJSONUsesStdoutOnly(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	application := newApp(nil, &stdout, &stderr, nil)
	if err := application.runCapabilities([]string{"--json"}); err != nil {
		t.Fatal(err)
	}
	var manifest capabilitiesManifest
	if err := json.Unmarshal(stdout.Bytes(), &manifest); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
	}
	if stderr.Len() != 0 || len(manifest.Compatibility) != 36 {
		t.Fatalf("stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
}

func TestNonInteractiveLossyInstallRequiresExplicitAllowance(t *testing.T) {
	application := newApp(nil, io.Discard, io.Discard, nil)
	application.installAllowed = map[artifact.CompatibilityLevel]bool{artifact.CompatibilityExact: true}
	application.installApprovedLossy = map[string]bool{}
	capability, exists := writer.CapabilityFor(writer.KindClaude, "agent")
	if !exists {
		t.Fatal("missing Claude agent capability")
	}
	if application.allowInstallCapability(capability, writer.KindClaude, "agents/reviewer.md", false) {
		t.Fatal("lossy conversion was allowed without explicit authorization")
	}
	application.installAllowed[artifact.CompatibilityLossy] = true
	if !application.allowInstallCapability(capability, writer.KindClaude, "agents/reviewer.md", false) {
		t.Fatal("explicit lossy authorization was ignored")
	}
}

func TestInteractiveLossyInstallReportsAndConfirmsOnStderr(t *testing.T) {
	var stderr bytes.Buffer
	application := newApp(strings.NewReader("y\n"), io.Discard, &stderr, nil)
	application.interactive = func() bool { return true }
	application.installAllowed = map[artifact.CompatibilityLevel]bool{artifact.CompatibilityExact: true}
	application.installApprovedLossy = map[string]bool{}
	capability, exists := writer.CapabilityFor(writer.KindClaude, "agent")
	if !exists {
		t.Fatal("missing Claude agent capability")
	}
	if !application.allowInstallCapability(capability, writer.KindClaude, "agents/reviewer.md", false) {
		t.Fatal("interactive confirmation was ignored")
	}
	if !strings.Contains(stderr.String(), "compatibility claude/agent: lossy") || !strings.Contains(stderr.String(), "Install this lossy conversion") {
		t.Fatalf("compatibility report or prompt missing from stderr: %q", stderr.String())
	}
}

func TestJSONLossyInstallNeverPrompts(t *testing.T) {
	var stderr bytes.Buffer
	application := newApp(strings.NewReader("y\n"), io.Discard, &stderr, nil)
	application.interactive = func() bool { return true }
	application.installAllowed = map[artifact.CompatibilityLevel]bool{artifact.CompatibilityExact: true}
	application.installApprovedLossy = map[string]bool{}
	capability, exists := writer.CapabilityFor(writer.KindClaude, "agent")
	if !exists {
		t.Fatal("missing Claude agent capability")
	}
	if application.allowInstallCapability(capability, writer.KindClaude, "agents/reviewer.md", true) {
		t.Fatal("JSON install accepted lossy conversion without --allow")
	}
	if strings.Contains(stderr.String(), "Install this lossy conversion") {
		t.Fatalf("JSON install prompted on stderr: %q", stderr.String())
	}
}
