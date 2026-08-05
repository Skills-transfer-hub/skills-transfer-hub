package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"sth/internal/cloud"
	"sth/internal/fs"
	"sth/internal/writer"
)

const bridgeCloudStateDirectory = ".sth/local/code"

type bridgeCloudResolvedSkill struct {
	asset    cloud.AssetSummary
	manifest cloud.ResourceVersionManifest
	slug     string
	sources  []string
	contents map[string][]byte
}

func resolveBridgeCloudSkill(ctx context.Context, assetID, expectedVersionID string) (bridgeCloudResolvedSkill, error) {
	assetID = strings.TrimSpace(assetID)
	if assetID == "" {
		return bridgeCloudResolvedSkill{}, fmt.Errorf("asset_id requis")
	}
	asset, err := cloud.GetAsset(ctx, nil, assetID)
	if err != nil {
		return bridgeCloudResolvedSkill{}, err
	}
	if asset.Family != "package" || asset.Kind != "skill" {
		return bridgeCloudResolvedSkill{}, fmt.Errorf("seuls les packages de type skill sont installables")
	}
	if asset.LatestVersionID == "" {
		return bridgeCloudResolvedSkill{}, fmt.Errorf("ce skill ne possède aucune version publiée")
	}
	if expected := strings.TrimSpace(expectedVersionID); expected != "" && asset.LatestVersionID != expected {
		return bridgeCloudResolvedSkill{}, fmt.Errorf("la version publiée a changé depuis le preview ; relancez le preview")
	}

	manifest, err := cloud.GetResourceVersionManifest(ctx, nil, asset.ID, asset.LatestVersionID)
	if err != nil {
		return bridgeCloudResolvedSkill{}, err
	}
	slug, err := bridgeCloudSkillSlug(asset, manifest)
	if err != nil {
		return bridgeCloudResolvedSkill{}, err
	}
	downloads, err := cloud.CreateResourceVersionDownloads(ctx, nil, asset.ID, asset.LatestVersionID)
	if err != nil {
		return bridgeCloudResolvedSkill{}, err
	}
	contents, err := cloud.DownloadResourceVersionBundle(ctx, nil, manifest, downloads)
	if err != nil {
		return bridgeCloudResolvedSkill{}, err
	}
	bundleRoot := strings.Split(asset.ID, "::")[1] + "/" + slug
	writableContents := make(map[string][]byte, len(contents))
	sources := make([]string, 0, len(contents))
	for source, content := range contents {
		writerSource := bundleRoot + "/" + source
		sources = append(sources, writerSource)
		writableContents[writerSource] = content
	}
	sort.Strings(sources)
	return bridgeCloudResolvedSkill{asset: asset, manifest: manifest, slug: slug, sources: sources, contents: writableContents}, nil
}

func bridgeCloudSkillSlug(asset cloud.AssetSummary, manifest cloud.ResourceVersionManifest) (string, error) {
	parts := strings.Split(asset.ID, "::")
	if len(parts) != 3 || parts[0] != "skill" || !validBridgeCloudIDSegment(parts[1]) || !validBridgeCloudIDSegment(parts[2]) {
		return "", fmt.Errorf("identifiant de package skill invalide")
	}
	root := parts[1] + "/" + parts[2]
	if manifest.Kind != "skill" || manifest.Entrypoint != "SKILL.md" {
		return "", fmt.Errorf("entrypoint du package skill incohérent")
	}
	if asset.SourcePath != "" && asset.SourcePath != root+"/"+manifest.Entrypoint {
		return "", fmt.Errorf("source_path du package skill incohérent")
	}
	for _, file := range manifest.Files {
		if strings.HasPrefix(file.Path, root+"/") {
			return "", fmt.Errorf("les chemins du manifest doivent être relatifs au bundle")
		}
	}
	return parts[2], nil
}

func validBridgeCloudIDSegment(value string) bool {
	if value == "" || len(value) > 128 || !isBridgeCloudLowerAlphaNumeric(value[0]) || !isBridgeCloudLowerAlphaNumeric(value[len(value)-1]) {
		return false
	}
	if len(value) == 1 {
		return true
	}
	for _, character := range value[1 : len(value)-1] {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' && character != '_' && character != '.' {
			return false
		}
	}
	return true
}

