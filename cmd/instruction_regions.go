package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"sth/internal/fs"
)

const codexInstructionRegionID = "codex-instructions-v1"
const geminiInstructionRegionID = "gemini-instructions-v1"
const instructionFragmentManagedMarker = "<!-- managed by sth -->\n"

type instructionRegionDefinition struct {
	path       string
	id         string
	targetID   string
	targetKind string
}

type instructionRegionContribution struct {
	name    string
	path    string
	content []byte
}

func instructionRegionDefinitionForTarget(target fs.TargetEntry) (instructionRegionDefinition, bool) {
	switch target.Kind {
	case "codex":
		return instructionRegionDefinition{path: "AGENTS.md", id: codexInstructionRegionID, targetID: target.ID, targetKind: target.Kind}, true
	case "gemini":
		return instructionRegionDefinition{path: "GEMINI.md", id: geminiInstructionRegionID, targetID: target.ID, targetKind: target.Kind}, true
	default:
		return instructionRegionDefinition{}, false
	}
}

func instructionRegionDefinitionFromManagedRegion(region fs.ManagedRegion) (instructionRegionDefinition, bool) {
	switch region.ID {
	case codexInstructionRegionID:
		return instructionRegionDefinition{path: region.Path, id: region.ID, targetKind: "codex"}, true
	case geminiInstructionRegionID:
		return instructionRegionDefinition{path: region.Path, id: region.ID, targetKind: "gemini"}, true
	default:
		return instructionRegionDefinition{}, false
	}
}

func managedRegionForInstructionTarget(target fs.TargetEntry) []fs.ManagedRegion {
	definition, exists := instructionRegionDefinitionForTarget(target)
	if !exists {
		return nil
	}
	return []fs.ManagedRegion{{Path: definition.path, ID: definition.id}}
}

func instructionRegionDefinitionsForPlans(plans []fanoutTargetPlan) []instructionRegionDefinition {
	definitions := make([]instructionRegionDefinition, 0)
	seen := make(map[string]struct{})
	for _, plan := range plans {
		definition, exists := instructionRegionDefinitionForTarget(plan.target)
		if !exists {
			continue
		}
		key := definition.path + "\x00" + definition.id
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		definitions = append(definitions, definition)
	}
	return definitions
}

func instructionRegionDefinitionsForRecords(records []fs.InstalledResourceRecordV2) []instructionRegionDefinition {
	definitions := make([]instructionRegionDefinition, 0)
	seen := make(map[string]struct{})
	for _, record := range records {
		for _, region := range record.ManagedRegions {
			definition, exists := instructionRegionDefinitionFromManagedRegion(region)
			if !exists {
				continue
			}
			key := definition.path + "\x00" + definition.id
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
			definitions = append(definitions, definition)
		}
	}
	return definitions
}

func recordContributesToInstructionRegion(record fs.InstalledResourceRecordV2, definition instructionRegionDefinition) bool {
	for _, region := range record.ManagedRegions {
		if region.Path == definition.path && region.ID == definition.id {
			return true
		}
	}
	return false
}

func instructionRegionContributions(projectRoot string, state fs.InstalledResourceStateV2, definition instructionRegionDefinition, contentOverrides map[string][]byte) ([]instructionRegionContribution, error) {
	contributions := make([]instructionRegionContribution, 0)
	seenFragmentPaths := make(map[string]struct{})
	for _, record := range state.Entries {
		if !recordContributesToInstructionRegion(record, definition) {
			continue
		}
		if len(record.ManagedFiles) != 1 {
			return nil, fmt.Errorf("instruction %s/%s : un fragment géré attendu", record.ResourceName, record.TargetID)
		}
		fragmentPath := record.ManagedFiles[0].Path
		if _, duplicate := seenFragmentPaths[fragmentPath]; duplicate {
			continue
		}
		seenFragmentPaths[fragmentPath] = struct{}{}
		fragmentContent, overridden := contentOverrides[fragmentPath]
		if !overridden {
			absoluteFragmentPath, pathErr := fanoutProjectPath(projectRoot, fragmentPath)
			if pathErr != nil {
				return nil, pathErr
			}
			var readErr error
			fragmentContent, readErr = os.ReadFile(absoluteFragmentPath)
			if readErr != nil {
				return nil, fmt.Errorf("lecture du fragment %s échouée : %w", fragmentPath, readErr)
			}
		}
		fragmentName := strings.TrimSuffix(filepath.Base(fragmentPath), filepath.Ext(fragmentPath))
		contributions = append(contributions, instructionRegionContribution{
			name:    fragmentName,
			path:    fragmentPath,
			content: append([]byte(nil), fragmentContent...),
		})
	}
	sort.Slice(contributions, func(leftIndex, rightIndex int) bool {
		return contributions[leftIndex].path < contributions[rightIndex].path
	})
	return contributions, nil
}

