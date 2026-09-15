package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"sth/internal/fs"
)

func fanoutFilesUnchanged(state fs.InstalledResourceStateV2, resourceName, providerID, resolvedVersion, projectRoot string, incoming []fs.ManagedFile) bool {
	existing := map[string]string{}
	for _, record := range state.Entries {
		if record.ResourceName != resourceName {
			continue
		}
		if record.ProviderID != providerID || record.ResolvedVersion != resolvedVersion || len(record.ManagedRegions) > 0 {
			return false
		}
		for _, file := range record.ManagedFiles {
			existing[file.Path] = file.SHA256
		}
	}
	if len(existing) == 0 || len(existing) != len(incoming) {
		return false
	}
	for _, file := range incoming {
		if existing[file.Path] != file.SHA256 {
			return false
		}
		absolutePath := file.Path
		if !filepath.IsAbs(absolutePath) {
			absolutePath = filepath.Join(projectRoot, absolutePath)
		}
		content, err := os.ReadFile(absolutePath)
		if err != nil || fs.ComputeSHA256(content) != file.SHA256 {
			return false
		}
	}
	return true
}

func (a *app) reportUnchanged(resourceName, resolvedVersion string) {
	if a.fanoutJSON != nil {
		a.fanoutJSON.results = append(a.fanoutJSON.results, updateJSONResult{ResourceName: resourceName, Outcome: "unchanged", ResolvedVersion: resolvedVersion})
		return
	}
	a.writeStdout(fmt.Sprintf("Déjà à jour : %s", resourceName))
}

func installedTargetConfig(config fs.ProjectConfigV2, state fs.InstalledResourceStateV2, resourceName, providerID string) fs.ProjectConfigV2 {
	selected := map[string]bool{}
	for _, record := range state.Entries {
		if record.ResourceName != resourceName || fs.RecordProviderID(config, record) != providerID {
			continue
		}
		if record.TargetID != "" {
			selected[record.TargetID] = true
		}
		for _, consumer := range record.Consumers {
			selected[consumer] = true
		}
	}
	if len(selected) == 0 {
		return config
	}
	targets := make([]fs.TargetEntry, 0)
	for _, target := range fs.EffectiveTargets(config) {
		if selected[target.ID] || selected[target.Kind] {
			targets = append(targets, target)
		}
	}
	config.Targets = targets
	return config
}
