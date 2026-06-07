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
}

type MaterializeResult struct {
	Skipped bool
	Reason  string
	Files   []MaterializedFile
}

func MaterializeArtifact(projectRoot string, target Target, artifactKind, artifactName, lang string, sources []string, fetch func(sourcePath string) ([]byte, error), addManagedHeader, write bool) (MaterializeResult, error) {
	plan := target.Plan(artifactKind, artifactName, lang, sources)
	if plan.Skipped {
		return MaterializeResult{Skipped: true, Reason: plan.Reason}, nil
	}

	files := make([]MaterializedFile, 0, len(plan.Files))
	for _, planned := range plan.Files {
		content, fetchErr := fetch(planned.SourcePath)
		if fetchErr != nil {
			return MaterializeResult{}, fetchErr
		}
		installedPath := filepath.Join(projectRoot, filepath.FromSlash(planned.RelativePath))
		finalContent := renderManaged(target, installedPath, content, addManagedHeader)
		if write {
			if err := os.MkdirAll(filepath.Dir(installedPath), 0o755); err != nil {
				return MaterializeResult{}, fmt.Errorf("création du dossier %s échouée : %w", filepath.Dir(installedPath), err)
			}
			if err := os.WriteFile(installedPath, finalContent, 0o644); err != nil {
				return MaterializeResult{}, fmt.Errorf("écriture de %s échouée : %w", installedPath, err)
			}
		}
		files = append(files, MaterializedFile{
			RelativePath: planned.RelativePath,
			SHA256:       computeSHA256(finalContent),
		})
	}
	return MaterializeResult{Files: files}, nil
}

func renderManaged(target Target, installedPath string, content []byte, addManagedHeader bool) []byte {
	if !addManagedHeader {
		return content
	}
	if _, isClaude := target.(ClaudeWriter); isClaude {
		return applyClaudeManagedHeader(installedPath, content)
	}
	if strings.EqualFold(filepath.Ext(installedPath), ".md") {
		return applyMinimalManagedMarker(installedPath, content)
	}
	return content
}
