package cmd

import (
	"context"
	"fmt"
	"sth/internal/fs"
	sthhttp "sth/internal/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (a *app) runUpdate(args []string) error {
	flags, _ := parseFlags(args, "--dry-run", "--json")
	dryRun := flags["--dry-run"]
	jsonOut := flags["--json"]

	profile, release, err := a.prepareStateAccess()
	if err != nil {
		return err
	}
	defer release()

	cfg, err := fs.ReadProjectConfigV2()
	if err != nil {
		return err
	}

	if dryRun {
		v2State, err := fs.ReadInstalledStateV2(profile.TargetDir)
		if err != nil {
			return err
		}
		if len(v2State.Entries) == 0 {
			a.writeStdout("Aucune ressource à mettre à jour.")
			return nil
		}
		workingDir, _ := filepath.Abs(".")
		ignorePats, _ := fs.ReadSTHIgnore(workingDir)
		var allChanges []fs.DryRunChange
		anyChanges := false
		for _, rec := range v2State.Entries {
			if pinnedVer := fs.PinnedVersion(cfg, rec.ResourceName); pinnedVer != "" {
				if !jsonOut {
					a.writeStdout(fmt.Sprintf("  [pinned at %s] %s", pinnedVer, rec.ResourceName))
				}
				continue
			}
			entry := fs.CatalogEntry{
				CatalogID:    rec.ResourceName,
				ResourceType: "skill",
				Name:         rec.ResourceName,
				FolderName:   rec.ResourceName,
				SourcePath:   sourcePath(rec, profile.TargetDir),
			}
			content, fetchErr := a.client.DownloadCatalogEntry(context.Background(), *profile, entry)
			if fetchErr != nil {
				a.writeStderr(fmt.Sprintf("Échec de récupération pour '%s' : %v", rec.ResourceName, fetchErr))
				continue
			}
			// Deep-copy ManagedFiles before RefreshManagedFileHashes mutates the slice.
			savedFiles := make([]fs.ManagedFile, len(rec.ManagedFiles))
			copy(savedFiles, rec.ManagedFiles)
			savedRec := rec
			savedRec.ManagedFiles = savedFiles
			incomingRecord := fs.RefreshManagedFileHashes(rec, content)
			changes := fs.ComputeDryRunChanges(&savedRec, incomingRecord.ManagedFiles, ignorePats)
			if jsonOut {
				for i := range changes {
					changes[i].ResourceName = rec.ResourceName
				}
				allChanges = append(allChanges, changes...)
				if len(changes) > 0 {
					anyChanges = true
				}
			} else {
				a.writeStdout(fmt.Sprintf("--- %s ---", rec.ResourceName))
				a.writeStdout(fs.FormatDryRunText(changes))
				if len(changes) > 0 {
					anyChanges = true
				}
			}
		}
		if jsonOut {
			if !anyChanges && allChanges == nil {
				allChanges = []fs.DryRunChange{}
			}
			data, marshalErr := fs.FormatDryRunJSON(allChanges)
			if marshalErr != nil {
				return marshalErr
			}
			a.writeStdout(string(data))
		}
		return nil
	}

	records, err := fs.ListInstalledResources(profile.TargetDir)
	if err != nil {
		return err
	}

	if len(records) == 0 {
		a.writeStdout("Aucune ressource à mettre à jour.")
		return nil
	}

	failures := 0
	v2State, v2StateErr := fs.ReadInstalledStateV2(profile.TargetDir)
	v2Updated := false

	workingDir, _ := filepath.Abs(".")
	ignorePats, _ := fs.ReadSTHIgnore(workingDir)

	// T018/T025: pre-fetch latest commit SHA once per unique providerID.
	latestSHAs := make(map[string]string)
	if v2StateErr == nil {
		seen := make(map[string]bool)
		for _, v2rec := range v2State.Entries {
			if seen[v2rec.ProviderID] {
				continue
			}
			seen[v2rec.ProviderID] = true
			if sha, shaErr := a.client.GetLatestCommitSHA(context.Background(), cfg, v2rec.ProviderID); shaErr == nil {
				latestSHAs[v2rec.ProviderID] = sha
			}
		}
	}

	for _, record := range records {
		if pinnedVer := fs.PinnedVersion(cfg, record.CatalogID); pinnedVer != "" {
			a.writeStdout(fmt.Sprintf("  [pinned at %s] %s", pinnedVer, record.CatalogID))
			// T018: Notify if a newer upstream version exists for the pinned resource.
			if v2StateErr == nil {
				for _, v2rec := range v2State.Entries {
					if v2rec.ResourceName == record.CatalogID {
						if latestSHA := latestSHAs[v2rec.ProviderID]; latestSHA != "" && latestSHA != v2rec.ResolvedVersion {
							a.writeStdout(fmt.Sprintf("Newer version available for %s (pinned at %s)", record.CatalogID, pinnedVer))
						}
						break
					}
				}
			}
			continue
		}

		entry := fs.CatalogEntry{
			CatalogID:    record.CatalogID,
			ResourceType: record.ResourceType,
			Name:         record.Name,
			FolderName:   record.FolderName,
			SourcePath:   record.SourcePath,
		}

		if len(record.ManagedFiles) > 0 {
			bundleContents, err := a.downloadBundleContents(context.Background(), *profile, entry)
			if err != nil {
				a.writeStderr(fmt.Sprintf("Échec de mise à jour pour '%s' : %v", record.Name, err))
				failures++
				continue
			}

			if _, err := fs.SyncManagedBundle(profile.TargetDir, entry, bundleContents); err != nil {
				a.writeStderr(fmt.Sprintf("Échec de mise à jour pour '%s' : %v", record.Name, err))
				failures++
				continue
			}

			a.writeStdout(fmt.Sprintf("Ressource '%s' mise à jour.", record.Name))
			continue
		}

		content, err := a.client.DownloadCatalogEntry(context.Background(), *profile, entry)
		if err != nil {
			a.writeStderr(fmt.Sprintf("Échec de mise à jour pour '%s' : %v", record.Name, err))
			failures++
			continue
		}

		// T021: Skip if installed path is excluded by .sthignore (SC-003).
		if fs.MatchesIgnorePattern(record.SourcePath, ignorePats) {
			a.writeStdout(fmt.Sprintf("Ressource '%s' exclue par .sthignore.", record.Name))
			continue
		}

		// T024: Skip resource if its installed path is owned by another resource.
		if v2StateErr == nil {
			if conflict := fs.CheckOwnershipConflict(&v2State, record.CatalogID, record.InstalledPath); conflict != nil {
				a.writeStderr(fmt.Sprintf("Conflit de propriété pour '%s' : %v", record.Name, conflict))
				failures++
				continue
			}
		}

		if err := fs.SaveInstalledResource(record, content); err != nil {
			a.writeStderr(fmt.Sprintf("Échec de mise à jour pour '%s' : %v", record.Name, err))
			failures++
			continue
		}

		if err := fs.UpsertInstalledResource(profile.TargetDir, record); err != nil {
			a.writeStderr(fmt.Sprintf("Échec de mise à jour pour '%s' : %v", record.Name, err))
			failures++
			continue
		}

		if v2StateErr == nil {
			for i, v2rec := range v2State.Entries {
				if v2rec.ResourceName == record.CatalogID {
					v2State.Entries[i] = fs.RefreshManagedFileHashes(v2rec, content)
					v2State.Entries[i].PreviousVersion = v2rec.ResolvedVersion
					// T025: prefer commit SHA; fall back to content SHA256.
					if sha := latestSHAs[v2rec.ProviderID]; sha != "" {
						v2State.Entries[i].ResolvedVersion = sha
					} else {
						v2State.Entries[i].ResolvedVersion = fs.ComputeSHA256(content)
					}
					v2State.Entries[i].InstalledAt = time.Now().UTC().Format(time.RFC3339)
					v2Updated = true
					break
				}
			}
		}

		a.writeStdout(fmt.Sprintf("Ressource '%s' mise à jour.", record.Name))
	}

	if v2Updated {
		if err := fs.WriteInstalledStateV2(profile.TargetDir, v2State); err != nil {
			return fmt.Errorf("impossible de mettre à jour l'état v2 : %w", err)
		}
	}

	if failures > 0 {
		return fmt.Errorf("mise à jour incomplète : %d ressource(s) en échec", failures)
	}

	return nil
}

