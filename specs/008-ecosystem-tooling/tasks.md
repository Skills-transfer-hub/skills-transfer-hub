# Tasks: Ecosystem Tooling

**Input**: Design documents from `/specs/008-ecosystem-tooling/`
**Prerequisites**: spec.md (user stories, FRs, acceptance scenarios), plan.md,
data-model.md, contracts/cli-commands.md

**Context**: Go 1.23, stdlib only. Depends on spec 005 (v2 state schema,
`previous_version` field — implemented) and spec 006 (managed file inventory —
implemented). Reuses `internal/scanner` (`FlatSkillScanner`, `MultiStackScanner`,
`ExtractFrontmatter`) and `internal/fs.CatalogEntry` unchanged.

**Tests**: Requested — each new file gets a companion `_test.go` per this repo's
convention (every existing `internal/fs` and `cmd` file has one).

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no unresolved dependencies)
- **[Story]**: Which user story this task belongs to
- No [Story] label in Setup or Foundational phases

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Register the two new commands in the router so they're reachable
before any story logic exists.

- [ ] T001 Register `catalog` and `new` entries in `commandCatalog` (`cmd/root.go`)
  with usage/summary/examples per `contracts/cli-commands.md`; add `case "catalog"`
  and `case "new"` arms in the `run` switch, each with an `isHelpFlag` short-circuit
  matching the existing `pack`/`remove` pattern

**Checkpoint**: `sth catalog init` and `sth new` are reachable (will error with
"not implemented" until their phases land); `sth help` lists both

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: None of the three user stories share a common blocking type — they
touch disjoint files (`catalog_init.go`, `skill_scaffold.go`, `changelog.go`) and
have no cross-dependencies. This phase is empty; all three stories can start
immediately after Phase 1.

**Checkpoint**: Proceed directly to Phase 3 — no foundational work required

---

## Phase 3: User Story 1 — `sth catalog init` (Priority: P1) 🎯 MVP

**Goal**: Generate a `SKILLS.md` from a local `category/skill/SKILL.md` tree that
`sth list` can parse without edits.

**Independent Test**: Create 3 categories × 2 skills locally, run
`sth catalog init`, verify `SKILLS.md` lists all 6 grouped/sorted by category and
that `sth list` parses it end-to-end.

- [ ] T002 [P] [US1] Implement `WalkLocalSkillTree(root string) ([]scanner.TreeEntry, error)`
  in new file `internal/fs/catalog_init.go` — `filepath.WalkDir` over `root`,
  emitting one `scanner.TreeEntry` per file/dir with `Path` relative to `root` and
  `filepath.ToSlash`-normalized, `Type` set to `"tree"` for directories and `"blob"`
  for files
- [ ] T003 [P] [US1] Implement `ReadSkillDescription(skillMDPath string) string` in
  `internal/fs/catalog_init.go` — reads the file, calls
  `scanner.ExtractFrontmatter`, returns `Description` or `""` on any read/parse
  failure (never errors — per Edge Cases, empty description is valid)
- [ ] T004 [US1] Implement `RenderSkillsMarkdown(entries []CatalogEntry) string` in
  `internal/fs/catalog_init.go` — emits the single-table format from
  `contracts/cli-commands.md` (`# SKILLS.md` heading, `| Type | Name | Folder |
  Source | Description |` table, one row per entry, in the order given — caller is
  responsible for sorting by `(FolderName, Name)`)
- [ ] T005 [P] [US1] Unit tests in `internal/fs/catalog_init_test.go`: WalkLocalSkillTree
  against a temp dir fixture (3 categories × 2 skills + noise files); ReadSkillDescription
  with/without front matter and with a missing file; RenderSkillsMarkdown output
  round-trips through `internal/http`'s `parseCatalogEntries` (exported test seam or
  a local re-parse using the same table-scan logic — confirm via `sth list`-equivalent
  assertion, not just string comparison, so a future format drift in the parser is
  caught here too)
- [ ] T006 [US1] Implement `runCatalog(args []string) error` (dispatch) and
  `runCatalogInit(args []string) error` in new file `cmd/catalog.go`:
  - `runCatalog`: require `args[0] == "init"` or print usage error; forward help flag
  - `runCatalogInit`: parse `--force` via `parseFlags`; call `fs.WalkLocalSkillTree(".")`;
    run `scanner.FlatSkillScanner{}.Detect`/`.Scan`, falling back to
    `scanner.MultiStackScanner{}` when the flat scanner detects nothing (same
    precedence as local scanning in `cmd/list.go`); for each `scanner.Artifact` build a
    `fs.CatalogEntry` (`ResourceType` from `Artifact.Kind`, `FolderName` from
    `Artifact.Category`, `Name`, `SourcePath` from `Artifact.Entrypoint`, `Description`
    via `fs.ReadSkillDescription`); exit 1 with `"No skills found matching the expected
    structure"` when zero artifacts; if `SKILLS.md` exists and `!force`, prompt
    `"SKILLS.md already exists. Overwrite? [y/N]: "` on `a.stdin`/`a.stdout`, abort on
    non-`y`; sort entries by `(FolderName, Name)`; write via `fs.RenderSkillsMarkdown`;
    print the success summary line
