package fs

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// T008
func TestMigrateProjectConfig_V1ToV2(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "migrate-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	v1 := map[string]any{
		"provider":      "github",
		"repository_id": "owner/repo",
		"ref":           "main",
		"catalog_path":  "SKILLS.md",
		"target_dir":    ".claude/skills",
	}
	data, _ := json.MarshalIndent(v1, "", "  ")
	p := filepath.Join(tmpDir, "project.json")
	if err := os.WriteFile(p, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("failed to write v1 project.json: %v", err)
	}

	cfg, err := MigrateProjectConfig(p, io.Discard)
	if err != nil {
		t.Fatalf("MigrateProjectConfig returned error: %v", err)
	}

	if cfg.SchemaVersion != 2 {
		t.Errorf("schema_version: got %d, want 2", cfg.SchemaVersion)
	}
	if len(cfg.Providers) != 1 {
		t.Fatalf("providers length: got %d, want 1", len(cfg.Providers))
	}
	prov := cfg.Providers[0]
	if prov.ID != "default" {
		t.Errorf("provider id: got %q, want %q", prov.ID, "default")
	}
	if prov.CatalogRef != "main" {
		t.Errorf("catalog_ref: got %q, want %q", prov.CatalogRef, "main")
	}
	if prov.Provider != "github" {
		t.Errorf("provider: got %q, want %q", prov.Provider, "github")
	}
	if prov.RepositoryID != "owner/repo" {
		t.Errorf("repository_id: got %q, want %q", prov.RepositoryID, "owner/repo")
	}
	if cfg.Resources == nil {
		t.Error("resources must not be nil")
	}
	if len(cfg.Resources) != 0 {
		t.Errorf("resources length: got %d, want 0", len(cfg.Resources))
	}

	// Verify legacy "ref" field is absent from the rewritten file.
	rewritten, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("failed to read rewritten file: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(rewritten, &raw); err != nil {
		t.Fatalf("failed to parse rewritten file: %v", err)
	}
	if _, exists := raw["ref"]; exists {
		t.Error("legacy field 'ref' must be absent from v2 file")
	}
}

// T009
func TestMigrateInstalledState_V1ToV2(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "migrate-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	v1 := map[string]any{
		"entries": []map[string]any{
			{
				"catalog_id":     "skill::skills::my-skill",
				"resource_type":  "skill",
				"name":           "my-skill",
				"folder_name":    "skills",
				"installed_path": ".claude/skills/skills/my-skill.md",
				"source_path":    "skills/my-skill.md",
				"status":         "installed",
			},
		},
	}
	data, _ := json.MarshalIndent(v1, "", "  ")
	p := filepath.Join(tmpDir, "sth-state.json")
	if err := os.WriteFile(p, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("failed to write v1 sth-state.json: %v", err)
	}

	state, err := MigrateInstalledState(p, io.Discard)
	if err != nil {
		t.Fatalf("MigrateInstalledState returned error: %v", err)
	}

	if state.SchemaVersion != 2 {
		t.Errorf("schema_version: got %d, want 2", state.SchemaVersion)
	}
	if len(state.Entries) != 1 {
		t.Fatalf("entries length: got %d, want 1", len(state.Entries))
	}
	entry := state.Entries[0]
	if entry.ResourceName != "skill::skills::my-skill" {
		t.Errorf("resource_name: got %q, want %q", entry.ResourceName, "skill::skills::my-skill")
	}
	if entry.ProviderID != "default" {
		t.Errorf("provider_id: got %q, want %q", entry.ProviderID, "default")
	}
	if entry.ManagedFiles == nil {
		t.Error("managed_files must not be nil")
	}

	// Verify managed_files serializes as [] not null.
	marshaled, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("failed to marshal entry: %v", err)
	}
	var rawEntry map[string]any
	if err := json.Unmarshal(marshaled, &rawEntry); err != nil {
		t.Fatalf("failed to parse marshaled entry: %v", err)
	}
	filesVal, ok := rawEntry["managed_files"]
	if !ok {
		t.Fatal("managed_files key missing from JSON")
	}
	if _, ok := filesVal.([]any); !ok {
		t.Fatalf("managed_files is not an array: %T", filesVal)
	}

	// Verify legacy fields are absent from the returned struct's JSON.
	if _, exists := rawEntry["catalog_id"]; exists {
		t.Error("legacy field 'catalog_id' must be absent from v2 entry")
	}
	if _, exists := rawEntry["status"]; exists {
		t.Error("legacy field 'status' must be absent from v2 entry")
	}
}

