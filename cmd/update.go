package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"sth/internal/fs"
	sthhttp "sth/internal/http"
	"os"
	"path/filepath"
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

	profile, release, err := a.prepareStateAccess()
	if err != nil {
		if jsonOut {
			emitJSONError(a.stdout, err, 1)
		}
		return err
	}
	defer release()

	cfg, err := fs.ReadProjectConfigV2()
	if err != nil {
		if jsonOut {
			emitJSONError(a.stdout, err, 1)
		}
		return err
	}

	if len(cfg.Targets) > 0 {
		return a.updateFanout(profile, cfg, dryRun, jsonOut, failOnChanges)
	}

	// FR-017/FR-018/FR-019: --fail-on-changes is a CI gate. Detect any
	// locally-modified managed file and exit 1 BEFORE doing anything else
	// (network calls, writes, etc.). Combines correctly with --dry-run.
	if failOnChanges {
		if state, stateErr := fs.ReadInstalledStateV2(profile.TargetDir); stateErr == nil {
			projectRoot, _ := filepath.Abs(".")
			results := fs.ComputeAllStatuses(state, cfg, nil, projectRoot)
			modified := []fs.StatusResult{}
			for _, result := range results {
				if result.Status == fs.ResourceStatusModifiedLocally {
					modified = append(modified, result)
				}
			}
			if len(modified) > 0 {
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
		}
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
				a.writeStdout("Aucune ressource à mettre à jour.")
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
		if jsonOut {
			emitJSONError(a.stdout, err, 1)
		}
		return err
	}

	if len(records) == 0 {
		if jsonOut {
			a.writeStdout("[]")
		} else {
			a.writeStdout("Aucune ressource à mettre à jour.")
		}
		return nil
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
		if cached, ok := latestByPath[queryPath]; ok {
			return cached
		}
		sha, shaErr := a.client.GetLatestCommitSHAForPath(context.Background(), cfg, providerID, queryPath)
		if shaErr != nil {
			return ""
		}
		latestByPath[queryPath] = sha
		return sha
	}

	for _, record := range records {
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
					a.writeStderr(fmt.Sprintf("Échec de mise à jour pour '%s' : %v", record.Name, bundleErr))
				}
				continue
			}

			if _, syncErr := fs.SyncManagedBundle(profile.TargetDir, entry, bundleContents); syncErr != nil {
				failures++
				if jsonOut {
					recordResult(record.Name, "failed", "", "", syncErr.Error())
				} else {
					a.writeStderr(fmt.Sprintf("Échec de mise à jour pour '%s' : %v", record.Name, syncErr))
				}
				continue
			}

			if jsonOut {
				recordResult(record.Name, "updated", "", installedVersionOf(v2State, v2StateErr, record.CatalogID), "")
			} else {
				a.writeStdout(fmt.Sprintf("Ressource '%s' mise à jour.", record.Name))
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
				a.writeStderr(fmt.Sprintf("Échec de mise à jour pour '%s' : %v", record.Name, downloadErr))
			}
			continue
		}

		if fs.MatchesIgnorePattern(record.SourcePath, ignorePats) {
			if jsonOut {
				recordResult(record.Name, "excluded", "", "", "")
			} else {
				a.writeStdout(fmt.Sprintf("Ressource '%s' exclue par .sthignore.", record.Name))
			}
			continue
		}

		if v2StateErr == nil {
			if conflict := fs.CheckOwnershipConflict(&v2State, record.CatalogID, record.InstalledPath); conflict != nil {
				failures++
				if jsonOut {
					recordResult(record.Name, "failed", "", "", conflict.Error())
				} else {
					a.writeStderr(fmt.Sprintf("Conflit de propriété pour '%s' : %v", record.Name, conflict))
				}
				continue
			}
		}

		if saveErr := fs.SaveInstalledResource(record, content); saveErr != nil {
			failures++
			if jsonOut {
				recordResult(record.Name, "failed", "", "", saveErr.Error())
			} else {
				a.writeStderr(fmt.Sprintf("Échec de mise à jour pour '%s' : %v", record.Name, saveErr))
			}
			continue
		}

		if upsertErr := fs.UpsertInstalledResource(profile.TargetDir, record); upsertErr != nil {
			failures++
			if jsonOut {
				recordResult(record.Name, "failed", "", "", upsertErr.Error())
			} else {
				a.writeStderr(fmt.Sprintf("Échec de mise à jour pour '%s' : %v", record.Name, upsertErr))
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
			a.writeStdout(fmt.Sprintf("Ressource '%s' mise à jour.", record.Name))
		}
	}

	if v2Updated {
		if writeErr := fs.WriteInstalledStateV2(profile.TargetDir, v2State); writeErr != nil {
			stateErr := fmt.Errorf("impossible de mettre à jour l'état v2 : %w", writeErr)
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
		return fmt.Errorf("mise à jour incomplète : %d ressource(s) en échec", failures)
	}

	return nil
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

	assetData, err := a.client.DownloadURL(context.Background(), release.DownloadURL, nil, sthhttp.ReleaseAssetResponseLimitBytes)
	if err != nil {
		return err
	}
	checksumData, err := a.client.DownloadURL(context.Background(), release.ChecksumURL, nil, sthhttp.ChecksumManifestResponseLimitBytes)
	if err != nil {
		return err
	}
	signatureData, err := a.client.DownloadURL(context.Background(), release.SignatureURL, nil, sthhttp.ChecksumManifestResponseLimitBytes)
	if err != nil {
		return err
	}

	executablePath, err := currentExecutablePath()
	if err != nil {
		return err
	}
	if err := fs.ReplaceExecutableWithAsset(executablePath, release.AssetName, assetData, checksumData, signatureData); err != nil {
		return fmt.Errorf("mise à jour stable %s détectée, mais le remplacement automatique a échoué : %w", fs.DisplayVersion(release.Version), err)
	}

	a.writeStdout(
		fmt.Sprintf("Version stable détectée : %s", fs.DisplayVersion(release.Version)),
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
		return "", fmt.Errorf("impossible de déterminer le binaire courant : %w", err)
	}

	return path, nil
}
