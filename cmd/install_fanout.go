package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"sth/internal/fs"
	"sth/internal/writer"
)

func (a *app) installFanout(profile fs.ProjectProfile, cfg fs.ProjectConfigV2, entry fs.CatalogEntry, providerID, resolvedVersion string, dryRun, jsonOut, force bool) error {
	if validationErr := validateFanoutTargetRoots(cfg); validationErr != nil {
		return validationErr
	}
	projectRoot, err := filepath.Abs(".")
	if err != nil {
		return err
	}

	if artifactKindOf(entry) == "skill" {
		return a.installSkillFanout(profile, entry, providerID, resolvedVersion, projectRoot, enabledTargetKinds(cfg), dryRun, jsonOut, force)
	}
	return a.installArtifactFanout(profile, cfg, entry, providerID, resolvedVersion, projectRoot, artifactKindOf(entry), dryRun, jsonOut, force)
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

func (a *app) installSkillFanout(profile fs.ProjectProfile, entry fs.CatalogEntry, providerID, resolvedVersion, projectRoot string, kinds []string, dryRun, jsonOut, force bool) error {
	a.progress.step(fmt.Sprintf("Téléchargement de %s…", entry.Name))
	sources, fetch, err := a.resolveSkillSources(profile, entry)
	if err != nil {
		return err
	}
	if validationErr := validateSkillConsumerContracts(projectRoot, entry.Name, sources, kinds, fetch); validationErr != nil {
		return validationErr
	}

	placement := writer.PlanSkillPlacement(kinds)
	if len(placement.Folders) == 0 {
		return fmt.Errorf("aucune destination de skill pour les cibles sélectionnées")
	}

	plans := make([]fanoutSkillPlan, 0, len(placement.Folders))
	plannedFiles := make([]fs.ManagedFile, 0)
	plannedFileIndexes := make(map[string]int)
	for _, folder := range placement.Folders {
		target, ok := writer.WriterForSkillsFolder(folder)
		if !ok {
			continue
		}
		result, matErr := writer.MaterializeArtifact(projectRoot, target, "skill", entry.Name, entry.Lang, sources, fetch, true, false)
		if matErr != nil {
			return matErr
		}
		for _, file := range result.Files {
			var appendErr error
			plannedFiles, appendErr = appendFanoutManagedFile(plannedFiles, plannedFileIndexes, fs.ManagedFile{Path: file.RelativePath, SHA256: file.SHA256})
			if appendErr != nil {
				return appendErr
			}
		}
		plans = append(plans, fanoutSkillPlan{target: target})
	}
	if len(plans) == 0 {
		return fmt.Errorf("aucune destination de skill pour les cibles sélectionnées")
	}

	state, stateErr := fs.ReadInstalledStateV2(profile.TargetDir)
	if stateErr != nil {
		return stateErr
	}
	if !force {
		for _, file := range plannedFiles {
			if conflict := fs.CheckOwnershipConflict(&state, entry.CatalogID, file.Path); conflict != nil {
				return conflict
			}
		}
	}
	if dryRun {
		return a.renderFanoutDryRun(projectRoot, profile.TargetDir, entry.CatalogID, plannedFiles, nil, nil, jsonOut)
	}

	previous, _ := fanoutInstalledRecord(state, entry.CatalogID, "")
	transactionPaths := fanoutTransactionPaths(profile.TargetDir, plannedFiles, previous.ManagedFiles)
	transaction, transactionErr := newFanoutFileTransaction(projectRoot, transactionPaths)
	if transactionErr != nil {
		return transactionErr
	}

	a.progress.step("Installation des fichiers…")
	managedFiles := make([]fs.ManagedFile, 0, len(plannedFiles))
	for _, plan := range plans {
		result, matErr := writer.MaterializeArtifact(projectRoot, plan.target, "skill", entry.Name, entry.Lang, sources, fetch, true, true)
		if matErr != nil {
			return transaction.rollback(matErr)
		}
		for _, file := range result.Files {
			managedFiles = append(managedFiles, fs.ManagedFile{Path: file.RelativePath, SHA256: file.SHA256})
		}
	}

	a.progress.step("Finalisation…")
	effectiveVersion := resolvedVersion
	if effectiveVersion == "" {
		effectiveVersion = previous.ResolvedVersion
	}
	record := fs.InstalledResourceRecordV2{
		ResourceName:    entry.CatalogID,
		ProviderID:      providerID,
		ResolvedVersion: effectiveVersion,
		PreviousVersion: previous.ResolvedVersion,
		InstalledAt:     time.Now().UTC().Format(time.RFC3339),
		ManagedFiles:    managedFiles,
		SourcePath:      entry.SourcePath,
		ArtifactKind:    "skill",
	}
	recordPlan := fanoutRecordPlan{record: record, previous: previous}
	ownershipMessages := make([]string, 0)
	if force {
		ownershipMessages = transferAllFanoutOwnership(&state, entry.CatalogID, plannedFiles)
	}
	if removeErr := removeFanoutStaleFiles(projectRoot, []fanoutRecordPlan{recordPlan}); removeErr != nil {
		return transaction.rollback(removeErr)
	}
	upsertFanoutRecord(&state, record)
	if ensureErr := fs.EnsureTargetDir(profile.TargetDir); ensureErr != nil {
		return transaction.rollback(ensureErr)
	}
	if writeErr := fs.WriteInstalledStateV2(profile.TargetDir, state); writeErr != nil {
		return transaction.rollback(writeErr)
	}
	a.writeFanoutOwnershipMessages(ownershipMessages, jsonOut)

	if a.fanoutJSON != nil {
		a.fanoutJSON.results = append(a.fanoutJSON.results, updateJSONResult{
			ResourceName:    entry.CatalogID,
			Outcome:         a.fanoutJSON.outcome,
			ResolvedVersion: record.ResolvedVersion,
			PreviousVersion: record.PreviousVersion,
		})
		return nil
	}
	a.printSkillPlacement(entry, placement)
	return nil
}

type fanoutTargetPlan struct {
	target  fs.TargetEntry
	tool    writer.Target
	preview writer.MaterializeResult
}

type fanoutSkillPlan struct {
	target writer.Target
}

type fanoutRecordPlan struct {
	record   fs.InstalledResourceRecordV2
	previous fs.InstalledResourceRecordV2
}

type fanoutPathSnapshot struct {
	absolutePath string
	content      []byte
	mode         os.FileMode
	existed      bool
}

type fanoutFileTransaction struct {
	projectRoot         string
	snapshots           []fanoutPathSnapshot
	directoryPreexisted map[string]bool
}

func newFanoutFileTransaction(projectRoot string, relativePaths []string) (*fanoutFileTransaction, error) {
	absoluteRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return nil, err
	}
	transaction := &fanoutFileTransaction{
		projectRoot:         filepath.Clean(absoluteRoot),
		snapshots:           make([]fanoutPathSnapshot, 0, len(relativePaths)),
		directoryPreexisted: make(map[string]bool),
	}
	seenPaths := make(map[string]struct{}, len(relativePaths))
	for _, relativePath := range relativePaths {
		absolutePath, pathErr := fanoutProjectPath(transaction.projectRoot, relativePath)
		if pathErr != nil {
			return nil, pathErr
		}
		if _, seen := seenPaths[absolutePath]; seen {
			continue
		}
		seenPaths[absolutePath] = struct{}{}
		transaction.captureParentDirectories(absolutePath)
		snapshot, snapshotErr := snapshotFanoutPath(absolutePath)
		if snapshotErr != nil {
			return nil, snapshotErr
		}
		transaction.snapshots = append(transaction.snapshots, snapshot)
	}
	return transaction, nil
}

