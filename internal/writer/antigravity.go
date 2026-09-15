package writer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type AntigravityWriter struct{}

func NewAntigravityWriter() AntigravityWriter { return AntigravityWriter{} }

func (AntigravityWriter) Kind() string { return "antigravity" }

func (AntigravityWriter) Write(req WriteRequest) (WriteResult, error) {
	if strings.TrimSpace(req.InstalledPath) == "" {
		return WriteResult{}, fmt.Errorf("installed_path is required")
	}
	if err := os.MkdirAll(filepath.Dir(req.InstalledPath), 0o755); err != nil {
		return WriteResult{}, fmt.Errorf("failed to create directory %s: %w", filepath.Dir(req.InstalledPath), err)
	}

	content := req.Content
	if req.AddManagedHeader && strings.EqualFold(filepath.Ext(req.InstalledPath), ".md") {
		content = applyMinimalManagedMarker(req.InstalledPath, content)
	}

	if err := os.WriteFile(req.InstalledPath, content, 0o644); err != nil {
		return WriteResult{}, fmt.Errorf("failed to write %s: %w", req.InstalledPath, err)
	}
	return WriteResult{
		Path:   filepath.ToSlash(filepath.Clean(req.InstalledPath)),
		SHA256: computeSHA256(content),
	}, nil
}

func (AntigravityWriter) Plan(artifactKind, artifactName, _ string, sources []string) PlanResult {
	if strings.TrimSpace(artifactName) == "" || len(sources) == 0 {
		return PlanResult{}
	}
	switch artifactKind {
	case "skill":
		return PlanResult{Files: relForSkillBundle(".agents/skills", artifactName, sources)}
	case "agent":
		return PlanResult{Files: []PlannedFile{{SourcePath: sources[0], RelativePath: ".agents/agents/" + artifactName + "/agent.md"}}}
	case "command":
		return PlanResult{Files: []PlannedFile{{SourcePath: sources[0], RelativePath: ".agents/workflows/" + artifactName + ".md"}}}
	case "instruction":
		return PlanResult{Files: []PlannedFile{{SourcePath: sources[0], RelativePath: ".agents/rules/" + artifactName + ".md"}}}
	case "rule":
		return PlanResult{Files: []PlannedFile{{SourcePath: sources[0], RelativePath: ".agents/rules/" + artifactName + ".md"}}}
	case "workflow":
		return PlanResult{Files: []PlannedFile{{SourcePath: sources[0], RelativePath: ".agents/workflows/" + artifactName + ".md"}}}
	}
	return PlanResult{Skipped: true, Reason: "unknown artifact kind: " + artifactKind}
}
