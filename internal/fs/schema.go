package fs

import (
	"fmt"

	"sth/internal/artifact"
)

// ResourceStatus represents the lifecycle state of an installed resource.
type ResourceStatus string

const (
	ResourceStatusUpToDate           ResourceStatus = "up-to-date"
	ResourceStatusOutdated           ResourceStatus = "outdated"
	ResourceStatusModifiedLocally    ResourceStatus = "modified-locally"
	ResourceStatusPartiallyConverted ResourceStatus = "partially-converted"
	ResourceStatusNotInstallable     ResourceStatus = "not-installable"
	ResourceStatusUnknownBaseline    ResourceStatus = "unknown-baseline"
)

// DryRunChange describes a single file change that would be made by install/update.
type DryRunChange struct {
	Action       string `json:"action"` // "add", "modify", "delete"
	Path         string `json:"path"`
	OldSHA256    string `json:"old_sha256,omitempty"`
	NewSHA256    string `json:"new_sha256,omitempty"`
	ResourceName string `json:"resource_name,omitempty"`
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

// ProjectConfigV2 is the v2 project configuration stored in .sth/project.json.
//
// Targets and ScannerHint are PR3+ extensions; omitempty keeps legacy v2
// configs untouched on write when these fields are not set. When Targets
// is empty, EffectiveTargets synthesizes a single Claude destination; target
// identity is the destination tool, never the source provider.
type ProjectConfigV2 struct {
	SchemaVersion int             `json:"schema_version"`
	Providers     []ProviderEntry `json:"providers"`
	Resources     []ResourceEntry `json:"resources"`
	Targets       []TargetEntry   `json:"targets,omitempty"`
	ScannerHint   string          `json:"scanner_hint,omitempty"`
}

type TargetEntry struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	RootDir string `json:"root_dir"`
	Enabled bool   `json:"enabled"`
}

// TargetKindClaude is the default target kind synthesized for legacy v2
// project configs that have no explicit Targets list.
const TargetKindClaude = "claude"

func EffectiveTargets(cfg ProjectConfigV2) []TargetEntry {
	if len(cfg.Targets) > 0 {
		return cfg.Targets
	}
	if len(cfg.Providers) == 0 {
		return []TargetEntry{}
	}
	return []TargetEntry{{
		ID:      TargetKindClaude,
		Kind:    TargetKindClaude,
		RootDir: fallbackTargetDirFor(cfg),
		Enabled: true,
	}}
}

// ProviderEntry describes one configured remote provider in v2 project.json.
type ProviderEntry struct {
	ID           string `json:"id"`
	Provider     string `json:"provider"`
	RepositoryID string `json:"repository_id"`
	CatalogRef   string `json:"catalog_ref"`
	CatalogPath  string `json:"catalog_path"`
	TargetDir    string `json:"target_dir"`
	Private      bool   `json:"private,omitempty"`
}

// ResourceEntry tracks a managed resource in the providers array.
type ResourceEntry struct {
	ResourceName  string `json:"resource_name"`
	ProviderID    string `json:"provider_id"`
	PinnedVersion string `json:"pinned_version,omitempty"`
}

// InstalledResourceStateV2 is the v2 state stored in sth-state.json.
type InstalledResourceStateV2 struct {
	SchemaVersion int                         `json:"schema_version"`
	Entries       []InstalledResourceRecordV2 `json:"entries"`
}

// InstalledResourceRecordV2 tracks one installed resource in sth-state.json v2.
//
// TargetID and ArtifactKind are PR3+ extensions; omitempty keeps legacy v2
// records untouched on write when these fields are not set. The state key
// becomes (ResourceName, TargetID) when multi-target install lands in PR5;
// today TargetID defaults to "" which is equivalent to the historical
// single-target ("claude") behavior.
type InstalledResourceRecordV2 struct {
	ResourceName        string                       `json:"resource_name"`
	ProviderID          string                       `json:"provider_id"`
	ResolvedVersion     string                       `json:"resolved_version"`
	PreviousVersion     string                       `json:"previous_version"`
	InstalledAt         string                       `json:"installed_at"`
	ManagedFiles        []ManagedFile                `json:"managed_files"`
	ManagedRegions      []ManagedRegion              `json:"managed_regions,omitempty"`
	SourcePath          string                       `json:"source_path,omitempty"`
	TargetID            string                       `json:"target_id,omitempty"`
	TargetKind          string                       `json:"target_kind,omitempty"`
	Consumers           []string                     `json:"consumers,omitempty"`
	ArtifactKind        string                       `json:"artifact_kind,omitempty"`
	SourceDialect       string                       `json:"source_dialect,omitempty"`
	SourceSHA256        string                       `json:"source_sha256,omitempty"`
	TargetDialect       string                       `json:"target_dialect,omitempty"`
	Compatibility       string                       `json:"compatibility,omitempty"`
	CompatibilityReport artifact.CompatibilityReport `json:"compatibility_report,omitempty"`
	TransformerVersion  string                       `json:"transformer_version,omitempty"`
}

// ManagedFile tracks a single file managed by an installed resource.
type ManagedFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type ManagedRegion struct {
	Path   string `json:"path"`
	ID     string `json:"id"`
	SHA256 string `json:"sha256"`
}
