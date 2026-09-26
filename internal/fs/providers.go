package fs

import (
	"fmt"
	"sort"
	"strings"
)

const CloudProviderName = "sth-cloud"

const FallbackProviderID = "default"

func IsCloudProviderName(provider string) bool {
	return strings.EqualFold(strings.TrimSpace(provider), CloudProviderName)
}

func ProviderByID(cfg ProjectConfigV2, providerID string) (ProviderEntry, bool) {
	trimmed := strings.TrimSpace(providerID)
	if trimmed == "" {
		return ProviderEntry{}, false
	}
	for _, provider := range cfg.Providers {
		if strings.EqualFold(strings.TrimSpace(provider.ID), trimmed) {
			return provider, true
		}
	}
	return ProviderEntry{}, false
}

func fallbackTargetDirFor(cfg ProjectConfigV2) string {
	for _, provider := range cfg.Providers {
		if strings.TrimSpace(provider.TargetDir) != "" {
			return provider.TargetDir
		}
	}
	return defaultTargetDir
}

func ProviderIsUsable(cfg ProjectConfigV2, provider ProviderEntry) bool {
	return validateProjectProfile(ProjectProfileFromProvider(provider, fallbackTargetDirFor(cfg))) == nil
}

func UsableProviders(cfg ProjectConfigV2) []ProviderEntry {
	usable := make([]ProviderEntry, 0, len(cfg.Providers))
	for _, provider := range cfg.Providers {
		if ProviderIsUsable(cfg, provider) {
			usable = append(usable, provider)
		}
	}
	return usable
}

func DefaultProviderID(cfg ProjectConfigV2) string {
	usable := UsableProviders(cfg)
	if len(usable) > 0 {
		return providerIdentifier(usable[0])
	}
	if len(cfg.Providers) > 0 {
		return providerIdentifier(cfg.Providers[0])
	}
	return FallbackProviderID
}

func providerIdentifier(provider ProviderEntry) string {
	trimmed := strings.TrimSpace(provider.ID)
	if trimmed == "" {
		return FallbackProviderID
	}
	return trimmed
}

func ProviderProfile(cfg ProjectConfigV2, providerID string) (ProjectProfile, error) {
	provider, found := ProviderByID(cfg, providerID)
	if !found {
		return ProjectProfile{}, fmt.Errorf("unknown provider: %q (configured: %s)", providerID, strings.Join(ProviderIDs(cfg), ", "))
	}
	profile := ProjectProfileFromProvider(provider, fallbackTargetDirFor(cfg))
	if err := validateProjectProfile(profile); err != nil {
		return ProjectProfile{}, fmt.Errorf("provider %q is misconfigured: %w", providerIdentifier(provider), err)
	}
	return *NormalizeProjectProfile(profile), nil
}

func ProviderIDs(cfg ProjectConfigV2) []string {
	ids := make([]string, 0, len(cfg.Providers))
	for _, provider := range cfg.Providers {
		ids = append(ids, providerIdentifier(provider))
	}
	return ids
}

func ProviderStateTargetDir(cfg ProjectConfigV2, providerID string) string {
	if provider, found := ProviderByID(cfg, providerID); found && strings.TrimSpace(provider.TargetDir) != "" {
		return provider.TargetDir
	}
	return fallbackTargetDirFor(cfg)
}

func StateTargetDirs(cfg ProjectConfigV2) []string {
	dirs := make([]string, 0, len(cfg.Providers))
	seen := make(map[string]struct{}, len(cfg.Providers))
	for _, provider := range cfg.Providers {
		targetDir := strings.TrimSpace(provider.TargetDir)
		if targetDir == "" {
			targetDir = fallbackTargetDirFor(cfg)
		}
		if _, exists := seen[targetDir]; exists {
			continue
		}
		seen[targetDir] = struct{}{}
		dirs = append(dirs, targetDir)
	}
	if len(dirs) == 0 {
		dirs = append(dirs, fallbackTargetDirFor(cfg))
	}
	return dirs
}

func RecordProviderID(cfg ProjectConfigV2, record InstalledResourceRecordV2) string {
	trimmed := strings.TrimSpace(record.ProviderID)
	if trimmed != "" {
		return trimmed
	}
	return DefaultProviderID(cfg)
}

func LatestVersionKey(providerID, queryPath string) string {
	return strings.TrimSpace(providerID) + "\x00" + queryPath
}

type CatalogRef struct {
	ProviderID string
	FolderName string
	Name       string
}

