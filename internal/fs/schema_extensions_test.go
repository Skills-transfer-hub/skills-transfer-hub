package fs

import (
	"encoding/json"
	"testing"
)

func TestProjectConfigV2_TargetsAndScannerHintOmittedWhenEmpty(t *testing.T) {
	cfg := ProjectConfigV2{
		SchemaVersion: 2,
		Providers:     []ProviderEntry{{ID: "default", Provider: "github", RepositoryID: "o/r", CatalogRef: "main", TargetDir: ".claude/skills"}},
		Resources:     []ResourceEntry{},
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["targets"]; ok {
		t.Error("targets should be omitted when empty (preserves legacy v2 round-trip)")
	}
	if _, ok := raw["scanner_hint"]; ok {
		t.Error("scanner_hint should be omitted when empty")
	}
}

func TestProjectConfigV2_TargetsRoundTrip(t *testing.T) {
	cfg := ProjectConfigV2{
		SchemaVersion: 2,
		Providers:     []ProviderEntry{{ID: "default", Provider: "github", TargetDir: ".claude/skills"}},
		Resources:     []ResourceEntry{},
		Targets: []TargetEntry{
			{ID: "claude", Kind: "claude", RootDir: ".claude", Enabled: true},
			{ID: "cursor", Kind: "cursor", RootDir: ".cursor", Enabled: false},
		},
		ScannerHint: "multi-stack",
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var got ProjectConfigV2
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Targets) != 2 || got.Targets[1].Kind != "cursor" || got.Targets[1].Enabled {
		t.Errorf("Targets round-trip mismatch: %+v", got.Targets)
	}
	if got.ScannerHint != "multi-stack" {
		t.Errorf("ScannerHint = %q, want multi-stack", got.ScannerHint)
	}
}

func TestEffectiveTargets_SynthesizesClaudeWhenEmpty(t *testing.T) {
	cfg := ProjectConfigV2{
		Providers: []ProviderEntry{
			{ID: "default", TargetDir: ".claude/skills"},
			{ID: "second", TargetDir: ".claude/skills"},
		},
	}
	got := EffectiveTargets(cfg)
	if len(got) != 2 {
		t.Fatalf("got %d targets, want 2", len(got))
	}
	for i, te := range got {
		if te.Kind != TargetKindClaude {
			t.Errorf("[%d] Kind = %q, want %q", i, te.Kind, TargetKindClaude)
		}
		if !te.Enabled {
			t.Errorf("[%d] synthesized targets must be enabled", i)
		}
	}
	if got[0].ID != "default" || got[0].RootDir != ".claude/skills" {
		t.Errorf("[0] = %+v, want id=default rootdir=.claude/skills", got[0])
	}
}

func TestEffectiveTargets_ReturnsExplicitTargetsAsIs(t *testing.T) {
	explicit := []TargetEntry{{ID: "cp", Kind: "copilot", RootDir: ".github", Enabled: true}}
	cfg := ProjectConfigV2{
		Providers: []ProviderEntry{{ID: "default", TargetDir: ".claude/skills"}},
		Targets:   explicit,
	}
	got := EffectiveTargets(cfg)
	if len(got) != 1 || got[0].Kind != "copilot" {
		t.Errorf("explicit targets should win, got %+v", got)
	}
}

func TestInstalledResourceRecordV2_NewFieldsOmittedWhenEmpty(t *testing.T) {
	rec := InstalledResourceRecordV2{
		ResourceName: "skill-a",
		ProviderID:   "default",
		ManagedFiles: []ManagedFile{},
	}

	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["target_id"]; ok {
		t.Error("target_id should be omitted when empty (legacy round-trip)")
	}
	if _, ok := raw["artifact_kind"]; ok {
		t.Error("artifact_kind should be omitted when empty (legacy round-trip)")
	}
}

func TestInstalledResourceRecordV2_NewFieldsRoundTrip(t *testing.T) {
	rec := InstalledResourceRecordV2{
		ResourceName: "agent-reviewer",
		ProviderID:   "default",
		TargetID:     "claude",
		ArtifactKind: "agent",
		ManagedFiles: []ManagedFile{{Path: ".claude/agents/reviewer.md", SHA256: "deadbeef"}},
	}
	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	var got InstalledResourceRecordV2
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.TargetID != "claude" || got.ArtifactKind != "agent" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}

func TestCatalogEntry_NewFieldsOmittedWhenEmpty(t *testing.T) {
	entry := CatalogEntry{
		CatalogID:    "bundle::react::form-wizard",
		ResourceType: "bundle",
		Name:         "form-wizard",
		FolderName:   "react",
		SourcePath:   "react/form-wizard/SKILL.md",
	}
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"artifact_kind", "source_kind", "applies_to", "lang"} {
		if _, ok := raw[key]; ok {
			t.Errorf("%s should be omitted when empty", key)
		}
	}
}

func TestCatalogEntry_NewFieldsRoundTrip(t *testing.T) {
	entry := CatalogEntry{
		CatalogID:    "instruction::react::components",
		ResourceType: "instruction",
		Name:         "components",
		FolderName:   "react",
		SourcePath:   "react/instructions/components.instructions.md",
		ArtifactKind: "instruction",
		SourceKind:   "instruction_glob",
		AppliesTo:    []string{"**/*.tsx", "**/*.jsx"},
		Lang:         "react",
	}
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	var got CatalogEntry
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.ArtifactKind != "instruction" || got.SourceKind != "instruction_glob" || got.Lang != "react" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
	if len(got.AppliesTo) != 2 || got.AppliesTo[0] != "**/*.tsx" {
		t.Errorf("AppliesTo mismatch: %+v", got.AppliesTo)
	}
}