func buildInstructionRegionBody(projectRoot string, state fs.InstalledResourceStateV2, definition instructionRegionDefinition, contentOverrides map[string][]byte) ([]byte, int, error) {
	contributions, err := instructionRegionContributions(projectRoot, state, definition, contentOverrides)
	if err != nil {
		return nil, 0, err
	}
	if len(contributions) == 0 {
		return nil, 0, nil
	}

	var body strings.Builder
	switch definition.targetKind {
	case "codex":
		body.WriteString("## Instructions managed by STH\n")
		for _, contribution := range contributions {
			body.WriteString("\n### ")
			body.WriteString(contribution.name)
			body.WriteString("\n\n")
			instructionBody := strings.TrimSpace(strings.TrimPrefix(string(contribution.content), instructionFragmentManagedMarker))
			body.WriteString(instructionBody)
			body.WriteByte('\n')
		}
	case "gemini":
		for _, contribution := range contributions {
			relativeImportPath, pathErr := filepath.Rel(filepath.Dir(definition.path), filepath.FromSlash(contribution.path))
			if pathErr != nil {
				return nil, 0, fmt.Errorf("import Gemini pour %s invalide : %w", contribution.path, pathErr)
			}
			portableImportPath := filepath.ToSlash(relativeImportPath)
			if !strings.HasPrefix(portableImportPath, "./") && !strings.HasPrefix(portableImportPath, "../") {
				portableImportPath = "./" + portableImportPath
			}
			body.WriteByte('@')
			body.WriteString(portableImportPath)
			body.WriteByte('\n')
		}
	default:
		return nil, 0, fmt.Errorf("cible de région d'instruction inconnue : %s", definition.targetKind)
	}
	return []byte(body.String()), len(contributions), nil
}

func reconcileInstructionRegion(projectRoot string, state *fs.InstalledResourceStateV2, definition instructionRegionDefinition) error {
	absoluteRootPath, pathErr := fanoutProjectPath(projectRoot, definition.path)
	if pathErr != nil {
		return pathErr
	}
	body, contributionCount, buildErr := buildInstructionRegionBody(projectRoot, *state, definition, nil)
	if buildErr != nil {
		return buildErr
	}
	if contributionCount == 0 {
		updated, removeErr := fs.RemoveManagedMarkdownRegionFile(absoluteRootPath, definition.id)
		if removeErr != nil {
			if os.IsNotExist(removeErr) || errors.Is(removeErr, fs.ErrManagedMarkdownRegionNotFound) {
				return nil
			}
			return removeErr
		}
		if len(updated) == 0 {
			if removeErr := os.Remove(absoluteRootPath); removeErr != nil && !os.IsNotExist(removeErr) {
				return fmt.Errorf("suppression de %s échouée : %w", definition.path, removeErr)
			}
		}
		return nil
	}
	region, writeErr := fs.WriteManagedMarkdownRegion(absoluteRootPath, definition.id, body)
	if writeErr != nil {
		return writeErr
	}
	for recordIndex := range state.Entries {
		if !recordContributesToInstructionRegion(state.Entries[recordIndex], definition) {
			continue
		}
		for regionIndex := range state.Entries[recordIndex].ManagedRegions {
			managedRegion := &state.Entries[recordIndex].ManagedRegions[regionIndex]
			if managedRegion.Path == definition.path && managedRegion.ID == definition.id {
				managedRegion.SHA256 = region.SHA256
			}
		}
	}
	return nil
}