func ParseCatalogRef(resourceRef string) (CatalogRef, error) {
	trimmed := strings.TrimSpace(resourceRef)
	if trimmed == "" {
		return CatalogRef{}, fmt.Errorf("the resource to install is required")
	}

	ref := CatalogRef{}
	if providerPart, rest, found := strings.Cut(trimmed, ":"); found {
		if strings.TrimSpace(providerPart) == "" || strings.TrimSpace(rest) == "" {
			return CatalogRef{}, fmt.Errorf("invalid resource reference: %q", resourceRef)
		}
		ref.ProviderID = strings.TrimSpace(providerPart)
		trimmed = strings.TrimSpace(rest)
	}

	if strings.ContainsAny(trimmed, "/\\") {
		normalized := strings.ReplaceAll(trimmed, "\\", "/")
		parts := strings.Split(normalized, "/")
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return CatalogRef{}, fmt.Errorf("invalid resource reference: %q", resourceRef)
		}
		ref.FolderName = strings.TrimSpace(parts[0])
		ref.Name = strings.TrimSpace(parts[1])
		return ref, nil
	}

	ref.Name = trimmed
	return ref, nil
}

type AmbiguousCatalogRefError struct {
	ResourceRef string
	Candidates  []CatalogEntry
}

func (e *AmbiguousCatalogRefError) Error() string {
	labels := make([]string, 0, len(e.Candidates))
	for _, candidate := range e.Candidates {
		labels = append(labels, fmt.Sprintf("%s:%s/%s", CatalogEntryProviderID(candidate), candidate.FolderName, candidate.Name))
	}
	sort.Strings(labels)
	return fmt.Sprintf("ambiguous resource: %q is available from several sources (%s); disambiguate with --provider or provider:folder/name", e.ResourceRef, strings.Join(labels, ", "))
}

func CatalogEntryProviderID(entry CatalogEntry) string {
	trimmed := strings.TrimSpace(entry.ProviderID)
	if trimmed == "" {
		return FallbackProviderID
	}
	return trimmed
}

func ResolveCatalogEntryForProvider(entries []CatalogEntry, resourceRef, preferredProviderID string) (CatalogEntry, error) {
	ref, err := ParseCatalogRef(resourceRef)
	if err != nil {
		return CatalogEntry{}, err
	}

	providerFilter := ref.ProviderID
	if providerFilter == "" {
		providerFilter = strings.TrimSpace(preferredProviderID)
	}

	candidates := make([]CatalogEntry, 0, len(entries))
	for _, entry := range entries {
		if providerFilter != "" && !strings.EqualFold(CatalogEntryProviderID(entry), providerFilter) {
			continue
		}
		if !strings.EqualFold(entry.Name, ref.Name) {
			continue
		}
		if ref.FolderName != "" && !strings.EqualFold(entry.FolderName, ref.FolderName) {
			continue
		}
		candidates = append(candidates, entry)
	}

	switch len(candidates) {
	case 0:
		if providerFilter != "" {
			return CatalogEntry{}, fmt.Errorf("resource not found in provider %q: %q", providerFilter, resourceRef)
		}
		return CatalogEntry{}, fmt.Errorf("resource not found: %q", resourceRef)
	case 1:
		return candidates[0], nil
	}

	if singleProviderCandidates(candidates) {
		folders := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			folders = append(folders, candidate.FolderName)
		}
		sort.Strings(folders)
		return CatalogEntry{}, fmt.Errorf("ambiguous resource: %q (specify folder/name from %s)", resourceRef, strings.Join(folders, ", "))
	}

	return CatalogEntry{}, &AmbiguousCatalogRefError{ResourceRef: resourceRef, Candidates: candidates}
}

func singleProviderCandidates(candidates []CatalogEntry) bool {
	if len(candidates) == 0 {
		return true
	}
	first := CatalogEntryProviderID(candidates[0])
	for _, candidate := range candidates[1:] {
		if !strings.EqualFold(CatalogEntryProviderID(candidate), first) {
			return false
		}
	}
	return true
}

func ReadInstalledStateAcrossDirs(targetDirs []string) (InstalledResourceStateV2, error) {
	merged := InstalledResourceStateV2{SchemaVersion: 2, Entries: []InstalledResourceRecordV2{}}
	seen := make(map[string]struct{})
	for _, targetDir := range targetDirs {
		state, err := ReadInstalledStateV2(targetDir)
		if err != nil {
			return InstalledResourceStateV2{}, err
		}
		for _, record := range state.Entries {
			key := record.ProviderID + "\x00" + record.ResourceName + "\x00" + record.TargetID
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			merged.Entries = append(merged.Entries, record)
		}
	}
	return merged, nil
}
