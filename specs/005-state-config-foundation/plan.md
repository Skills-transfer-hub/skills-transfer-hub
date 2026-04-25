# Implementation Plan: State & Config Foundation

**Branch**: `007-ci-auth-observability` | **Date**: 2026-04-25 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `specs/005-state-config-foundation/spec.md`

## Summary

Extend `project.json` and `sth-state.json` to support multi-provider configuration and
per-resource file inventory with SHA256 checksums. The v1 → v2 migration is automatic on
first command run, atomic (write-then-rename), concurrency-safe (advisory lock), and
observable (stderr progress lines). All logic lives in the existing `internal/fs/` layer
using Go stdlib only; no new external dependencies.

## Technical Context

**Language/Version**: Go 1.22 (go.mod)  
**Primary Dependencies**: stdlib only — `os`, `encoding/json`, `crypto/sha256`,
`path/filepath`, `io`, `time`, `regexp`; no third-party modules  
**Storage**: JSON files — `.sth/project.json` (project config), `sth-state.json`
(installed resource state), `.sth-state.lock` (advisory lock), `*.bak` (migration backups)  
**Testing**: `go test ./...` with standard `testing` package; no external assertion libs  
**Target Platform**: Cross-platform — Windows, macOS, Linux; `CGO_ENABLED=0`  
**Project Type**: CLI tool  
**Performance Goals**: Migration ≤ 2 seconds for 50 resources (SC-002)  
**Constraints**: stdlib only; no CGO; no third-party modules; no hardcoded paths/URLs  
**Scale/Scope**: Local project-level tool; state files hold O(10–100) resource records

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Evidence |
|-----------|--------|---------|
| I. Zero External Dependency | ✅ PASS | Only stdlib: `os`, `encoding/json`, `crypto/sha256`, `regexp`, `path/filepath`, `io`, `time` |
| II. No Hardcoded Values | ✅ PASS | All file paths derived from `os.Getwd()`; no repo URLs, resource IDs, or dir names hardcoded |
| III. Cross-Platform / CGO Disabled | ✅ PASS | `os.Rename()` used for atomic writes (cross-platform); `O_CREATE\|O_EXCL` for lock (POSIX + Windows); no platform APIs without build tags |
| IV. Strict Modular Separation | ✅ PASS | All schema, migration, atomic-write, and lock logic stays in `internal/fs/`; `cmd/` only threads `stderr` writer; no downward calls from `fs/` to `cmd/` |

**Post-design re-check**: ✅ PASS — design introduces no new violations; `io.Writer` for
migration progress is injected from `cmd/` downward into `internal/fs/`, preserving layer
boundaries.

## Project Structure

### Documentation (this feature)

```text
specs/005-state-config-foundation/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   ├── project-json-v2.md        # project.json v2 schema contract
│   └── sth-state-v2.md     # sth-state.json v2 schema contract
└── tasks.md             # Phase 2 output (/speckit.tasks - NOT created here)
```

### Source Code (repository root)

```text
cmd/
├── root.go              # No change — routing
├── init.go              # Already supports multi-provider (no change needed)
├── install.go           # Thread stderr writer to migration; acquire/release lock
├── update.go            # Thread stderr writer to migration; acquire/release lock
├── list.go              # Thread stderr writer to migration; acquire/release lock
├── status.go            # Thread stderr writer to migration; acquire/release lock
└── remove.go            # Thread stderr writer to migration; acquire/release lock

internal/fs/
├── schema.go            # Existing — ProjectConfigV2, InstalledResourceStateV2, etc.
├── migrate.go           # Add io.Writer param; add stderr progress; use atomicWriteFile
├── atomic.go            # NEW — atomicWriteFile() helper (write-to-tmp then os.Rename)
├── lock.go              # NEW — AcquireStateLock(), ReleaseStateLock()
├── fs.go                # Update WriteInstalledStateV2 to use atomicWriteFile + lock
├── pin.go               # Existing — no change
├── ownership.go         # Existing — no change
├── status.go            # Existing — no change
├── dryrun.go            # Existing — no change
├── ignore.go            # Existing — no change
└── remove.go            # Existing — no change

internal/fs/
├── atomic_test.go       # NEW — tests for atomicWriteFile
└── lock_test.go         # NEW — tests for AcquireStateLock / ReleaseStateLock
```

**Structure Decision**: Single Go module with `cmd/` + `internal/fs/` layers as mandated
by the constitution. All new code targets `internal/fs/` (atomic writes, lock) and thin
wiring in `cmd/` (pass stderr writer, call lock/unlock around state operations).

## Complexity Tracking

> No constitution violations detected — table omitted.
