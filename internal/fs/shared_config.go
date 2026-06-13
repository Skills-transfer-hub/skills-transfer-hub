package fs

import "fmt"

type FieldProvenance string

const (
	ProvenanceOrg   FieldProvenance = "org"
	ProvenanceLocal FieldProvenance = "local"
)

type MergeAction string

const (
	MergeActionApply     MergeAction = "apply"
	MergeActionOverwrite MergeAction = "overwrite"
	MergeActionKeep      MergeAction = "keep"
	MergeActionUnchanged MergeAction = "unchanged"
)

type ConfigEntryKind string

const (
	ConfigEntryProvider ConfigEntryKind = "provider"
	ConfigEntryResource ConfigEntryKind = "resource"
	ConfigEntryTarget   ConfigEntryKind = "target"
)

type ConfigMergeEntry struct {
	Kind       ConfigEntryKind `json:"kind"`
	Field      string          `json:"field"`
	Provenance FieldProvenance `json:"provenance"`
	Action     MergeAction     `json:"action"`
	Detail     string          `json:"detail,omitempty"`
}

type ConfigMergeResult struct {
	Merged  ProjectConfigV2
	Entries []ConfigMergeEntry
}

func providerByID(providers []ProviderEntry, id string) (ProviderEntry, bool) {
	for _, provider := range providers {
		if provider.ID == id {
			return provider, true
		}
	}
	return ProviderEntry{}, false
}

func resourceByName(resources []ResourceEntry, name string) (ResourceEntry, bool) {
	for _, resource := range resources {
		if resource.ResourceName == name {
			return resource, true
		}
	}
	return ResourceEntry{}, false
}

func MergeOrgConfig(local, org ProjectConfigV2) ConfigMergeResult {
	merged := ProjectConfigV2{SchemaVersion: 2, ScannerHint: local.ScannerHint}
	entries := make([]ConfigMergeEntry, 0)

	mergedProviders := make([]ProviderEntry, 0, len(org.Providers)+len(local.Providers))
	for _, orgProvider := range org.Providers {
		mergedProviders = append(mergedProviders, orgProvider)
		localProvider, exists := providerByID(local.Providers, orgProvider.ID)
		action := MergeActionApply
		detail := ""
		if exists {
			if localProvider == orgProvider {
				action = MergeActionUnchanged
			} else {
				action = MergeActionOverwrite
				detail = providerDiffDetail(localProvider, orgProvider)
			}
		}
		entries = append(entries, ConfigMergeEntry{
			Kind:       ConfigEntryProvider,
			Field:      orgProvider.ID,
			Provenance: ProvenanceOrg,
			Action:     action,
			Detail:     detail,
		})
	}
	for _, localProvider := range local.Providers {
		if _, exists := providerByID(org.Providers, localProvider.ID); exists {
			continue
		}
		mergedProviders = append(mergedProviders, localProvider)
		entries = append(entries, ConfigMergeEntry{
			Kind:       ConfigEntryProvider,
			Field:      localProvider.ID,
			Provenance: ProvenanceLocal,
			Action:     MergeActionKeep,
		})
	}
	merged.Providers = mergedProviders

	mergedResources := make([]ResourceEntry, 0, len(org.Resources)+len(local.Resources))
	for _, orgResource := range org.Resources {
		mergedResources = append(mergedResources, orgResource)
		localResource, exists := resourceByName(local.Resources, orgResource.ResourceName)
		action := MergeActionApply
		detail := pinLabel(orgResource.PinnedVersion)
		if exists {
			if localResource == orgResource {
				action = MergeActionUnchanged
			} else {
				action = MergeActionOverwrite
				detail = fmt.Sprintf("%s → %s", pinLabel(localResource.PinnedVersion), pinLabel(orgResource.PinnedVersion))
			}
		}
		entries = append(entries, ConfigMergeEntry{
			Kind:       ConfigEntryResource,
			Field:      orgResource.ResourceName,
			Provenance: ProvenanceOrg,
			Action:     action,
			Detail:     detail,
		})
	}
	for _, localResource := range local.Resources {
		if _, exists := resourceByName(org.Resources, localResource.ResourceName); exists {
			continue
		}
		mergedResources = append(mergedResources, localResource)
		entries = append(entries, ConfigMergeEntry{
			Kind:       ConfigEntryResource,
			Field:      localResource.ResourceName,
			Provenance: ProvenanceLocal,
			Action:     MergeActionKeep,
			Detail:     pinLabel(localResource.PinnedVersion),
		})
	}
	merged.Resources = mergedResources

	if len(org.Targets) > 0 {
		merged.Targets = org.Targets
		action := MergeActionApply
		if len(local.Targets) > 0 {
			action = MergeActionOverwrite
		}
		entries = append(entries, ConfigMergeEntry{
			Kind:       ConfigEntryTarget,
			Field:      "targets",
			Provenance: ProvenanceOrg,
			Action:     action,
		})
	} else {
		merged.Targets = local.Targets
	}

	return ConfigMergeResult{Merged: merged, Entries: entries}
}

func ShareableSubset(local ProjectConfigV2) ProjectConfigV2 {
	subset := ProjectConfigV2{
		SchemaVersion: 2,
		Providers:     make([]ProviderEntry, len(local.Providers)),
		Resources:     make([]ResourceEntry, len(local.Resources)),
	}
	copy(subset.Providers, local.Providers)
	copy(subset.Resources, local.Resources)
	if len(local.Targets) > 0 {
		subset.Targets = make([]TargetEntry, len(local.Targets))
		copy(subset.Targets, local.Targets)
	}
	return subset
}

func providerDiffDetail(localProvider, orgProvider ProviderEntry) string {
	if localProvider.CatalogRef != orgProvider.CatalogRef {
		return fmt.Sprintf("catalog_ref %s → %s", pinLabel(localProvider.CatalogRef), pinLabel(orgProvider.CatalogRef))
	}
	if localProvider.RepositoryID != orgProvider.RepositoryID {
		return fmt.Sprintf("repository %s → %s", pinLabel(localProvider.RepositoryID), pinLabel(orgProvider.RepositoryID))
	}
	if localProvider.CatalogPath != orgProvider.CatalogPath {
		return fmt.Sprintf("catalog_path %s → %s", pinLabel(localProvider.CatalogPath), pinLabel(orgProvider.CatalogPath))
	}
	return "updated"
}

func pinLabel(value string) string {
	if value == "" {
		return "(none)"
	}
	return value
}
