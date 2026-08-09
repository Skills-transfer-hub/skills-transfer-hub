package fs

import (
	"os"
	"path"
	"path/filepath"
	"strings"
)

func CommitQueryPath(sourcePath, artifactKind string) string {
	normalized := path.Clean(strings.ReplaceAll(strings.TrimSpace(sourcePath), "\\", "/"))
	if normalized == "." || normalized == "/" || normalized == "" {
		return ""
	}
	if artifactKind == "skill" || strings.HasSuffix(strings.ToLower(normalized), "/skill.md") {
		return path.Dir(normalized)
	}
	return normalized
}

// StatusResult holds the computed lifecycle status of an installed resource.
type StatusResult struct {
	ResourceName     string         `json:"resource_name"`
	ProviderID       string         `json:"provider_id"`
	InstalledVersion string         `json:"installed_version"`
	LatestVersion    string         `json:"latest_version"`
	Status           ResourceStatus `json:"status"`
	ModifiedFiles    []string       `json:"modified_files,omitempty"`
	Pinned           bool           `json:"pinned"`
	PinnedVersion    string         `json:"pinned_version,omitempty"`
}

// StatusUpToDate is a convenience alias for ResourceStatusUpToDate.
const StatusUpToDate = ResourceStatusUpToDate

func ComputeAllStatuses(state InstalledResourceStateV2, config ProjectConfigV2, latestByPath map[string]string, projectRoot string) []StatusResult {
	results := make([]StatusResult, 0, len(state.Entries))
	for _, rec := range state.Entries {
		resolved := rec
		if projectRoot != "" {
			files := make([]ManagedFile, len(rec.ManagedFiles))
			for i, managedFile := range rec.ManagedFiles {
				if !filepath.IsAbs(managedFile.Path) {
					managedFile.Path = filepath.Join(projectRoot, managedFile.Path)
				}
				files[i] = managedFile
			}
			resolved.ManagedFiles = files
			regions := make([]ManagedRegion, len(rec.ManagedRegions))
			for i, managedRegion := range rec.ManagedRegions {
				if !filepath.IsAbs(managedRegion.Path) {
					managedRegion.Path = filepath.Join(projectRoot, managedRegion.Path)
				}
				regions[i] = managedRegion
			}
			resolved.ManagedRegions = regions
		}
		latestVersion := latestByPath[CommitQueryPath(rec.SourcePath, rec.ArtifactKind)]
		status, modFiles := ComputeResourceStatus(resolved, latestVersion)
		pinnedVer := PinnedVersion(config, rec.ResourceName)
		results = append(results, StatusResult{
			ResourceName:     rec.ResourceName,
			ProviderID:       rec.ProviderID,
			InstalledVersion: rec.ResolvedVersion,
			LatestVersion:    latestVersion,
			Status:           status,
			ModifiedFiles:    modFiles,
			Pinned:           pinnedVer != "",
			PinnedVersion:    pinnedVer,
		})
	}
	return results
}

func ComputeResourceStatus(record InstalledResourceRecordV2, latestVersion string) (ResourceStatus, []string) {
	if len(record.ManagedFiles) == 0 && len(record.ManagedRegions) == 0 {
		return ResourceStatusUnknownBaseline, nil
	}

	var modifiedPaths []string
	for _, managedFile := range record.ManagedFiles {
		data, err := os.ReadFile(managedFile.Path)
		if err != nil {
			modifiedPaths = append(modifiedPaths, managedFile.Path)
			continue
		}
		actual := ComputeSHA256(data)
		if actual != managedFile.SHA256 {
			modifiedPaths = append(modifiedPaths, managedFile.Path)
		}
	}
	for _, managedRegion := range record.ManagedRegions {
		document, err := os.ReadFile(managedRegion.Path)
		if err != nil {
			modifiedPaths = append(modifiedPaths, managedRegion.Path)
			continue
		}
		actual, regionErr := ManagedMarkdownRegionSHA256(document, managedRegion.ID)
		if regionErr != nil || actual != managedRegion.SHA256 {
			modifiedPaths = append(modifiedPaths, managedRegion.Path)
		}
	}

	if len(modifiedPaths) > 0 {
		return ResourceStatusModifiedLocally, modifiedPaths
	}

	if latestVersion != "" && record.ResolvedVersion != "" && record.ResolvedVersion != latestVersion {
		return ResourceStatusOutdated, nil
	}

	return ResourceStatusUpToDate, nil
}
