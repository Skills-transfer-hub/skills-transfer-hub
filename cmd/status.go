package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"loreskills/internal/fs"
)

type statusJSON struct {
	ResourceName     string   `json:"resource_name"`
	ProviderID       string   `json:"provider_id"`
	InstalledVersion string   `json:"installed_version"`
	LatestVersion    string   `json:"latest_version"`
	Status           string   `json:"status"`
	ModifiedFiles    []string `json:"modified_files,omitempty"`
	Pinned           bool     `json:"pinned"`
	PinnedVersion    string   `json:"pinned_version,omitempty"`
}

func (a *app) runStatus(args []string) error {
	flags, _ := parseFlags(args, "--json")
	jsonOut := flags["--json"]

	profile, err := fs.ReadProjectProfile()
	if err != nil {
		return err
	}

	state, err := fs.ReadInstalledStateV2(profile.TargetDir)
	if err != nil {
		return err
	}
	if len(state.Entries) == 0 {
		a.writeStdout("No resources installed.")
		return nil
	}

	cfg, _ := fs.ReadProjectConfigV2()

	var results []statusJSON
	for _, rec := range state.Entries {
		entry := fs.CatalogEntry{
			CatalogID:    rec.ResourceName,
			ResourceType: "skill",
			Name:         rec.ResourceName,
			FolderName:   rec.ResourceName,
			SourcePath:   rec.ResourceName,
		}
		content, fetchErr := a.client.DownloadCatalogEntry(context.Background(), *profile, entry)
		latestVersion := ""
		if fetchErr == nil {
			latestVersion = fs.ComputeSHA256(content)
		}

		status, modFiles := fs.ComputeResourceStatus(rec, latestVersion)

		pinnedVer := fs.PinnedVersion(cfg, rec.ResourceName)
		pinned := pinnedVer != ""

		if jsonOut {
			results = append(results, statusJSON{
				ResourceName:     rec.ResourceName,
				ProviderID:       rec.ProviderID,
				InstalledVersion: rec.ResolvedVersion,
				LatestVersion:    latestVersion,
				Status:           string(status),
				ModifiedFiles:    modFiles,
				Pinned:           pinned,
				PinnedVersion:    pinnedVer,
			})
			continue
		}

		line := fmt.Sprintf("%-30s  %s", rec.ResourceName, status)
		if status == fs.ResourceStatusOutdated {
			line += fmt.Sprintf("  installed: %.8s  available: %.8s", rec.ResolvedVersion, latestVersion)
		}
		if pinned {
			line += fmt.Sprintf("  (pinned at %s)", pinnedVer)
		}
		a.writeStdout(line)
		if status == fs.ResourceStatusModifiedLocally {
			for _, f := range modFiles {
				a.writeStdout(fmt.Sprintf("    modified: %s", f))
			}
		}
		if status == fs.ResourceStatusUnknownBaseline {
			a.writeStdout(fmt.Sprintf("  → re-install with `loreskills install --force %s` to establish integrity tracking", rec.ResourceName))
		}
	}

	if jsonOut {
		data, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			return err
		}
		a.writeStdout(string(data))
	}
	return nil
}