func isBridgeCloudLowerAlphaNumeric(character byte) bool {
	return (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9')
}

func bridgeCloudTargetKinds() ([]string, error) {
	config, err := fs.ReadProjectConfigV2ReadOnly()
	if err != nil {
		return nil, err
	}
	if len(config.Targets) == 0 {
		return []string{writer.KindClaude, writer.KindCodex}, nil
	}
	if validationErr := validateFanoutTargetRoots(config); validationErr != nil {
		return nil, validationErr
	}
	kinds := enabledTargetKinds(config)
	if len(kinds) == 0 {
		return nil, fmt.Errorf("aucune cible agent activée dans .sth/project.json")
	}
	return kinds, nil
}

func planBridgeCloudSkill(resolved bridgeCloudResolvedSkill, force bool) ([]fs.ManagedFile, []string, []fs.DryRunChange, []string, error) {
	projectRoot, err := canonicalBridgeProjectRoot()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	kinds, err := bridgeCloudTargetKinds()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	placement := writer.PlanSkillPlacement(kinds)
	if len(placement.Folders) == 0 {
		return nil, nil, nil, nil, fmt.Errorf("aucune destination compatible pour ce skill")
	}
	fetch := func(source string) ([]byte, error) {
		content, ok := resolved.contents[source]
		if !ok {
			return nil, fmt.Errorf("fichier du bundle absent : %s", source)
		}
		return content, nil
	}
	if validationErr := validateSkillConsumerContracts(projectRoot, resolved.slug, resolved.sources, kinds, fetch); validationErr != nil {
		return nil, nil, nil, nil, validationErr
	}
	managedFiles := make([]fs.ManagedFile, 0, len(resolved.sources)*len(placement.Folders))
	for _, folder := range placement.Folders {
		target, ok := writer.WriterForSkillsFolder(folder)
		if !ok {
			return nil, nil, nil, nil, fmt.Errorf("writer de skill indisponible pour %s", folder)
		}
		result, err := writer.MaterializeArtifact(projectRoot, target, "skill", resolved.slug, "", resolved.sources, fetch, true, false)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		for _, file := range result.Files {
			if err := verifyBridgeManagedPath(projectRoot, file.RelativePath); err != nil {
				return nil, nil, nil, nil, err
			}
			managedFiles = append(managedFiles, fs.ManagedFile{Path: filepath.ToSlash(file.RelativePath), SHA256: file.SHA256})
		}
	}
	sort.Slice(managedFiles, func(leftIndex, rightIndex int) bool {
		return managedFiles[leftIndex].Path < managedFiles[rightIndex].Path
	})

	if err := verifyBridgeManagedPath(projectRoot, filepath.ToSlash(filepath.Join(bridgeCloudStateDirectory, "sth-state-v2.json"))); err != nil {
		return nil, nil, nil, nil, err
	}
	state, err := fs.ReadInstalledStateV2(bridgeCloudStateDirectory)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	previous, found := bridgeCloudRecord(state, resolved.asset.ID)
	selfOwned := make(map[string]struct{})
	if found {
		for _, file := range previous.ManagedFiles {
			if err := verifyBridgeManagedPath(projectRoot, file.Path); err != nil {
				return nil, nil, nil, nil, fmt.Errorf("état local invalide : %w", err)
			}
			selfOwned[file.Path] = struct{}{}
		}
	}
	warnings := []string{}
	for _, file := range managedFiles {
		if conflict := fs.CheckOwnershipConflict(&state, resolved.asset.ID, file.Path); conflict != nil {
			if !force {
				return nil, nil, nil, nil, conflict
			}
			warnings = append(warnings, fmt.Sprintf("L’ownership de %s sera transféré depuis %s.", file.Path, conflict.ExistingOwner))
		}
		if _, owned := selfOwned[file.Path]; owned {
			continue
		}
		content, readErr := os.ReadFile(filepath.Join(projectRoot, filepath.FromSlash(file.Path)))
		if readErr == nil && fs.ComputeSHA256(content) != file.SHA256 {
			if !force {
				return nil, nil, nil, nil, fmt.Errorf("le fichier non géré %s existe déjà ; relancez le preview avec force", file.Path)
			}
			warnings = append(warnings, fmt.Sprintf("Le fichier non géré %s sera remplacé.", file.Path))
		} else if readErr != nil && !os.IsNotExist(readErr) {
			return nil, nil, nil, nil, fmt.Errorf("lecture de %s échouée : %w", file.Path, readErr)
		}
	}
	changes, err := bridgeCloudDryRunChanges(projectRoot, resolved.asset.ID, previous, found, managedFiles)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return managedFiles, append([]string(nil), placement.Folders...), changes, warnings, nil
}

func previewBridgeCloudSkill(assetID string, force bool) (string, []fs.DryRunChange, []string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	resolved, err := resolveBridgeCloudSkill(ctx, assetID, "")
	if err != nil {
		return "", nil, nil, err
	}
	_, _, changes, warnings, err := planBridgeCloudSkill(resolved, force)
	return resolved.manifest.VersionID, changes, warnings, err
}

func installBridgeCloudSkill(assetID, expectedVersionID string, force bool) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	resolved, err := resolveBridgeCloudSkill(ctx, assetID, expectedVersionID)
	if err != nil {
		return "", err
	}
	releaseLock, err := acquireBridgeCloudStateLock()
	if err != nil {
		return "", err
	}
	defer releaseLock()
	managedFiles, folders, _, _, err := planBridgeCloudSkill(resolved, force)
	if err != nil {
		return "", err
	}
	if err := applyBridgeCloudSkill(resolved, managedFiles, folders, force); err != nil {
		return "", err
	}
	return resolved.manifest.VersionID, nil
}

