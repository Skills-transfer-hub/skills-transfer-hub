package fs

import (
	"os"
	"path/filepath"
	"testing"

	"sth/internal/artifact"
)

func TestReadInstalledStateV3OrLegacyAggregatesWithoutMutation(t *testing.T) {
	projectRoot := t.TempDir()
	restoreWorkingDirectory := enterStateV3TestDirectory(t, projectRoot)
	defer restoreWorkingDirectory()
	firstDirectory := ".sth/provider-one"
	secondDirectory := ".sth/provider-two"
	if err := os.MkdirAll(firstDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(secondDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	firstState := InstalledResourceStateV2{SchemaVersion: 2, Entries: []InstalledResourceRecordV2{{ResourceName: "agent::agents::reviewer", ProviderID: "one", ArtifactKind: "agent", TargetID: "claude", TargetKind: "claude", SourcePath: "agents/reviewer.md", ManagedFiles: []ManagedFile{{Path: ".claude/agents/reviewer.md", SHA256: "one"}}}}}
	secondState := InstalledResourceStateV2{SchemaVersion: 2, Entries: []InstalledResourceRecordV2{{ResourceName: "agent::agents::reviewer", ProviderID: "two", ArtifactKind: "agent", TargetID: "codex", TargetKind: "codex", SourcePath: "agents/reviewer.md", ManagedFiles: []ManagedFile{{Path: ".codex/agents/reviewer.toml", SHA256: "two"}}}}}
	if err := WriteInstalledStateV2(firstDirectory, firstState); err != nil {
		t.Fatal(err)
	}
	if err := WriteInstalledStateV2(secondDirectory, secondState); err != nil {
		t.Fatal(err)
	}

	state, err := ReadInstalledStateV3OrLegacy(projectRoot, []string{firstDirectory, secondDirectory})
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Resources) != 2 {
		t.Fatalf("resources = %+v", state.Resources)
	}
	if _, err := os.Stat(InstalledStateV3Path(projectRoot)); !os.IsNotExist(err) {
		t.Fatalf("read-only aggregation created v3: %v", err)
	}
}

func TestPromoteInstalledStateV3PreservesV2AndCoalescesPlans(t *testing.T) {
	projectRoot := t.TempDir()
	restoreWorkingDirectory := enterStateV3TestDirectory(t, projectRoot)
	defer restoreWorkingDirectory()
	firstDirectory := ".sth/provider-one"
	secondDirectory := ".sth/provider-two"
	if err := os.MkdirAll(firstDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(secondDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	resourceName := "instruction::instructions::go"
	firstState := InstalledResourceStateV2{SchemaVersion: 2, Entries: []InstalledResourceRecordV2{{ResourceName: resourceName, ProviderID: "default", ArtifactKind: "instruction", TargetID: "codex", TargetKind: "codex", Compatibility: "lossy", SourcePath: "instructions/go.md", ManagedFiles: []ManagedFile{{Path: ".codex/sth/instructions/go.md", SHA256: "one"}}}}}
	secondState := InstalledResourceStateV2{SchemaVersion: 2, Entries: []InstalledResourceRecordV2{{ResourceName: resourceName, ProviderID: "default", ArtifactKind: "instruction", TargetID: "gemini", TargetKind: "gemini", Compatibility: "lossy", SourcePath: "instructions/go.md", ManagedFiles: []ManagedFile{{Path: ".gemini/sth/instructions/go.md", SHA256: "two"}}}}}
	if err := WriteInstalledStateV2(firstDirectory, firstState); err != nil {
		t.Fatal(err)
	}
	if err := WriteInstalledStateV2(secondDirectory, secondState); err != nil {
		t.Fatal(err)
	}
	firstV2Path := filepath.Join(firstDirectory, stateFileNameV2)
	firstV2Before, err := os.ReadFile(firstV2Path)
	if err != nil {
		t.Fatal(err)
	}

	if err := PromoteInstalledStateV3(projectRoot, []string{firstDirectory, secondDirectory}); err != nil {
		t.Fatal(err)
	}
	state, err := ReadInstalledStateV3(projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Resources) != 1 || len(state.Resources[0].Plans) != 2 {
		t.Fatalf("promoted state = %+v", state)
	}
	firstV2After, err := os.ReadFile(firstV2Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstV2After) != string(firstV2Before) {
		t.Fatal("v2 state changed during v3 promotion")
	}
}

func TestComputeStatusesV3AppliesRequiredPrecedence(t *testing.T) {
	projectRoot := t.TempDir()
	outputPath := filepath.Join(projectRoot, "output.md")
	if err := os.WriteFile(outputPath, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	state := InstalledResourceStateV3{SchemaVersion: 3, Resources: []InstalledResourceRecordV3{{
		ResourceID: "agent::reviewer", ProviderID: "default", ArtifactKind: "agent", InstalledAt: "now",
		Plans: []InstalledTransformationV3{{TargetID: "claude", Compatibility: artifact.CompatibilityLossy, Outputs: []InstalledOutputV3{{Path: "output.md", SHA256: ComputeSHA256([]byte("original"))}}}},
	}}}
	results := ComputeStatusesV3(state, projectRoot)
	if len(results) != 1 || results[0].Status != ResourceStatusModifiedLocally {
		t.Fatalf("modified precedence = %+v", results)
	}
	state.Resources[0].Plans[0].Outputs[0].SHA256 = ComputeSHA256([]byte("changed"))
	results = ComputeStatusesV3(state, projectRoot)
	if results[0].Status != ResourceStatusPartiallyConverted {
		t.Fatalf("lossy status = %+v", results)
	}
	state.Resources[0].Plans = append(state.Resources[0].Plans, InstalledTransformationV3{TargetID: "cursor", Compatibility: artifact.CompatibilityUnsupported})
	results = ComputeStatusesV3(state, projectRoot)
	if results[0].Status != ResourceStatusNotInstallable {
		t.Fatalf("not-installable status = %+v", results)
	}
}

func TestCoalescedOutputsTrackAllSharedConsumers(t *testing.T) {
	plans := []InstalledTransformationV3{
		{TargetID: "codex", Outputs: []InstalledOutputV3{{Consumers: []string{"codex"}, Path: "AGENTS.md", SHA256: "shared"}}},
		{TargetID: "copilot", Outputs: []InstalledOutputV3{{Consumers: []string{"copilot"}, Path: "AGENTS.md", SHA256: "shared"}}},
		{TargetID: "cursor", Outputs: []InstalledOutputV3{{Consumers: []string{"cursor"}, Path: "AGENTS.md", SHA256: "shared"}}},
	}
	outputs := coalescedOutputs(plans)
	if len(outputs) != 1 || len(outputs[0].Consumers) != 3 || outputs[0].Consumers[0] != "codex" || outputs[0].Consumers[1] != "copilot" || outputs[0].Consumers[2] != "cursor" {
		t.Fatalf("outputs = %+v", outputs)
	}
}

func enterStateV3TestDirectory(t *testing.T, directory string) func() {
	t.Helper()
	previousDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(directory); err != nil {
		t.Fatal(err)
	}
	return func() {
		if err := os.Chdir(previousDirectory); err != nil {
			t.Fatal(err)
		}
	}
}
