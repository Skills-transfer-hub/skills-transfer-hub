package writer

import (
	"os"
	"path/filepath"
)

// DetectedTarget describes one target inferred from the project tree.
// Auto-detection rules are intentionally conservative — STH only marks a
// target as detected when a *conventional file* unique to that tool is
// present, never when an ambiguous parent directory is. The wizard surfaces
// the detected list to the user, who has the last word.
type DetectedTarget struct {
	Kind    string // "claude" | "copilot" | "codex" | "cursor"
	RootDir string // project-relative root directory for the target
	Evidence string // path that triggered detection (for UX transparency)
}

// DetectAvailableTargets walks projectRoot for the conventional markers of
// the four supported tools and returns one DetectedTarget per match.
// Returns an empty slice (never nil) when projectRoot is empty or unreadable;
// callers should fall back to a Claude-only default in that case.
func DetectAvailableTargets(projectRoot string) []DetectedTarget {
	out := make([]DetectedTarget, 0, 4)
	if projectRoot == "" {
		return out
	}

	// Claude: presence of .claude/ directory OR a root CLAUDE.md.
	if exists(filepath.Join(projectRoot, ".claude")) {
		out = append(out, DetectedTarget{Kind: "claude", RootDir: ".claude", Evidence: ".claude"})
	} else if exists(filepath.Join(projectRoot, "CLAUDE.md")) {
		out = append(out, DetectedTarget{Kind: "claude", RootDir: ".claude", Evidence: "CLAUDE.md"})
	}

	// Copilot: only the conventional file or instructions directory counts —
	// .github/ alone is too ambiguous (workflows, CODEOWNERS, etc.).
	switch {
	case exists(filepath.Join(projectRoot, ".github", "copilot-instructions.md")):
		out = append(out, DetectedTarget{Kind: "copilot", RootDir: ".github", Evidence: ".github/copilot-instructions.md"})
	case exists(filepath.Join(projectRoot, ".github", "instructions")):
		out = append(out, DetectedTarget{Kind: "copilot", RootDir: ".github", Evidence: ".github/instructions/"})
	}

	// Codex: .codex/ directory OR AGENTS.md at the repo root.
	if exists(filepath.Join(projectRoot, ".codex")) {
		out = append(out, DetectedTarget{Kind: "codex", RootDir: ".codex", Evidence: ".codex"})
	} else if exists(filepath.Join(projectRoot, "AGENTS.md")) {
		out = append(out, DetectedTarget{Kind: "codex", RootDir: ".codex", Evidence: "AGENTS.md"})
	}

	// Cursor: .cursor/ directory OR legacy .cursorrules file.
	if exists(filepath.Join(projectRoot, ".cursor")) {
		out = append(out, DetectedTarget{Kind: "cursor", RootDir: ".cursor", Evidence: ".cursor"})
	} else if exists(filepath.Join(projectRoot, ".cursorrules")) {
		out = append(out, DetectedTarget{Kind: "cursor", RootDir: ".cursor", Evidence: ".cursorrules"})
	}

	return out
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
