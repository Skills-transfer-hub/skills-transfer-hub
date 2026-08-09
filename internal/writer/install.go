package writer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type MaterializedFile struct {
	RelativePath string
	SHA256       string
	Content      []byte
}

type MaterializeResult struct {
	Skipped       bool
	Reason        string
	Compatibility CompatibilityLevel
	Surface       string
	Files         []MaterializedFile
}

func MaterializeArtifact(projectRoot string, target Target, artifactKind, artifactName, lang string, sources []string, fetch func(sourcePath string) ([]byte, error), addManagedHeader, write bool) (MaterializeResult, error) {
	capability, hasCapability := CapabilityFor(target.Kind(), artifactKind)
	if hasCapability && !capability.Implemented {
		return MaterializeResult{
			Skipped:       true,
			Reason:        capability.Reason,
			Compatibility: capability.Level,
			Surface:       capability.Surface,
		}, nil
	}
	if nameErr := validateArtifactName(artifactName); nameErr != nil {
		return MaterializeResult{}, nameErr
	}

	plan := target.Plan(artifactKind, artifactName, lang, sources)
	if plan.Skipped {
		return MaterializeResult{
			Skipped:       true,
			Reason:        plan.Reason,
			Compatibility: capability.Level,
			Surface:       capability.Surface,
		}, nil
	}

	destinationRoots := []string(nil)
	if len(plan.Files) > 0 {
		var rootErr error
		destinationRoots, rootErr = resolveArtifactDestinationRoots(projectRoot, target, artifactKind, artifactName, lang)
		if rootErr != nil {
			return MaterializeResult{}, rootErr
		}
	}

	files := make([]MaterializedFile, 0, len(plan.Files))
	for _, planned := range plan.Files {
		installedPath, pathErr := resolveInstalledPathWithinRoots(projectRoot, destinationRoots, planned.RelativePath)
		if pathErr != nil {
			return MaterializeResult{}, pathErr
		}
		content, fetchErr := fetch(planned.SourcePath)
		if fetchErr != nil {
			return MaterializeResult{}, fetchErr
		}
		request := WriteRequest{InstalledPath: installedPath, Content: content, ArtifactKind: artifactKind, ArtifactName: artifactName, AddManagedHeader: addManagedHeader}
		finalContent, renderErr := renderTargetContent(target, request)
		if renderErr != nil {
			return MaterializeResult{}, renderErr
		}
		sha256 := computeSHA256(finalContent)
		if write {
			writeResult, writeErr := target.Write(request)
			if writeErr != nil {
				return MaterializeResult{}, writeErr
			}
			sha256 = writeResult.SHA256
		}
		files = append(files, MaterializedFile{
			RelativePath: planned.RelativePath,
			SHA256:       sha256,
			Content:      append([]byte(nil), finalContent...),
		})
	}
	return MaterializeResult{
		Compatibility: capability.Level,
		Surface:       capability.Surface,
		Files:         files,
	}, nil
}

func ValidateSkillConsumer(projectRoot string, target Target, artifactName string, sources []string, fetch func(sourcePath string) ([]byte, error)) error {
	if nameErr := validateArtifactName(artifactName); nameErr != nil {
		return nameErr
	}
	placement := PlanSkillPlacement([]string{target.Kind()})
	skillsFolder, exists := placement.ReadsFrom[target.Kind()]
	if !exists {
		return fmt.Errorf("hub de skills introuvable pour %s", target.Kind())
	}
	absoluteProjectRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return fmt.Errorf("résolution de la racine projet échouée : %w", err)
	}
	destinationRoot := filepath.Join(absoluteProjectRoot, filepath.FromSlash(skillsFolder), artifactName)
	entrypointPath := filepath.Join(destinationRoot, "SKILL.md")
	entrypointValidated := false
	for _, plannedFile := range relForSkillBundle(skillsFolder, artifactName, sources) {
		installedPath, pathErr := resolveInstalledPath(absoluteProjectRoot, destinationRoot, plannedFile.RelativePath)
		if pathErr != nil {
			return pathErr
		}
		content, fetchErr := fetch(plannedFile.SourcePath)
		if fetchErr != nil {
			return fetchErr
		}
		if filepath.Clean(installedPath) == filepath.Clean(entrypointPath) {
			entrypointValidated = true
			if metadataErr := validateSkillConsumerMetadata(target.Kind(), artifactName, content); metadataErr != nil {
				return metadataErr
			}
		}
		request := WriteRequest{
			InstalledPath:    installedPath,
			Content:          content,
			ArtifactKind:     "skill",
			ArtifactName:     artifactName,
			AddManagedHeader: true,
		}
		if _, renderErr := renderTargetContent(target, request); renderErr != nil {
			return renderErr
		}
	}
	if !entrypointValidated {
		return fmt.Errorf("skill %s invalide : SKILL.md introuvable", artifactName)
	}
	return nil
}

func validateArtifactName(artifactName string) error {
	if strings.TrimSpace(artifactName) == "" || artifactName == "." || artifactName == ".." || strings.ContainsAny(artifactName, "/\\") || strings.ContainsRune(artifactName, '\x00') || filepath.IsAbs(artifactName) || filepath.VolumeName(artifactName) != "" {
		return fmt.Errorf("nom d'artifact invalide : %s", artifactName)
	}
	return nil
}

