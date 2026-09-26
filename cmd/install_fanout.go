package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"sth/internal/artifact"
	"sth/internal/fs"
	"sth/internal/writer"
)

func (a *app) installFanout(profile fs.ProjectProfile, stateDir string, cfg fs.ProjectConfigV2, entry fs.CatalogEntry, providerID, resolvedVersion string, dryRun, jsonOut, force bool) error {
	filteredTargets := make([]fs.TargetEntry, 0, len(cfg.Targets))
	for _, target := range fs.EffectiveTargets(cfg) {
		if target.Enabled && a.targetSelected(target.ID, target.Kind) {
			filteredTargets = append(filteredTargets, target)
		}
	}
	if len(filteredTargets) == 0 {
		return fmt.Errorf("no configured target matched --target")
	}
	cfg.Targets = filteredTargets
	if validationErr := validateFanoutTargetRoots(cfg); validationErr != nil {
		return validationErr
	}
	projectRoot, err := filepath.Abs(".")
	if err != nil {
		return err
	}

	if artifactKindOf(entry) == "skill" {
		return a.installSkillFanout(profile, stateDir, entry, providerID, resolvedVersion, projectRoot, enabledTargetKinds(cfg), dryRun, jsonOut, force)
	}
	return a.installArtifactFanout(profile, stateDir, cfg, entry, providerID, resolvedVersion, projectRoot, artifactKindOf(entry), dryRun, jsonOut, force)
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
				return nil, fmt.Errorf("missing bundle file: %s", sourcePath)
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

func (a *app) installSkillFanout(profile fs.ProjectProfile, stateDir string, entry fs.CatalogEntry, providerID, resolvedVersion, projectRoot string, kinds []string, dryRun, jsonOut, force bool) error {
	a.progress.step(fmt.Sprintf("Downloading %s…", entry.Name))
	sources, fetch, err := a.resolveSkillSources(profile, entry)
	if err != nil {
		return err
	}
	if validationErr := validateSkillConsumerContracts(projectRoot, entry.Name, sources, kinds, fetch); validationErr != nil {
		return validationErr
	}
	sourceFiles := make([]artifact.File, 0, len(sources))
	for _, sourcePath := range sources {
		content, fetchErr := fetch(sourcePath)
		if fetchErr != nil {
			return fetchErr
		}
		sourceFiles = append(sourceFiles, artifact.File{Path: sourcePath, Content: content})
	}
	sourceSHA256 := artifact.HashFiles(sourceFiles)

	placement := writer.PlanSkillPlacement(kinds)
	if len(placement.Folders) == 0 {
		return fmt.Errorf("no skill destination for the selected targets")
	}

	plans := make([]fanoutSkillPlan, 0, len(placement.Folders))
	plannedFiles := make([]fs.ManagedFile, 0)
	plannedFileIndexes := make(map[string]int)
	for _, folder := range placement.Folders {
		target, ok := writer.WriterForSkillsFolder(folder)
		if !ok {
			continue
		}
		result, matErr := writer.MaterializeArtifact(projectRoot, target, "skill", entry.Name, entry.Lang, sources, fetch, false, false)
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
		return fmt.Errorf("no skill destination for the selected targets")
	}

	state, stateErr := fs.ReadInstalledStateV2(stateDir)
	if stateErr != nil {
		return stateErr
	}
	if !dryRun {
		if driftErr := refuseModifiedResources(state, fs.ProjectConfigV2{}, entry.CatalogID); driftErr != nil {
			return driftErr
		}
	}
	if !force {
		for _, file := range plannedFiles {
			if conflict := fs.CheckOwnershipConflict(&state, entry.CatalogID, file.Path); conflict != nil {
				return conflict
			}
		}
	}
	if dryRun {
		return a.renderFanoutDryRun(projectRoot, stateDir, entry.CatalogID, plannedFiles, nil, nil, jsonOut)
	}

	previousConsumers, _ := fanoutInstalledRecord(state, entry.CatalogID, "")
	if slices.Equal(previousConsumers.Consumers, kinds) && fanoutFilesUnchanged(state, entry.CatalogID, providerID, resolvedVersion, projectRoot, plannedFiles) {
		a.reportUnchanged(entry.CatalogID, resolvedVersion)
		return nil
	}
	previous, _ := fanoutInstalledRecord(state, entry.CatalogID, "")
	transactionPaths := fanoutTransactionPaths(stateDir, plannedFiles, previous.ManagedFiles)
	transaction, transactionErr := newFanoutFileTransaction(projectRoot, transactionPaths)
	if transactionErr != nil {
		return transactionErr
	}

	a.progress.step("Installing files…")
	managedFiles := make([]fs.ManagedFile, 0, len(plannedFiles))
	for _, plan := range plans {
		result, matErr := writer.MaterializeArtifact(projectRoot, plan.target, "skill", entry.Name, entry.Lang, sources, fetch, false, true)
		if matErr != nil {
			return transaction.rollback(matErr)
		}
		for _, file := range result.Files {
			managedFiles = append(managedFiles, fs.ManagedFile{Path: file.RelativePath, SHA256: file.SHA256})
		}
	}

	a.progress.step("Finalizing…")
	effectiveVersion := resolvedVersion
	if effectiveVersion == "" {
		effectiveVersion = previous.ResolvedVersion
	}
	record := fs.InstalledResourceRecordV2{
		ResourceName:        entry.CatalogID,
		ProviderID:          providerID,
		ResolvedVersion:     effectiveVersion,
		PreviousVersion:     previous.ResolvedVersion,
		InstalledAt:         time.Now().UTC().Format(time.RFC3339),
		ManagedFiles:        managedFiles,
		SourcePath:          entry.SourcePath,
		ArtifactKind:        "skill",
		SourceDialect:       string(artifact.DialectAgentSkills),
		SourceSHA256:        sourceSHA256,
		TargetDialect:       string(artifact.DialectAgentSkills),
		Compatibility:       string(artifact.CompatibilityExact),
		CompatibilityReport: artifact.CompatibilityReport{Preserved: []string{"name", "body", "files"}},
		TransformerVersion:  "1",
		Consumers:           append([]string(nil), kinds...),
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
	if ensureErr := fs.EnsureTargetDir(stateDir); ensureErr != nil {
		return transaction.rollback(ensureErr)
	}
	if writeErr := fs.WriteInstalledStateV2(stateDir, state); writeErr != nil {
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
	target           fs.TargetEntry
	tool             writer.Target
	preview          writer.MaterializeResult
	compatibility    artifact.CompatibilityLevel
	report           artifact.CompatibilityReport
	addManagedHeader bool
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
		return "", fmt.Errorf("invalid transactional path: %s", relativePath)
	}
	absolutePath := filepath.Join(projectRoot, cleanRelativePath)
	relativeToRoot, err := filepath.Rel(projectRoot, absolutePath)
	if err != nil || relativeToRoot == ".." || strings.HasPrefix(relativeToRoot, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("transactional path outside project: %s", relativePath)
	}
	resolvedProjectRoot, resolveErr := resolveFanoutExistingPath(projectRoot)
	if resolveErr != nil {
		return "", fmt.Errorf("failed to resolve transactional root: %w", resolveErr)
	}
	resolvedPath, resolveErr := resolveFanoutExistingPath(absolutePath)
	if resolveErr != nil {
		return "", fmt.Errorf("failed to resolve transactional path: %w", resolveErr)
	}
	if !fanoutPathIsWithin(resolvedProjectRoot, resolvedPath) {
		return "", fmt.Errorf("transactional path redirected outside project: %s", relativePath)
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
		return fanoutPathSnapshot{}, fmt.Errorf("failed to snapshot %s: %w", absolutePath, err)
	}
	if fileInfo.Mode()&os.ModeSymlink != 0 {
		return fanoutPathSnapshot{}, fmt.Errorf("non-transactional symbolic link: %s", absolutePath)
	}
	snapshot := fanoutPathSnapshot{absolutePath: absolutePath, mode: fileInfo.Mode(), existed: true}
	if fileInfo.IsDir() {
		return snapshot, nil
	}
	if !fileInfo.Mode().IsRegular() {
		return fanoutPathSnapshot{}, fmt.Errorf("non-transactional file type: %s", absolutePath)
	}
	content, readErr := os.ReadFile(absolutePath)
	if readErr != nil {
		return fanoutPathSnapshot{}, fmt.Errorf("failed to read snapshot %s: %w", absolutePath, readErr)
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
			rollbackErrors = append(rollbackErrors, fmt.Sprintf("removing %s: %v", snapshot.absolutePath, removeErr))
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
			rollbackErrors = append(rollbackErrors, fmt.Sprintf("removing directory %s: %v", directory, removeErr))
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
		return fmt.Errorf("failed to inspect %s during rollback: %w", snapshot.absolutePath, statErr)
	}
	if snapshot.mode.IsDir() {
		if statErr == nil && !currentInfo.IsDir() {
			if removeErr := os.Remove(snapshot.absolutePath); removeErr != nil {
				return fmt.Errorf("failed to remove %s during rollback: %w", snapshot.absolutePath, removeErr)
			}
		}
		if mkdirErr := os.MkdirAll(snapshot.absolutePath, snapshot.mode.Perm()); mkdirErr != nil {
			return fmt.Errorf("restoring directory %s: %w", snapshot.absolutePath, mkdirErr)
		}
		if chmodErr := os.Chmod(snapshot.absolutePath, snapshot.mode.Perm()); chmodErr != nil {
			return fmt.Errorf("restoring permissions for %s: %w", snapshot.absolutePath, chmodErr)
		}
		return nil
	}
	if statErr == nil && currentInfo.IsDir() {
		if removeErr := os.Remove(snapshot.absolutePath); removeErr != nil {
			return fmt.Errorf("failed to remove directory %s during rollback: %w", snapshot.absolutePath, removeErr)
		}
	}
	if mkdirErr := os.MkdirAll(filepath.Dir(snapshot.absolutePath), 0o755); mkdirErr != nil {
		return fmt.Errorf("restoring parent directory of %s: %w", snapshot.absolutePath, mkdirErr)
	}
	if writeErr := os.WriteFile(snapshot.absolutePath, snapshot.content, snapshot.mode.Perm()); writeErr != nil {
		return fmt.Errorf("restoring %s: %w", snapshot.absolutePath, writeErr)
	}
	if chmodErr := os.Chmod(snapshot.absolutePath, snapshot.mode.Perm()); chmodErr != nil {
		return fmt.Errorf("restoring permissions for %s: %w", snapshot.absolutePath, chmodErr)
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
	paths := make([]string, 0, len(plannedFiles)+len(previousFiles)+2)
	for _, file := range plannedFiles {
		paths = append(paths, file.Path)
	}
	for _, file := range previousFiles {
		paths = append(paths, file.Path)
	}
	paths = append(paths, filepath.ToSlash(filepath.Join(targetDir, "sth-state-v2.json")))
	paths = append(paths, filepath.ToSlash(filepath.Join(".sth", fs.StateFileNameV3)))
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
				return fmt.Errorf("failed to remove %s: %w", previousFile.Path, removeErr)
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
		return false, fmt.Errorf("failed to inspect %s: %w", leftPath, leftStatErr)
	}
	rightInfo, rightStatErr := os.Stat(rightAbsolutePath)
	if os.IsNotExist(rightStatErr) {
		return false, nil
	}
	if rightStatErr != nil {
		return false, fmt.Errorf("failed to inspect %s: %w", rightPath, rightStatErr)
	}
	return os.SameFile(leftInfo, rightInfo), nil
}

func appendFanoutManagedFile(files []fs.ManagedFile, indexes map[string]int, incoming fs.ManagedFile) ([]fs.ManagedFile, error) {
	pathKey := strings.ToLower(filepath.ToSlash(filepath.Clean(filepath.FromSlash(incoming.Path))))
	if existingIndex, exists := indexes[pathKey]; exists {
		existing := files[existingIndex]
		if existing.Path != incoming.Path {
			return nil, fmt.Errorf("incompatible paths on a case-insensitive system: %s and %s", existing.Path, incoming.Path)
		}
		if existing.SHA256 != incoming.SHA256 {
			return nil, fmt.Errorf("conflicting content planned for %s", incoming.Path)
		}
		return files, nil
	}
	indexes[pathKey] = len(files)
	return append(files, incoming), nil
}

func (a *app) writeFanoutSkip(targetKind, reason string, jsonOut bool) {
	message := fmt.Sprintf("skipped target %s: %s", targetKind, reason)
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

func (a *app) installArtifactFanout(profile fs.ProjectProfile, stateDir string, cfg fs.ProjectConfigV2, entry fs.CatalogEntry, providerID, resolvedVersion, projectRoot, kind string, dryRun, jsonOut, force bool) error {
	sources := []string{entry.SourcePath}
	eligiblePlans := make([]fanoutTargetPlan, 0)
	for _, target := range fs.EffectiveTargets(cfg) {
		if !target.Enabled {
			continue
		}
		tool, ok := writer.WriterForKind(target.Kind)
		if !ok {
			a.writeStderr(fmt.Sprintf("warning: skipped unknown target: %s", target.Kind))
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
		eligiblePlans = append(eligiblePlans, fanoutTargetPlan{target: target, tool: tool, compatibility: capability.Level, addManagedHeader: capability.Level != artifact.CompatibilityExact})
	}
	if len(eligiblePlans) == 0 {
		return fmt.Errorf("no compatible target for artifact %s (%s)", entry.Name, kind)
	}

	a.progress.step(fmt.Sprintf("Downloading %s…", entry.Name))
	content, err := a.client.DownloadCatalogEntry(context.Background(), profile, entry)
	if err != nil {
		return err
	}
	definition := artifact.DefinitionFromContent(artifact.Kind(kind), entry.Name, entry.SourcePath, content)
	if definition.Common.Description == "" {
		definition.Common.Description = entry.Description
	}
	diagnostics := writer.ValidateForKind(eligiblePlans[0].target.Kind, definition)
	for _, diagnostic := range diagnostics {
		a.writeStderr(fmt.Sprintf("%s %s %s: %s", diagnostic.Severity, diagnostic.Path, diagnostic.Code, diagnostic.Message))
	}
	if artifact.HasErrors(diagnostics, false) {
		return fmt.Errorf("artifact validation failed for %s", entry.Name)
	}
	approvedPlans := make([]fanoutTargetPlan, 0, len(eligiblePlans))
	for _, eligiblePlan := range eligiblePlans {
		compatibilityPlan := writer.PlanCompatibility(definition, eligiblePlan.target.Kind)
		capability, _ := writer.CapabilityFor(eligiblePlan.target.Kind, kind)
		if !a.allowInstallPlan(compatibilityPlan, capability, eligiblePlan.target.Kind, entry.SourcePath, jsonOut) {
			a.writeFanoutSkip(eligiblePlan.target.Kind, fmt.Sprintf("compatibility %s requires explicit approval", compatibilityPlan.Level), jsonOut)
			continue
		}
		eligiblePlan.compatibility = compatibilityPlan.Level
		eligiblePlan.report = compatibilityPlan.Report
		eligiblePlan.addManagedHeader = compatibilityPlan.Level != artifact.CompatibilityExact
		approvedPlans = append(approvedPlans, eligiblePlan)
	}
	eligiblePlans = approvedPlans
	if len(eligiblePlans) == 0 {
		return fmt.Errorf("no compatible target for artifact %s (%s)", entry.Name, kind)
	}
	fetch := func(string) ([]byte, error) { return content, nil }

	plans := make([]fanoutTargetPlan, 0)
	plannedFiles := make([]fs.ManagedFile, 0)
	plannedFileIndexes := make(map[string]int)
	for _, eligiblePlan := range eligiblePlans {
		preview, matErr := writer.MaterializeArtifact(projectRoot, eligiblePlan.tool, kind, entry.Name, entry.Lang, sources, fetch, eligiblePlan.addManagedHeader, false)
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
		preview.Compatibility = eligiblePlan.compatibility
		plans = append(plans, fanoutTargetPlan{target: eligiblePlan.target, tool: eligiblePlan.tool, preview: preview, compatibility: eligiblePlan.compatibility, addManagedHeader: eligiblePlan.addManagedHeader})
	}
	if len(plans) == 0 {
		return fmt.Errorf("no compatible target for artifact %s (%s)", entry.Name, kind)
	}

	state, stateErr := fs.ReadInstalledStateV2(stateDir)
	if stateErr != nil {
		return stateErr
	}
	if !dryRun {
		if driftErr := refuseModifiedResources(state, cfg, entry.CatalogID); driftErr != nil {
			return driftErr
		}
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
		return a.renderFanoutDryRun(projectRoot, stateDir, entry.CatalogID, plannedFiles, derivedCurrentFiles, derivedIncomingFiles, jsonOut)
	}

	if fanoutFilesUnchanged(state, entry.CatalogID, providerID, resolvedVersion, projectRoot, plannedFiles) {
		a.reportUnchanged(entry.CatalogID, resolvedVersion)
		return nil
	}
	previousRecords := make(map[string]fs.InstalledResourceRecordV2, len(plans))
	previousFiles := make([]fs.ManagedFile, 0)
	for _, plan := range plans {
		previous, _ := fanoutInstalledRecord(state, entry.CatalogID, plan.target.ID)
		previousRecords[plan.target.ID] = previous
		previousFiles = append(previousFiles, previous.ManagedFiles...)
	}
	transactionPaths := fanoutTransactionPaths(stateDir, plannedFiles, previousFiles)
	if kind == "instruction" {
		for _, definition := range instructionRegionDefinitionsForPlans(plans) {
			transactionPaths = append(transactionPaths, definition.path)
		}
	}
	transaction, transactionErr := newFanoutFileTransaction(projectRoot, transactionPaths)
	if transactionErr != nil {
		return transactionErr
	}

	a.progress.step("Installing files…")
	installedCount := 0
	recordPlans := make([]fanoutRecordPlan, 0, len(plans))
	for _, plan := range plans {
		result, matErr := writer.MaterializeArtifact(projectRoot, plan.tool, kind, entry.Name, entry.Lang, sources, fetch, plan.addManagedHeader, true)
		if matErr != nil {
			return transaction.rollback(matErr)
		}
		if result.Skipped {
			continue
		}
		result.Compatibility = plan.compatibility
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
			ResourceName:        entry.CatalogID,
			ProviderID:          providerID,
			ResolvedVersion:     effectiveVersion,
			PreviousVersion:     previous.ResolvedVersion,
			InstalledAt:         time.Now().UTC().Format(time.RFC3339),
			ManagedFiles:        files,
			ManagedRegions:      nil,
			SourcePath:          entry.SourcePath,
			TargetID:            plan.target.ID,
			TargetKind:          plan.target.Kind,
			ArtifactKind:        kind,
			SourceDialect:       "sth.catalog.v1",
			SourceSHA256:        fs.ComputeSHA256(content),
			TargetDialect:       string(writer.DialectForTargetKind(plan.target.Kind, artifact.Kind(kind))),
			Compatibility:       string(result.Compatibility),
			CompatibilityReport: plan.report,
			TransformerVersion:  "1",
		}
		if kind == "instruction" {
			record.ManagedRegions = managedRegionForInstructionTarget(plan.target)
		}
		recordPlans = append(recordPlans, fanoutRecordPlan{record: record, previous: previous})
		installedCount++
	}

	a.progress.step("Finalizing…")
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
	if ensureErr := fs.EnsureTargetDir(stateDir); ensureErr != nil {
		return transaction.rollback(ensureErr)
	}
	if writeErr := fs.WriteInstalledStateV2(stateDir, state); writeErr != nil {
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
	a.writeStdout(fmt.Sprintf("Resource '%s' (%s) installed on %d target(s).", entry.Name, kind, installedCount))
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
	lines := []string{fmt.Sprintf("Skill '%s' installed:", entry.Name)}
	for _, folder := range placement.Folders {
		lines = append(lines, fmt.Sprintf("  → %s/%s (read by %s)", folder, entry.Name, strings.Join(readers[folder], ", ")))
	}
	lines = append(lines, "Next step: use `sth update` to refresh tracked resources.")
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

type updateGroup struct {
	stateDir   string
	providerID string
	names      []string
}

func updateStateDirs(cfg fs.ProjectConfigV2, primaryStateDir string) []string {
	dirs := make([]string, 0, len(cfg.Providers)+1)
	seen := make(map[string]struct{})
	for _, candidate := range append([]string{primaryStateDir}, fs.StateTargetDirs(cfg)...) {
		cleaned := filepath.Clean(strings.TrimSpace(candidate))
		if cleaned == "" || cleaned == "." {
			continue
		}
		if _, exists := seen[cleaned]; exists {
			continue
		}
		seen[cleaned] = struct{}{}
		dirs = append(dirs, cleaned)
	}
	return dirs
}

func groupUpdateTargets(cfg fs.ProjectConfigV2, stateDirs []string, states map[string]fs.InstalledResourceStateV2) []updateGroup {
	defaultProviderID := fs.DefaultProviderID(cfg)
	groups := make([]updateGroup, 0)
	indexes := make(map[string]int)
	seenNames := make(map[string]struct{})
	for _, stateDir := range stateDirs {
		for _, record := range states[stateDir].Entries {
			providerID := strings.TrimSpace(record.ProviderID)
			if providerID == "" {
				providerID = defaultProviderID
			}
			nameKey := stateDir + "\x00" + providerID + "\x00" + record.ResourceName
			if _, exists := seenNames[nameKey]; exists {
				continue
			}
			seenNames[nameKey] = struct{}{}
			groupKey := stateDir + "\x00" + providerID
			groupIndex, exists := indexes[groupKey]
			if !exists {
				groups = append(groups, updateGroup{stateDir: stateDir, providerID: providerID})
				groupIndex = len(groups) - 1
				indexes[groupKey] = groupIndex
			}
			groups[groupIndex].names = append(groups[groupIndex].names, record.ResourceName)
		}
	}
	return groups
}

func (a *app) reportUpdateFailure(accumulator *fanoutJSONAccumulator, name, outcome string, cause error) {
	if accumulator != nil {
		accumulator.results = append(accumulator.results, updateJSONResult{ResourceName: name, Outcome: outcome, Error: cause.Error()})
		return
	}
	a.writeStderr(fmt.Sprintf("Failed to update '%s': %v", name, cause))
}

func (a *app) updateFanout(primaryProfile *fs.ProjectProfile, cfg fs.ProjectConfigV2, dryRun, jsonOut, failOnChanges bool) error {
	stateDirs := updateStateDirs(cfg, primaryProfile.TargetDir)
	states := make(map[string]fs.InstalledResourceStateV2, len(stateDirs))
	for _, stateDir := range stateDirs {
		state, err := fs.ReadInstalledStateV2(stateDir)
		if err != nil {
			if jsonOut {
				emitJSONError(a.stdout, err, 1)
			}
			return err
		}
		states[stateDir] = state
		if !dryRun {
			if err := refuseModifiedResources(state, cfg, ""); err != nil {
				if jsonOut {
					emitJSONError(a.stdout, err, 1)
				}
				return err
			}
		}
		if dryRun {
			continue
		}
		release, lockErr := a.prepareProviderStateAccess(stateDir, primaryProfile.TargetDir)
		if lockErr != nil {
			if jsonOut {
				emitJSONError(a.stdout, lockErr, 1)
			}
			return lockErr
		}
		defer release()
	}

	if failOnChanges {
		projectRoot, _ := filepath.Abs(".")
		modified := make([]fs.StatusResult, 0)
		for _, stateDir := range stateDirs {
			for _, result := range fs.ComputeAllStatuses(states[stateDir], cfg, nil, projectRoot) {
				if result.Status == fs.ResourceStatusModifiedLocally {
					modified = append(modified, result)
				}
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

	groups := groupUpdateTargets(cfg, stateDirs, states)
	updatableCount := 0
	for _, group := range groups {
		for _, name := range group.names {
			if fs.PinnedVersion(cfg, name) == "" {
				updatableCount++
			}
		}
	}
	if len(groups) == 0 {
		if jsonOut {
			a.writeStdout("[]")
		} else {
			a.writeStdout("No resources to update.")
		}
		return nil
	}

	var accumulator *fanoutJSONAccumulator
	if jsonOut {
		accumulator = &fanoutJSONAccumulator{outcome: "updated"}
		a.fanoutJSON = accumulator
		defer func() { a.fanoutJSON = nil }()
	}

	if !dryRun && !jsonOut && updatableCount > 0 {
		a.progress.start(updatableCount * 3)
	}

	failures := 0
	for _, group := range groups {
		profile, profileErr := a.sourceProfile(cfg, group.providerID)
		if profileErr != nil {
			failures += len(group.names)
			for _, name := range group.names {
				a.reportUpdateFailure(accumulator, name, "failed", profileErr)
			}
			continue
		}

		resolvedProfile, resolveErr := a.client.ResolveGitHubProfile(context.Background(), profile)
		if resolveErr != nil {
			failures += len(group.names)
			for _, name := range group.names {
				a.reportUpdateFailure(accumulator, name, "failed", fmt.Errorf("unable to resolve update source: %w", resolveErr))
			}
			continue
		}
		profile = resolvedProfile
		groupConfig := cfg
		groupConfig.Providers = append([]fs.ProviderEntry(nil), cfg.Providers...)
		if strings.EqualFold(profile.Provider, "github") && len(profile.Ref) == 40 {
			for providerIndex := range groupConfig.Providers {
				if groupConfig.Providers[providerIndex].ID == group.providerID {
					groupConfig.Providers[providerIndex].CatalogRef = profile.Ref
				}
			}
		}

		var entries []fs.CatalogEntry
		listErr := a.withAuthRecovery(context.Background(), func() error {
			var innerErr error
			entries, innerErr = a.client.ListCatalog(context.Background(), profile)
			return innerErr
		})
		if listErr != nil {
			failures += len(group.names)
			for _, name := range group.names {
				a.reportUpdateFailure(accumulator, name, "failed", fmt.Errorf("provider %s unavailable: %w", group.providerID, listErr))
			}
			continue
		}
		byID := make(map[string]fs.CatalogEntry, len(entries))
		for _, entry := range entries {
			entry.ProviderID = group.providerID
			byID[entry.CatalogID] = entry
		}

		latestCache := make(map[string]string)
		for _, name := range group.names {
			if pinned := fs.PinnedVersion(cfg, name); pinned != "" {
				if accumulator != nil {
					accumulator.results = append(accumulator.results, updateJSONResult{ResourceName: name, Outcome: "pinned", PreviousVersion: pinned})
					continue
				}
				a.writeStdout(fmt.Sprintf("  [pinned at %s] %s", pinned, name))
				continue
			}
			entry, found := byID[name]
			if !found {
				if accumulator != nil {
					accumulator.results = append(accumulator.results, updateJSONResult{ResourceName: name, Outcome: "skipped", Error: "resource missing from catalog"})
					continue
				}
				a.writeStderr(fmt.Sprintf("Resource '%s' is missing from the catalog of provider %s; skipped.", name, group.providerID))
				continue
			}
			resolvedVersion := ""
			if !dryRun {
				resolvedVersion = a.resolveCommitForEntry(context.Background(), groupConfig, group.providerID, entry, latestCache)
			}
			if installErr := a.installFanout(profile, group.stateDir, installedTargetConfig(cfg, states[group.stateDir], name, group.providerID), entry, group.providerID, resolvedVersion, dryRun, jsonOut, false); installErr != nil {
				failures++
				a.reportUpdateFailure(accumulator, name, "failed", installErr)
				continue
			}
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
		return fmt.Errorf("update incomplete: %d resource(s) failed", failures)
	}
	return nil
}
