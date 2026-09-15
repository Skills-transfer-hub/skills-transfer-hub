package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"sth/internal/artifact"
	"sth/internal/fs"
)

const bundleDownloadConcurrency = 4

type resolvedVersion struct {
	sha string
	err error
}

func (a *app) runInstall(args []string) error {
	options, optionErr := parseCommandOptions(args, []string{"--dry-run", "--json", "--force"}, []string{"--kind", "--target", "--allow", "--provider"})
	if optionErr != nil {
		return optionErr
	}
	dryRun := options.flags["--dry-run"]
	jsonOut := options.flags["--json"]
	force := options.flags["--force"]
	kinds := optionValues(options, "--kind")
	if len(kinds) > 1 {
		return fmt.Errorf("--kind accepts one artifact kind")
	}
	a.installKind = ""
	if len(kinds) == 1 {
		a.installKind = kinds[0]
	}
	a.installTargets = stringSet(optionValues(options, "--target"))
	a.installAllowed = map[artifact.CompatibilityLevel]bool{artifact.CompatibilityExact: true}
	for _, level := range optionValues(options, "--allow") {
		switch artifact.CompatibilityLevel(level) {
		case artifact.CompatibilityExact, artifact.CompatibilityLossy:
			a.installAllowed[artifact.CompatibilityLevel(level)] = true
		default:
			return fmt.Errorf("unsupported compatibility level for --allow: %s", level)
		}
	}
	a.installApprovedLossy = map[string]bool{}
	defer func() {
		a.installKind = ""
		a.installTargets = nil
		a.installAllowed = nil
		a.installApprovedLossy = nil
	}()
	providers := optionValues(options, "--provider")
	if len(providers) > 1 {
		return fmt.Errorf("--provider accepts one source ID")
	}
	providerFlag := ""
	if len(providers) == 1 {
		providerFlag = providers[0]
	}
	positional := options.positional
	if len(positional) == 0 && a.installKind != "" {
		return a.installKindResources(a.installKind, providerFlag, dryRun, force, jsonOut)
	}

	err := a.installResource(dryRun, jsonOut, force, providerFlag, positional)
	if err != nil && jsonOut {
		emitJSONError(a.stdout, err, 1)
	}
	return err
}

func (a *app) installKindResources(kind, providerFlag string, dryRun, force, jsonOut bool) error {
	config, err := fs.ReadProjectConfigV2ReadOnly()
	if err != nil {
		return err
	}
	catalog, err := a.aggregateCatalog(context.Background(), config)
	if err != nil {
		return err
	}
	matchingReferences := make([]string, 0)
	for _, entry := range catalog.entries {
		if artifactKindOf(entry) == kind && (providerFlag == "" || fs.CatalogEntryProviderID(entry) == providerFlag) {
			matchingReferences = append(matchingReferences, fs.CatalogEntryProviderID(entry)+":"+catalogDisplayRef(entry))
		}
	}
	if len(matchingReferences) == 0 {
		return fmt.Errorf("no catalog artifacts found for kind %s", kind)
	}
	sort.Strings(matchingReferences)
	if jsonOut {
		a.fanoutJSON = &fanoutJSONAccumulator{outcome: "installed"}
		defer func() { a.fanoutJSON = nil }()
	}
	for _, resourceReference := range matchingReferences {
		if err := a.installResource(dryRun, jsonOut, force, providerFlag, []string{resourceReference}); err != nil {
			return err
		}
	}
	if jsonOut {
		if dryRun {
			data, formatErr := fs.FormatDryRunJSON(a.fanoutJSON.changes)
			if formatErr != nil {
				return formatErr
			}
			a.writeStdout(string(data))
			return nil
		}
		payload, marshalErr := marshalUpdateResults(a.fanoutJSON.results)
		if marshalErr != nil {
			return marshalErr
		}
		a.writeStdout(payload)
	}
	return nil
}

func (a *app) installResource(dryRun, jsonOut, force bool, providerFlag string, positional []string) error {
	if len(positional) != 1 {
		return fmt.Errorf("usage: sth install <resource-ref> [--provider <id>] [--dry-run] [--force]")
	}
	resourceRef := positional[0]

	var primaryProfile *fs.ProjectProfile
	var release func()
	var err error
	if dryRun {
		primaryProfile, err = a.prepareStateReadAccess()
		release = func() {}
	} else {
		primaryProfile, release, err = a.prepareStateAccess()
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
	if providerFlag != "" {
		if _, found := fs.ProviderByID(cfg, providerFlag); !found && len(cfg.Providers) > 0 {
			return fmt.Errorf("unknown provider: %q (configured: %s)", providerFlag, strings.Join(fs.ProviderIDs(cfg), ", "))
		}
	}

	if !dryRun && !jsonOut {
		a.progress = newInstallProgress(a.stdout, a.stderr, a.interactiveProgress())
		a.progress.start(installStepCount)
		defer func() {
			a.progress.stop()
			a.progress = nil
		}()
	}

	ownsJSONAccumulator := false
	if jsonOut && !dryRun && a.fanoutJSON == nil {
		a.fanoutJSON = &fanoutJSONAccumulator{outcome: "installed"}
		ownsJSONAccumulator = true
		defer func() { a.fanoutJSON = nil }()
	}

	a.progress.step("Fetching catalog…")
	catalog, err := a.aggregateCatalog(context.Background(), cfg)
	if err != nil {
		return err
	}
	a.reportProviderFailures(catalog.failures)

	entry, err := a.resolveCatalogEntry(catalog, resourceRef, providerFlag)
	if err != nil {
		return err
	}
	providerID := fs.CatalogEntryProviderID(entry)
	sourceProfile, err := a.sourceProfile(cfg, providerID)
	if err != nil {
		return err
	}
	if !dryRun {
		providerRelease, lockErr := a.prepareProviderStateAccess(sourceProfile.TargetDir, primaryProfile.TargetDir)
		if lockErr != nil {
			return lockErr
		}
		defer providerRelease()
	}
	if a.installKind != "" && artifactKindOf(entry) != a.installKind {
		return fmt.Errorf("resource %s is %s, not requested kind %s", resourceRef, artifactKindOf(entry), a.installKind)
	}

	if len(cfg.Targets) > 0 {
		resolvedVersion := ""
		if !dryRun {
			resolvedVersion = a.resolveCommitForEntry(context.Background(), cfg, providerID, entry, make(map[string]string))
		}
		if fanoutErr := a.installFanout(sourceProfile, sourceProfile.TargetDir, cfg, entry, providerID, resolvedVersion, dryRun, jsonOut, force); fanoutErr != nil {
			return fanoutErr
		}
	} else {
		if len(a.installTargets) > 0 && !a.targetSelected("claude", "claude") {
			return fmt.Errorf("legacy project configuration only supports the Claude target")
		}
		if legacyErr := a.installLegacy(sourceProfile, entry, providerID, dryRun, jsonOut, force); legacyErr != nil {
			return legacyErr
		}
	}

	if ownsJSONAccumulator {
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
	if driftErr := refuseModifiedResources(state, fs.ProjectConfigV2{}, entry.CatalogID); driftErr != nil {
		return driftErr
	}
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
