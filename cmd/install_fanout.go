package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"sth/internal/fs"
	"sth/internal/writer"
)

func (a *app) installFanout(profile fs.ProjectProfile, cfg fs.ProjectConfigV2, entry fs.CatalogEntry, providerID string, dryRun, jsonOut, force bool) error {
	projectRoot, err := filepath.Abs(".")
	if err != nil {
		return err
	}

	if artifactKindOf(entry) == "skill" {
		return a.installSkillFanout(profile, entry, providerID, projectRoot, enabledTargetKinds(cfg), dryRun, jsonOut)
	}
	return a.installArtifactFanout(profile, cfg, entry, providerID, projectRoot, artifactKindOf(entry), dryRun, jsonOut, force)
}

func artifactKindOf(entry fs.CatalogEntry) string {
	if entry.ArtifactKind != "" {
		return entry.ArtifactKind
	}
	if entry.ResourceType == "bundle" {
		return "skill"
	}
	return entry.ResourceType
}

func isSkillBundle(entry fs.CatalogEntry) bool {
	if entry.ResourceType == "bundle" {
		return true
	}
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(entry.SourcePath)), "/skill.md")
}

func (a *app) resolveSkillSources(profile fs.ProjectProfile, entry fs.CatalogEntry) ([]string, func(string) ([]byte, error), error) {
	if isSkillBundle(entry) {
		bundleContents, err := a.downloadBundleContents(context.Background(), profile, entry)
		if err != nil {
			return nil, nil, err
		}
		sources := make([]string, 0, len(bundleContents))
		for sourcePath := range bundleContents {
			sources = append(sources, sourcePath)
		}
		sort.Strings(sources)
		fetch := func(sourcePath string) ([]byte, error) {
			content, ok := bundleContents[sourcePath]
			if !ok {
				return nil, fmt.Errorf("fichier bundle manquant : %s", sourcePath)
			}
			return content, nil
		}
		return sources, fetch, nil
	}

	content, err := a.client.DownloadCatalogEntry(context.Background(), profile, entry)
	if err != nil {
		return nil, nil, err
	}
	return []string{entry.SourcePath}, func(string) ([]byte, error) { return content, nil }, nil
}

func enabledTargetKinds(cfg fs.ProjectConfigV2) []string {
	kinds := make([]string, 0, len(cfg.Targets))
	for _, target := range fs.EffectiveTargets(cfg) {
		if target.Enabled {
			kinds = append(kinds, target.Kind)
		}
	}
	return kinds
}

func (a *app) installSkillFanout(profile fs.ProjectProfile, entry fs.CatalogEntry, providerID, projectRoot string, kinds []string, dryRun, jsonOut bool) error {
	a.progress.step(fmt.Sprintf("Téléchargement de %s…", entry.Name))
	sources, fetch, err := a.resolveSkillSources(profile, entry)
	if err != nil {
		return err
	}

	placement := writer.PlanSkillPlacement(kinds)
	if len(placement.Folders) == 0 {
		return fmt.Errorf("aucune destination de skill pour les cibles sélectionnées")
	}

	a.progress.step("Installation des fichiers…")
	managedFiles := make([]fs.ManagedFile, 0)
	for _, folder := range placement.Folders {
		target, ok := writer.WriterForSkillsFolder(folder)
		if !ok {
			continue
		}
		result, matErr := writer.MaterializeArtifact(projectRoot, target, "skill", entry.Name, entry.Lang, sources, fetch, true, !dryRun)
		if matErr != nil {
			return matErr
		}
		for _, file := range result.Files {
			managedFiles = append(managedFiles, fs.ManagedFile{Path: file.RelativePath, SHA256: file.SHA256})
		}
	}

	if dryRun {
		return a.renderFanoutDryRun(projectRoot, profile.TargetDir, entry.CatalogID, managedFiles, jsonOut)
	}

	if placement.NeedsCodexCfg {
		a.progress.note("Configuration de Codex…")
		codexSkillPath := placement.CodexFolder + "/" + entry.Name + "/SKILL.md"
		if _, cfgErr := fs.EnsureCodexSkillsConfig(projectRoot, codexSkillPath); cfgErr != nil {
			a.writeStderr(fmt.Sprintf("warning: configuration codex non mise à jour : %v", cfgErr))
		}
	}

	a.progress.step("Finalisation…")
	previous, _, _ := fs.InstalledRecordV2For(profile.TargetDir, entry.CatalogID, "")
	record := fs.InstalledResourceRecordV2{
		ResourceName: entry.CatalogID,
		ProviderID:   providerID,
		InstalledAt:  time.Now().UTC().Format(time.RFC3339),
		ManagedFiles: managedFiles,
		ArtifactKind: "skill",
	}
	if err := fs.SyncManagedFilesV2(profile.TargetDir, record, previous.ManagedFiles); err != nil {
		return err
	}

	a.printSkillPlacement(entry, placement)
	return nil
}

