package fs

// ProjectConfigV2 is the v2 project configuration stored in .loreskills/project.json.
type ProjectConfigV2 struct {
	SchemaVersion int             `json:"schema_version"`
	Providers     []ProviderEntry `json:"providers"`
	Resources     []ResourceEntry `json:"resources"`
}

// ProviderEntry describes one configured remote provider in v2 project.json.
type ProviderEntry struct {
	ID           string `json:"id"`
	Provider     string `json:"provider"`
	RepositoryID string `json:"repository_id"`
	CatalogRef   string `json:"catalog_ref"`
	CatalogPath  string `json:"catalog_path"`
	TargetDir    string `json:"target_dir"`
}

// ResourceEntry tracks a managed resource in the providers array.
type ResourceEntry struct {
	ResourceName  string `json:"resource_name"`
	ProviderID    string `json:"provider_id"`
	PinnedVersion string `json:"pinned_version,omitempty"`
}

// InstalledResourceStateV2 is the v2 state stored in lore-state.json.
type InstalledResourceStateV2 struct {
	SchemaVersion int                         `json:"schema_version"`
	Entries       []InstalledResourceRecordV2 `json:"entries"`
}

// InstalledResourceRecordV2 tracks one installed resource in lore-state.json v2.
type InstalledResourceRecordV2 struct {
	ResourceName    string        `json:"resource_name"`
	ProviderID      string        `json:"provider_id"`
	ResolvedVersion string        `json:"resolved_version"`
	PreviousVersion string        `json:"previous_version"`
	InstalledAt     string        `json:"installed_at"`
	ManagedFiles    []ManagedFile `json:"managed_files"`
}

// ManagedFile tracks a single file managed by an installed resource.
type ManagedFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}