func acquireBridgeCloudStateLock() (func(), error) {
	projectRoot, err := canonicalBridgeProjectRoot()
	if err != nil {
		return nil, err
	}
	stateRelativePath := filepath.ToSlash(filepath.Join(bridgeCloudStateDirectory, "sth-state-v2.json"))
	if err := verifyBridgeManagedPath(projectRoot, stateRelativePath); err != nil {
		return nil, err
	}
	stateDirectory := filepath.Join(projectRoot, filepath.FromSlash(bridgeCloudStateDirectory))
	if err := os.MkdirAll(stateDirectory, 0o700); err != nil {
		return nil, fmt.Errorf("création de l’état local STH Code échouée : %w", err)
	}
	if err := os.Chmod(stateDirectory, 0o700); err != nil {
		return nil, fmt.Errorf("protection de l’état local STH Code échouée : %w", err)
	}
	return fs.AcquireStateLock(stateDirectory)
}

func applyBridgeCloudSkill(resolved bridgeCloudResolvedSkill, managedFiles []fs.ManagedFile, folders []string, force bool) (returnErr error) {
	projectRoot, err := canonicalBridgeProjectRoot()
	if err != nil {
		return err
	}
	if err := verifyBridgeManagedPath(projectRoot, filepath.ToSlash(filepath.Join(bridgeCloudStateDirectory, "sth-state-v2.json"))); err != nil {
		return err
	}
	state, err := fs.ReadInstalledStateV2(bridgeCloudStateDirectory)
	if err != nil {
		return err
	}
	previous, found := bridgeCloudRecord(state, resolved.asset.ID)
	selfOwned := make(map[string]struct{}, len(previous.ManagedFiles))
	for _, file := range previous.ManagedFiles {
		if err := verifyBridgeManagedPath(projectRoot, file.Path); err != nil {
			return fmt.Errorf("état local invalide : %w", err)
		}
		selfOwned[file.Path] = struct{}{}
	}
	for _, file := range managedFiles {
		if err := verifyBridgeManagedPath(projectRoot, file.Path); err != nil {
			return err
		}
		if conflict := fs.CheckOwnershipConflict(&state, resolved.asset.ID, file.Path); conflict != nil && !force {
			return conflict
		}
		if _, owned := selfOwned[file.Path]; owned {
			continue
		}
		content, readErr := os.ReadFile(filepath.Join(projectRoot, filepath.FromSlash(file.Path)))
		if readErr == nil && fs.ComputeSHA256(content) != file.SHA256 && !force {
			return fmt.Errorf("le fichier non géré %s existe déjà ; relancez le preview avec force", file.Path)
		}
		if readErr != nil && !os.IsNotExist(readErr) {
			return fmt.Errorf("lecture de %s échouée : %w", file.Path, readErr)
		}
	}
	paths := make([]string, 0, len(managedFiles)+len(previous.ManagedFiles)+1)
	for _, file := range managedFiles {
		paths = append(paths, filepath.Join(projectRoot, filepath.FromSlash(file.Path)))
	}
	if found {
		for _, file := range previous.ManagedFiles {
			if err := verifyBridgeManagedPath(projectRoot, file.Path); err != nil {
				return fmt.Errorf("état local invalide : %w", err)
			}
			paths = append(paths, filepath.Join(projectRoot, filepath.FromSlash(file.Path)))
		}
	}
	statePath := filepath.Join(projectRoot, filepath.FromSlash(bridgeCloudStateDirectory), "sth-state-v2.json")
	paths = append(paths, statePath)
	snapshots, createdDirectories, err := snapshotBridgeFiles(projectRoot, paths)
	if err != nil {
		return err
	}
	defer func() {
		if returnErr != nil {
			_ = restoreBridgeFiles(snapshots)
			cleanupBridgeDirectories(createdDirectories)
		}
	}()

	for _, file := range managedFiles {
		if conflict := fs.CheckOwnershipConflict(&state, resolved.asset.ID, file.Path); conflict != nil {
			if !force {
				return conflict
			}
			fs.TransferOwnership(&state, resolved.asset.ID, file.Path)
		}
	}
	fetch := func(source string) ([]byte, error) {
		content, ok := resolved.contents[source]
		if !ok {
			return nil, fmt.Errorf("fichier du bundle absent : %s", source)
		}
		return content, nil
	}
	for _, folder := range folders {
		target, ok := writer.WriterForSkillsFolder(folder)
		if !ok {
			return fmt.Errorf("writer de skill indisponible pour %s", folder)
		}
		if _, err := writer.MaterializeArtifact(projectRoot, target, "skill", resolved.slug, "", resolved.sources, fetch, true, true); err != nil {
			return err
		}
	}
	incoming := make(map[string]struct{}, len(managedFiles))
	for _, file := range managedFiles {
		incoming[file.Path] = struct{}{}
	}
	if found {
		for _, file := range previous.ManagedFiles {
			if _, keep := incoming[file.Path]; keep {
				continue
			}
			if err := os.Remove(filepath.Join(projectRoot, filepath.FromSlash(file.Path))); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("suppression de %s échouée : %w", file.Path, err)
			}
		}
	}
	record := fs.InstalledResourceRecordV2{
		ResourceName:    resolved.asset.ID,
		ProviderID:      "sth-cloud",
		ResolvedVersion: resolved.manifest.VersionID,
		InstalledAt:     time.Now().UTC().Format(time.RFC3339),
		ManagedFiles:    managedFiles,
		SourcePath:      resolved.manifest.Entrypoint,
		ArtifactKind:    "skill",
	}
	if found {
		record.PreviousVersion = previous.ResolvedVersion
	}
	upsertBridgeCloudRecord(&state, record)
	stateDirectory := filepath.Join(projectRoot, filepath.FromSlash(bridgeCloudStateDirectory))
	if err := os.MkdirAll(stateDirectory, 0o700); err != nil {
		return fmt.Errorf("création de l’état local STH Code échouée : %w", err)
	}
	if err := os.Chmod(stateDirectory, 0o700); err != nil {
		return fmt.Errorf("protection de l’état local STH Code échouée : %w", err)
	}
	if err := fs.WriteInstalledStateV2(bridgeCloudStateDirectory, state); err != nil {
		return err
	}
	return nil
}

