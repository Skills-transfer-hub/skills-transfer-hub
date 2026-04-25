package fs

import "fmt"

// ResourceStatus represents the lifecycle state of an installed resource.
type ResourceStatus string

const (
	ResourceStatusUpToDate        ResourceStatus = "up-to-date"
	ResourceStatusOutdated        ResourceStatus = "outdated"
	ResourceStatusModifiedLocally ResourceStatus = "modified-locally"
	ResourceStatusUnknownBaseline ResourceStatus = "unknown-baseline"
)

// DryRunChange describes a single file change that would be made by install/update.
type DryRunChange struct {
	Action    string `json:"action"` // "add", "modify", "delete"
	Path      string `json:"path"`
	OldSHA256 string `json:"old_sha256,omitempty"`
	NewSHA256 string `json:"new_sha256,omitempty"`
}

// OwnershipConflict describes a file path claimed by two resources.
type OwnershipConflict struct {
	FilePath      string
	ExistingOwner string
	NewOwner      string
}

func (c *OwnershipConflict) Error() string {
	return fmt.Sprintf("conflict: file %q is already owned by resource %q (installing %q); use --force to override",
		c.FilePath, c.ExistingOwner, c.NewOwner)
}

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
