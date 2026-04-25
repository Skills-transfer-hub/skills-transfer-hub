package fs

import "os"

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

// ComputeAllStatuses returns a StatusResult for every entry in state.
// latestSHAs maps providerID → latest HEAD commit SHA (empty = unknown).
func ComputeAllStatuses(state InstalledResourceStateV2, config ProjectConfigV2, latestSHAs map[string]string) []StatusResult {
	results := make([]StatusResult, 0, len(state.Entries))
	for _, rec := range state.Entries {
		latestVersion := latestSHAs[rec.ProviderID]
		status, modFiles := ComputeResourceStatus(rec, latestVersion)
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

// ComputeResourceStatus determines the lifecycle status of an installed resource.
// latestVersion is the SHA256 of the latest upstream content (empty string = unknown).
// Returns the status and list of locally modified file paths (only for modified-locally).
func ComputeResourceStatus(record InstalledResourceRecordV2, latestVersion string) (ResourceStatus, []string) {
	if len(record.ManagedFiles) == 0 {
		return ResourceStatusUnknownBaseline, nil
	}

	var modifiedPaths []string
	for _, mf := range record.ManagedFiles {
		data, err := os.ReadFile(mf.Path)
		if err != nil {
			// File missing or unreadable — treat as modified.
			modifiedPaths = append(modifiedPaths, mf.Path)
			continue
		}
		actual := ComputeSHA256(data)
		if actual != mf.SHA256 {
			modifiedPaths = append(modifiedPaths, mf.Path)
		}
	}

	if len(modifiedPaths) > 0 {
		return ResourceStatusModifiedLocally, modifiedPaths
	}

	if latestVersion != "" && record.ResolvedVersion != latestVersion {
		return ResourceStatusOutdated, nil
	}

	return ResourceStatusUpToDate, nil
}
