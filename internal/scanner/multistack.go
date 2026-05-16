package scanner

import (
	"fmt"
	"sort"
	"strings"
)

// MultiStackScanner discovers Lore-style enterprise repositories that bundle
// skills, agents, commands and per-language instructions in a single repo.
//
// Layouts recognized:
//   - skills      : <any-dir>/<name>/SKILL.md
//   - agents      : agents/<name>.md         (single-file with frontmatter)
//   - commands    : commands/<name>.md       (single-file) OR commands/<name>/SKILL.md (skill-style)
//   - instructions: <lang>/instructions/<name>.instructions.md
//
// Heuristic priority: a manifest.json at the root marks the repo as multi-stack
// regardless of layout; otherwise it triggers when at least two of the
// conventional containers are present.
type MultiStackScanner struct{}

// Name implements RepoScanner.
func (MultiStackScanner) Name() string { return "multi-stack" }

// Detect returns true when the tree shape matches a multi-stack enterprise repo.
func (MultiStackScanner) Detect(tree []TreeEntry) bool {
	containers := map[string]bool{}
	for _, entry := range tree {
		normalized := NormalizePath(entry.Path)
		if normalized == "manifest.json" {
			return true
		}
		top, _, found := strings.Cut(normalized, "/")
		if !found {
			continue
		}
		switch top {
		case "skills", "agents", "commands":
			containers[top] = true
		}
		if strings.Contains(normalized, "/instructions/") && strings.HasSuffix(normalized, ".instructions.md") {
			containers["instructions"] = true
		}
	}
	return len(containers) >= 2
}

// Scan walks the tree and produces one Artifact per discovered item. The
// returned slice is sorted by (Kind, Category, Name) so output is stable.
func (MultiStackScanner) Scan(tree []TreeEntry) ([]Artifact, error) {
	commandsAsDir := commandsWithSkillDir(tree)
	skillSeen := map[string]bool{}
	artifacts := make([]Artifact, 0, len(tree))

	for _, entry := range tree {
		normalized := NormalizePath(entry.Path)
		if normalized == "" {
			continue
		}

		switch {
		case strings.HasSuffix(normalized, "/SKILL.md"):
			art, ok := skillFromPath(normalized)
			if !ok || skillSeen[art.Entrypoint] {
				continue
			}
			skillSeen[art.Entrypoint] = true
			artifacts = append(artifacts, art)

		case strings.HasPrefix(normalized, "agents/") && strings.HasSuffix(normalized, ".md"):
			parts := strings.Split(normalized, "/")
			if len(parts) != 2 {
				continue
			}
			name := strings.TrimSuffix(parts[1], ".md")
			if name == "" {
				continue
			}
			artifacts = append(artifacts, Artifact{
				Kind:       KindAgent,
				Name:       name,
				Category:   "agents",
				Files:      []string{normalized},
				Entrypoint: normalized,
			})

		case strings.HasPrefix(normalized, "commands/") && strings.HasSuffix(normalized, ".md"):
			parts := strings.Split(normalized, "/")
			if len(parts) != 2 {
				continue
			}
			name := strings.TrimSuffix(parts[1], ".md")
			// If `commands/<name>/SKILL.md` also exists, the skill-style entry wins
			// and the single-file form is ignored to avoid duplicates.
			if name == "" || commandsAsDir[name] {
				continue
			}
			artifacts = append(artifacts, Artifact{
				Kind:       KindCommand,
				Name:       name,
				Category:   "commands",
				Files:      []string{normalized},
				Entrypoint: normalized,
			})

		case strings.Contains(normalized, "/instructions/") && strings.HasSuffix(normalized, ".instructions.md"):
			parts := strings.Split(normalized, "/")
			if len(parts) < 3 {
				continue
			}
			lang := parts[0]
			fileName := parts[len(parts)-1]
			name := strings.TrimSuffix(fileName, ".instructions.md")
			if name == "" {
				continue
			}
			artifacts = append(artifacts, Artifact{
				Kind:       KindInstruction,
				Name:       name,
				Category:   lang + "/instructions",
				Lang:       lang,
				Files:      []string{normalized},
				Entrypoint: normalized,
			})
		}
	}

	if len(artifacts) == 0 {
		return nil, fmt.Errorf("aucun artefact détecté dans l'arborescence du dépôt")
	}

	sort.SliceStable(artifacts, func(left, right int) bool {
		if artifacts[left].Kind != artifacts[right].Kind {
			return string(artifacts[left].Kind) < string(artifacts[right].Kind)
		}
		if artifacts[left].Category != artifacts[right].Category {
			return artifacts[left].Category < artifacts[right].Category
		}
		return artifacts[left].Name < artifacts[right].Name
	})
	return artifacts, nil
}

// skillFromPath turns "react/form-wizard/SKILL.md" or
// "commands/fix-pr/SKILL.md" into a skill Artifact. The category is the
// path component immediately before the skill folder (so the convention
// captures both top-level and language-scoped skills uniformly).
func skillFromPath(p string) (Artifact, bool) {
	parts := strings.Split(p, "/")
	if len(parts) < 3 || parts[len(parts)-1] != "SKILL.md" {
		return Artifact{}, false
	}
	name := parts[len(parts)-2]
	category := strings.Join(parts[:len(parts)-2], "/")
	if name == "" || category == "" {
		return Artifact{}, false
	}
	lang := parts[0]
	if lang == name || lang == category {
		lang = ""
	}
	return Artifact{
		Kind:       KindSkill,
		Name:       name,
		Category:   category,
		Lang:       lang,
		Files:      []string{p},
		Entrypoint: p,
	}, true
}

// commandsWithSkillDir returns the set of <name> for which a
// `commands/<name>/SKILL.md` entry exists in the tree.
func commandsWithSkillDir(tree []TreeEntry) map[string]bool {
	seen := map[string]bool{}
	for _, entry := range tree {
		normalized := NormalizePath(entry.Path)
		if !strings.HasPrefix(normalized, "commands/") || !strings.HasSuffix(normalized, "/SKILL.md") {
			continue
		}
		parts := strings.Split(normalized, "/")
		if len(parts) == 3 {
			seen[parts[1]] = true
		}
	}
	return seen
}

// PickScanner picks the RepoScanner to use against tree. When hint is set
// ("flat" or "multi-stack") it is honored unconditionally; otherwise the
// multi-stack scanner is tried first and FlatSkillScanner is the fallback.
func PickScanner(tree []TreeEntry, hint string) RepoScanner {
	switch strings.ToLower(strings.TrimSpace(hint)) {
	case "flat":
		return FlatSkillScanner{}
	case "multi-stack", "multistack":
		return MultiStackScanner{}
	}
	if (MultiStackScanner{}).Detect(tree) {
		return MultiStackScanner{}
	}
	return FlatSkillScanner{}
}

// Compile-time guards: both scanners satisfy RepoScanner.
var (
	_ RepoScanner = FlatSkillScanner{}
	_ RepoScanner = MultiStackScanner{}
)
