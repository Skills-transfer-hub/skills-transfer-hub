# Data Model: Ecosystem Tooling (spec 008)

## Overview

Spec 008 introduces new value types for changelog parsing and reuses existing types
(`scanner.TreeEntry`, `scanner.Artifact`, `fs.CatalogEntry`) unchanged. No new persisted
storage files are introduced — `SKILLS.md` is a plain generated file (not tracked in
`sth-state-v2.json`), and changelog display reads content already downloaded for an
install/update, holding nothing new on disk.

---

## Reused Types (no change)

### scanner.TreeEntry

**File**: `internal/scanner/scanner.go` (existing)

`sth catalog init` builds `[]scanner.TreeEntry` from a local `filepath.WalkDir` instead
of a remote provider tree listing — same struct, different source:

```go
type TreeEntry struct {
    Path string // POSIX-style, relative to repo root (filepath.ToSlash'd)
    Type string // "blob" for files, "tree" for directories
}
```

### scanner.Artifact

**File**: `internal/scanner/scanner.go` (existing). Output of
`FlatSkillScanner.Scan()` / `MultiStackScanner.Scan()` run against the locally-built
tree. `Category` + `Name` map directly to `CatalogEntry.FolderName` + `CatalogEntry.Name`.

### fs.CatalogEntry

**File**: `internal/fs/fs.go` (existing, unchanged). This is the round-trip contract:
whatever `catalog_init.go` writes to `SKILLS.md` must produce exactly these fields when
re-read by `internal/http.parseCatalogEntries`.

```go
type CatalogEntry struct {
    CatalogID    string
    ResourceType string
    Name         string
    FolderName   string
    SourcePath   string
    Description  string
    // ArtifactKind, SourceKind, AppliesTo, Lang: left empty by catalog init —
    // these are PR3+ extensions for multi-target install, out of scope here.
}
```

---

## New Types

### ChangelogEntry

**File**: `internal/fs/changelog.go` (new)

A single versioned section parsed from a `CHANGELOG.md` in
[Keep a Changelog](https://keepachangelog.com/) format.

```go
// ChangelogEntry is one versioned section of a CHANGELOG.md file.
type ChangelogEntry struct {
    Version string // e.g. "1.2.0", or "Unreleased"
    Date    string // "YYYY-MM-DD", empty for Unreleased
    Body    string // raw markdown body of the section, excluding the heading line
}
```

**Parsing rule (FR-011)**: a heading line matching `## [<version>] - <date>` or
`## [Unreleased]` starts a new entry; everything until the next matching heading (or
EOF) is that entry's `Body`. Headings that don't match the pattern do not start a new
entry — they are folded into the current entry's `Body` (keeps sub-sections like
`### Added` intact).

**Non-conforming files**: when zero headings match the pattern anywhere in the file,
`ParseChangelog` returns `(nil, ErrUnrecognizedFormat)`. The caller falls back to
displaying the raw content (up to the 50 KB cap) per FR-011's raw-fallback clause,
and — per the Edge Cases table in spec.md — emits the warning
`"Could not parse changelog for <resource-name>"`.

### ChangelogDisplay (rendering only, no new stored type)

Not a struct — `RenderChangelogSection` takes `(resourceName string, entries
[]ChangelogEntry)` and returns the text block printed under
`"What's new in <resource-name>:"`. For `--json` mode, the same entries are joined into
a single string and set on the existing per-resource result object as `changelog`
(see contracts/cli-commands.md for the exact JSON field placement — this only adds a
field to update's existing JSON output, no new top-level type).

---

## New Functions

### internal/fs/catalog_init.go

```go
// WalkLocalSkillTree builds a []scanner.TreeEntry from the filesystem rooted at root,
// suitable as input to scanner.FlatSkillScanner / scanner.MultiStackScanner. Paths are
// relative to root and forward-slash normalized regardless of OS.
func WalkLocalSkillTree(root string) ([]scanner.TreeEntry, error)

// ReadSkillDescription reads the SKILL.md at the given local path and returns its
// front-matter description, or "" if absent or the file has no parseable front matter.
func ReadSkillDescription(skillMDPath string) string

// RenderSkillsMarkdown renders entries as the single-table SKILLS.md format consumed by
// internal/http.parseCatalogEntries. Entries must already be sorted by (FolderName, Name)
// — RenderSkillsMarkdown does not re-sort.
func RenderSkillsMarkdown(entries []CatalogEntry) string
```

### internal/fs/skill_scaffold.go

```go
// ValidateSkillRef validates a "<category>/<name>" argument for `sth new`.
// Accepts alphanumeric, hyphen, underscore per segment, exactly one "/" separator.
// Returns a descriptive error (including the single-segment "Invalid format: use
// <category>/<name>" case) when invalid.
func ValidateSkillRef(ref string) (category, name string, err error)

// ScaffoldSkill creates <root>/<category>/<name>/SKILL.md with the template front
// matter (name, description placeholder) and a "## Usage" section. Returns an error
// without writing anything if the target directory already exists.
func ScaffoldSkill(root, category, name string) error
```

### internal/fs/changelog.go

```go
// ErrUnrecognizedFormat is returned by ParseChangelog when no Keep a Changelog
// heading is found anywhere in content.
var ErrUnrecognizedFormat = errors.New("changelog format not recognized")

// ParseChangelog parses Keep a Changelog formatted content into entries, newest first
// (document order is preserved — Keep a Changelog lists newest first by convention).
func ParseChangelog(content []byte) ([]ChangelogEntry, error)

// EntriesSince returns the entries in entries that come before previousVersion in
// document order (i.e. newer than the installed version). When previousVersion is not
// found among entries (non-semver tag, or renamed), all entries are returned per the
// spec's Assumptions section.
func EntriesSince(entries []ChangelogEntry, previousVersion string) []ChangelogEntry

// RenderChangelogSection formats entries under a "What's new in <resourceName>:" header
// for text-mode display.
func RenderChangelogSection(resourceName string, entries []ChangelogEntry) string

// TruncateChangelog caps content at 50 KB, appending "(changelog truncated)" when it does.
func TruncateChangelog(content []byte) (truncated []byte, wasTruncated bool)
```

---

## Validation Rules Summary

| Rule | Enforced by | Spec ref |
|------|-------------|----------|
| `<category>/<name>` — exactly one `/`, alnum/`-`/`_` per segment | `ValidateSkillRef` | FR-009 |
| Target skill directory must not already exist | `ScaffoldSkill` | FR-008 |
| `SKILLS.md` overwrite requires confirmation unless `--force` | `cmd/catalog.go` | FR-003 |
| No skills found → exit 1, no file written | `cmd/catalog.go` (checks len(entries) before writing) | FR-004 |
| Changelog > 50 KB → truncate + note | `TruncateChangelog` | FR-013 |
| Changelog parse failure → warn, never block update | `cmd/update.go` (ignores `ParseChangelog` error after logging) | FR-012 |

---

## Non-Goals (explicitly out of scope, per spec.md Assumptions)

- OSC 8 clickable terminal links for `SKILLS.md` URLs.
- Full YAML front-matter parsing (reuses `scanner.ExtractFrontmatter`'s existing
  best-effort scalar/list subset — no new parser).
- Any new `sth-state-v2.json` fields — `previous_version` already exists (spec 005).