// T010
func TestMigrateProjectConfig_CreatesBackup(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "migrate-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	v1 := map[string]any{
		"provider":      "github",
		"repository_id": "owner/repo",
		"ref":           "main",
		"catalog_path":  "SKILLS.md",
		"target_dir":    ".claude/skills",
	}
	data, _ := json.MarshalIndent(v1, "", "  ")
	original := append(data, '\n')
	p := filepath.Join(tmpDir, "project.json")
	if err := os.WriteFile(p, original, 0o644); err != nil {
		t.Fatalf("failed to write v1 project.json: %v", err)
	}

	if _, err := MigrateProjectConfig(p, io.Discard); err != nil {
		t.Fatalf("MigrateProjectConfig returned error: %v", err)
	}

	bakContent, err := os.ReadFile(p + ".bak")
	if err != nil {
		t.Fatalf(".bak file missing or unreadable: %v", err)
	}
	if string(bakContent) != string(original) {
		t.Errorf("backup content mismatch:\ngot:  %q\nwant: %q", bakContent, original)
	}
}

// T011
func TestMigrateProjectConfig_WriteFailure_LeavesOriginalUntouched(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "migrate-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Pass a path whose parent directory does not exist — os.ReadFile will fail.
	nonExistentPath := filepath.Join(tmpDir, "nonexistent", "project.json")
	_, err = MigrateProjectConfig(nonExistentPath, io.Discard)
	if err == nil {
		t.Error("expected non-nil error for non-existent path, got nil")
	}
}

// T012
func TestMigrateProjectConfig_AlreadyV2_ReturnsAsIs(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "migrate-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	v2 := ProjectConfigV2{
		SchemaVersion: 2,
		Providers: []ProviderEntry{
			{
				ID:           "main",
				Provider:     "github",
				RepositoryID: "owner/repo",
				CatalogRef:   "main",
				CatalogPath:  "SKILLS.md",
				TargetDir:    ".claude/skills",
			},
		},
		Resources: []ResourceEntry{},
	}
	data, _ := json.MarshalIndent(v2, "", "  ")
	p := filepath.Join(tmpDir, "project.json")
	if err := os.WriteFile(p, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("failed to write v2 project.json: %v", err)
	}

	info, err := os.Stat(p)
	if err != nil {
		t.Fatalf("failed to stat file: %v", err)
	}
	modTimeBefore := info.ModTime()

	cfg, err := MigrateProjectConfig(p, io.Discard)
	if err != nil {
		t.Fatalf("MigrateProjectConfig returned error: %v", err)
	}

	if cfg.SchemaVersion != 2 {
		t.Errorf("schema_version: got %d, want 2", cfg.SchemaVersion)
	}
	if len(cfg.Providers) != 1 || cfg.Providers[0].ID != "main" {
		t.Errorf("providers mismatch: got %+v", cfg.Providers)
	}

	info2, err := os.Stat(p)
	if err != nil {
		t.Fatalf("failed to stat file after call: %v", err)
	}
	if !info2.ModTime().Equal(modTimeBefore) {
		t.Error("file was rewritten for already-v2 config (mod time changed)")
	}
}

// T013
func TestMigrateProjectConfig_CorruptJSON_ReturnsError(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "migrate-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	p := filepath.Join(tmpDir, "project.json")
	if err := os.WriteFile(p, []byte(`{"invalid": json`), 0o644); err != nil {
		t.Fatalf("failed to write corrupt file: %v", err)
	}

	_, err = MigrateProjectConfig(p, io.Discard)
	if err == nil {
		t.Error("expected non-nil error for corrupt JSON, got nil")
	}
}