func instructionRegionDryRunFiles(projectRoot string, state fs.InstalledResourceStateV2, plans []fanoutTargetPlan, entry fs.CatalogEntry) ([]fs.ManagedFile, []fs.ManagedFile, error) {
	prospectiveState := fs.InstalledResourceStateV2{SchemaVersion: state.SchemaVersion, Entries: append([]fs.InstalledResourceRecordV2(nil), state.Entries...)}
	contentOverrides := make(map[string][]byte)
	for _, plan := range plans {
		managedFiles := make([]fs.ManagedFile, 0, len(plan.preview.Files))
		for _, materializedFile := range plan.preview.Files {
			managedFiles = append(managedFiles, fs.ManagedFile{Path: materializedFile.RelativePath, SHA256: materializedFile.SHA256})
			contentOverrides[materializedFile.RelativePath] = append([]byte(nil), materializedFile.Content...)
		}
		record := fs.InstalledResourceRecordV2{
			ResourceName:   entry.CatalogID,
			ManagedFiles:   managedFiles,
			ManagedRegions: managedRegionForInstructionTarget(plan.target),
			TargetID:       plan.target.ID,
			ArtifactKind:   "instruction",
		}
		upsertFanoutRecord(&prospectiveState, record)
	}

	currentFiles := make([]fs.ManagedFile, 0)
	incomingFiles := make([]fs.ManagedFile, 0)
	for _, definition := range instructionRegionDefinitionsForPlans(plans) {
		body, contributionCount, buildErr := buildInstructionRegionBody(projectRoot, prospectiveState, definition, contentOverrides)
		if buildErr != nil {
			return nil, nil, buildErr
		}
		if contributionCount == 0 {
			continue
		}
		absoluteRootPath, pathErr := fanoutProjectPath(projectRoot, definition.path)
		if pathErr != nil {
			return nil, nil, pathErr
		}
		currentDocument, readErr := os.ReadFile(absoluteRootPath)
		if readErr != nil && !os.IsNotExist(readErr) {
			return nil, nil, fmt.Errorf("lecture de %s échouée : %w", definition.path, readErr)
		}
		if readErr == nil {
			currentFiles = append(currentFiles, fs.ManagedFile{Path: definition.path, SHA256: fs.ComputeSHA256(currentDocument)})
		}
		incomingDocument, mergeErr := fs.MergeManagedMarkdownRegion(currentDocument, definition.id, body)
		if mergeErr != nil {
			return nil, nil, mergeErr
		}
		incomingFiles = append(incomingFiles, fs.ManagedFile{Path: definition.path, SHA256: fs.ComputeSHA256(incomingDocument)})
	}
	return currentFiles, incomingFiles, nil
}

func (a *app) removeRecordsWithManagedRegions(targetDir, resourceName string, records []fs.InstalledResourceRecordV2) (bool, error) {
	definitions := instructionRegionDefinitionsForRecords(records)
	if len(definitions) == 0 {
		return false, nil
	}
	projectRoot, rootErr := filepath.Abs(".")
	if rootErr != nil {
		return true, rootErr
	}
	state, stateErr := fs.ReadInstalledStateV2(targetDir)
	if stateErr != nil {
		return true, stateErr
	}
	managedFiles := make([]fs.ManagedFile, 0)
	for _, record := range records {
		managedFiles = append(managedFiles, record.ManagedFiles...)
	}
	transactionPaths := fanoutTransactionPaths(targetDir, managedFiles, nil)
	for _, definition := range definitions {
		transactionPaths = append(transactionPaths, definition.path)
	}
	transaction, transactionErr := newFanoutFileTransaction(projectRoot, transactionPaths)
	if transactionErr != nil {
		return true, transactionErr
	}

	seenFiles := make(map[string]struct{}, len(managedFiles))
	for _, managedFile := range managedFiles {
		if _, duplicate := seenFiles[managedFile.Path]; duplicate {
			continue
		}
		seenFiles[managedFile.Path] = struct{}{}
		absolutePath, pathErr := fanoutProjectPath(projectRoot, managedFile.Path)
		if pathErr != nil {
			return true, transaction.rollback(pathErr)
		}
		if removeErr := os.Remove(absolutePath); removeErr != nil && !os.IsNotExist(removeErr) {
			return true, transaction.rollback(fmt.Errorf("suppression de %s échouée : %w", managedFile.Path, removeErr))
		}
	}

	remainingEntries := make([]fs.InstalledResourceRecordV2, 0, len(state.Entries))
	for _, record := range state.Entries {
		if record.ResourceName != resourceName {
			remainingEntries = append(remainingEntries, record)
		}
	}
	state.Entries = remainingEntries
	for _, definition := range definitions {
		if reconcileErr := reconcileInstructionRegion(projectRoot, &state, definition); reconcileErr != nil {
			return true, transaction.rollback(reconcileErr)
		}
	}
	if ensureErr := fs.EnsureTargetDir(targetDir); ensureErr != nil {
		return true, transaction.rollback(ensureErr)
	}
	if writeErr := fs.WriteInstalledStateV2(targetDir, state); writeErr != nil {
		return true, transaction.rollback(writeErr)
	}
	a.writeStdout(fmt.Sprintf("Resource '%s' removed.", resourceName))
	return true, nil
}
