package writer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CursorWriter installs artifacts under the Cursor conventions in `.cursor/`.
// See https://docs.cursor.com/.
//
// Concepts:
//   - skill       -> .cursor/rules/<name>.mdc
//   - agent       -> .cursor/rules/agents/<name>.mdc (Cursor has no native
//                    agent concept; rules namespace is the closest fit)
//   - command     -> .cursor/commands/<name>.md
//   - instruction -> .cursor/rules/<name>.mdc (with globs derived from
//                    upstream `applyTo` when present)
type CursorWriter struct{}

// NewCursorWriter returns a ready-to-use Cursor writer.
func NewCursorWriter() CursorWriter { return CursorWriter{} }

// Kind implements TargetWriter.
func (CursorWriter) Kind() string { return "cursor" }

// Write writes a Cursor artifact with the minimal STH HTML-comment marker
// for Markdown files (.md and .mdc both render Markdown).
func (CursorWriter) Write(req WriteRequest) (WriteResult, error) {
	if strings.TrimSpace(req.InstalledPath) == "" {
		return WriteResult{}, fmt.Errorf("installed_path doit être renseigné")
	}
	if err := os.MkdirAll(filepath.Dir(req.InstalledPath), 0o755); err != nil {
		return WriteResult{}, fmt.Errorf("création du dossier %s échouée : %w", filepath.Dir(req.InstalledPath), err)
	}

	content := req.Content
	if req.AddManagedHeader && isCursorMarkdownExt(req.InstalledPath) {
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

// Plan implements TargetPlanner for Cursor.
func (CursorWriter) Plan(artifactKind, artifactName, _ string, sources []string) PlanResult {
	if artifactName == "" || len(sources) == 0 {
		return PlanResult{}
	}
	switch artifactKind {
	case "skill", "instruction":
		return PlanResult{Files: []PlannedFile{{SourcePath: sources[0], RelativePath: ".cursor/rules/" + artifactName + ".mdc"}}}
	case "agent":
		return PlanResult{Files: []PlannedFile{{SourcePath: sources[0], RelativePath: ".cursor/rules/agents/" + artifactName + ".mdc"}}}
	case "command":
		return PlanResult{Files: []PlannedFile{{SourcePath: sources[0], RelativePath: ".cursor/commands/" + artifactName + ".md"}}}
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
