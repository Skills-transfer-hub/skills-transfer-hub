# Implementation Plan: Ecosystem Tooling

**Branch**: `feat/008-ecosystem-tooling` | **Date**: 2026-07-06 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `specs/008-ecosystem-tooling/spec.md`

## Summary

Build three independent capabilities for skill-repository maintainers and consumers,
reusing the discovery/parsing primitives already delivered by specs 005/006 rather than
introducing new ones:

1. **`sth catalog init`** generates `SKILLS.md` from a local `category/skill/SKILL.md`
   tree. It runs the existing `scanner.FlatSkillScanner` / `scanner.MultiStackScanner`
   against a locally-built `[]scanner.TreeEntry` (instead of a remote provider tree),
   reads each `SKILL.md`'s front matter with the existing `scanner.ExtractFrontmatter`,
   and renders a single markdown table — the exact format `internal/http.parseCatalogEntries`
   already parses (`| Type | Name | Folder | Source | Description |`, sorted by folder
   then name). No new discovery or parsing logic is needed on the read side; only a
   local tree walker and a table renderer are new.
2. **`sth new <category>/<name>`** scaffolds a single skill directory with a
   `SKILL.md` template (`name:`, `description:`, `## Usage`). Independent of catalog
   init and of `sth pack init` (which scaffolds a whole repo, not one skill).
3. **Changelog display on update** parses `CHANGELOG.md` (Keep a Changelog format)
   and prints entries introduced since `previous_version` (already tracked in the v2
   state schema from spec 005) under a "What's new in `<resource-name>`:" header;
   raw fallback for non-conforming formats, 50 KB truncation, never blocks the update,
   and a `changelog` field is added to `--json` output.

All logic is pure Go stdlib, `CGO_ENABLED=0`, no external modules.

## Design Decision: single table, not per-category headings

The spec's prose ("group skills by category") could be read as one markdown table per
category with its own heading. That would break round-tripping: `parseCatalogEntries`
scans line-by-line for the *first* header+separator match and returns as soon as it
successfully parses that one table — it does not continue looking for a second table
further down the file. Generating multiple tables would silently drop every category
after the first from `sth list`.

Decision: `sth catalog init` emits **one single table**, with entries sorted by
`(folder, name)` — the same sort `parseCatalogEntries` already applies on read. This
satisfies FR-002 ("group skills by category") through sort order while guaranteeing
FR-005 (round-trip compatibility with `sth list`) by construction, and requires no
change to the existing parser.

## Technical Context

**Language/Version**: Go 1.23
**Primary Dependencies**: stdlib only — `os`, `path/filepath`, `strings`, `sort`,
`bufio` — plus existing internal packages `internal/scanner` (discovery + front
matter) and `internal/fs` (`CatalogEntry`, v2 state schema)
**Storage**: no new persisted files. `sth catalog init` writes `SKILLS.md` at the
repository root (plain file write, not managed state). Changelog display reads
`CHANGELOG.md` from an already-downloaded bundle; no new state fields.
**Testing**: `go test ./...`, standard `testing` package only
**Target Platform**: Windows, macOS, Linux — `CGO_ENABLED=0`, pure Go
**Project Type**: CLI tool
**Performance Goals**: `sth catalog init` processes 100 skills / 10 categories in
under 5 s (SC-005) — a single `filepath.WalkDir` plus one file read per skill, well
within budget
**Constraints**: Zero external dependencies; stdlib only; no CGO; no hardcoded paths;
`go vet ./...` clean
**Scale/Scope**: Single-binary CLI; skill repositories up to ~200 skills

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Zéro Dépendance Externe | ✅ PASS | No new modules; changelog parsing is a line-oriented scan over `## [version] - date` headings, same style as the existing release-notes skip logic in `internal/http/client.go` |
| II. Agnostique — Aucune Valeur Codée en Dur | ✅ PASS | `sth catalog init` and `sth new` operate on the current working directory only; no remote URL or repo identifier is assumed |
| III. Cross-Platform — CGO Désactivé | ✅ PASS | Pure Go; local tree walk uses `filepath.WalkDir` + `filepath.ToSlash` to build POSIX-style `scanner.TreeEntry.Path` values, matching what the scanners expect from remote trees |
| IV. Découpage Modulaire Strict | ✅ PASS | New commands in `cmd/`; local walker, table rendering, scaffold template, and changelog parsing all live in `internal/fs/`; no network calls in any new code path (`catalog init` and `new` are local-only; changelog parsing runs on already-downloaded content) |

**Post-Phase-1 re-check**: ✅ PASS — data-model.md and contracts/cli-commands.md
confirm all new code is local-only (no network calls) or additive to `update`'s
existing authenticated path; no principle is at risk.

## Project Structure

### Documentation (this feature)

```text
specs/008-ecosystem-tooling/
├── plan.md              # This file
├── data-model.md         # Phase 1 output
├── contracts/
│   └── cli-commands.md   # Phase 1 output
└── tasks.md              # Phase 2 output — 23 tasks across 3 independent user stories
```

### Source Code (repository root)

```text
cmd/
├── root.go                  # MODIFY: route "catalog" and "new" subcommands
├── catalog.go                # NEW: runCatalog() dispatch; runCatalogInit() — --force flag, confirmation prompt
├── catalog_test.go           # NEW
├── new.go                    # NEW: runNew(args) — arg validation, delegates to fs.ScaffoldSkill
├── new_test.go                # NEW
├── update.go                  # MODIFY: after a successful version change, look up CHANGELOG.md
│                              #          in the bundle, print "What's new in <name>:" section
├── update_test.go             # MODIFY
└── install.go                  # NO CHANGE (first install has no previous_version → no changelog, per FR-010)

internal/fs/
├── catalog_init.go            # NEW: WalkLocalSkillTree(root string) ([]scanner.TreeEntry, error)
│                              #      RenderSkillsMarkdown(entries []CatalogEntry) string
├── catalog_init_test.go        # NEW
├── skill_scaffold.go           # NEW: ValidateSkillRef(ref string) (category, name string, err error)
│                              #      ScaffoldSkill(root, category, name string) error
├── skill_scaffold_test.go      # NEW
├── changelog.go                # NEW: ParseChangelog(content []byte) ([]ChangelogEntry, error)
│                              #      EntriesSince(entries []ChangelogEntry, previousVersion string) []ChangelogEntry
│                              #      RenderChangelogSection(resourceName string, entries []ChangelogEntry) string
└── changelog_test.go           # NEW

internal/scanner/
└── (no change — FlatSkillScanner, MultiStackScanner, ExtractFrontmatter reused as-is)

internal/http/
└── (no change — parseCatalogEntries is the read-side contract catalog_init.go targets)
```

**Structure Decision**: Same layered `cmd/` + `internal/` architecture as specs
005/006. All three sub-features are additive — no existing command's behavior changes
except `update` gaining an extra display step gated on `previous_version` + `CHANGELOG.md`
presence, which is a strict no-op when either is absent (FR-010, FR-012).

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

No violations detected — complexity table not required.