func resolveArtifactDestinationRoots(projectRoot string, target Target, artifactKind, artifactName, lang string) ([]string, error) {
	rootPlan := target.Plan(artifactKind, artifactName, lang, []string{"SKILL.md"})
	if rootPlan.Skipped || len(rootPlan.Files) == 0 {
		return nil, fmt.Errorf("racine de destination introuvable pour %s/%s", target.Kind(), artifactKind)
	}
	absoluteProjectRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return nil, fmt.Errorf("résolution de la racine projet échouée : %w", err)
	}
	destinationRoots := make([]string, 0, len(rootPlan.Files))
	seenRoots := make(map[string]struct{}, len(rootPlan.Files))
	for _, plannedFile := range rootPlan.Files {
		cleanRelativePath, pathErr := cleanRelativeDestinationPath(plannedFile.RelativePath)
		if pathErr != nil {
			return nil, pathErr
		}
		destinationRoot := filepath.Dir(filepath.Join(absoluteProjectRoot, cleanRelativePath))
		if !pathIsWithin(absoluteProjectRoot, destinationRoot) {
			return nil, fmt.Errorf("racine de destination hors projet : %s", plannedFile.RelativePath)
		}
		rootKey := filepath.Clean(destinationRoot)
		if _, exists := seenRoots[rootKey]; exists {
			continue
		}
		seenRoots[rootKey] = struct{}{}
		destinationRoots = append(destinationRoots, destinationRoot)
	}
	return destinationRoots, nil
}

func resolveInstalledPathWithinRoots(projectRoot string, destinationRoots []string, relativePath string) (string, error) {
	cleanRelativePath, pathErr := cleanRelativeDestinationPath(relativePath)
	if pathErr != nil {
		return "", pathErr
	}
	absoluteProjectRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return "", fmt.Errorf("résolution de la racine projet échouée : %w", err)
	}
	installedPath := filepath.Join(absoluteProjectRoot, cleanRelativePath)
	for _, destinationRoot := range destinationRoots {
		if pathIsWithin(destinationRoot, installedPath) {
			return resolveInstalledPath(projectRoot, destinationRoot, relativePath)
		}
	}
	return "", fmt.Errorf("chemin de destination hors racines du writer : %s", relativePath)
}

func resolveInstalledPath(projectRoot, destinationRoot, relativePath string) (string, error) {
	cleanRelativePath, pathErr := cleanRelativeDestinationPath(relativePath)
	if pathErr != nil {
		return "", pathErr
	}
	absoluteProjectRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return "", fmt.Errorf("résolution de la racine projet échouée : %w", err)
	}
	installedPath := filepath.Join(absoluteProjectRoot, cleanRelativePath)
	if !pathIsWithin(absoluteProjectRoot, installedPath) {
		return "", fmt.Errorf("chemin de destination hors projet : %s", relativePath)
	}
	if !pathIsWithin(destinationRoot, installedPath) {
		return "", fmt.Errorf("chemin de destination hors racine du writer : %s", relativePath)
	}
	if pathErr := validateResolvedDestination(absoluteProjectRoot, destinationRoot, installedPath); pathErr != nil {
		return "", pathErr
	}
	return installedPath, nil
}

func cleanRelativeDestinationPath(relativePath string) (string, error) {
	portableRelativePath := strings.ReplaceAll(relativePath, "\\", "/")
	cleanRelativePath := filepath.Clean(filepath.FromSlash(portableRelativePath))
	if relativePath == "" || strings.ContainsRune(relativePath, '\x00') || filepath.IsAbs(cleanRelativePath) || cleanRelativePath == ".." || strings.HasPrefix(cleanRelativePath, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("chemin de destination invalide : %s", relativePath)
	}
	return cleanRelativePath, nil
}

func validateResolvedDestination(projectRoot, destinationRoot, installedPath string) error {
	resolvedProjectRoot, err := resolveExistingSymlinks(projectRoot)
	if err != nil {
		return fmt.Errorf("résolution de la racine projet échouée : %w", err)
	}
	resolvedDestinationRoot, err := resolveExistingSymlinks(destinationRoot)
	if err != nil {
		return fmt.Errorf("résolution de la racine du writer échouée : %w", err)
	}
	if !pathIsWithin(resolvedProjectRoot, resolvedDestinationRoot) {
		return fmt.Errorf("racine du writer redirigée hors projet : %s", destinationRoot)
	}
	resolvedInstalledPath, err := resolveExistingSymlinks(installedPath)
	if err != nil {
		return fmt.Errorf("résolution de la destination échouée : %w", err)
	}
	if !pathIsWithin(resolvedDestinationRoot, resolvedInstalledPath) {
		return fmt.Errorf("destination redirigée hors racine du writer : %s", installedPath)
	}
	return nil
}

func resolveExistingSymlinks(path string) (string, error) {
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

func pathIsWithin(rootPath, candidatePath string) bool {
	relativePath, err := filepath.Rel(rootPath, candidatePath)
	return err == nil && relativePath != ".." && !strings.HasPrefix(relativePath, ".."+string(filepath.Separator))
}

func renderTargetContent(target Target, request WriteRequest) ([]byte, error) {
	if renderer, ok := target.(TargetRenderer); ok {
		return renderer.Render(request)
	}
	return renderManaged(target, request.InstalledPath, request.Content, request.AddManagedHeader), nil
}

func renderManaged(target Target, installedPath string, content []byte, addManagedHeader bool) []byte {
	if !addManagedHeader {
		return content
	}
	if _, isClaude := target.(ClaudeWriter); isClaude {
		return applyClaudeManagedHeader(installedPath, content)
	}
	if _, isCursor := target.(CursorWriter); isCursor && isCursorMarkdownExt(installedPath) {
		return applyMinimalManagedMarker(installedPath, content)
	}
	if strings.EqualFold(filepath.Ext(installedPath), ".md") {
		return applyMinimalManagedMarker(installedPath, content)
	}
	return content
}
