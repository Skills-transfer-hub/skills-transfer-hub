package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"sth/internal/fs"
)

const bundleDownloadConcurrency = 4

type resolvedVersion struct {
	sha string
	err error
}

func (a *app) runInstall(args []string) error {
	flags, positional := parseFlags(args, "--dry-run", "--json", "--force")
	dryRun := flags["--dry-run"]
	jsonOut := flags["--json"]
	force := flags["--force"]

	err := a.installResource(dryRun, jsonOut, force, positional)
	if err != nil && jsonOut {
		emitJSONError(a.stdout, err, 1)
	}
	return err
}

func (a *app) installResource(dryRun, jsonOut, force bool, positional []string) error {
	if len(positional) != 1 {
		return fmt.Errorf("usage: sth install <resource-ref> [--dry-run] [--force]")
	}
	resourceRef := positional[0]

	var profile *fs.ProjectProfile
	var release func()
	var err error
	if dryRun {
		profile, err = a.prepareStateReadAccess()
		release = func() {}
	} else {
		profile, release, err = a.prepareStateAccess()
	}
	if err != nil {
		return err
	}
	defer release()

	var cfg fs.ProjectConfigV2
	if dryRun {
		cfg, err = fs.ReadProjectConfigV2ReadOnly()
	} else {
		cfg, err = fs.ReadProjectConfigV2()
	}
	if err != nil {
		return err
	}
	providerID := "default"
	if len(cfg.Providers) > 0 {
		providerID = cfg.Providers[0].ID
	}

	if !dryRun && !jsonOut {
		a.progress = newInstallProgress(a.stdout, a.stderr, a.interactiveProgress())
		a.progress.start(installStepCount)
		defer func() {
			a.progress.stop()
			a.progress = nil
		}()
	}

	if jsonOut && !dryRun {
		a.fanoutJSON = &fanoutJSONAccumulator{outcome: "installed"}
		defer func() { a.fanoutJSON = nil }()
	}

	a.progress.step("Fetching catalog…")
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
		resolvedVersion := ""
		if !dryRun {
			resolvedVersion = a.resolveCommitForEntry(context.Background(), cfg, providerID, entry, make(map[string]string))
		}
		if fanoutErr := a.installFanout(*profile, cfg, entry, providerID, resolvedVersion, dryRun, jsonOut, force); fanoutErr != nil {
			return fanoutErr
		}
	} else {
		if legacyErr := a.installLegacy(*profile, entry, providerID, dryRun, jsonOut, force); legacyErr != nil {
			return legacyErr
		}
	}

	if a.fanoutJSON != nil {
		payload, marshalErr := marshalUpdateResults(a.fanoutJSON.results)
		if marshalErr != nil {
			return marshalErr
		}
		a.writeStdout(payload)
	}
	a.progress.complete()
	return nil
}

