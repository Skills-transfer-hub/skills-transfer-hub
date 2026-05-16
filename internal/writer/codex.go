package writer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CodexWriter installs artifacts under the OpenAI Codex CLI conventions in
// `.codex/` and `AGENTS.md`. See https://github.com/openai/codex.
//
// Concepts:
//   - skill       -> .codex/prompts/<name>.md
//   - agent       -> .codex/agents/<name>.md
//   - command     -> .codex/commands/<name>.md
//   - instruction -> .codex/prompts/<name>.md (closest match to Copilot
//                    instructions; native AGENTS.md is repo-wide and not
//                    artifact-scoped)
type CodexWriter struct{}

// NewCodexWriter returns a ready-to-use Codex writer.
func NewCodexWriter() CodexWriter { return CodexWriter{} }

// Kind implements TargetWriter.
func (CodexWriter) Kind() string { return "codex" }

// Write writes a Codex artifact with the minimal STH HTML-comment marker.
func (CodexWriter) Write(req WriteRequest) (WriteResult, error) {
	if strings.TrimSpace(req.InstalledPath) == "" {
		return WriteResult{}, fmt.Errorf("installed_path doit être renseigné")
	}
	if err := os.MkdirAll(filepath.Dir(req.InstalledPath), 0o755); err != nil {
		return WriteResult{}, fmt.Errorf("création du dossier %s échouée : %w", filepath.Dir(req.InstalledPath), err)
	}

	content := req.Content
	if req.AddManagedHeader && strings.EqualFold(filepath.Ext(req.InstalledPath), ".md") {
		content = applyMinimalManagedMarker(req.InstalledPath, content)
	}

	if err := os.WriteFile(req.InstalledPath, content, 0o644); err != nil {
		return WriteResult{}, fmt.Errorf("écriture de %s échouée : %w", req.InstalledPath, err)
	}
	return WriteResult{
		Path:   filepath.ToSlash(filepath.Clean(req.InstalledPath)),
		SHA256: computeSHA256(content),
	}, nil
}

// Plan implements TargetPlanner for Codex.
func (CodexWriter) Plan(artifactKind, artifactName, _ string, sources []string) PlanResult {
	if artifactName == "" || len(sources) == 0 {
		return PlanResult{}
	}
	switch artifactKind {
	case "skill", "instruction":
		return PlanResult{Files: []PlannedFile{{SourcePath: sources[0], RelativePath: ".codex/prompts/" + artifactName + ".md"}}}
	case "agent":
		return PlanResult{Files: []PlannedFile{{SourcePath: sources[0], RelativePath: ".codex/agents/" + artifactName + ".md"}}}
	case "command":
		return PlanResult{Files: []PlannedFile{{SourcePath: sources[0], RelativePath: ".codex/commands/" + artifactName + ".md"}}}
	}
	return PlanResult{Skipped: true, Reason: "unknown artifact kind: " + artifactKind}
}