func bridgeCloudRecord(state fs.InstalledResourceStateV2, assetID string) (fs.InstalledResourceRecordV2, bool) {
	for _, record := range state.Entries {
		if record.ResourceName == assetID && record.TargetID == "" {
			return record, true
		}
	}
	return fs.InstalledResourceRecordV2{}, false
}

func upsertBridgeCloudRecord(state *fs.InstalledResourceStateV2, record fs.InstalledResourceRecordV2) {
	for index, existing := range state.Entries {
		if existing.ResourceName == record.ResourceName && existing.TargetID == record.TargetID {
			state.Entries[index] = record
			return
		}
	}
	state.Entries = append(state.Entries, record)
	sort.Slice(state.Entries, func(leftIndex, rightIndex int) bool {
		if state.Entries[leftIndex].ResourceName == state.Entries[rightIndex].ResourceName {
			return state.Entries[leftIndex].TargetID < state.Entries[rightIndex].TargetID
		}
		return state.Entries[leftIndex].ResourceName < state.Entries[rightIndex].ResourceName
	})
}

func removeBridgeCloudResource(resourceRef string) (bool, error) {
	state, err := fs.ReadInstalledStateV2(bridgeCloudStateDirectory)
	if err != nil {
		return false, err
	}
	_, found, err := bridgeCloudRemovalRecord(state, resourceRef)
	if err != nil || !found {
		return false, err
	}
	projectRoot, err := canonicalBridgeProjectRoot()
	if err != nil {
		return false, err
	}
	releaseLock, err := acquireBridgeCloudStateLock()
	if err != nil {
		return false, err
	}
	defer releaseLock()

	state, err = fs.ReadInstalledStateV2(bridgeCloudStateDirectory)
	if err != nil {
		return false, err
	}
	recordIndex, found, err := bridgeCloudRemovalRecord(state, resourceRef)
	if err != nil || !found {
		return false, err
	}
	record := state.Entries[recordIndex]
	paths := make([]string, 0, len(record.ManagedFiles)+1)
	for _, file := range record.ManagedFiles {
		if err := verifyBridgeManagedPath(projectRoot, file.Path); err != nil {
			return false, fmt.Errorf("état local invalide : %w", err)
		}
		paths = append(paths, filepath.Join(projectRoot, filepath.FromSlash(file.Path)))
	}
	statePath := filepath.Join(projectRoot, filepath.FromSlash(bridgeCloudStateDirectory), "sth-state-v2.json")
	paths = append(paths, statePath)
	snapshots, _, err := snapshotBridgeFiles(projectRoot, paths)
	if err != nil {
		return false, err
	}
	completed := false
	defer func() {
		if !completed {
			_ = restoreBridgeFiles(snapshots)
		}
	}()

	for _, file := range record.ManagedFiles {
		path := filepath.Join(projectRoot, filepath.FromSlash(file.Path))
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return false, fmt.Errorf("suppression de %s échouée : %w", file.Path, err)
		}
	}
	state.Entries = append(state.Entries[:recordIndex], state.Entries[recordIndex+1:]...)
	if err := fs.WriteInstalledStateV2(bridgeCloudStateDirectory, state); err != nil {
		return false, err
	}
	completed = true
	return true, nil
}