type fanoutTargetPlan struct {
	target fs.TargetEntry
	tool   writer.Target
}

func (a *app) installArtifactFanout(profile fs.ProjectProfile, cfg fs.ProjectConfigV2, entry fs.CatalogEntry, providerID, projectRoot, kind string, dryRun, jsonOut, force bool) error {
	a.progress.step(fmt.Sprintf("Téléchargement de %s…", entry.Name))
	content, err := a.client.DownloadCatalogEntry(context.Background(), profile, entry)
	if err != nil {
		return err
	}
	fetch := func(string) ([]byte, error) { return content, nil }
	sources := []string{entry.SourcePath}

	plans := make([]fanoutTargetPlan, 0)
	plannedFiles := make([]fs.ManagedFile, 0)
	for _, target := range fs.EffectiveTargets(cfg) {
		if !target.Enabled {
			continue
		}
		tool, ok := writer.WriterForKind(target.Kind)
		if !ok {
			a.writeStderr(fmt.Sprintf("warning: cible inconnue ignorée : %s", target.Kind))
			continue
		}
		preview, matErr := writer.MaterializeArtifact(projectRoot, tool, kind, entry.Name, entry.Lang, sources, fetch, true, false)
		if matErr != nil {
			return matErr
		}
		if preview.Skipped {
			a.writeStdout(fmt.Sprintf("  ⤫ %s : %s", target.Kind, preview.Reason))
			continue
		}
		for _, file := range preview.Files {
			plannedFiles = append(plannedFiles, fs.ManagedFile{Path: file.RelativePath, SHA256: file.SHA256})
		}
		plans = append(plans, fanoutTargetPlan{target: target, tool: tool})
	}

	state, _ := fs.ReadInstalledStateV2(profile.TargetDir)
	if !force {
		for _, file := range plannedFiles {
			if conflict := fs.CheckOwnershipConflict(&state, entry.CatalogID, file.Path); conflict != nil {
				return conflict
			}
		}
	} else {
		transferred := false
		for _, file := range plannedFiles {
			if conflict := fs.CheckOwnershipConflict(&state, entry.CatalogID, file.Path); conflict != nil {
				a.writeStdout(fmt.Sprintf("warning: ownership of %s transferred from %s to %s", file.Path, conflict.ExistingOwner, entry.CatalogID))
				fs.TransferOwnership(&state, entry.CatalogID, file.Path)
				transferred = true
			}
		}
		if transferred {
			if writeErr := fs.WriteInstalledStateV2(profile.TargetDir, state); writeErr != nil {
				a.writeStderr(fmt.Sprintf("warning: failed to persist ownership transfer: %v", writeErr))
			}
		}
	}

	if dryRun {
		return a.renderFanoutDryRun(projectRoot, profile.TargetDir, entry.CatalogID, plannedFiles, jsonOut)
	}

	a.progress.step("Installation des fichiers…")
	installedCount := 0
	for _, plan := range plans {
		result, matErr := writer.MaterializeArtifact(projectRoot, plan.tool, kind, entry.Name, entry.Lang, sources, fetch, true, true)
		if matErr != nil {
			return matErr
		}
		if result.Skipped {
			continue
		}
		files := make([]fs.ManagedFile, 0, len(result.Files))
		for _, file := range result.Files {
			files = append(files, fs.ManagedFile{Path: file.RelativePath, SHA256: file.SHA256})
		}
		previous, _, _ := fs.InstalledRecordV2For(profile.TargetDir, entry.CatalogID, plan.target.ID)
		record := fs.InstalledResourceRecordV2{
			ResourceName: entry.CatalogID,
			ProviderID:   providerID,
			InstalledAt:  time.Now().UTC().Format(time.RFC3339),
			ManagedFiles: files,
			TargetID:     plan.target.ID,
			ArtifactKind: kind,
		}
		if err := fs.SyncManagedFilesV2(profile.TargetDir, record, previous.ManagedFiles); err != nil {
			return err
		}
		installedCount++
	}

	a.progress.step("Finalisation…")
	a.writeStdout(fmt.Sprintf("Ressource '%s' (%s) installée sur %d cible(s).", entry.Name, kind, installedCount))
	return nil
}