func (a *app) installLegacy(profile fs.ProjectProfile, entry fs.CatalogEntry, providerID string, dryRun, jsonOut, force bool) error {
	if entry.ResourceType == "bundle" {
		a.progress.step(fmt.Sprintf("Downloading %s…", entry.Name))
		bundleContents, bundleErr := a.downloadBundleContents(context.Background(), profile, entry)
		if bundleErr != nil {
			return bundleErr
		}
		if dryRun {
			plannedFiles, planErr := fs.PlanManagedBundle(profile.TargetDir, entry, bundleContents)
			if planErr != nil {
				return planErr
			}
			projectRoot, pathErr := filepath.Abs(".")
			if pathErr != nil {
				return pathErr
			}
			return a.renderFanoutDryRun(projectRoot, profile.TargetDir, entry.CatalogID, plannedFiles, nil, nil, jsonOut)
		}
		a.progress.step("Installing files…")
		record, syncErr := fs.SyncManagedBundle(profile.TargetDir, entry, bundleContents)
		if syncErr != nil {
			return syncErr
		}
		a.progress.step("Finalizing…")
		if a.fanoutJSON != nil {
			a.fanoutJSON.results = append(a.fanoutJSON.results, updateJSONResult{
				ResourceName: entry.CatalogID,
				Outcome:      a.fanoutJSON.outcome,
			})
			return nil
		}
		a.writeStdout(
			fmt.Sprintf("Resource '%s' installed in %s.", entry.Name, record.InstalledPath),
			fmt.Sprintf("Type: %s", entry.ResourceType),
			fmt.Sprintf("Folder: %s", entry.FolderName),
			"Next step: use `sth update` to refresh tracked resources.",
		)
		return nil
	}

	a.progress.step(fmt.Sprintf("Downloading %s…", entry.Name))

	var resolvedVersionCh chan resolvedVersion
	if !dryRun {
		resolvedVersionCh = make(chan resolvedVersion, 1)
		go func() {
			v2cfg, cfgErr := fs.ReadProjectConfigV2()
			if cfgErr != nil {
				resolvedVersionCh <- resolvedVersion{err: cfgErr}
				return
			}
			queryPath := fs.CommitQueryPath(entry.SourcePath, artifactKindOf(entry))
			sha, shaErr := a.client.GetLatestCommitSHAForPath(context.Background(), v2cfg, providerID, queryPath)
			resolvedVersionCh <- resolvedVersion{sha: sha, err: shaErr}
		}()
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
		if a.fanoutJSON != nil {
			a.fanoutJSON.results = append(a.fanoutJSON.results, updateJSONResult{
				ResourceName: entry.CatalogID,
				Outcome:      "excluded",
			})
		}
		return nil
	}

	a.progress.step("Installing files…")
	record, err := fs.InstallCatalogEntry(profile.TargetDir, entry, content)
	if err != nil {
		return err
	}
	v2rec, err := fs.InstallCatalogEntryV2(profile.TargetDir, entry, content, providerID)
	if err != nil {
		return err
	}
	a.progress.step("Finalizing…")
	if resolvedVersionCh != nil {
		result := <-resolvedVersionCh
		if result.err == nil && result.sha != "" {
			v2rec.ResolvedVersion = result.sha
			if upsertErr := fs.UpsertInstalledResourceV2(profile.TargetDir, v2rec); upsertErr != nil {
				a.writeStderr(fmt.Sprintf("warning: failed to persist resolved version: %v", upsertErr))
			}
		}
	}

	if a.fanoutJSON != nil {
		a.fanoutJSON.results = append(a.fanoutJSON.results, updateJSONResult{
			ResourceName:    entry.CatalogID,
			Outcome:         a.fanoutJSON.outcome,
			ResolvedVersion: v2rec.ResolvedVersion,
		})
		return nil
	}
	a.writeStdout(
		fmt.Sprintf("Resource '%s' installed in %s.", entry.Name, record.InstalledPath),
		fmt.Sprintf("Type: %s", entry.ResourceType),
		fmt.Sprintf("Folder: %s", entry.FolderName),
		"Next step: use `sth update` to refresh tracked resources.",
	)
	return nil
}

func (a *app) downloadBundleContents(ctx context.Context, profile fs.ProjectProfile, entry fs.CatalogEntry) (map[string][]byte, error) {
	bundlePaths, err := a.client.ListBundleFiles(ctx, profile, entry)
	if err != nil {
		return nil, err
	}

	bundleContents := make(map[string][]byte, len(bundlePaths))
	var mutex sync.Mutex
	var firstErr error
	var group sync.WaitGroup
	semaphore := make(chan struct{}, bundleDownloadConcurrency)

	for _, sourcePath := range bundlePaths {
		sourcePath := sourcePath
		group.Add(1)
		semaphore <- struct{}{}
		go func() {
			defer group.Done()
			defer func() { <-semaphore }()

			fileEntry := entry
			fileEntry.SourcePath = sourcePath
			content, downloadErr := a.client.DownloadCatalogEntry(ctx, profile, fileEntry)

			mutex.Lock()
			defer mutex.Unlock()
			if downloadErr != nil {
				if firstErr == nil {
					firstErr = downloadErr
				}
				return
			}
			bundleContents[sourcePath] = content
		}()
	}

	group.Wait()
	if firstErr != nil {
		return nil, firstErr
	}

	return bundleContents, nil
}
