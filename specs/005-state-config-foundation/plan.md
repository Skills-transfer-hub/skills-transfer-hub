# Implementation Plan: State & Config Foundation

**Branch**: `006-cli-product-roadmap` | **Date**: 2026-04-23 | **Spec**: [spec.md](spec.md)  
**Input**: Feature specification from `specs/005-state-config-foundation/spec.md`

## Summary

Extends the LoreSkills data model from single-provider flat-config (v1) to a multi-provider
array-based config (v2), adds schema versioning to both `project.json` and `lore-state.json`,
introduces automatic migration with `.bak` safety, and tracks per-resource managed-file
inventories with SHA256 checksums. This is the prerequisite for all lifecycle, CI, and
ecosystem features in the roadmap (specs 006-008).

**Approach**: Extend `internal/fs/` with new v2 types and a migration layer that detects
`schema_version` at read-time and rewrites v1 files to v2 transparently. Extend
`loreskills init` to support sequential multi-provider configuration. Update install/update
paths to write `managed_files` entries with SHA256 on every write.

## Technical Context

**Language/Version**: Go 1.22 (stdlib only)  
**Primary Dependencies**: stdlib only (`encoding/json`, `crypto/sha256`, `os`, `path/filepath`)  
**Storage**: JSON files — `.loreskills/project.json` (config), `<target_dir>/lore-state.json` (state)  
**Testing**: `go test ./...` with stdlib `testing` package only  
**Target Platform**: Windows, macOS, Linux — CGO_ENABLED=0, static binary  
**Project Type**: CLI tool (Go)  
**Performance Goals**: Migration under 2 seconds for up to 50 resources (SC-002)  
**Constraints**: Zero external dependencies; no hardcoded paths; strict layering  
**Scale/Scope**: Single project, up to 50 installed resources

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Zero external dependencies | PASS | `crypto/sha256` already imported; all changes stdlib-only |
| II. No hardcoded values | PASS | Provider id defaults to "default" only during migration; all config values come from user input |
| III. Cross-platform / CGO disabled | PASS | `filepath.Clean`, `filepath.Join` throughout; `sha256.Sum256` is pure Go |
| IV. Strict modular layering | PASS | All schema/migration logic stays in `internal/fs/`; cmd/ only calls fs functions |
| TDD Strict | REQUIRED | Every new function in `internal/fs/` needs `_test.go` coverage |

## Project Structure

### Documentation (this feature)

```text
specs/005-state-config-foundation/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   ├── project-config-v2.json   # JSON schema for project.json v2
│   └── lore-state-v2.json       # JSON schema for lore-state.json v2
└── tasks.md             # Phase 2 output (/speckit.tasks)
```

### Source Code (repository root)

```text
internal/
└── fs/
    ├── fs.go              # Extend: SHA256 helper, updated Read/Write to use v2 types,
    │                      #         migration trigger on read
    ├── schema.go          # New: v2 type definitions (ProjectConfigV2, ProviderEntry,
    │                      #      ResourceEntry, InstalledResourceRecordV2, ManagedFile,
    │                      #      InstalledResourceStateV2)
    ├── migrate.go         # New: MigrateProjectConfig(), MigrateInstalledState(),
    │                      #      backupFile(), validateProviderID()
    ├── fs_test.go         # Extend: migration trigger tests
    ├── schema_test.go     # New: v2 struct marshal/unmarshal round-trip tests
    └── migrate_test.go    # New: migration edge-case tests

cmd/
├── init.go               # Extend: multi-provider append flow (detect existing providers)
├── init_test.go          # Extend: multi-provider tests
├── install.go            # Extend: write managed_files + sha256 on install
├── install_test.go       # Extend: verify managed_files written
├── update.go             # Extend: update managed_files + previous_version on update
└── update_test.go        # Extend: verify previous_version populated
```

**Structure Decision**: Two new files in `internal/fs/` (`schema.go`, `migrate.go`) keep the
already-large `fs.go` readable. All cmd/ changes are additive — no existing function signatures
are removed, only new v2 variants added alongside them until callers are updated.

## Complexity Tracking

> No constitution violations to justify.
