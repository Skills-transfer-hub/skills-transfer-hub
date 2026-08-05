package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	packLayoutFlat       = "flat"
	packLayoutMultiStack = "multi-stack"
)

// runPackInit walks the user through scaffolding a brand-new skills
// repository in one of two layouts:
//
//   - flat: anthropics/skills-style — every skill is a top-level
//     <name>/SKILL.md.
//   - multi-stack: enterprise / Lore-style — skills/agents/commands/instructions
//     organized by techno container, with a root manifest.json so STH's
//     MultiStackScanner detects the layout deterministically.
//
// Scaffolding is always written even when post-scaffold validation fails;
// validation errors still surface as a non-zero exit so CI catches them
// (matches the `sth init` exit-code policy).
func (a *app) runPackInit(args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("sth pack init n'accepte pas d'argument positionnel (reçu : %v)", args)
	}

	reader := bufio.NewReader(a.stdin)
	a.writeStdout(
		"sth pack init — scaffolde un repo de skills compatible STH.",
		"",
	)

	name, err := promptValue(reader, a.stdout, "Nom du pack (ex: my-skills)", "")
	if err != nil {
		return err
	}
	if !isValidPackName(name) {
		return fmt.Errorf("nom de pack invalide %q : caractères autorisés [a-z0-9-_]", name)
	}

	layout, err := promptValue(reader, a.stdout, "Layout (flat|multi-stack)", packLayoutMultiStack)
	if err != nil {
		return err
	}
	layout = strings.ToLower(strings.TrimSpace(layout))
	if layout != packLayoutFlat && layout != packLayoutMultiStack {
		return fmt.Errorf("layout inconnu %q (attendu : flat ou multi-stack)", layout)
	}

	targetDir, err := promptValue(reader, a.stdout, "Dossier cible", "./"+name)
	if err != nil {
		return err
	}
	targetDir = filepath.Clean(targetDir)

	a.writeStdout(
		"",
		fmt.Sprintf("Récap : pack %q | layout %s | dossier %s", name, layout, targetDir),
	)
	confirm, err := promptYesNo(reader, a.stdout, "Confirmer la création", true)
	if err != nil {
		return err
	}
	if !confirm {
		return errors.New("annulé par l'utilisateur")
	}

	if err := scaffoldPack(targetDir, name, layout); err != nil {
		return err
	}

	a.writeStdout(
		"",
		fmt.Sprintf("Pack scaffold dans %s.", targetDir),
		"Étapes suivantes :",
		"  1. cd "+targetDir,
		"  2. git init && git add . && git commit -m \"initial pack\"",
		"  3. Publier sur ton provider git puis pointer un consommateur via `sth init`.",
	)
	return validatePack(targetDir, layout)
}

// isValidPackName accepts the same character set as a typical npm / Go
// module name segment: lowercase letters, digits, `-` and `_`.
func isValidPackName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_':
		default:
			return false
		}
	}
	return true
}

// scaffoldPack writes the seed file tree for the chosen layout.
//
// Refuses to write into a non-empty existing directory so an accidental
// `sth pack init` inside a real project does not silently overwrite
// SKILLS.md / README.md / sample-skill files. The caller can either
// pick a fresh path or empty the directory first; we intentionally do
// NOT offer a --force flag here — recovering from a destroyed README is
// expensive and the user has time to delete one folder.
func scaffoldPack(targetDir, name, layout string) error {
	if entries, err := os.ReadDir(targetDir); err == nil && len(entries) > 0 {
		return fmt.Errorf("le dossier cible %s existe déjà et n'est pas vide ; choisis un chemin neuf ou vide-le d'abord", targetDir)
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("création de %s échouée : %w", targetDir, err)
	}

	files := map[string]string{
		"README.md":     renderReadme(name, layout),
		".sthignore":    defaultSthignore,
		"SKILLS.md":     renderSkillsMd(layout),
		"manifest.json": renderManifest(name, layout),
	}

	switch layout {
	case packLayoutFlat:
		files["sample-skill/SKILL.md"] = sampleSkillContent("sample-skill", "Exemple minimal de skill.")
	case packLayoutMultiStack:
		files["skills/sample/SKILL.md"] = sampleSkillContent("sample", "Skill multi-stack d'exemple.")
		files["agents/sample.md"] = sampleAgentContent("sample")
		files["commands/sample.md"] = sampleCommandContent("sample")
		files["common/instructions/sample.instructions.md"] = sampleInstructionContent()
	}

	for relPath, content := range files {
		full := filepath.Join(targetDir, relPath)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return fmt.Errorf("création de %s échouée : %w", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			return fmt.Errorf("écriture de %s échouée : %w", full, err)
		}
	}
	return nil
}