// sourcePath derives the catalog source path for a v2 record during dry-run.
// When the record's ResourceName uses the catalog ID format (e.g., "skill::skills::alpha"),
// the first managed file's path is resolved relative to targetDir to reconstruct
// the original source path (e.g., "skills/alpha.md").
// Falls back to ResourceName when no managed files are present or path resolution
// fails; in both cases the DownloadCatalogEntry call will use the raw ResourceName,
// which is acceptable for legacy records that already store a valid source path.
func sourcePath(rec fs.InstalledResourceRecordV2, targetDir string) string {
	if len(rec.ManagedFiles) > 0 {
		rel, err := filepath.Rel(targetDir, rec.ManagedFiles[0].Path)
		if err == nil {
			return filepath.ToSlash(rel)
		}
	}
	// Fallback: ResourceName may itself be a valid source path (legacy records).
	return rec.ResourceName
}

func (a *app) runSelfUpdate() error {
	// Bail before doing any work if the binary is owned by a package manager —
	// `update self` would otherwise overwrite the file in place and corrupt
	// the manager's tracking (Cellar/<v>, scoop/apps/<v>). The user's right
	// move is `brew upgrade` / `scoop update`.
	execPath, _ := currentExecutablePath()
	if pm := detectPackageManager(execPath); pm.name != "" {
		return fmt.Errorf(
			"sth a été installé via %s. Pour mettre à jour, lance `%s` au lieu de `sth update self`",
			pm.name, pm.upgradeHint,
		)
	}

	release, _, err := a.resolveStableUpdate(context.Background(), true)
	if err != nil {
		return err
	}
	if release == nil {
		a.writeStdout("Aucune mise à jour stable disponible.")
		return nil
	}

	assetData, err := a.client.DownloadURL(context.Background(), release.DownloadURL, nil)
	if err != nil {
		return err
	}
	checksumData, err := a.client.DownloadURL(context.Background(), release.ChecksumURL, nil)
	if err != nil {
		return err
	}

	executablePath, err := currentExecutablePath()
	if err != nil {
		return err
	}
	if err := fs.ReplaceExecutableWithAsset(executablePath, release.AssetName, assetData, checksumData); err != nil {
		return fmt.Errorf("mise à jour stable %s détectée, mais le remplacement automatique a échoué : %w", release.Version, err)
	}

	a.writeStdout(
		fmt.Sprintf("Version stable détectée : %s", release.Version),
		"Mise à jour appliquée.",
	)
	_, _ = fs.WriteUpdateCheckState(fs.NewNoUpdateCheckState(time.Now().UTC(), release.Version))
	return nil
}

