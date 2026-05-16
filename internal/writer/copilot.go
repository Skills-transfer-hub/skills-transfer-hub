package writer

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CopilotWriter installs artifacts under GitHub Copilot's conventions in
// `.github/`. See https://docs.github.com/en/copilot/customizing-copilot.
//
// Concepts:
//   - skill       -> .github/instructions/<name>.instructions.md (consolidated)
//   - agent       -> .github/agents/<name>.agent.md (renamed from chatmode)
//   - command     -> NOT supported (Copilot has no slash-command concept)
//   - instruction -> .github/instructions/<name>.instructions.md (native)
type CopilotWriter struct{}

// NewCopilotWriter returns a ready-to-use Copilot writer.
func NewCopilotWriter() CopilotWriter { return CopilotWriter{} }

// Kind implements TargetWriter.
func (CopilotWriter) Kind() string { return "copilot" }

// Write writes a Copilot artifact with the minimal STH HTML-comment marker.
func (CopilotWriter) Write(req WriteRequest) (WriteResult, error) {
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

// Plan implements TargetPlanner for Copilot.
func (CopilotWriter) Plan(artifactKind, artifactName, _ string, sources []string) PlanResult {
	if artifactName == "" || len(sources) == 0 {
		return PlanResult{}
	}
	switch artifactKind {
	case "skill", "instruction":
		// Skills are consolidated into a single Copilot instruction file
		// (Copilot does not have the concept of a multi-file skill bundle).
		return PlanResult{Files: []PlannedFile{{SourcePath: sources[0], RelativePath: ".github/instructions/" + artifactName + ".instructions.md"}}}
	case "agent":
		return PlanResult{Files: []PlannedFile{{SourcePath: sources[0], RelativePath: ".github/agents/" + artifactName + ".agent.md"}}}
	case "command":
		return PlanResult{
			Skipped: true,
			Reason:  "copilot has no slash-command concept; skipping command " + artifactName,
		}
	}
	return PlanResult{Skipped: true, Reason: "unknown artifact kind: " + artifactKind}
}

// minimalManagedMarker is a tool-agnostic HTML comment STH uses to flag
// files it manages outside Claude. It is invisible at Markdown render time
// but matchable on disk and idempotent (a second write does not duplicate it).
const minimalManagedMarker = "<!-- managed by sth -->\n"

// applyMinimalManagedMarker prepends the minimal STH marker to Markdown
// content, preserving any YAML frontmatter (which must remain on line 1).
// Idempotent: re-applying it on already-marked content is a no-op.
func applyMinimalManagedMarker(_ string, content []byte) []byte {
	marker := []byte(minimalManagedMarker)
	if bodyStart := FrontmatterBodyOffset(content); bodyStart >= 0 {
		if bytes.HasPrefix(content[bodyStart:], marker) {
			return content
		}
		out := make([]byte, 0, len(content)+len(marker))
		out = append(out, content[:bodyStart]...)
		out = append(out, marker...)
		out = append(out, content[bodyStart:]...)
		return out
	}
	if bytes.HasPrefix(content, marker) {
		return content
	}
	return append(marker, content...)
}
