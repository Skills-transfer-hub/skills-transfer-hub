package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"sth/internal/fs"
)

func (a *app) runInstall(args []string) error {
	flags, positional := parseFlags(args, "--dry-run", "--json", "--force")
	dryRun := flags["--dry-run"]
	jsonOut := flags["--json"]
	force := flags["--force"]

	if len(positional) != 1 {
		return fmt.Errorf("usage: sth install <resource-ref> [--dry-run] [--force]")
	}
	resourceRef := positional[0]

	profile, release, err := a.prepareStateAccess()
	if err != nil {
		return err
	}
	defer release()

	cfg, _ := fs.ReadProjectConfigV2()
	providerID := "default"
	if len(cfg.Providers) > 0 {
		providerID = cfg.Providers[0].ID
	}

	var entries []fs.CatalogEntry
	if err := a.withAuthRecovery(context.Background(), func() error {
		var listErr error
		entries, listErr = a.client.ListCatalog(context.Background(), *profile)
		return listErr
	}); err != nil {
		return err
	}

	entry, err := fs.ResolveCatalogEntry(entries, resourceRef)
	if err != nil {
		return err
	}

	if len(cfg.Targets) > 0 {
		return a.installFanout(*profile, cfg, entry, providerID, dryRun, jsonOut, force)
	}
	return a.installLegacy(*profile, entry, providerID, dryRun, jsonOut, force)
}

func (a *app) installLegacy(profile fs.ProjectProfile, entry fs.CatalogEntry, providerID string, dryRun, jsonOut, force bool) error {
	if entry.ResourceType == "bundle" {
		bundleContents, bundleErr := a.downloadBundleContents(context.Background(), profile, entry)
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
			"Prochaine étape : utilisez `sth update` pour rafraîchir les ressources suivies.",
		)
		return nil
	}

	content, err := a.client.DownloadCatalogEntry(context.Background(), profile, entry)
	if err != nil {
		return err
	}

	workingDir, _ := filepath.Abs(".")
	ignorePats, _ := fs.ReadSTHIgnore(workingDir)

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

	relPath := filepath.ToSlash(entry.SourcePath)
	if len(ignorePats) > 0 && fs.MatchIgnore(ignorePats, relPath) {
		a.writeStderr("warning: no files installed — all excluded by .sthignore")
		v2rec := fs.InstalledResourceRecordV2{
			ResourceName: entry.CatalogID,
			ProviderID:   providerID,
			ManagedFiles: []fs.ManagedFile{},
		}
		if upsertErr := fs.UpsertInstalledResourceV2(profile.TargetDir, v2rec); upsertErr != nil {
			return upsertErr
		}
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
		"Prochaine étape : utilisez `sth update` pour rafraîchir les ressources suivies.",
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
