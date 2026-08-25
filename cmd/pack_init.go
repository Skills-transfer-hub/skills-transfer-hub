package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"sth/internal/tui"
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
		return fmt.Errorf("sth pack init does not accept positional arguments (received: %v)", args)
	}

	session := tui.NewSession(a.stdin, a.stdout)
	a.writeStdout(
		"sth pack init — scaffold an STH-compatible skill repository.",
		"",
	)

	name, err := tui.Input(session, tui.InputConfig{Label: "Pack name (example: my-skills)", Required: true})
	if err != nil {
		return err
	}
	if !isValidPackName(name) {
		return fmt.Errorf("invalid pack name %q: allowed characters are [a-z0-9-_]", name)
	}

	layoutIndex, err := tui.Select(session, tui.SelectConfig{
		Title: "Pack layout",
		Options: []tui.Option{
			{Label: "multi-stack — one folder per target stack", Aliases: []string{"multi-stack", "multi", "m"}},
			{Label: "flat — everything at the repository root", Aliases: []string{"flat", "f"}},
		},
	})
	if err != nil {
		return err
	}
	layout := packLayoutMultiStack
	if layoutIndex == 1 {
		layout = packLayoutFlat
	}

	targetDir, err := tui.Input(session, tui.InputConfig{Label: "Target folder", Default: "./" + name, Required: true})
	if err != nil {
		return err
	}
	targetDir = filepath.Clean(targetDir)

	a.writeStdout(
		"",
		fmt.Sprintf("Summary: pack %q | layout %s | folder %s", name, layout, targetDir),
	)
	confirm, err := tui.Confirm(session, tui.ConfirmConfig{Question: "Confirm creation?", DefaultYes: true})
	if err != nil {
		return err
	}
	if !confirm {
		return errors.New("canceled by user")
	}

	if err := scaffoldPack(targetDir, name, layout); err != nil {
		return err
	}

	a.writeStdout(
		"",
		fmt.Sprintf("Pack scaffolded in %s.", targetDir),
		"Next steps:",
		"  1. cd "+targetDir,
		"  2. git init && git add . && git commit -m \"initial pack\"",
		"  3. Publish to your Git provider, then point a consumer at it with `sth init`.",
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
		return fmt.Errorf("target folder %s already exists and is not empty; choose a new path or empty it first", targetDir)
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("failed to create %s: %w", targetDir, err)
	}

	files := map[string]string{
		"README.md":     renderReadme(name, layout),
		".sthignore":    defaultSthignore,
		"SKILLS.md":     renderSkillsMd(layout),
		"manifest.json": renderManifest(name, layout),
	}

	switch layout {
	case packLayoutFlat:
		files["sample-skill/SKILL.md"] = sampleSkillContent("sample-skill", "Minimal skill example.")
	case packLayoutMultiStack:
		files["skills/sample/SKILL.md"] = sampleSkillContent("sample", "Multi-stack skill example.")
		files["agents/sample.md"] = sampleAgentContent("sample")
		files["commands/sample.md"] = sampleCommandContent("sample")
		files["common/instructions/sample.instructions.md"] = sampleInstructionContent()
	}

	for relPath, content := range files {
		full := filepath.Join(targetDir, relPath)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return fmt.Errorf("failed to create %s: %w", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			return fmt.Errorf("failed to write %s: %w", full, err)
		}
	}
	return nil
}

// validatePack performs a smoke test on the freshly scaffolded layout to
// catch a corrupted template before the user pushes it.
func validatePack(targetDir, layout string) error {
	manifest := filepath.Join(targetDir, "manifest.json")
	if _, err := os.Stat(manifest); err != nil {
		return fmt.Errorf("post-scaffold validation: missing manifest.json: %w", err)
	}
	switch layout {
	case packLayoutFlat:
		if _, err := os.Stat(filepath.Join(targetDir, "sample-skill", "SKILL.md")); err != nil {
			return fmt.Errorf("post-scaffold validation: missing sample-skill/SKILL.md: %w", err)
		}
	case packLayoutMultiStack:
		if _, err := os.Stat(filepath.Join(targetDir, "skills", "sample", "SKILL.md")); err != nil {
			return fmt.Errorf("post-scaffold validation: missing skills/sample/SKILL.md: %w", err)
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

Skills pack scaffolded by sth (`+"`sth pack init`"+`).

Layout: **%s**

## Usage

From a consumer project:

    sth init    # point to this repository
    sth list    # list exposed skills
    sth install <ref>

`, name, layout)
}

func renderSkillsMd(layout string) string {
	header := `# Catalog

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

Describe the skill and its usage here. You can add supporting files in the
`+"`references/`"+` folder next to this SKILL.md.
`, name, description, name)
}

func sampleAgentContent(name string) string {
	return fmt.Sprintf(`---
name: %s
description: Sample agent scaffolded by sth pack init.
---

You are a sample agent. Replace this prompt with your agent's actual
instructions.
`, name)
}

func sampleCommandContent(name string) string {
	return fmt.Sprintf(`---
name: %s
description: Sample slash command scaffolded by sth pack init.
---

Document what the command should do here. The content is exposed as `+"`/%s`"+`
in compatible tools (Claude Code, Copilot CLI, Cursor, Antigravity, and Gemini CLI).
`, name, name)
}

func sampleInstructionContent() string {
	return `---
description: Sample instruction scaffolded by sth pack init.
applyTo: '**/*.md'
---

This instruction is injected as ambient context for files matching ` + "`applyTo`" + `.
It is compatible with GitHub Copilot, mapped to ` + "`globs:`" + ` for Cursor,
and mapped to project rules for Claude Code and Antigravity.
`
}