func (a *app) startupStableUpdateOffer() *fs.StableUpdateOffer {
	_, offer, _ := a.resolveStableUpdate(context.Background(), false)
	return offer
}

func (a *app) resolveStableUpdate(ctx context.Context, force bool) (*sthhttp.ReleaseAsset, *fs.StableUpdateOffer, error) {
	metadata := currentReleaseMetadata()
	if strings.TrimSpace(metadata.ReleasesAPIURL) == "" {
		return nil, nil, nil
	}

	now := time.Now().UTC()
	if !force {
		cachedState, err := fs.ReadUpdateCheckState()
		if err == nil && cachedState != nil && cachedState.IsFresh(now, metadata.CurrentVersion) {
			offer := fs.StableUpdateOfferFromState(*cachedState)
			return nil, &offer, nil
		}
	}

	release, err := a.client.CheckForStableUpdate(ctx, metadata)
	if err != nil {
		state := fs.NewErrorUpdateCheckState(now, metadata.CurrentVersion, err.Error())
		_, _ = fs.WriteUpdateCheckState(state)
		offer := fs.StableUpdateOfferFromState(state)
		return nil, &offer, err
	}

	if release == nil {
		state := fs.NewNoUpdateCheckState(now, metadata.CurrentVersion)
		_, _ = fs.WriteUpdateCheckState(state)
		offer := fs.StableUpdateOfferFromState(state)
		return nil, &offer, nil
	}

	state := fs.NewAvailableUpdateCheckState(now, metadata.CurrentVersion, release.Version, release.DownloadURL, release.ChecksumURL, release.AssetName)
	_, _ = fs.WriteUpdateCheckState(state)
	offer := fs.StableUpdateOfferFromState(state)

	return release, &offer, nil
}

func currentExecutablePath() (string, error) {
	if override := strings.TrimSpace(os.Getenv("STH_EXECUTABLE_PATH")); override != "" {
		return override, nil
	}

	path, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("impossible de déterminer le binaire courant : %w", err)
	}

	return path, nil
}