func fanoutProjectPath(projectRoot, relativePath string) (string, error) {
	cleanRelativePath := filepath.Clean(filepath.FromSlash(relativePath))
	if relativePath == "" || filepath.IsAbs(cleanRelativePath) || cleanRelativePath == ".." || strings.HasPrefix(cleanRelativePath, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("chemin transactionnel invalide : %s", relativePath)
	}
	absolutePath := filepath.Join(projectRoot, cleanRelativePath)
	relativeToRoot, err := filepath.Rel(projectRoot, absolutePath)
	if err != nil || relativeToRoot == ".." || strings.HasPrefix(relativeToRoot, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("chemin transactionnel hors projet : %s", relativePath)
	}
	resolvedProjectRoot, resolveErr := resolveFanoutExistingPath(projectRoot)
	if resolveErr != nil {
		return "", fmt.Errorf("résolution de la racine transactionnelle échouée : %w", resolveErr)
	}
	resolvedPath, resolveErr := resolveFanoutExistingPath(absolutePath)
	if resolveErr != nil {
		return "", fmt.Errorf("résolution du chemin transactionnel échouée : %w", resolveErr)
	}
	if !fanoutPathIsWithin(resolvedProjectRoot, resolvedPath) {
		return "", fmt.Errorf("chemin transactionnel redirigé hors projet : %s", relativePath)
	}
	return absolutePath, nil
}

