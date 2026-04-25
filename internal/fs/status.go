package fs

import "os"

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
