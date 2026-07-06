# CLI Command Contracts: Ecosystem Tooling (spec 008)

Formal schema for every new or modified CLI surface introduced by spec 008.

---

## New Command: `sth catalog init`

### Synopsis

```
sth catalog init [--force]
```

`sth catalog` follows the same dispatch pattern as `sth pack` (`cmd/pack.go`): a
thin router with one subcommand today, room for `catalog validate` later without
touching the router.

### Flags

| Flag | Type | Default | Description |
|------|------|---------|--------------|
| `--force` | bool | false | Skip the overwrite confirmation when `SKILLS.md` already exists (FR-003) |

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | `SKILLS.md` written successfully |
| 1 | No conforming skill directories found (FR-004); or user declined the overwrite prompt |

### Behavior

1. Walk the current working directory (`WalkLocalSkillTree`).
2. Run `scanner.FlatSkillScanner` (and `scanner.MultiStackScanner` when the flat
   scanner finds nothing — same precedence `sth list` already uses for local scans)
   against the resulting tree.
3. For each `scanner.Artifact`, read the local `SKILL.md` and extract its
   description via `scanner.ExtractFrontmatter`.
4. If zero artifacts were found: print
   `"No skills found matching the expected structure"` to stderr, exit 1, write
   nothing.
5. If `SKILLS.md` already exists and `--force` is not set: prompt
   `"SKILLS.md already exists. Overwrite? [y/N]: "` on stdin/stdout; abort (exit 1,
   no write) on anything other than `y`/`Y`.
6. Sort entries by `(FolderName, Name)`, render via `RenderSkillsMarkdown`, write
   `SKILLS.md` at the repository root.
7. Print `"SKILLS.md generated with N skill(s) across M categorie(s)."` on success.

### Generated File Format

Single markdown table — the exact shape `internal/http.parseCatalogEntries` already
parses. No per-category headings (see plan.md's Design Decision).

```markdown
# SKILLS.md

| Type | Name | Folder | Source | Description |
| ---- | ---- | ------ | ------ | ----------- |
| skill | code-review | prompts | prompts/code-review/SKILL.md | Review a diff for bugs and style |
| skill | deep-research | prompts | prompts/deep-research/SKILL.md | Multi-source research with citations |
| skill | go-style | conventions | conventions/go-style/SKILL.md | |
```

Empty `Description` cells are left blank (FR — see spec.md Edge Cases: "empty or no
front matter → listed with empty description; no error").

### Routing in `cmd/root.go`

```go
case "catalog":
    return a.runCatalog(args)
```

`commandCatalog` entry:

```go
{
    name:    "catalog",
    usage:   "sth catalog init [--force]",
    summary: "Génère SKILLS.md à partir de la structure category/skill/SKILL.md du dépôt courant",
    examples: []string{
        "sth catalog init",
        "sth catalog init --force",
    },
    flags: []string{"--force"},
},
```

---

## New Command: `sth new <category>/<name>`

### Synopsis

```
sth new <category>/<name>
```

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Skill scaffolded successfully |
| 1 | Invalid `<category>/<name>` argument, or target directory already exists |

### Behavior

1. Require exactly one positional argument.
2. `fs.ValidateSkillRef(arg)` — on error, print the returned message (includes the
   single-segment case `"Invalid format: use <category>/<name>"` per spec.md Edge
   Cases) and exit 1.
3. `fs.ScaffoldSkill(cwd, category, name)` — on `os.IsExist`-equivalent failure,
   print `"Skill already exists at <category>/<name>"` and exit 1 without touching
   the existing directory.
4. On success, print `"Created <category>/<name>/SKILL.md"`.

### Scaffolded `SKILL.md` Template

```markdown
---
name: <name>
description: TODO — describe what this skill does and when to use it
---

## Usage

TODO — describe how to invoke this skill.
```

### Routing in `cmd/root.go`

```go
case "new":
    return a.runNew(args)
```

```go
{
    name:    "new",
    usage:   "sth new <category>/<name>",
    summary: "Scaffold un nouveau skill conforme dans <category>/<name>/SKILL.md",
    examples: []string{
        "sth new prompts/code-review",
    },
},
```

---

## Modified Command: `sth update` — changelog display

### Synopsis (unchanged)

```
sth update [self] [--dry-run] [--fail-on-changes] [--json]
```

No new flags. Behavior is additive and strictly gated: it only activates when both
a `CHANGELOG.md` is present in the downloaded bundle **and** the resource's state
record has a non-empty `previous_version` (FR-010).

### Text Output Addition

Printed immediately after a resource's version-change line, before moving to the
next resource:

```
myorg/react-forms  outdated → up-to-date  (abc1234 → def5678)

What's new in react-forms:

## [1.3.0] - 2026-07-01
### Added
- Support for nested field arrays

## [1.2.1] - 2026-06-20
### Fixed
- Validation message not clearing on reset
```

Truncation note when the source `CHANGELOG.md` exceeds 50 KB:

```
What's new in react-forms:
(changelog truncated)

## [1.3.0] - 2026-07-01
...
```

Parse failure (non-Keep-a-Changelog format) — warning only, update proceeds:

```
Warning: Could not parse changelog for react-forms
```

### JSON Output (FR-014) — deferred

Implementation finding: `sth update` (non-dry-run) emits no per-resource JSON
objects today — the `--json` flag is parsed but only affects the `--dry-run` path.
There is no existing object to attach a `changelog` field to; fulfilling FR-014
would mean designing a whole JSON output mode for `update`, which is its own
contract decision.

FR-014 is therefore **deferred** until `sth update --json` gains real per-resource
output. When it does, the field contract is: a `changelog` string field, omitted
entirely (not `null`, not `""`) when no `CHANGELOG.md` is present, when
`previous_version` was empty, or when no version change occurred.

### No Changes To

- `sth install` — first install never has a `previous_version` baseline, so no
  changelog is ever shown (FR-010's "since" requirement cannot be satisfied); this
  is a no-op path, not a new branch to test beyond confirming it stays silent.
- `--fail-on-changes` and `--dry-run` semantics — changelog display only runs after
  a real version change is committed to state, so it never interacts with either
  gate.
