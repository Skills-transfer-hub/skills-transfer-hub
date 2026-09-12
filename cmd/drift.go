package cmd

import (
	"fmt"
	"path/filepath"

	"sth/internal/fs"
)

func refuseModifiedResources(state fs.InstalledResourceStateV2, config fs.ProjectConfigV2, resourceName string) error {
	projectRoot, err := filepath.Abs(".")
	if err != nil {
		return err
	}
	results := fs.ComputeAllStatuses(state, config, nil, projectRoot)
	for _, result := range results {
		if resourceName != "" && result.ResourceName != resourceName {
			continue
		}
		if result.Status == fs.ResourceStatusModifiedLocally {
			return fmt.Errorf("resource %q has local modifications in %v; restore or remove it before continuing", result.ResourceName, result.ModifiedFiles)
		}
	}
	return nil
}