func bridgeCloudRemovalRecord(state fs.InstalledResourceStateV2, resourceRef string) (int, bool, error) {
	resourceRef = strings.TrimSpace(resourceRef)
	for recordIndex, record := range state.Entries {
		if record.ProviderID == "sth-cloud" && record.TargetID == "" && record.ResourceName == resourceRef {
			return recordIndex, true, nil
		}
	}
	matchingIndexes := make([]int, 0, 1)
	for recordIndex, record := range state.Entries {
		kind := strings.TrimSpace(record.ArtifactKind)
		if kind == "" {
			kind = "skill"
		}
		if record.ProviderID == "sth-cloud" && record.TargetID == "" && bridgeCloudRecordShortName(record, kind) == resourceRef {
			matchingIndexes = append(matchingIndexes, recordIndex)
		}
	}
	if len(matchingIndexes) > 1 {
		return 0, false, fmt.Errorf("plusieurs ressources Cloud correspondent à %q ; utilisez leur resource_ref", resourceRef)
	}
	if len(matchingIndexes) == 0 {
		return 0, false, nil
	}
	return matchingIndexes[0], true, nil
}

func bridgeCloudRecordShortName(record fs.InstalledResourceRecordV2, kind string) string {
	if skillName := bridgeInstalledSkillName(record, kind); skillName != "" {
		return skillName
	}
	parts := strings.Split(record.ResourceName, "::")
	if kind == "skill" && len(parts) == 3 && validBridgeCloudIDSegment(parts[2]) {
		return parts[2]
	}
	return ""
}

