package writer

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ClaudeManagedHeader is the visible STH banner prepended to Markdown files
// installed for Claude Code. The wording is intentionally loud — users that
// edit a managed file should see it immediately.
//
// Exported so the fs package (which still owns a few legacy write paths)
// can share the single source of truth; previously fs.go duplicated this
// constant under the name skillHeader.
const ClaudeManagedHeader = "> ⚠️ ATTENTION : Skill géré par STH. Ne pas modifier manuellement.\n\n"

// ClaudeWriter installs artifacts under a Claude Code project tree.
// It is the historical (and currently only) writer; other tools land in PR5.
type ClaudeWriter struct{}

// NewClaudeWriter returns a ready-to-use Claude writer.
func NewClaudeWriter() ClaudeWriter { return ClaudeWriter{} }

// Kind implements TargetWriter.
func (ClaudeWriter) Kind() string { return "claude" }

// Plan implements TargetPlanner for Claude Code.
//
//   - skill       -> .claude/skills/<category>/<name>/<file>
//   - agent       -> .claude/agents/<name>.md
//   - command     -> .claude/commands/<name>.md
//   - instruction -> .claude/skills/instructions/<name>.md (no native concept)
func (ClaudeWriter) Plan(artifactKind, artifactName, _ string, sources []string) PlanResult {
	if artifactName == "" || len(sources) == 0 {
		return PlanResult{}
	}
	switch artifactKind {
	case "skill":
		files := make([]PlannedFile, 0, len(sources))
		for _, source := range sources {
			files = append(files, PlannedFile{
				SourcePath:   source,
				RelativePath: relForClaudeSkill(artifactName, source),
			})
		}
		return PlanResult{Files: files}
	case "agent":
		return PlanResult{Files: []PlannedFile{{SourcePath: sources[0], RelativePath: ".claude/agents/" + artifactName + ".md"}}}
	case "command":
		return PlanResult{Files: []PlannedFile{{SourcePath: sources[0], RelativePath: ".claude/commands/" + artifactName + ".md"}}}
	case "instruction":
		return PlanResult{Files: []PlannedFile{{SourcePath: sources[0], RelativePath: ".claude/skills/instructions/" + artifactName + ".md"}}}
	}
	return PlanResult{Skipped: true, Reason: "unknown artifact kind: " + artifactKind}
}

// relForClaudeSkill maps an upstream source path under a skill bundle to
// its Claude-Code on-disk relative path under .claude/skills/<name>/.
// The historical layout keeps the skill folder name as the leaf; any extra
// assets are nested below it preserving the upstream structure.
func relForClaudeSkill(name, source string) string {
	base := ".claude/skills/" + name
	// Keep the leaf path component(s) after the skill folder if any.
	if idx := strings.LastIndex(source, "/"); idx >= 0 {
		leaf := source[idx+1:]
		if leaf == "SKILL.md" {
			return base + "/SKILL.md"
		}
		return base + "/" + leaf
	}
	return base + "/" + source
}

// Write materializes req on disk and returns the on-disk SHA-256.
func (ClaudeWriter) Write(req WriteRequest) (WriteResult, error) {
	if strings.TrimSpace(req.InstalledPath) == "" {
		return WriteResult{}, fmt.Errorf("installed_path doit être renseigné")
	}
	if err := os.MkdirAll(filepath.Dir(req.InstalledPath), 0o755); err != nil {
		return WriteResult{}, fmt.Errorf("création du dossier %s échouée : %w", filepath.Dir(req.InstalledPath), err)
	}

	content := req.Content
	if req.AddManagedHeader {
		content = applyClaudeManagedHeader(req.InstalledPath, content)
	}

	if err := os.WriteFile(req.InstalledPath, content, 0o644); err != nil {
		return WriteResult{}, fmt.Errorf("écriture de %s échouée : %w", req.InstalledPath, err)
	}

	return WriteResult{
		Path:   filepath.ToSlash(filepath.Clean(req.InstalledPath)),
		SHA256: computeSHA256(content),
	}, nil
}

// applyClaudeManagedHeader prepends the STH banner to Markdown content,
// preserving any YAML frontmatter (which must remain on line 1).
func applyClaudeManagedHeader(installedPath string, content []byte) []byte {
	if !strings.EqualFold(filepath.Ext(installedPath), ".md") {
		return content
	}

	header := []byte(ClaudeManagedHeader)

	if bodyStart := FrontmatterBodyOffset(content); bodyStart >= 0 {
		if bytes.HasPrefix(content[bodyStart:], header) {
			return content
		}
		out := make([]byte, 0, len(content)+len(header))
		out = append(out, content[:bodyStart]...)
		out = append(out, header...)
		out = append(out, content[bodyStart:]...)
		return out
	}

	if bytes.HasPrefix(content, header) {
		return content
	}
	return append(header, content...)
}

// FrontmatterBodyOffset returns the byte offset of the body (i.e. the index
// just past the closing `---` line and its terminating newline) when content
// starts with a YAML frontmatter block. Returns -1 when there is no
// frontmatter. Tolerates an optional UTF-8 BOM and both LF and CRLF.
//
// Exported so the fs package can drop its own private copy and route
// through the single implementation.
func FrontmatterBodyOffset(content []byte) int {
	bomLen := 0
	if bytes.HasPrefix(content, []byte{0xEF, 0xBB, 0xBF}) {
		bomLen = 3
	}
	rest := content[bomLen:]

	var openLen int
	switch {
	case bytes.HasPrefix(rest, []byte("---\n")):
		openLen = 4
	case bytes.HasPrefix(rest, []byte("---\r\n")):
		openLen = 5
	default:
		return -1
	}

	body := rest[openLen:]
	for offset := 0; offset <= len(body); {
		nl := bytes.IndexByte(body[offset:], '\n')
		var line []byte
		var nextOffset int
		if nl == -1 {
			line = body[offset:]
			nextOffset = len(body)
		} else {
			line = body[offset : offset+nl]
			nextOffset = offset + nl + 1
		}
		if bytes.Equal(bytes.TrimRight(line, "\r"), []byte("---")) {
			return bomLen + openLen + nextOffset
		}
		if nl == -1 {
			break
		}
		offset = nextOffset
	}
	return -1
}

func computeSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