func (a *app) renderFanoutDryRun(projectRoot, targetDir, catalogID string, incoming []fs.ManagedFile, jsonOut bool) error {
	ignorePats, _ := fs.ReadSTHIgnore(projectRoot)
	state, _ := fs.ReadInstalledStateV2(targetDir)
	merged := fs.InstalledResourceRecordV2{ResourceName: catalogID}
	for _, record := range state.Entries {
		if record.ResourceName == catalogID {
			merged.ManagedFiles = append(merged.ManagedFiles, record.ManagedFiles...)
		}
	}
	var existing *fs.InstalledResourceRecordV2
	if len(merged.ManagedFiles) > 0 {
		existing = &merged
	}
	changes := fs.ComputeDryRunChanges(existing, incoming, ignorePats)
	if jsonOut {
		data, err := fs.FormatDryRunJSON(changes)
		if err != nil {
			return err
		}
		a.writeStdout(string(data))
		return nil
	}
	a.writeStdout(fs.FormatDryRunText(changes))
	return nil
}

func (a *app) printSkillPlacement(entry fs.CatalogEntry, placement writer.SkillPlacement) {
	readers := invertReadsFrom(placement.ReadsFrom)
	lines := []string{fmt.Sprintf("Skill '%s' installé :", entry.Name)}
	for _, folder := range placement.Folders {
		lines = append(lines, fmt.Sprintf("  → %s/%s (lu par %s)", folder, entry.Name, strings.Join(readers[folder], ", ")))
	}
	if placement.NeedsCodexCfg {
		lines = append(lines, fmt.Sprintf("  → .codex/config.toml (skills.config → %s/%s/SKILL.md)", placement.CodexFolder, entry.Name))
	}
	lines = append(lines, "Prochaine étape : utilisez `sth update` pour rafraîchir les ressources suivies.")
	a.writeStdout(lines...)
}

func invertReadsFrom(readsFrom map[string]string) map[string][]string {
	out := map[string][]string{}
	kinds := make([]string, 0, len(readsFrom))
	for kind := range readsFrom {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	for _, kind := range kinds {
		out[readsFrom[kind]] = append(out[readsFrom[kind]], kind)
	}
	return out
}

func (a *app) updateFanout(profile *fs.ProjectProfile, cfg fs.ProjectConfigV2, dryRun, jsonOut, failOnChanges bool) error {
	state, err := fs.ReadInstalledStateV2(profile.TargetDir)
	if err != nil {
		return err
	}

	if failOnChanges {
		projectRoot, _ := filepath.Abs(".")
		results := fs.ComputeAllStatuses(state, cfg, nil, projectRoot)
		modified := make([]fs.StatusResult, 0)
		for _, result := range results {
			if result.Status == fs.ResourceStatusModifiedLocally {
				modified = append(modified, result)
			}
		}
		if len(modified) > 0 {
			a.writeStdout("Modified files detected (--fail-on-changes):")
			for _, result := range modified {
				for _, file := range result.ModifiedFiles {
					a.writeStdout(fmt.Sprintf("  %s — %s", result.ResourceName, file))
				}
			}
			return fmt.Errorf("%d resource(s) have local modifications", len(modified))
		}
	}

	installedNames := make([]string, 0, len(state.Entries))
	seen := make(map[string]bool)
	for _, record := range state.Entries {
		if seen[record.ResourceName] {
			continue
		}
		seen[record.ResourceName] = true
		installedNames = append(installedNames, record.ResourceName)
	}
	if len(installedNames) == 0 {
		a.writeStdout("Aucune ressource à mettre à jour.")
		return nil
	}

	var entries []fs.CatalogEntry
	if err := a.withAuthRecovery(context.Background(), func() error {
		var listErr error
		entries, listErr = a.client.ListCatalog(context.Background(), *profile)
		return listErr
	}); err != nil {
		return err
	}
	byID := make(map[string]fs.CatalogEntry, len(entries))
	for _, entry := range entries {
		byID[entry.CatalogID] = entry
	}

	providerID := "default"
	if len(cfg.Providers) > 0 {
		providerID = cfg.Providers[0].ID
	}

	failures := 0
	for _, name := range installedNames {
		if pinned := fs.PinnedVersion(cfg, name); pinned != "" {
			a.writeStdout(fmt.Sprintf("  [pinned at %s] %s", pinned, name))
			continue
		}
		entry, ok := byID[name]
		if !ok {
			a.writeStderr(fmt.Sprintf("Ressource '%s' absente du catalogue ; ignorée.", name))
			continue
		}
		if err := a.installFanout(*profile, cfg, entry, providerID, dryRun, jsonOut, false); err != nil {
			a.writeStderr(fmt.Sprintf("Échec de mise à jour pour '%s' : %v", name, err))
			failures++
			continue
		}
	}
	if failures > 0 {
		return fmt.Errorf("mise à jour incomplète : %d ressource(s) en échec", failures)
	}
	return nil
}
