package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"sth/internal/fs"
	"sth/internal/writer"
)

func validateFanoutTargetRoots(config fs.ProjectConfigV2) error {
	for _, target := range config.Targets {
		if !target.Enabled {
			continue
		}
		targetSpecification, exists := writer.SpecForKind(target.Kind)
		if !exists {
			continue
		}
		configuredRoot := filepath.ToSlash(filepath.Clean(strings.TrimSpace(target.RootDir)))
		canonicalRoot := filepath.ToSlash(filepath.Clean(targetSpecification.RootDir))
		if configuredRoot != canonicalRoot {
			return fmt.Errorf("root_dir %q is not supported for target %q of type %s: expected %q", target.RootDir, target.ID, target.Kind, targetSpecification.RootDir)
		}
	}
	return nil
}

func validateSkillConsumerContracts(projectRoot, artifactName string, sourcePaths, consumerKinds []string, fetch func(string) ([]byte, error)) error {
	validatedKinds := make(map[string]struct{}, len(consumerKinds))
	for _, consumerKind := range consumerKinds {
		if _, validated := validatedKinds[consumerKind]; validated {
			continue
		}
		consumer, exists := writer.WriterForKind(consumerKind)
		if !exists {
			continue
		}
		validatedKinds[consumerKind] = struct{}{}
		if validationErr := writer.ValidateSkillConsumer(projectRoot, consumer, artifactName, sourcePaths, fetch); validationErr != nil {
			return fmt.Errorf("skill %s is incompatible with target %s: %w", artifactName, consumerKind, validationErr)
		}
	}
	return nil
}
