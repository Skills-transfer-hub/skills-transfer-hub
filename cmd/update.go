package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sth/internal/fs"
	sthhttp "sth/internal/http"
	"strings"
	"time"
)

type updateJSONResult struct {
	ResourceName    string `json:"resource_name"`
	Outcome         string `json:"outcome"`
	ResolvedVersion string `json:"resolved_version,omitempty"`
	PreviousVersion string `json:"previous_version,omitempty"`
	Error           string `json:"error,omitempty"`
}

type fanoutJSONAccumulator struct {
	outcome string
	changes []fs.DryRunChange
	results []updateJSONResult
}

func marshalUpdateResults(results []updateJSONResult) (string, error) {
	if results == nil {
		results = []updateJSONResult{}
	}
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (a *app) runUpdate(args []string) error {
	flags, _ := parseFlags(args, "--dry-run", "--json", "--fail-on-changes")
	dryRun := flags["--dry-run"]
	jsonOut := flags["--json"]
	failOnChanges := flags["--fail-on-changes"]

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
		if jsonOut {
			emitJSONError(a.stdout, err, 1)
		}
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
		if jsonOut {
			emitJSONError(a.stdout, err, 1)
		}
		return err
	}
	if failOnChanges {
		modifiedErr := a.reportModifiedResources(profile.TargetDir, cfg, jsonOut)
		if modifiedErr != nil {
			return modifiedErr
		}
	}
	if !dryRun {
		currentState, stateErr := fs.ReadInstalledStateV2(profile.TargetDir)
		if stateErr != nil {
			return stateErr
		}
		if driftErr := refuseModifiedResources(currentState, cfg, ""); driftErr != nil {
			if jsonOut {
				emitJSONError(a.stdout, driftErr, 1)
			}
			return driftErr
		}
	}

	if len(cfg.Targets) > 0 {
		if !dryRun && !jsonOut {
			a.progress = newInstallProgress(a.stdout, a.stderr, a.interactiveProgress())
			defer func() {
				a.progress.stop()
				a.progress = nil
			}()
		}
		updateErr := a.updateFanout(profile, cfg, dryRun, jsonOut, failOnChanges)
		if updateErr == nil {
			a.progress.complete()
		}
		return updateErr
	}

	if dryRun {
		v2State, err := fs.ReadInstalledStateV2(profile.TargetDir)
		if err != nil {
			if jsonOut {
				emitJSONError(a.stdout, err, 1)
			}
			return err
		}
		if len(v2State.Entries) == 0 {
			if jsonOut {
				a.writeStdout("[]")
			} else {
				a.writeStdout("No resources to update.")
			}
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
				a.writeStderr(fmt.Sprintf("Failed to fetch '%s': %v", rec.ResourceName, fetchErr))
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
		if jsonOut {
			emitJSONError(a.stdout, err, 1)
		}
		return err
	}

	if len(records) == 0 {
		if jsonOut {
			a.writeStdout("[]")
		} else {
			a.writeStdout("No resources to update.")
		}
		return nil
	}

	if !dryRun && !jsonOut {
		a.progress = newInstallProgress(a.stdout, a.stderr, a.interactiveProgress())
		a.progress.start(len(records) + 1)
		a.progress.step("Preparing update…")
		defer func() {
			a.progress.stop()
			a.progress = nil
		}()
	}

	failures := 0
	var updateResults []updateJSONResult
	recordResult := func(name, outcome, resolved, previous, failure string) {
		updateResults = append(updateResults, updateJSONResult{
			ResourceName:    name,
			Outcome:         outcome,
			ResolvedVersion: resolved,
			PreviousVersion: previous,
			Error:           failure,
		})
	}
	v2State, v2StateErr := fs.ReadInstalledStateV2(profile.TargetDir)
	v2Updated := false

	workingDir, _ := filepath.Abs(".")
	ignorePats, _ := fs.ReadSTHIgnore(workingDir)

	latestByPath := make(map[string]string)
	resolveLatest := func(providerID, sourcePath, artifactKind string) string {
		queryPath := fs.CommitQueryPath(sourcePath, artifactKind)
		if queryPath == "" {
			return ""
		}
		cacheKey := fs.LatestVersionKey(providerID, queryPath)
		if cached, ok := latestByPath[cacheKey]; ok {
			return cached
		}
		sha, shaErr := a.client.GetLatestCommitSHAForPath(context.Background(), cfg, providerID, queryPath)
		if shaErr != nil {
			return ""
		}
		latestByPath[cacheKey] = sha
		return sha
	}

	for _, record := range records {
		if !jsonOut {
			a.progress.step(fmt.Sprintf("Updating %s…", record.CatalogID))
		}
		if pinnedVer := fs.PinnedVersion(cfg, record.CatalogID); pinnedVer != "" {
			if jsonOut {
				recordResult(record.CatalogID, "pinned", "", pinnedVer, "")
				continue
			}
			a.writeStdout(fmt.Sprintf("  [pinned at %s] %s", pinnedVer, record.CatalogID))
			if v2StateErr == nil {
				for _, v2rec := range v2State.Entries {
					if v2rec.ResourceName == record.CatalogID {
						latestSHA := resolveLatest(v2rec.ProviderID, record.SourcePath, record.ResourceType)
						if latestSHA != "" && v2rec.ResolvedVersion != "" && latestSHA != v2rec.ResolvedVersion {
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
			bundleContents, bundleErr := a.downloadBundleContents(context.Background(), *profile, entry)
			if bundleErr != nil {
				failures++
				if jsonOut {
					recordResult(record.Name, "failed", "", "", bundleErr.Error())
				} else {
					a.writeStderr(fmt.Sprintf("Failed to update '%s': %v", record.Name, bundleErr))
				}
				continue
			}

			if _, syncErr := fs.SyncManagedBundle(profile.TargetDir, entry, bundleContents); syncErr != nil {
				failures++
				if jsonOut {
					recordResult(record.Name, "failed", "", "", syncErr.Error())
				} else {
					a.writeStderr(fmt.Sprintf("Failed to update '%s': %v", record.Name, syncErr))
				}
				continue
			}

			if jsonOut {
				recordResult(record.Name, "updated", "", installedVersionOf(v2State, v2StateErr, record.CatalogID), "")
			} else {
				a.writeStdout(fmt.Sprintf("Resource '%s' updated.", record.Name))
				a.printBundleChangelog(record.Name, bundleContents, installedVersionOf(v2State, v2StateErr, record.CatalogID))
			}
			continue
		}

		content, downloadErr := a.client.DownloadCatalogEntry(context.Background(), *profile, entry)
		if downloadErr != nil {
			failures++
			if jsonOut {
				recordResult(record.Name, "failed", "", "", downloadErr.Error())
			} else {
				a.writeStderr(fmt.Sprintf("Failed to update '%s': %v", record.Name, downloadErr))
			}
			continue
		}

		if fs.MatchesIgnorePattern(record.SourcePath, ignorePats) {
			if jsonOut {
				recordResult(record.Name, "excluded", "", "", "")
			} else {
				a.writeStdout(fmt.Sprintf("Resource '%s' excluded by .sthignore.", record.Name))
			}
			continue
		}

		if v2StateErr == nil {
			if conflict := fs.CheckOwnershipConflict(&v2State, record.CatalogID, record.InstalledPath); conflict != nil {
				failures++
				if jsonOut {
					recordResult(record.Name, "failed", "", "", conflict.Error())
				} else {
					a.writeStderr(fmt.Sprintf("Ownership conflict for '%s': %v", record.Name, conflict))
				}
				continue
			}
		}

		if saveErr := fs.SaveInstalledResource(record, content); saveErr != nil {
			failures++
			if jsonOut {
				recordResult(record.Name, "failed", "", "", saveErr.Error())
			} else {
				a.writeStderr(fmt.Sprintf("Failed to update '%s': %v", record.Name, saveErr))
			}
			continue
		}

		if upsertErr := fs.UpsertInstalledResource(profile.TargetDir, record); upsertErr != nil {
			failures++
			if jsonOut {
				recordResult(record.Name, "failed", "", "", upsertErr.Error())
			} else {
				a.writeStderr(fmt.Sprintf("Failed to update '%s': %v", record.Name, upsertErr))
			}
			continue
		}

		resolvedVersion := ""
		previousVersion := ""
		if v2StateErr == nil {
			for i, v2rec := range v2State.Entries {
				if v2rec.ResourceName == record.CatalogID {
					previousVersion = v2rec.ResolvedVersion
					v2State.Entries[i] = fs.RefreshManagedFileHashes(v2rec, content)
					v2State.Entries[i].PreviousVersion = v2rec.ResolvedVersion
					v2State.Entries[i].SourcePath = record.SourcePath
					if sha := resolveLatest(v2rec.ProviderID, record.SourcePath, record.ResourceType); sha != "" {
						v2State.Entries[i].ResolvedVersion = sha
						resolvedVersion = sha
					}
					v2State.Entries[i].InstalledAt = time.Now().UTC().Format(time.RFC3339)
					v2Updated = true
					break
				}
			}
		}

		if jsonOut {
			recordResult(record.Name, "updated", resolvedVersion, previousVersion, "")
		} else {
			a.writeStdout(fmt.Sprintf("Resource '%s' updated.", record.Name))
		}
	}

	if v2Updated {
		if writeErr := fs.WriteInstalledStateV2(profile.TargetDir, v2State); writeErr != nil {
			stateErr := fmt.Errorf("unable to update v2 state: %w", writeErr)
			if jsonOut {
				emitJSONError(a.stdout, stateErr, 1)
			}
			return stateErr
		}
	}

	if jsonOut {
		payload, marshalErr := marshalUpdateResults(updateResults)
		if marshalErr != nil {
			return marshalErr
		}
		a.writeStdout(payload)
	}

	if failures > 0 {
		return fmt.Errorf("update incomplete: %d resource(s) failed", failures)
	}

	a.progress.complete()
	return nil
}

func (a *app) reportModifiedResources(targetDirectory string, config fs.ProjectConfigV2, jsonOut bool) error {
	state, err := fs.ReadInstalledStateV2(targetDirectory)
	if err != nil {
		return err
	}
	projectRoot, _ := filepath.Abs(".")
	results := fs.ComputeAllStatuses(state, config, nil, projectRoot)
	modified := make([]fs.StatusResult, 0)
	for _, result := range results {
		if result.Status == fs.ResourceStatusModifiedLocally {
			modified = append(modified, result)
		}
	}
	if len(modified) == 0 {
		return nil
	}
	changeErr := fmt.Errorf("%d resource(s) have local modifications", len(modified))
	if jsonOut {
		emitJSONError(a.stdout, changeErr, 1)
		return changeErr
	}
	a.writeStdout("Modified files detected (--fail-on-changes):")
	for _, result := range modified {
		for _, file := range result.ModifiedFiles {
			a.writeStdout(fmt.Sprintf("  %s — %s", result.ResourceName, file))
		}
	}
	return changeErr
}

func installedVersionOf(v2State fs.InstalledResourceStateV2, v2StateErr error, resourceName string) string {
	if v2StateErr != nil {
		return ""
	}
	for _, record := range v2State.Entries {
		if record.ResourceName == resourceName {
			return record.ResolvedVersion
		}
	}
	return ""
}

func sourcePath(rec fs.InstalledResourceRecordV2, targetDir string) string {
	if len(rec.ManagedFiles) > 0 {
		rel, err := filepath.Rel(targetDir, rec.ManagedFiles[0].Path)
		if err == nil {
			return filepath.ToSlash(rel)
		}
	}
	return rec.ResourceName
}

func (a *app) runSelfUpdate() error {
	_, err := a.runSelfUpdateResult()
	return err
}

func (a *app) runSelfUpdateResult() (bool, error) {
	execPath, _ := currentExecutablePath()
	if pm := detectPackageManager(execPath); pm.name != "" {
		return false, fmt.Errorf(
			"sth was installed with %s. Run `%s` instead of `sth update self`",
			pm.name, pm.upgradeHint,
		)
	}

	release, _, err := a.resolveStableUpdate(context.Background(), true)
	if err != nil {
		return false, err
	}
	if release == nil {
		a.writeStdout("No stable update is available.")
		return false, nil
	}

	progress := newInstallProgress(a.stdout, a.stderr, a.interactiveProgress())
	progress.start(4)
	defer progress.stop()

	progress.step("Downloading binary…")
	assetData, err := a.client.DownloadURL(context.Background(), release.DownloadURL, nil, sthhttp.ReleaseAssetResponseLimitBytes)
	if err != nil {
		return false, err
	}
	progress.step("Downloading checksum…")
	checksumData, err := a.client.DownloadURL(context.Background(), release.ChecksumURL, nil, sthhttp.ChecksumManifestResponseLimitBytes)
	if err != nil {
		return false, err
	}
	progress.step("Downloading signature…")
	signatureData, err := a.client.DownloadURL(context.Background(), release.SignatureURL, nil, sthhttp.ChecksumManifestResponseLimitBytes)
	if err != nil {
		return false, err
	}
	progress.step("Downloading signed release manifest…")
	manifestData, err := a.client.DownloadURL(context.Background(), release.ManifestURL, nil, sthhttp.ChecksumManifestResponseLimitBytes)
	if err != nil {
		return false, err
	}
	if err := fs.VerifyReleaseEngineManifest(release.Version, manifestData, checksumData, signatureData); err != nil {
		return false, fmt.Errorf("stable update %s is available, but its signed release manifest is invalid: %w", fs.DisplayVersion(release.Version), err)
	}

	executablePath, err := currentExecutablePath()
	if err != nil {
		return false, err
	}
	if err := fs.ReplaceExecutableWithAsset(executablePath, release.AssetName, assetData, checksumData, signatureData); err != nil {
		return false, fmt.Errorf("stable update %s is available, but automatic replacement failed: %w", fs.DisplayVersion(release.Version), err)
	}

	progress.complete()
	a.writeStdout(
		fmt.Sprintf("Stable version available: %s", fs.DisplayVersion(release.Version)),
		"Update applied.",
	)
	_, _ = fs.WriteUpdateCheckState(fs.NewNoUpdateCheckState(time.Now().UTC(), release.Version))
	return true, nil
}

func (a *app) startupStableUpdateOffer() *fs.StableUpdateOffer {
	_, offer, _ := a.resolveStableUpdate(context.Background(), false)
	return offer
}

func (a *app) cachedStableUpdateOffer() *fs.StableUpdateOffer {
	state, err := fs.ReadUpdateCheckState()
	if err != nil || state == nil {
		return nil
	}
	if !state.IsFresh(time.Now().UTC(), valueOrDefault(version, "dev")) {
		return nil
	}
	offer := fs.StableUpdateOfferFromState(*state)
	return &offer
}

func (a *app) refreshUpdateOfferInBackground() {
	a.updateRefreshOnce.Do(func() {
		metadata := currentReleaseMetadata()
		client := a.client.SilentCopy()
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_, _, _ = a.resolveStableUpdateWithClient(ctx, metadata, false, client)
		}()
	})
}

func (a *app) resolveStableUpdate(ctx context.Context, force bool) (*sthhttp.ReleaseAsset, *fs.StableUpdateOffer, error) {
	return a.resolveStableUpdateWithMetadata(ctx, currentReleaseMetadata(), force)
}

func (a *app) resolveStableUpdateWithMetadata(ctx context.Context, metadata sthhttp.ReleaseMetadata, force bool) (*sthhttp.ReleaseAsset, *fs.StableUpdateOffer, error) {
	return a.resolveStableUpdateWithClient(ctx, metadata, force, a.client)
}

func (a *app) resolveStableUpdateWithClient(ctx context.Context, metadata sthhttp.ReleaseMetadata, force bool, client *sthhttp.Client) (*sthhttp.ReleaseAsset, *fs.StableUpdateOffer, error) {
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

	release, err := client.CheckForStableUpdate(ctx, metadata)
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

	state := fs.NewAvailableUpdateCheckState(now, metadata.CurrentVersion, release.Version, release.DownloadURL, release.ChecksumURL, release.AssetName).
		WithReleaseSummary(release.Highlights, release.ReleaseNotesURL)
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
		return "", fmt.Errorf("unable to locate the current binary: %w", err)
	}

	return path, nil
}