- [ ] T007 [US1] Tests in `cmd/catalog_test.go`: happy path (writes file, correct
  content), existing `SKILLS.md` without `--force` (prompts, declines → no write),
  existing `SKILLS.md` with `--force` (overwrites without prompting), zero-skills
  directory (exit 1, no file created), mixed non-skill files ignored

**Checkpoint**: `sth catalog init` is fully functional and independently testable

---

## Phase 4: User Story 2 — `sth new <category>/<name>` (Priority: P1)

**Goal**: Scaffold a single conformant skill directory with a templated `SKILL.md`.

**Independent Test**: Run `sth new prompts/code-review`, verify
`prompts/code-review/SKILL.md` exists with `name:`, `description:`, and `## Usage`.

- [ ] T008 [P] [US2] Implement `ValidateSkillRef(ref string) (category, name string, err error)`
  in new file `internal/fs/skill_scaffold.go` — split on exactly one `/`; each
  segment must be non-empty and match `^[A-Za-z0-9_-]+$`; single-segment input
  (`no "/"`) returns the specific error `"Invalid format: use <category>/<name>"`;
  any other violation returns a message listing the valid character set (FR-009)
- [ ] T009 [P] [US2] Implement `ScaffoldSkill(root, category, name string) error` in
  `internal/fs/skill_scaffold.go` — computes `filepath.Join(root, category, name)`;
  returns a descriptive "already exists" error without writing anything if that
  directory exists; otherwise `os.MkdirAll`s it and writes `SKILL.md` from the
  template in `contracts/cli-commands.md` with `name` substituted
- [ ] T010 [P] [US2] Unit tests in `internal/fs/skill_scaffold_test.go`:
  ValidateSkillRef valid/invalid cases (spaces, extra `/`, no `/`, empty segment);
  ScaffoldSkill happy path, pre-existing directory (no overwrite, original content
  untouched), nested category creation, works in an empty directory (no `SKILLS.md`
  present — FR/Acceptance Scenario 5)
- [ ] T011 [US2] Implement `runNew(args []string) error` in new file `cmd/new.go` —
  require exactly one positional arg; call `fs.ValidateSkillRef`, print its error and
  return non-zero on failure; call `fs.ScaffoldSkill(".", category, name)`, print
  `"Skill already exists at <category>/<name>"` and return non-zero on the
  already-exists error; print `"Created <category>/<name>/SKILL.md"` on success
- [ ] T012 [US2] Tests in `cmd/new_test.go`: happy path, already-exists path, invalid
  ref (both "no slash" and "bad characters" messages), zero/multiple positional args

**Checkpoint**: `sth new` is fully functional and independently testable
(independent of `sth catalog init` — no shared code beyond the router registration
from Phase 1)

---

## Phase 5: User Story 3 — Changelog Display on Update (Priority: P2)

**Goal**: Surface `CHANGELOG.md` entries introduced since `previous_version` during
`sth update`, without ever blocking or failing the update.

**Independent Test**: Install a resource at v1 (bundle has `CHANGELOG.md`), update
to v2 (adds entries), verify the new entries print during `sth update`.

- [ ] T013 [P] [US3] Define `ChangelogEntry` struct and `ErrUnrecognizedFormat` in
  new file `internal/fs/changelog.go`
- [ ] T014 [US3] Implement `ParseChangelog(content []byte) ([]ChangelogEntry, error)`
  in `internal/fs/changelog.go` — line-scan for `## [<version>] - <date>` or
  `## [Unreleased]` headings; accumulate everything up to the next matching heading
  as `Body`; return `(nil, ErrUnrecognizedFormat)` when no heading matches anywhere
- [ ] T015 [P] [US3] Implement `EntriesSince(entries []ChangelogEntry, previousVersion string) []ChangelogEntry`
  in `internal/fs/changelog.go` — returns entries preceding the one whose `Version`
  matches `previousVersion`; returns all entries when no match is found (per spec.md
  Assumptions — non-comparable/non-semver tags)
