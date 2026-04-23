package fs

import (
	"encoding/json"
	"testing"
)

func TestProjectConfigV2_RoundTrip(t *testing.T) {
	original := ProjectConfigV2{
		SchemaVersion: 2,
		Providers: []ProviderEntry{
			{
				ID:           "github-main",
				Provider:     "github",
				RepositoryID: "owner/repo",
				CatalogRef:   "main",
				CatalogPath:  "SKILLS.md",
				TargetDir:    ".claude/skills",
			},
		},
		Resources: []ResourceEntry{
			{ResourceName: "my-skill", ProviderID: "github-main", PinnedVersion: "v1.2.3"},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var got ProjectConfigV2
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if got.SchemaVersion != original.SchemaVersion {
		t.Errorf("SchemaVersion: got %d, want %d", got.SchemaVersion, original.SchemaVersion)
	}
	if len(got.Providers) != 1 {
		t.Fatalf("Providers length: got %d, want 1", len(got.Providers))
	}
	p := got.Providers[0]
	if p.ID != "github-main" || p.Provider != "github" || p.RepositoryID != "owner/repo" ||
		p.CatalogRef != "main" || p.CatalogPath != "SKILLS.md" || p.TargetDir != ".claude/skills" {
		t.Errorf("ProviderEntry fields mismatch: %+v", p)
	}
	if len(got.Resources) != 1 {
		t.Fatalf("Resources length: got %d, want 1", len(got.Resources))
	}
	r := got.Resources[0]
	if r.ResourceName != "my-skill" || r.ProviderID != "github-main" || r.PinnedVersion != "v1.2.3" {
		t.Errorf("ResourceEntry fields mismatch: %+v", r)
	}
}

func TestProviderEntry_RoundTrip(t *testing.T) {
	original := ProviderEntry{
		ID:           "test-id",
		Provider:     "github",
		RepositoryID: "org/repo",
		CatalogRef:   "v2",
		CatalogPath:  "docs/SKILLS.md",
		TargetDir:    "skills",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var got ProviderEntry
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if got != original {
		t.Errorf("got %+v, want %+v", got, original)
	}
}

func TestResourceEntry_RoundTrip(t *testing.T) {
	original := ResourceEntry{
		ResourceName:  "skill-a",
		ProviderID:    "prov-1",
		PinnedVersion: "v0.1.0",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var got ResourceEntry
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if got != original {
		t.Errorf("got %+v, want %+v", got, original)
	}
}

func TestResourceEntry_PinnedVersionOmitted(t *testing.T) {
	r := ResourceEntry{ResourceName: "skill-b", ProviderID: "prov-2"}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal to map failed: %v", err)
	}

	if _, exists := raw["pinned_version"]; exists {
		t.Error("pinned_version should be omitted from JSON when empty")
	}
}

func TestInstalledResourceStateV2_RoundTrip(t *testing.T) {
	original := InstalledResourceStateV2{
		SchemaVersion: 2,
		Entries: []InstalledResourceRecordV2{
			{
				ResourceName:    "skill-x",
				ProviderID:      "prov-a",
				ResolvedVersion: "abc123",
				PreviousVersion: "def456",
				InstalledAt:     "2024-01-01T00:00:00Z",
				ManagedFiles: []ManagedFile{
					{Path: "skills/skill-x/file.md", SHA256: "deadbeef"},
				},
			},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var got InstalledResourceStateV2
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if got.SchemaVersion != 2 {
		t.Errorf("SchemaVersion: got %d, want 2", got.SchemaVersion)
	}
	if len(got.Entries) != 1 {
		t.Fatalf("Entries length: got %d, want 1", len(got.Entries))
	}
	e := got.Entries[0]
	if e.ResourceName != "skill-x" || e.ProviderID != "prov-a" ||
		e.ResolvedVersion != "abc123" || e.PreviousVersion != "def456" ||
		e.InstalledAt != "2024-01-01T00:00:00Z" {
		t.Errorf("record fields mismatch: %+v", e)
	}
	if len(e.ManagedFiles) != 1 || e.ManagedFiles[0].Path != "skills/skill-x/file.md" || e.ManagedFiles[0].SHA256 != "deadbeef" {
		t.Errorf("ManagedFiles mismatch: %+v", e.ManagedFiles)
	}
}

func TestInstalledResourceStateV2_EmptyEntriesNotNull(t *testing.T) {
	state := InstalledResourceStateV2{
		SchemaVersion: 2,
		Entries:       []InstalledResourceRecordV2{},
	}

	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal to map failed: %v", err)
	}

	entries, ok := raw["entries"]
	if !ok {
		t.Fatal("entries key missing from JSON")
	}

	arr, ok := entries.([]any)
	if !ok {
		t.Fatalf("entries is not an array: %T", entries)
	}
	if len(arr) != 0 {
		t.Errorf("expected empty array, got %v", arr)
	}
}

func TestInstalledResourceRecordV2_RoundTrip(t *testing.T) {
	original := InstalledResourceRecordV2{
		ResourceName:    "skill-y",
		ProviderID:      "prov-b",
		ResolvedVersion: "v2.0.0",
		PreviousVersion: "v1.0.0",
		InstalledAt:     "2024-06-15T12:00:00Z",
		ManagedFiles:    []ManagedFile{{Path: "a/b.md", SHA256: "cafebabe"}},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var got InstalledResourceRecordV2
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if got.ResourceName != original.ResourceName || got.ProviderID != original.ProviderID ||
		got.ResolvedVersion != original.ResolvedVersion || got.PreviousVersion != original.PreviousVersion ||
		got.InstalledAt != original.InstalledAt {
		t.Errorf("record mismatch: got %+v, want %+v", got, original)
	}
	if len(got.ManagedFiles) != 1 || got.ManagedFiles[0] != original.ManagedFiles[0] {
		t.Errorf("ManagedFiles mismatch: got %+v, want %+v", got.ManagedFiles, original.ManagedFiles)
	}
}

func TestInstalledResourceRecordV2_EmptyManagedFilesNotNull(t *testing.T) {
	record := InstalledResourceRecordV2{
		ResourceName: "skill-z",
		ProviderID:   "prov-c",
		ManagedFiles: []ManagedFile{},
	}

	data, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal to map failed: %v", err)
	}

	files, ok := raw["managed_files"]
	if !ok {
		t.Fatal("managed_files key missing from JSON")
	}

	arr, ok := files.([]any)
	if !ok {
		t.Fatalf("managed_files is not an array: %T", files)
	}
	if len(arr) != 0 {
		t.Errorf("expected empty array, got %v", arr)
	}
}

func TestManagedFile_RoundTrip(t *testing.T) {
	original := ManagedFile{
		Path:   "path/to/file.md",
		SHA256: "0123456789abcdef",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var got ManagedFile
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if got != original {
		t.Errorf("got %+v, want %+v", got, original)
	}
}

// T028
func TestResourceEntry_PinnedVersionRoundTrip(t *testing.T) {
	original := ResourceEntry{ResourceName: "coding", ProviderID: "default", PinnedVersion: "abc123"}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var got ResourceEntry
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if got.PinnedVersion != "abc123" {
		t.Errorf("PinnedVersion: got %q, want %q", got.PinnedVersion, "abc123")
	}
}

// T029
func TestResourceEntry_AbsentPinnedVersion_TreatedAsUnpinned(t *testing.T) {
	raw := `{"resource_name":"coding","provider_id":"default"}`

	var got ResourceEntry
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if got.PinnedVersion != "" {
		t.Errorf("PinnedVersion: got %q, want empty string", got.PinnedVersion)
	}
}