// validatePack performs a smoke test on the freshly scaffolded layout to
// catch a corrupted template before the user pushes it.
func validatePack(targetDir, layout string) error {
	manifest := filepath.Join(targetDir, "manifest.json")
	if _, err := os.Stat(manifest); err != nil {
		return fmt.Errorf("validation post-scaffold : manifest.json manquant : %w", err)
	}
	switch layout {
	case packLayoutFlat:
		if _, err := os.Stat(filepath.Join(targetDir, "sample-skill", "SKILL.md")); err != nil {
			return fmt.Errorf("validation post-scaffold : sample-skill/SKILL.md manquant : %w", err)
		}
	case packLayoutMultiStack:
		if _, err := os.Stat(filepath.Join(targetDir, "skills", "sample", "SKILL.md")); err != nil {
			return fmt.Errorf("validation post-scaffold : skills/sample/SKILL.md manquant : %w", err)
		}
	}
	return nil
}

const defaultSthignore = `# Files/dirs ignored by sth list / sth install
.git/
node_modules/
dist/
*.tmp
`

func renderReadme(name, layout string) string {
	return fmt.Sprintf(`# %s

Skills pack scaffold par sth (`+"`sth pack init`"+`).

Layout : **%s**

## Usage

Depuis un projet consommateur :

    sth init    # pointer ce repo
    sth list    # lister les skills exposés
    sth install <ref>

`, name, layout)
}

func renderSkillsMd(layout string) string {
	header := `# Catalogue

| Type | Name | Folder | Source |
|------|------|--------|--------|
`
	switch layout {
	case packLayoutFlat:
		return header + `| skill | sample-skill | . | sample-skill/SKILL.md |
`
	}
	return header + `| skill | sample | skills | skills/sample/SKILL.md |
| agent | sample | agents | agents/sample.md |
| command | sample | commands | commands/sample.md |
| instruction | sample | common/instructions | common/instructions/sample.instructions.md |
`
}

func renderManifest(name, layout string) string {
	supportedTargets := `["claude","copilot","codex","cursor","antigravity","gemini"]`
	return fmt.Sprintf(`{
  "name": %q,
  "layout": %q,
  "supported_targets": %s,
  "schema_version": 1
}
`, name, layout, supportedTargets)
}

func sampleSkillContent(name, description string) string {
	return fmt.Sprintf(`---
name: %s
description: %s
---

# %s

Décris ici la skill et son usage. Tu peux ajouter des sous-fichiers dans le
dossier `+"`references/`"+` à côté de ce SKILL.md.
`, name, description, name)
}

func sampleAgentContent(name string) string {
	return fmt.Sprintf(`---
name: %s
description: Agent d'exemple scaffold par sth pack init.
---

Tu es un agent d'exemple. Remplace ce prompt par les instructions réelles
de ton agent.
`, name)
}

func sampleCommandContent(name string) string {
	return fmt.Sprintf(`---
name: %s
description: Commande slash d'exemple scaffold par sth pack init.
---

Documente ici ce que la commande doit accomplir. Le contenu est exposé en
tant que `+"`/%s`"+` dans les outils compatibles (Claude Code, Copilot CLI,
Cursor, Antigravity et Gemini CLI).
`, name, name)
}

func sampleInstructionContent() string {
	return `---
description: Instruction d'exemple scaffold par sth pack init.
applyTo: '**/*.md'
---

Cette instruction sera injectée comme contexte ambiant pour les fichiers
matchant ` + "`applyTo`" + `. Format compatible GitHub Copilot, mappé en
` + "`globs:`" + ` côté Cursor et en règles projet pour Claude Code et Antigravity.
`
}