func bridgeCloudDryRunChanges(projectRoot, assetID string, previous fs.InstalledResourceRecordV2, found bool, incoming []fs.ManagedFile) ([]fs.DryRunChange, error) {
	incomingByPath := make(map[string]fs.ManagedFile, len(incoming))
	for _, file := range incoming {
		incomingByPath[file.Path] = file
	}
	changes := make([]fs.DryRunChange, 0)
	for _, file := range incoming {
		content, err := os.ReadFile(filepath.Join(projectRoot, filepath.FromSlash(file.Path)))
		if os.IsNotExist(err) {
			changes = append(changes, fs.DryRunChange{Action: "add", Path: file.Path, NewSHA256: file.SHA256, ResourceName: assetID})
			continue
		}
		if err != nil {
			return nil, err
		}
		oldSHA := fs.ComputeSHA256(content)
		if oldSHA != file.SHA256 {
			changes = append(changes, fs.DryRunChange{Action: "modify", Path: file.Path, OldSHA256: oldSHA, NewSHA256: file.SHA256, ResourceName: assetID})
		}
	}
	if found {
		for _, old := range previous.ManagedFiles {
			if _, keep := incomingByPath[old.Path]; keep {
				continue
			}
			if _, err := os.Stat(filepath.Join(projectRoot, filepath.FromSlash(old.Path))); err == nil {
				changes = append(changes, fs.DryRunChange{Action: "delete", Path: old.Path, OldSHA256: old.SHA256, ResourceName: assetID})
			} else if !os.IsNotExist(err) {
				return nil, err
			}
		}
	}
	sort.Slice(changes, func(leftIndex, rightIndex int) bool {
		return changes[leftIndex].Path < changes[rightIndex].Path
	})
	return changes, nil
}

func canonicalBridgeProjectRoot() (string, error) {
	root, err := filepath.Abs(".")
	if err != nil {
		return "", err
	}
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	return canonical, nil
}

func verifyBridgeManagedPath(projectRoot, relativePath string) error {
	if filepath.IsAbs(relativePath) {
		return fmt.Errorf("chemin de matérialisation absolu refusé")
	}
	clean := filepath.Clean(filepath.FromSlash(relativePath))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("chemin de matérialisation hors projet refusé")
	}
	current := projectRoot
	parts := strings.Split(clean, string(filepath.Separator))
	for _, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("lien symbolique refusé dans %s", relativePath)
		}
	}
	return nil
}

type bridgeFileSnapshot struct {
	path    string
	existed bool
	mode    os.FileMode
	content []byte
}

func snapshotBridgeFiles(projectRoot string, paths []string) ([]bridgeFileSnapshot, []string, error) {
	seen := map[string]struct{}{}
	snapshots := make([]bridgeFileSnapshot, 0, len(paths))
	missingDirectories := map[string]struct{}{}
	for _, value := range paths {
		path := filepath.Clean(value)
		for directory := filepath.Dir(path); directory != projectRoot; directory = filepath.Dir(directory) {
			if directory == "." || directory == string(filepath.Separator) || !strings.HasPrefix(directory, projectRoot+string(filepath.Separator)) {
				return nil, nil, fmt.Errorf("dossier de rollback hors projet refusé")
			}
			info, err := os.Lstat(directory)
			if os.IsNotExist(err) {
				missingDirectories[directory] = struct{}{}
				continue
			}
			if err != nil {
				return nil, nil, err
			}
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return nil, nil, fmt.Errorf("dossier non sûr refusé : %s", directory)
			}
		}
		if _, duplicate := seen[path]; duplicate {
			continue
		}
		seen[path] = struct{}{}
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			snapshots = append(snapshots, bridgeFileSnapshot{path: path})
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, nil, fmt.Errorf("fichier non régulier refusé : %s", path)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, nil, err
		}
		snapshots = append(snapshots, bridgeFileSnapshot{path: path, existed: true, mode: info.Mode().Perm(), content: content})
	}
	directories := make([]string, 0, len(missingDirectories))
	for directory := range missingDirectories {
		directories = append(directories, directory)
	}
	sort.Slice(directories, func(leftIndex, rightIndex int) bool {
		return strings.Count(directories[leftIndex], string(filepath.Separator)) > strings.Count(directories[rightIndex], string(filepath.Separator))
	})
	return snapshots, directories, nil
}

func restoreBridgeFiles(snapshots []bridgeFileSnapshot) error {
	var firstErr error
	for _, snapshot := range snapshots {
		if !snapshot.existed {
			if err := os.Remove(snapshot.path); err != nil && !os.IsNotExist(err) && firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(snapshot.path), 0o755); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := os.WriteFile(snapshot.path, snapshot.content, snapshot.mode); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func cleanupBridgeDirectories(directories []string) {
	for _, directory := range directories {
		_ = os.Remove(directory)
	}
}
