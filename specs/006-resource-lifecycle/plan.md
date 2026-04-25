# Implementation Plan: Resource Lifecycle

**Branch**: `006-cli-product-roadmap` | **Date**: 2026-04-23 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `specs/006-resource-lifecycle/spec.md`

## Summary

Build the full resource lifecycle on top of the v2 state schema delivered by spec 005.
Five capabilities are added: (1) `sth status` inspects every installed resource and
reports `up-to-date`, `outdated`, `modified-locally`, or `unknown-baseline`; (2)
`sth remove <name>` cleanly uninstalls a resource with SHA-based modification
detection and confirmation; (3) `--dry-run` on `install`/`update` previews all file
changes without touching disk or state; (4) version pinning skips pinned resources
during `update` and reports a notice when a newer version is available; (5)
`.sthignore` excludes unwanted bundle files using gitignore-style patterns.
A file ownership conflict guard is added to `install` to prevent silent overwrites
across resources.

All logic is pure Go stdlib, CGO_ENABLED=0, no external modules.

## Technical Context

**Language/Version**: Go 1.22  
**Primary Dependencies**: stdlib only — `os`, `path/filepath`, `crypto/sha256`,
`encoding/json`, `bufio`, `strings`, `net/http` (already in `internal/http/`)  
**Storage**: `.sth/sth-state-v2.json` (v2 state, spec 005) +
`.sth/project.json` (v2 config with `pinned_version` already in schema)  
**Testing**: `go test ./...` using standard `testing` package only (no testify or
external assertion libs per constitution)  
**Target Platform**: Windows, macOS, Linux — `CGO_ENABLED=0`, pure Go  
**Project Type**: CLI tool  
**Performance Goals**: `sth status` < 3 s for 20 resources across 2 providers
(one HTTP round-trip per provider, not per resource)  
**Constraints**: Zero external dependencies; stdlib only; no CGO; no hardcoded paths
or URLs; `go vet ./...` must pass clean  
**Scale/Scope**: Single-binary CLI; projects with up to ~50 installed resources

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Zéro Dépendance Externe | ✅ PASS | No new modules; gitignore matching via custom stdlib implementation using `path.Match` + `**` segment splitting |
| II. Agnostique — Aucune Valeur Codée en Dur | ✅ PASS | All paths and provider refs read from `project.json` / `sth-state-v2.json` at runtime |
| III. Cross-Platform — CGO Désactivé | ✅ PASS | Pure Go; `filepath.ToSlash` normalises separators for ignore-pattern matching on Windows |
| IV. Découpage Modulaire Strict | ✅ PASS | New commands in `cmd/`; status computation, ignore parsing, and ownership index in `internal/fs/`; HTTP version-check method in `internal/http/`; no layer inversion |

**Post-Phase-1 re-check**: ✅ PASS — design confirms all four principles upheld.

## Project Structure

### Documentation (this feature)

```text
specs/006-resource-lifecycle/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   └── cli-commands.md  # Phase 1 output
└── tasks.md             # Phase 2 output (/speckit.tasks — not created by /speckit.plan)
```

### Source Code (repository root)

```text
cmd/
├── root.go              # MODIFY: add status/remove routing; --dry-run/--yes/--force parsing
├── status.go            # NEW: runStatus(); --json flag
├── status_test.go       # NEW
├── remove.go            # NEW: runRemove(); --yes flag
├── remove_test.go       # NEW
├── install.go           # MODIFY: --dry-run flag; --force flag; ownership conflict check
├── install_test.go      # MODIFY: dry-run tests; conflict tests
├── update.go            # MODIFY: --dry-run flag; pinning skip logic; commit-SHA version store
└── update_test.go       # MODIFY: pinning tests; dry-run tests

internal/fs/
├── schema.go            # NO CHANGE (pinned_version already present in ResourceEntry)
├── fs.go                # MODIFY: add ComputeResourceStatus(), BuildOwnerIndex(),
│                        #         InstallCatalogEntryV2 dry-run variant
├── ignore.go            # NEW: ParseIgnoreFile(), MatchIgnore()
├── ignore_test.go       # NEW
├── status.go            # NEW: StatusResult type, ComputeAllStatuses()
└── status_test.go       # NEW

internal/http/
└── client.go            # MODIFY: add GetLatestCommitSHA(ctx, profile, providerID) string
```

**Structure Decision**: Single-project layout following the existing `cmd/` + `internal/`
layered architecture. Each new command is its own file in `cmd/`. Pure logic (ignore
parsing, status computation, ownership indexing) lives in `internal/fs/` so it can be
unit-tested without any CLI or network wiring.

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

No violations detected — complexity table not required.