func resolveFanoutExistingPath(path string) (string, error) {
	currentPath := filepath.Clean(path)
	missingSegments := make([]string, 0)
	for {
		_, err := os.Lstat(currentPath)
		if err == nil {
			resolvedPath, resolveErr := filepath.EvalSymlinks(currentPath)
			if resolveErr != nil {
				return "", resolveErr
			}
			for segmentIndex := len(missingSegments) - 1; segmentIndex >= 0; segmentIndex-- {
				resolvedPath = filepath.Join(resolvedPath, missingSegments[segmentIndex])
			}
			return filepath.Clean(resolvedPath), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parentPath := filepath.Dir(currentPath)
		if parentPath == currentPath {
			return "", err
		}
		missingSegments = append(missingSegments, filepath.Base(currentPath))
		currentPath = parentPath
	}
}

func fanoutPathIsWithin(rootPath, candidatePath string) bool {
	relativePath, err := filepath.Rel(rootPath, candidatePath)
	return err == nil && relativePath != ".." && !strings.HasPrefix(relativePath, ".."+string(filepath.Separator))
}

func snapshotFanoutPath(absolutePath string) (fanoutPathSnapshot, error) {
	fileInfo, err := os.Lstat(absolutePath)
	if os.IsNotExist(err) {
		return fanoutPathSnapshot{absolutePath: absolutePath}, nil
	}
	if err != nil {
		return fanoutPathSnapshot{}, fmt.Errorf("instantané de %s échoué : %w", absolutePath, err)
	}
	if fileInfo.Mode()&os.ModeSymlink != 0 {
		return fanoutPathSnapshot{}, fmt.Errorf("chemin symbolique non transactionnel : %s", absolutePath)
	}
	snapshot := fanoutPathSnapshot{absolutePath: absolutePath, mode: fileInfo.Mode(), existed: true}
	if fileInfo.IsDir() {
		return snapshot, nil
	}
	if !fileInfo.Mode().IsRegular() {
		return fanoutPathSnapshot{}, fmt.Errorf("type de fichier non transactionnel : %s", absolutePath)
	}
	content, readErr := os.ReadFile(absolutePath)
	if readErr != nil {
		return fanoutPathSnapshot{}, fmt.Errorf("lecture de l'instantané %s échouée : %w", absolutePath, readErr)
	}
	snapshot.content = content
	return snapshot, nil
}

func (transaction *fanoutFileTransaction) captureParentDirectories(absolutePath string) {
	for directory := filepath.Dir(absolutePath); directory != transaction.projectRoot; directory = filepath.Dir(directory) {
		if _, captured := transaction.directoryPreexisted[directory]; captured {
			continue
		}
		_, err := os.Stat(directory)
		transaction.directoryPreexisted[directory] = err == nil || !os.IsNotExist(err)
	}
}

func (transaction *fanoutFileTransaction) rollback(cause error) error {
	rollbackErrors := make([]string, 0)
	snapshots := append([]fanoutPathSnapshot(nil), transaction.snapshots...)
	sort.Slice(snapshots, func(leftIndex, rightIndex int) bool {
		return len(snapshots[leftIndex].absolutePath) > len(snapshots[rightIndex].absolutePath)
	})
	for _, snapshot := range snapshots {
		if snapshot.existed {
			continue
		}
		if removeErr := os.Remove(snapshot.absolutePath); removeErr != nil && !os.IsNotExist(removeErr) {
			rollbackErrors = append(rollbackErrors, fmt.Sprintf("suppression de %s : %v", snapshot.absolutePath, removeErr))
		}
	}
	for _, snapshot := range snapshots {
		if !snapshot.existed {
			continue
		}
		if restoreErr := restoreFanoutSnapshot(snapshot); restoreErr != nil {
			rollbackErrors = append(rollbackErrors, restoreErr.Error())
		}
	}
	directories := make([]string, 0, len(transaction.directoryPreexisted))
	for directory, preexisted := range transaction.directoryPreexisted {
		if !preexisted {
			directories = append(directories, directory)
		}
	}
	sort.Slice(directories, func(leftIndex, rightIndex int) bool {
		return len(directories[leftIndex]) > len(directories[rightIndex])
	})
	for _, directory := range directories {
		if removeErr := os.Remove(directory); removeErr != nil && !os.IsNotExist(removeErr) {
			rollbackErrors = append(rollbackErrors, fmt.Sprintf("suppression du dossier %s : %v", directory, removeErr))
		}
	}
	if len(rollbackErrors) == 0 {
		return cause
	}
	return fmt.Errorf("%w; rollback incomplet : %s", cause, strings.Join(rollbackErrors, "; "))
}

func restoreFanoutSnapshot(snapshot fanoutPathSnapshot) error {
	currentInfo, statErr := os.Lstat(snapshot.absolutePath)
	if statErr != nil && !os.IsNotExist(statErr) {
		return fmt.Errorf("inspection de %s pendant rollback : %w", snapshot.absolutePath, statErr)
	}
	if snapshot.mode.IsDir() {
		if statErr == nil && !currentInfo.IsDir() {
			if removeErr := os.Remove(snapshot.absolutePath); removeErr != nil {
				return fmt.Errorf("suppression de %s pendant rollback : %w", snapshot.absolutePath, removeErr)
			}
		}
		if mkdirErr := os.MkdirAll(snapshot.absolutePath, snapshot.mode.Perm()); mkdirErr != nil {
			return fmt.Errorf("restauration du dossier %s : %w", snapshot.absolutePath, mkdirErr)
		}
		if chmodErr := os.Chmod(snapshot.absolutePath, snapshot.mode.Perm()); chmodErr != nil {
			return fmt.Errorf("restauration des permissions de %s : %w", snapshot.absolutePath, chmodErr)
		}
		return nil
	}
	if statErr == nil && currentInfo.IsDir() {
		if removeErr := os.Remove(snapshot.absolutePath); removeErr != nil {
			return fmt.Errorf("suppression du dossier %s pendant rollback : %w", snapshot.absolutePath, removeErr)
		}
	}
	if mkdirErr := os.MkdirAll(filepath.Dir(snapshot.absolutePath), 0o755); mkdirErr != nil {
		return fmt.Errorf("restauration du dossier parent de %s : %w", snapshot.absolutePath, mkdirErr)
	}
	if writeErr := os.WriteFile(snapshot.absolutePath, snapshot.content, snapshot.mode.Perm()); writeErr != nil {
		return fmt.Errorf("restauration de %s : %w", snapshot.absolutePath, writeErr)
	}
	if chmodErr := os.Chmod(snapshot.absolutePath, snapshot.mode.Perm()); chmodErr != nil {
		return fmt.Errorf("restauration des permissions de %s : %w", snapshot.absolutePath, chmodErr)
	}
	return nil
}

func fanoutInstalledRecord(state fs.InstalledResourceStateV2, resourceName, targetID string) (fs.InstalledResourceRecordV2, bool) {
	legacyRecord := fs.InstalledResourceRecordV2{}
	hasLegacyRecord := false
	for _, record := range state.Entries {
		if record.ResourceName == resourceName && record.TargetID == targetID {
			return record, true
		}
		if targetID != "" && record.ResourceName == resourceName && record.TargetID == "" {
			legacyRecord = record
			hasLegacyRecord = true
		}
	}
	if hasLegacyRecord {
		return legacyRecord, true
	}
	return fs.InstalledResourceRecordV2{}, false
}

func fanoutTransactionPaths(targetDir string, plannedFiles, previousFiles []fs.ManagedFile) []string {
	paths := make([]string, 0, len(plannedFiles)+len(previousFiles)+1)
	for _, file := range plannedFiles {
		paths = append(paths, file.Path)
	}
	for _, file := range previousFiles {
		paths = append(paths, file.Path)
	}
	paths = append(paths, filepath.ToSlash(filepath.Join(targetDir, "sth-state-v2.json")))
	return paths
}

func upsertFanoutRecord(state *fs.InstalledResourceStateV2, record fs.InstalledResourceRecordV2) {
	legacyIndex := -1
	for index, existing := range state.Entries {
		if existing.ResourceName == record.ResourceName && existing.TargetID == record.TargetID {
			state.Entries[index] = record
			return
		}
		if record.TargetID != "" && existing.ResourceName == record.ResourceName && existing.TargetID == "" {
			legacyIndex = index
		}
	}
	if legacyIndex >= 0 {
		state.Entries[legacyIndex] = record
	} else {
		state.Entries = append(state.Entries, record)
	}
	sort.Slice(state.Entries, func(leftIndex, rightIndex int) bool {
		if state.Entries[leftIndex].ResourceName != state.Entries[rightIndex].ResourceName {
			return state.Entries[leftIndex].ResourceName < state.Entries[rightIndex].ResourceName
		}
		return state.Entries[leftIndex].TargetID < state.Entries[rightIndex].TargetID
	})
}

func removeFanoutStaleFiles(projectRoot string, plans []fanoutRecordPlan) error {
	keptPaths := make(map[string]struct{})
	for _, plan := range plans {
		for _, file := range plan.record.ManagedFiles {
			keptPaths[file.Path] = struct{}{}
		}
	}
	for _, plan := range plans {
		for _, previousFile := range plan.previous.ManagedFiles {
			if _, kept := keptPaths[previousFile.Path]; kept {
				continue
			}
			physicalFileKept := false
			for keptPath := range keptPaths {
				sameFile, sameFileErr := fanoutPathsReferToSameFile(projectRoot, previousFile.Path, keptPath)
				if sameFileErr != nil {
					return sameFileErr
				}
				if sameFile {
					physicalFileKept = true
					break
				}
			}
			if physicalFileKept {
				continue
			}
			absolutePath, pathErr := fanoutProjectPath(projectRoot, previousFile.Path)
			if pathErr != nil {
				return pathErr
			}
			if removeErr := os.Remove(absolutePath); removeErr != nil && !os.IsNotExist(removeErr) {
				return fmt.Errorf("suppression de %s échouée : %w", previousFile.Path, removeErr)
			}
		}
	}
	return nil
}

func fanoutPathsReferToSameFile(projectRoot, leftPath, rightPath string) (bool, error) {
	leftAbsolutePath, leftPathErr := fanoutProjectPath(projectRoot, leftPath)
	if leftPathErr != nil {
		return false, leftPathErr
	}
	rightAbsolutePath, rightPathErr := fanoutProjectPath(projectRoot, rightPath)
	if rightPathErr != nil {
		return false, rightPathErr
	}
	leftInfo, leftStatErr := os.Stat(leftAbsolutePath)
	if os.IsNotExist(leftStatErr) {
		return false, nil
	}
	if leftStatErr != nil {
		return false, fmt.Errorf("inspection de %s échouée : %w", leftPath, leftStatErr)
	}
	rightInfo, rightStatErr := os.Stat(rightAbsolutePath)
	if os.IsNotExist(rightStatErr) {
		return false, nil
	}
	if rightStatErr != nil {
		return false, fmt.Errorf("inspection de %s échouée : %w", rightPath, rightStatErr)
	}
	return os.SameFile(leftInfo, rightInfo), nil
}

func appendFanoutManagedFile(files []fs.ManagedFile, indexes map[string]int, incoming fs.ManagedFile) ([]fs.ManagedFile, error) {
	pathKey := strings.ToLower(filepath.ToSlash(filepath.Clean(filepath.FromSlash(incoming.Path))))
	if existingIndex, exists := indexes[pathKey]; exists {
		existing := files[existingIndex]
		if existing.Path != incoming.Path {
			return nil, fmt.Errorf("chemins incompatibles sur un système insensible à la casse : %s et %s", existing.Path, incoming.Path)
		}
		if existing.SHA256 != incoming.SHA256 {
			return nil, fmt.Errorf("contenus divergents planifiés pour %s", incoming.Path)
		}
		return files, nil
	}
	indexes[pathKey] = len(files)
	return append(files, incoming), nil
}

func (a *app) writeFanoutSkip(targetKind, reason string, jsonOut bool) {
	message := fmt.Sprintf("cible ignorée %s : %s", targetKind, reason)
	if jsonOut || a.fanoutJSON != nil {
		a.writeStderr(message)
		return
	}
	a.writeStdout("  ⤫ " + targetKind + " : " + reason)
}

func transferAllFanoutOwnership(state *fs.InstalledResourceStateV2, resourceName string, files []fs.ManagedFile) []string {
	messages := make([]string, 0)
	reportedTransfers := make(map[string]struct{})
	for _, file := range files {
		for {
			conflict := fs.CheckOwnershipConflict(state, resourceName, file.Path)
			if conflict == nil {
				break
			}
			transferKey := conflict.FilePath + "\x00" + conflict.ExistingOwner
			if _, reported := reportedTransfers[transferKey]; !reported {
				messages = append(messages, fmt.Sprintf("warning: ownership of %s transferred from %s to %s", file.Path, conflict.ExistingOwner, resourceName))
				reportedTransfers[transferKey] = struct{}{}
			}
			fs.TransferOwnership(state, resourceName, file.Path)
			for recordIndex := range state.Entries {
				record := &state.Entries[recordIndex]
				if record.ResourceName == conflict.ExistingOwner && len(record.ManagedFiles) == 0 {
					record.ManagedRegions = nil
				}
			}
		}
	}
	return messages
}

func (a *app) writeFanoutOwnershipMessages(messages []string, jsonOut bool) {
	if len(messages) == 0 {
		return
	}
	if jsonOut || a.fanoutJSON != nil {
		a.writeStderr(messages...)
		return
	}
	a.writeStdout(messages...)
}

func (a *app) installArtifactFanout(profile fs.ProjectProfile, cfg fs.ProjectConfigV2, entry fs.CatalogEntry, providerID, resolvedVersion, projectRoot, kind string, dryRun, jsonOut, force bool) error {
	sources := []string{entry.SourcePath}
	eligiblePlans := make([]fanoutTargetPlan, 0)
	for _, target := range fs.EffectiveTargets(cfg) {
		if !target.Enabled {
			continue
		}
		tool, ok := writer.WriterForKind(target.Kind)
		if !ok {
			a.writeStderr(fmt.Sprintf("warning: cible inconnue ignorée : %s", target.Kind))
			continue
		}
		capability, hasCapability := writer.CapabilityFor(target.Kind, kind)
		if hasCapability && !capability.Implemented {
			a.writeFanoutSkip(target.Kind, capability.Reason, jsonOut)
			continue
		}
		preliminaryPlan := tool.Plan(kind, entry.Name, entry.Lang, sources)
		if preliminaryPlan.Skipped {
			a.writeFanoutSkip(target.Kind, preliminaryPlan.Reason, jsonOut)
			continue
		}
		eligiblePlans = append(eligiblePlans, fanoutTargetPlan{target: target, tool: tool})
	}
	if len(eligiblePlans) == 0 {
		return fmt.Errorf("aucune cible compatible pour l'artefact %s (%s)", entry.Name, kind)
	}

	a.progress.step(fmt.Sprintf("Téléchargement de %s…", entry.Name))
	content, err := a.client.DownloadCatalogEntry(context.Background(), profile, entry)
	if err != nil {
		return err
	}
	fetch := func(string) ([]byte, error) { return content, nil }

	plans := make([]fanoutTargetPlan, 0)
	plannedFiles := make([]fs.ManagedFile, 0)
	plannedFileIndexes := make(map[string]int)
	for _, eligiblePlan := range eligiblePlans {
		preview, matErr := writer.MaterializeArtifact(projectRoot, eligiblePlan.tool, kind, entry.Name, entry.Lang, sources, fetch, true, false)
		if matErr != nil {
			return matErr
		}
		if preview.Skipped {
			a.writeFanoutSkip(eligiblePlan.target.Kind, preview.Reason, jsonOut)
			continue
		}
		for _, file := range preview.Files {
			var appendErr error
			plannedFiles, appendErr = appendFanoutManagedFile(plannedFiles, plannedFileIndexes, fs.ManagedFile{Path: file.RelativePath, SHA256: file.SHA256})
			if appendErr != nil {
				return appendErr
			}
		}
		plans = append(plans, fanoutTargetPlan{target: eligiblePlan.target, tool: eligiblePlan.tool, preview: preview})
	}
	if len(plans) == 0 {
		return fmt.Errorf("aucune cible compatible pour l'artefact %s (%s)", entry.Name, kind)
	}

	state, stateErr := fs.ReadInstalledStateV2(profile.TargetDir)
	if stateErr != nil {
		return stateErr
	}
	if !force {
		for _, file := range plannedFiles {
			if conflict := fs.CheckOwnershipConflict(&state, entry.CatalogID, file.Path); conflict != nil {
				return conflict
			}
		}
	}

	if dryRun {
		derivedCurrentFiles := []fs.ManagedFile(nil)
		derivedIncomingFiles := []fs.ManagedFile(nil)
		if kind == "instruction" {
			var regionErr error
			derivedCurrentFiles, derivedIncomingFiles, regionErr = instructionRegionDryRunFiles(projectRoot, state, plans, entry)
			if regionErr != nil {
				return regionErr
			}
		}
		return a.renderFanoutDryRun(projectRoot, profile.TargetDir, entry.CatalogID, plannedFiles, derivedCurrentFiles, derivedIncomingFiles, jsonOut)
	}

	previousRecords := make(map[string]fs.InstalledResourceRecordV2, len(plans))
	previousFiles := make([]fs.ManagedFile, 0)
	for _, plan := range plans {
		previous, _ := fanoutInstalledRecord(state, entry.CatalogID, plan.target.ID)
		previousRecords[plan.target.ID] = previous
		previousFiles = append(previousFiles, previous.ManagedFiles...)
	}
	transactionPaths := fanoutTransactionPaths(profile.TargetDir, plannedFiles, previousFiles)
	if kind == "instruction" {
		for _, definition := range instructionRegionDefinitionsForPlans(plans) {
			transactionPaths = append(transactionPaths, definition.path)
		}
	}
	transaction, transactionErr := newFanoutFileTransaction(projectRoot, transactionPaths)
	if transactionErr != nil {
		return transactionErr
	}

	a.progress.step("Installation des fichiers…")
	installedCount := 0
	recordPlans := make([]fanoutRecordPlan, 0, len(plans))
	for _, plan := range plans {
		result, matErr := writer.MaterializeArtifact(projectRoot, plan.tool, kind, entry.Name, entry.Lang, sources, fetch, true, true)
		if matErr != nil {
			return transaction.rollback(matErr)
		}
		if result.Skipped {
			continue
		}
		files := make([]fs.ManagedFile, 0, len(result.Files))
		for _, file := range result.Files {
			files = append(files, fs.ManagedFile{Path: file.RelativePath, SHA256: file.SHA256})
		}
		previous := previousRecords[plan.target.ID]
		effectiveVersion := resolvedVersion
		if effectiveVersion == "" {
			effectiveVersion = previous.ResolvedVersion
		}
		record := fs.InstalledResourceRecordV2{
			ResourceName:    entry.CatalogID,
			ProviderID:      providerID,
			ResolvedVersion: effectiveVersion,
			PreviousVersion: previous.ResolvedVersion,
			InstalledAt:     time.Now().UTC().Format(time.RFC3339),
			ManagedFiles:    files,
			ManagedRegions:  nil,
			SourcePath:      entry.SourcePath,
			TargetID:        plan.target.ID,
			ArtifactKind:    kind,
		}
		if kind == "instruction" {
			record.ManagedRegions = managedRegionForInstructionTarget(plan.target)
		}
		recordPlans = append(recordPlans, fanoutRecordPlan{record: record, previous: previous})
		installedCount++
	}

	a.progress.step("Finalisation…")
	ownershipMessages := make([]string, 0)
	if force {
		ownershipMessages = transferAllFanoutOwnership(&state, entry.CatalogID, plannedFiles)
	}
	if removeErr := removeFanoutStaleFiles(projectRoot, recordPlans); removeErr != nil {
		return transaction.rollback(removeErr)
	}
	for _, recordPlan := range recordPlans {
		upsertFanoutRecord(&state, recordPlan.record)
	}
	if kind == "instruction" {
		for _, definition := range instructionRegionDefinitionsForPlans(plans) {
			if regionErr := reconcileInstructionRegion(projectRoot, &state, definition); regionErr != nil {
				return transaction.rollback(regionErr)
			}
		}
	}
	if ensureErr := fs.EnsureTargetDir(profile.TargetDir); ensureErr != nil {
		return transaction.rollback(ensureErr)
	}
	if writeErr := fs.WriteInstalledStateV2(profile.TargetDir, state); writeErr != nil {
		return transaction.rollback(writeErr)
	}
	a.writeFanoutOwnershipMessages(ownershipMessages, jsonOut)
	if a.fanoutJSON != nil {
		a.fanoutJSON.results = append(a.fanoutJSON.results, updateJSONResult{
			ResourceName:    entry.CatalogID,
			Outcome:         a.fanoutJSON.outcome,
			ResolvedVersion: resolvedVersion,
		})
		return nil
	}
	a.writeStdout(fmt.Sprintf("Ressource '%s' (%s) installée sur %d cible(s).", entry.Name, kind, installedCount))
	return nil
}

func (a *app) renderFanoutDryRun(projectRoot, targetDir, catalogID string, incoming, derivedCurrent, derivedIncoming []fs.ManagedFile, jsonOut bool) error {
	ignorePats, _ := fs.ReadSTHIgnore(projectRoot)
	state, _ := fs.ReadInstalledStateV2(targetDir)
	merged := fs.InstalledResourceRecordV2{ResourceName: catalogID}
	for _, record := range state.Entries {
		if record.ResourceName == catalogID {
			merged.ManagedFiles = append(merged.ManagedFiles, record.ManagedFiles...)
		}
	}
	merged.ManagedFiles = append(merged.ManagedFiles, derivedCurrent...)
	incoming = append(append([]fs.ManagedFile(nil), incoming...), derivedIncoming...)
	var existing *fs.InstalledResourceRecordV2
	if len(merged.ManagedFiles) > 0 {
		existing = &merged
	}
	changes := fs.ComputeDryRunChanges(existing, incoming, ignorePats)
	if a.fanoutJSON != nil {
		for index := range changes {
			changes[index].ResourceName = catalogID
		}
		a.fanoutJSON.changes = append(a.fanoutJSON.changes, changes...)
		return nil
	}
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
		if jsonOut {
			emitJSONError(a.stdout, err, 1)
		}
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
		if jsonOut {
			a.writeStdout("[]")
		} else {
			a.writeStdout("Aucune ressource à mettre à jour.")
		}
		return nil
	}

	var entries []fs.CatalogEntry
	if err := a.withAuthRecovery(context.Background(), func() error {
		var listErr error
		entries, listErr = a.client.ListCatalog(context.Background(), *profile)
		return listErr
	}); err != nil {
		if jsonOut {
			emitJSONError(a.stdout, err, 1)
		}
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

	var accumulator *fanoutJSONAccumulator
	if jsonOut {
		accumulator = &fanoutJSONAccumulator{outcome: "updated"}
		a.fanoutJSON = accumulator
		defer func() { a.fanoutJSON = nil }()
	}

	failures := 0
	latestCache := make(map[string]string)
	for _, name := range installedNames {
		if pinned := fs.PinnedVersion(cfg, name); pinned != "" {
			if accumulator != nil {
				accumulator.results = append(accumulator.results, updateJSONResult{ResourceName: name, Outcome: "pinned", PreviousVersion: pinned})
				continue
			}
			a.writeStdout(fmt.Sprintf("  [pinned at %s] %s", pinned, name))
			continue
		}
		entry, ok := byID[name]
		if !ok {
			if accumulator != nil {
				accumulator.results = append(accumulator.results, updateJSONResult{ResourceName: name, Outcome: "skipped", Error: "ressource absente du catalogue"})
				continue
			}
			a.writeStderr(fmt.Sprintf("Ressource '%s' absente du catalogue ; ignorée.", name))
			continue
		}
		resolvedVersion := ""
		if !dryRun {
			resolvedVersion = a.resolveCommitForEntry(context.Background(), cfg, providerID, entry, latestCache)
		}
		if err := a.installFanout(*profile, cfg, entry, providerID, resolvedVersion, dryRun, jsonOut, false); err != nil {
			failures++
			if accumulator != nil {
				accumulator.results = append(accumulator.results, updateJSONResult{ResourceName: name, Outcome: "failed", Error: err.Error()})
				continue
			}
			a.writeStderr(fmt.Sprintf("Échec de mise à jour pour '%s' : %v", name, err))
			continue
		}
	}

	if accumulator != nil {
		if dryRun {
			data, marshalErr := fs.FormatDryRunJSON(accumulator.changes)
			if marshalErr != nil {
				return marshalErr
			}
			a.writeStdout(string(data))
		} else {
			payload, marshalErr := marshalUpdateResults(accumulator.results)
			if marshalErr != nil {
				return marshalErr
			}
			a.writeStdout(payload)
		}
	}

	if failures > 0 {
		return fmt.Errorf("mise à jour incomplète : %d ressource(s) en échec", failures)
	}
	return nil
}
