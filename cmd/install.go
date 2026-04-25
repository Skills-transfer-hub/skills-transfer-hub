package cmd

import (
	"context"
	"fmt"
	"loreskills/internal/fs"
	"path/filepath"
)

func (a *app) runInstall(args []string) error {
	flags, positional := parseFlags(args, "--dry-run", "--json", "--force")
	dryRun := flags["--dry-run"]
	jsonOut := flags["--json"]
	force := flags["--force"]

	if len(positional) != 1 {
		return fmt.Errorf("usage: loreskills install <resource-ref> [--dry-run] [--force]")
	}
	resourceRef := positional[0]

	profile, release, err := a.prepareStateAccess()
	if err != nil {
		return err
	}
	defer release()

	providerID := "default"
	if cfg, cfgErr := fs.ReadProjectConfigV2(); cfgErr == nil && len(cfg.Providers) > 0 {
		providerID = cfg.Providers[0].ID
	}

	entries, err := a.client.ListCatalog(context.Background(), *profile)
	if err != nil {
		return err
	}

	entry, err := fs.ResolveCatalogEntry(entries, resourceRef)
	if err != nil {
		return err
	}

	if entry.ResourceType == "bundle" {
		bundleContents, bundleErr := a.downloadBundleContents(context.Background(), *profile, entry)
		if bundleErr != nil {
			return bundleErr
		}
		record, syncErr := fs.SyncManagedBundle(profile.TargetDir, entry, bundleContents)
		if syncErr != nil {
			return syncErr
		}
		a.writeStdout(
			fmt.Sprintf("Ressource '%s' installée dans %s.", entry.Name, record.InstalledPath),
			fmt.Sprintf("Type: %s", entry.ResourceType),
			fmt.Sprintf("Dossier: %s", entry.FolderName),
			"Prochaine étape : utilisez `loreskills update` pour rafraîchir les ressources suivies.",
		)
		return nil
	}

	content, err := a.client.DownloadCatalogEntry(context.Background(), *profile, entry)
	if err != nil {
		return err
	}

	workingDir, _ := filepath.Abs(".")
	ignorePats, _ := fs.ReadLoreSkillsIgnore(workingDir)

	// Build incoming managed files for dry-run or conflict check.
	incomingRecord := fs.RefreshManagedFileHashes(fs.InstalledResourceRecordV2{
		ResourceName: entry.CatalogID,
		ManagedFiles: []fs.ManagedFile{{Path: fs.ResolveManagedFilePath(profile.TargetDir, entry)}},
	}, content)

	if dryRun {
		state, _ := fs.ReadInstalledStateV2(profile.TargetDir)
		var existing *fs.InstalledResourceRecordV2
		for _, rec := range state.Entries {
			rec := rec
			if rec.ResourceName == entry.CatalogID {
				existing = &rec
				break
			}
		}
		changes := fs.ComputeDryRunChanges(existing, incomingRecord.ManagedFiles, ignorePats)
		if jsonOut {
			data, err := fs.FormatDryRunJSON(changes)
			if err != nil {
				return err
			}
			a.writeStdout(string(data))
		} else {
			a.writeStdout(fs.FormatDryRunText(changes))
		}
		return nil
	}

	// Ownership conflict check (T023: --force transfers ownership).
	state, _ := fs.ReadInstalledStateV2(profile.TargetDir)
	if !force {
		for _, mf := range incomingRecord.ManagedFiles {
			if conflict := fs.CheckOwnershipConflict(&state, entry.CatalogID, mf.Path); conflict != nil {
				return conflict
			}
		}
	} else {
		transferred := false
		for _, mf := range incomingRecord.ManagedFiles {
			if conflict := fs.CheckOwnershipConflict(&state, entry.CatalogID, mf.Path); conflict != nil {
				a.writeStdout(fmt.Sprintf("warning: ownership of %s transferred from %s to %s",
					mf.Path, conflict.ExistingOwner, entry.CatalogID))
				fs.TransferOwnership(&state, entry.CatalogID, mf.Path)
				transferred = true
			}
		}
		if transferred {
			if writeErr := fs.WriteInstalledStateV2(profile.TargetDir, state); writeErr != nil {
				a.writeStderr(fmt.Sprintf("warning: failed to persist ownership transfer: %v", writeErr))
			}
		}
	}

	// T020: Skip file write when excluded by .loreskillsignore (SC-003).
	relPath := filepath.ToSlash(entry.SourcePath)
	if len(ignorePats) > 0 && fs.MatchIgnore(ignorePats, relPath) {
		a.writeStderr("warning: no files installed — all excluded by .loreskillsignore")
		v2rec := fs.InstalledResourceRecordV2{
			ResourceName: entry.CatalogID,
			ProviderID:   providerID,
			ManagedFiles: []fs.ManagedFile{},
		}
		if upsertErr := fs.UpsertInstalledResourceV2(profile.TargetDir, v2rec); upsertErr != nil {
			return upsertErr
		}
		// Also register in v1 state so update/remove can discover the resource.
		v1rec := fs.InstalledResourceRecord{
			CatalogID:     entry.CatalogID,
			ResourceType:  entry.ResourceType,
			Name:          entry.Name,
			FolderName:    entry.FolderName,
			InstalledPath: fs.ResolveManagedFilePath(profile.TargetDir, entry),
			SourcePath:    entry.SourcePath,
			Status:        "installed",
		}
		if upsertErr := fs.UpsertInstalledResource(profile.TargetDir, v1rec); upsertErr != nil {
			return upsertErr
		}
		return nil
	}

	record, err := fs.InstallCatalogEntry(profile.TargetDir, entry, content)
	if err != nil {
		return err
	}
	v2rec, err := fs.InstallCatalogEntryV2(profile.TargetDir, entry, content, providerID)
	if err != nil {
		return err
	}
	// T025: update resolved_version to commit SHA when available.
	if v2cfg, v2err := fs.ReadProjectConfigV2(); v2err == nil {
		if sha, shaErr := a.client.GetLatestCommitSHA(context.Background(), v2cfg, providerID); shaErr == nil && sha != "" {
			v2rec.ResolvedVersion = sha
			if upsertErr := fs.UpsertInstalledResourceV2(profile.TargetDir, v2rec); upsertErr != nil {
				a.writeStderr(fmt.Sprintf("warning: failed to persist resolved version: %v", upsertErr))
			}
		}
	}

	a.writeStdout(
		fmt.Sprintf("Ressource '%s' installée dans %s.", entry.Name, record.InstalledPath),
		fmt.Sprintf("Type: %s", entry.ResourceType),
		fmt.Sprintf("Dossier: %s", entry.FolderName),
		"Prochaine étape : utilisez `loreskills update` pour rafraîchir les ressources suivies.",
	)
	return nil
}

func (a *app) downloadBundleContents(ctx context.Context, profile fs.ProjectProfile, entry fs.CatalogEntry) (map[string][]byte, error) {
	bundlePaths, err := a.client.ListBundleFiles(ctx, profile, entry)
	if err != nil {
		return nil, err
	}

	bundleContents := make(map[string][]byte, len(bundlePaths))
	for _, sourcePath := range bundlePaths {
		fileEntry := entry
		fileEntry.SourcePath = sourcePath

		content, err := a.client.DownloadCatalogEntry(ctx, profile, fileEntry)
		if err != nil {
			return nil, err
		}
		bundleContents[sourcePath] = content
	}

	return bundleContents, nil
}
