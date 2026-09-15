package writer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type CursorWriter struct{}

func NewCursorWriter() CursorWriter { return CursorWriter{} }

func (CursorWriter) Kind() string { return "cursor" }

func (CursorWriter) Write(req WriteRequest) (WriteResult, error) {
	if strings.TrimSpace(req.InstalledPath) == "" {
		return WriteResult{}, fmt.Errorf("installed_path is required")
	}
	if err := os.MkdirAll(filepath.Dir(req.InstalledPath), 0o755); err != nil {
		return WriteResult{}, fmt.Errorf("failed to create directory %s: %w", filepath.Dir(req.InstalledPath), err)
	}

	content := req.Content
	if req.AddManagedHeader && isCursorMarkdownExt(req.InstalledPath) {
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

func (CursorWriter) Plan(artifactKind, artifactName, _ string, sources []string) PlanResult {
	if artifactName == "" || len(sources) == 0 {
		return PlanResult{}
	}
	switch artifactKind {
	case "skill":
		return PlanResult{Files: relForSkillBundle(".agents/skills", artifactName, sources)}
	case "instruction":
		return PlanResult{Files: []PlannedFile{{SourcePath: sources[0], RelativePath: ".cursor/sth/instructions/" + artifactName + ".md"}}}
	case "agent":
		return PlanResult{Files: []PlannedFile{{SourcePath: sources[0], RelativePath: ".cursor/agents/" + artifactName + ".md"}}}
	case "rule":
		return PlanResult{Files: []PlannedFile{{SourcePath: sources[0], RelativePath: ".cursor/rules/" + artifactName + ".mdc"}}}
	case "command":
		return PlanResult{Skipped: true, Reason: "Cursor commands are not advertised without a current certified contract"}
	case "workflow":
		return PlanResult{Skipped: true, Reason: "Cursor workflows are outside the V1 artifact contract"}
	}
	return PlanResult{Skipped: true, Reason: "unknown artifact kind: " + artifactKind}
}

func isCursorMarkdownExt(p string) bool {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".md", ".mdc":
		return true
	}
	return false
}