- [ ] T016 [P] [US3] Implement `TruncateChangelog(content []byte) ([]byte, bool)` in
  `internal/fs/changelog.go` — caps at 50 KB (51200 bytes), returns
  `(truncated, true)` when the cap was applied
- [ ] T017 [P] [US3] Implement `RenderChangelogSection(resourceName string, entries []ChangelogEntry) string`
  in `internal/fs/changelog.go` — `"What's new in <resourceName>:\n\n"` followed by
  each entry's heading + body, in the order given
- [ ] T018 [P] [US3] Unit tests in `internal/fs/changelog_test.go`: ParseChangelog
  against a well-formed Keep a Changelog fixture, an unrecognized-format fixture
  (returns `ErrUnrecognizedFormat`), and an `[Unreleased]`-only fixture;
  EntriesSince with a matching/non-matching `previousVersion`; TruncateChangelog at
  exactly/over/under the 50 KB boundary; RenderChangelogSection output shape
- [ ] T019 [US3] Wire changelog display into `cmd/update.go`: after a resource's
  version actually changes (existing update loop, non-dry-run path) and only when
  the pre-update state record's `PreviousVersion != ""`, look for `CHANGELOG.md` in
  the downloaded bundle content; on found, call `fs.TruncateChangelog` then
  `fs.ParseChangelog` — on `ErrUnrecognizedFormat`, print
  `"Warning: Could not parse changelog for <resource-name>"` and continue (raw
  content is not printed in text mode per FR-011's parse-failure branch feeding
  straight into the warning, not a raw dump, to keep terminal output bounded); on
  success, call `fs.EntriesSince` then `fs.RenderChangelogSection` and print the
  result; in `--json` mode set a `changelog` field (joined entry text) on the
  resource's result object instead of printing, omitting the field entirely when no
  `CHANGELOG.md` was found or `PreviousVersion` was empty
- [ ] T020 [US3] Tests in `cmd/update_test.go`: changelog shown after a real version
  bump with a conforming `CHANGELOG.md`; nothing shown on first install (no
  `previous_version`); nothing shown when the bundle has no `CHANGELOG.md`; warning
  (not a failure) shown for a non-conforming `CHANGELOG.md`, update still succeeds;
  `--json` output includes/omits the `changelog` field per the above matrix;
  `--dry-run` never triggers changelog display (no version change is committed)

**Checkpoint**: Changelog display works end-to-end and is fully independent of US1/US2

---

## Phase 6: Polish

**Purpose**: Cross-cutting documentation and final validation once all three
stories are independently green.

- [ ] T021 [P] Update `README.md` — add `sth catalog init` and `sth new` to the
  "Commandes visibles" list; document the changelog-on-update behavior under
  `sth update`
- [ ] T022 [P] Update `cmd/completion.go` fixtures/tests if shell completion enumerates
  `commandCatalog` entries statically (confirm whether this is generated or hand-listed
  before assuming a change is needed)
- [ ] T023 Run `go build ./...`, `go vet ./...`, and the full `go test ./...` suite;
  fix any cross-package regressions surfaced by the new `catalog`/`new` router entries

---

## Implementation Strategy

### MVP First

1. Complete Phase 1: Setup
2. Phase 2 is empty — proceed directly
3. Complete Phase 3: US1 `sth catalog init`
4. **STOP and VALIDATE**: `sth catalog init` independently (3×2 fixture → `sth list`)
5. Complete Phase 4: US2 `sth new`
6. **STOP and VALIDATE**: `sth new` independently
7. Complete Phase 5: US3 changelog display
8. **STOP and VALIDATE**: full update flow with a real `CHANGELOG.md` fixture
9. Phase 6: Polish

### Parallel Team Strategy

All three user stories are mutually independent after Phase 1 — with three
developers, US1/US2/US3 can proceed fully in parallel with zero merge conflicts
(disjoint new files: `catalog_init.go`+`catalog.go`, `skill_scaffold.go`+`new.go`,
`changelog.go`+`update.go` changes).

---

## Notes

- [P] tasks operate on different files — safe to execute simultaneously
- [Story] label maps each task to its user story for traceability
- T004's sort-then-render split matters: `RenderSkillsMarkdown` must stay a pure
  formatter so T005's round-trip test can validate it independently of discovery
- T019 is the only task touching an existing hot path (`cmd/update.go`) — keep the
  diff additive (new branch, gated by two existing conditions) to avoid regressing
  the well-covered existing update tests
- Spec assumption carried into T015: comparing `previousVersion` against
  `ChangelogEntry.Version` is a string/document-order match, not semver-aware
  comparison — matches spec.md's explicit fallback-to-all-entries rule
