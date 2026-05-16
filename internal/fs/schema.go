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
// is empty, callers should resolve the effective targets via EffectiveTargets,
// which synthesizes a single Claude target from each provider's TargetDir.
type ProjectConfigV2 struct {
	SchemaVersion int             `json:"schema_version"`
	Providers     []ProviderEntry `json:"providers"`
	Resources     []ResourceEntry `json:"resources"`
	Targets       []TargetEntry   `json:"targets,omitempty"`
	ScannerHint   string          `json:"scanner_hint,omitempty"`
}

// TargetEntry describes one writable destination tool for installed artifacts.
// Kind values today: "claude", "copilot", "codex", "cursor". RootDir is the
// project-relative directory where the writer for that kind installs files
// (e.g. ".claude/skills" for Claude, ".github" for Copilot).
type TargetEntry struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	RootDir string `json:"root_dir"`
	Enabled bool   `json:"enabled"`
}

// TargetKindClaude is the default target kind synthesized for legacy v2
// project configs that have no explicit Targets list.
const TargetKindClaude = "claude"

// EffectiveTargets returns the targets STH should install to for cfg.
// When cfg.Targets is non-empty it is returned as-is. Otherwise one
// Claude target is synthesized per provider, using the provider's TargetDir
// as the destination root. This preserves the pre-multi-target behavior:
// any v2 config written before PR5 still installs to the same Claude
// .claude/skills directory it always did.
func EffectiveTargets(cfg ProjectConfigV2) []TargetEntry {
	if len(cfg.Targets) > 0 {
		return cfg.Targets
	}
	result := make([]TargetEntry, 0, len(cfg.Providers))
	for _, provider := range cfg.Providers {
		result = append(result, TargetEntry{
			ID:      provider.ID,
			Kind:    TargetKindClaude,
			RootDir: provider.TargetDir,
			Enabled: true,
		})
	}
	return result
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
	ResourceName    string        `json:"resource_name"`
	ProviderID      string        `json:"provider_id"`
	ResolvedVersion string        `json:"resolved_version"`
	PreviousVersion string        `json:"previous_version"`
	InstalledAt     string        `json:"installed_at"`
	ManagedFiles    []ManagedFile `json:"managed_files"`
	TargetID        string        `json:"target_id,omitempty"`
	ArtifactKind    string        `json:"artifact_kind,omitempty"`
}

// ManagedFile tracks a single file managed by an installed resource.
type ManagedFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}
