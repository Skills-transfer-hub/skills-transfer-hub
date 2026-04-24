package cmd

import (
	"context"
	"fmt"
	"loreskills/internal/fs"
	lorehttp "loreskills/internal/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (a *app) runUpdate(args []string) error {
	flags, _ := parseFlags(args, "--dry-run")
	dryRun := flags["--dry-run"]

	profile, err := fs.ReadProjectProfile()
	if err != nil {
		return err
	}

	cfg, _ := fs.ReadProjectConfigV2()

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
		ignorePats, _ := fs.ReadLoreSkillsIgnore(workingDir)
		for _, rec := range v2State.Entries {
			if pinnedVer := fs.PinnedVersion(cfg, rec.ResourceName); pinnedVer != "" {
				a.writeStdout(fmt.Sprintf("  [pinned at %s] %s", pinnedVer, rec.ResourceName))
				continue
			}
			entry := fs.CatalogEntry{
				CatalogID:    rec.ResourceName,
				ResourceType: "skill",
				Name:         rec.ResourceName,
				FolderName:   rec.ResourceName,
				SourcePath:   rec.ResourceName,
			}
			content, fetchErr := a.client.DownloadCatalogEntry(context.Background(), *profile, entry)
			if fetchErr != nil {
				a.writeStderr(fmt.Sprintf("Échec de récupération pour '%s' : %v", rec.ResourceName, fetchErr))
				continue
			}
			incomingRecord := fs.RefreshManagedFileHashes(fs.InstalledResourceRecordV2{
				ResourceName: rec.ResourceName,
				ManagedFiles: []fs.ManagedFile{{Path: fs.ResolveManagedFilePath(profile.TargetDir, entry)}},
			}, content)
			changes := fs.ComputeDryRunChanges(&rec, incomingRecord.ManagedFiles, ignorePats)
			a.writeStdout(fmt.Sprintf("--- %s ---", rec.ResourceName))
			a.writeStdout(fs.FormatDryRunText(changes))
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
	for _, record := range records {
		if pinnedVer := fs.PinnedVersion(cfg, record.CatalogID); pinnedVer != "" {
			a.writeStdout(fmt.Sprintf("  [pinned at %s] %s", pinnedVer, record.CatalogID))
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

		if v2State, err2 := fs.ReadInstalledStateV2(profile.TargetDir); err2 == nil {
			for _, v2rec := range v2State.Entries {
				if v2rec.ResourceName == record.CatalogID {
					v2rec = fs.RefreshManagedFileHashes(v2rec, content)
					v2rec.PreviousVersion = v2rec.ResolvedVersion
					v2rec.ResolvedVersion = fs.ComputeSHA256(content)
					v2rec.InstalledAt = time.Now().UTC().Format(time.RFC3339)
					_ = fs.UpsertInstalledResourceV2(profile.TargetDir, v2rec)
					break
				}
			}
		}

		a.writeStdout(fmt.Sprintf("Ressource '%s' mise à jour.", record.Name))
	}

	if failures > 0 {
		return fmt.Errorf("mise à jour incomplète : %d ressource(s) en échec", failures)
	}

	return nil
}

func (a *app) runSelfUpdate() error {
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

func (a *app) resolveStableUpdate(ctx context.Context, force bool) (*lorehttp.ReleaseAsset, *fs.StableUpdateOffer, error) {
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
	if override := strings.TrimSpace(os.Getenv("LORESKILLS_EXECUTABLE_PATH")); override != "" {
		return override, nil
	}

	path, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("impossible de déterminer le binaire courant : %w", err)
	}

	return path, nil
}
