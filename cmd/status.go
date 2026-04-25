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

	profile, release, err := a.prepareStateAccess()
	if err != nil {
		return err
	}
	defer release()

	state, err := fs.ReadInstalledStateV2(profile.TargetDir)
	if err != nil {
		return err
	}
	if len(state.Entries) == 0 {
		a.writeStdout("No resources installed.")
		return nil
	}

	cfg, err := fs.ReadProjectConfigV2()
	if err != nil {
		return err
	}

	// Build per-provider catalog maps (one ListCatalog request per unique provider).
	catalogs, catalogErrs := buildProviderCatalogs(context.Background(), a, state.Entries, cfg, *profile)
	for provID, err := range catalogErrs {
		a.writeStderr(fmt.Sprintf("avertissement : impossible de récupérer le catalogue du fournisseur %q : %v", provID, err))
	}

	var results []statusJSON
	for _, rec := range state.Entries {
		latestVersion := ""
		if catalog, ok := catalogs[rec.ProviderID]; ok {
			if entry, ok := catalog[rec.ResourceName]; ok {
				provProf := providerProfile(rec.ProviderID, cfg, *profile)
				content, fetchErr := a.client.DownloadCatalogEntry(context.Background(), provProf, entry)
				if fetchErr == nil {
					latestVersion = fs.ComputeSHA256(content)
				}
			}
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

// buildProviderCatalogs fetches the catalog once per unique provider in entries.
// Returns providerID → (catalogID → CatalogEntry) and providerID → fetch error.
// Failed fetches are recorded in the error map and produce an empty catalog entry.
func buildProviderCatalogs(ctx context.Context, a *app, entries []fs.InstalledResourceRecordV2, cfg fs.ProjectConfigV2, fallback fs.ProjectProfile) (map[string]map[string]fs.CatalogEntry, map[string]error) {
	result := map[string]map[string]fs.CatalogEntry{}
	errs := map[string]error{}
	for _, rec := range entries {
		if _, seen := result[rec.ProviderID]; seen {
			continue
		}
		provProf := providerProfile(rec.ProviderID, cfg, fallback)
		catalogEntries, err := a.client.ListCatalog(ctx, provProf)
		if err != nil {
			errs[rec.ProviderID] = err
			result[rec.ProviderID] = map[string]fs.CatalogEntry{}
			continue
		}
		m := make(map[string]fs.CatalogEntry, len(catalogEntries))
		for _, e := range catalogEntries {
			m[e.CatalogID] = e
		}
		result[rec.ProviderID] = m
	}
	return result, errs
}

// providerProfile returns the ProjectProfile for the given providerID from cfg,
// falling back to fallback if the provider is not found.
func providerProfile(providerID string, cfg fs.ProjectConfigV2, fallback fs.ProjectProfile) fs.ProjectProfile {
	for _, p := range cfg.Providers {
		if p.ID == providerID {
			return fs.ProjectProfileFromProvider(p, fallback.TargetDir)
		}
	}
	return fallback
}
