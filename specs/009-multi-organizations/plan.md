# Implementation Plan: Multiple Organizations — CLI Org Context

**Branch**: `009-multi-organizations` | **Date**: 2026-06-12 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `specs/009-multi-organizations/spec.md`

## Summary

Give the CLI a persistent, switchable **active organization** on top of the already-existing
many-to-many membership model. Three new subcommands (`sth org current|list|use`) let a user
inspect and change which org their CLI acts as. The selection is persisted in user settings and
sent to the backend as an active-org selector that `resolveAuth` honors only against the key
identity's memberships, falling back to the license home org when absent (zero regression).
Listing and switching are gated by a new paid capability `multi_org` (Pro/Team), added to the
capability map in both the backend and its dashboard mirror. No database migration is required.

All CLI logic is pure Go stdlib (`CGO_ENABLED=0`); the backend change is a small extension to
`resolveAuth` plus a read-only memberships lookup.

## Technical Context

**Language/Version**: Go 1.22 (CLI), Deno/TypeScript (sth-back), Next.js/TS (dashboard mirror only)
**Primary Dependencies**: stdlib only on the CLI — `os`, `encoding/json`, `net/http` (existing `internal/http/`), keychain via the existing `internal/auth/` wrapper
**Storage**: `~/.config/sth/settings.json` (`UserSettings`, extended with an `active_org` block); OS keychain for the API key (unchanged). No new files, no DB migration.
**Testing**: `go test ./internal/fs/... ./cmd/...` (stdlib `testing` only; `cmd` tests are sequential via `STH_USER_CONFIG_DIR` sandbox — no `t.Parallel()`); Deno tests for the `resolveAuth` selector path in sth-back.
**Target Platform**: Windows, macOS, Linux — `CGO_ENABLED=0`, pure Go.
**Project Type**: CLI tool + Supabase Edge backend (+ a one-line dashboard capability mirror).
**Performance Goals**: `sth org current` < 1s online, instant from cache offline; `sth org list` one HTTP round-trip.
**Constraints**: Zero external Go dependencies; no hardcoded URLs/paths; `go vet ./...` clean; strict repo Go style (no single-letter vars, no comments).
**Scale/Scope**: A user is typically a member of a handful of orgs.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Zéro Dépendance Externe | ✅ PASS | No new modules; org selection uses the existing `internal/http` client and stdlib JSON. |
| II. Agnostique — Aucune Valeur Codée en Dur | ✅ PASS | Org ids/names and the cloud base URL are read at runtime; nothing org-specific is compiled in. |
| III. Cross-Platform — CGO Désactivé | ✅ PASS | Pure Go; keychain access already abstracted in `internal/auth/keyring.go` with per-OS backends. |
| IV. Découpage Modulaire Strict | ✅ PASS | Command routing in `cmd/`; persistence in `internal/fs/`; the selector header in `internal/http/`; no layer inversion. |
| Tests (TDD strict) | ✅ PASS | Each new command + the settings extension ships `*_test.go` using stdlib `testing`. |

**Post-Phase-1 re-check**: ✅ PASS — design adds no dependency and keeps the three-layer split.

## Project Structure

### Documentation (this feature)

```text
specs/009-multi-organizations/
├── plan.md                          # This file
├── spec.md                          # Feature spec
├── data-model.md                    # Types & persistence shape (no migration)
└── contracts/
    └── cli-org-and-selection.md     # CLI command contracts + wire selector + resolveAuth change
```

### Source Code (per repo)

```text
# skills-transfer-hub (CLI)
cmd/
├── root.go                  # MODIFY: register "org" in commandCatalog + run() switch
├── org.go                   # NEW: runOrg() dispatch (current|list|use); --json
└── org_test.go              # NEW

internal/fs/
├── user_settings.go         # MODIFY: add ActiveOrg block + Read/Write helpers (settingsWriteMu-guarded)
└── user_settings_test.go    # MODIFY: round-trip + absence-means-home-org

internal/http/
├── cloud.go                 # MODIFY: GET /v1/orgs (list) + send active-org selector on org-scoped calls
└── auth.go                  # MODIFY: attach the active-org selector header from settings

internal/auth/
└── keyring.go               # NO CHANGE for the recommended single-key path (see contracts alt.)

# sth-back (backend)
shared/
├── auth.ts                  # MODIFY: honor active-org selector via memberships(assignee_id); fallback home org
└── capabilities.ts          # MODIFY: add `multi_org` to pro/team/alpha
functions/cli/
├── index.ts                 # MODIFY: add GET /v1/orgs route
└── routes/orgs_list.ts      # NEW: list targetable orgs for the caller identity

# sth-dashboard (capability mirror only; UI lives in specs/003)
src/lib/billing/
└── plans.ts                 # MODIFY: mirror `multi_org` into PLAN_CAPABILITIES (KEEP IN SYNC)
```

**Structure Decision**: CLI-primary. The hard, novel work (persistence, selector, gating) is CLI +
a thin backend extension. The dashboard touch here is a single capability-map mirror; the actual
org-switcher UI is specified separately in `sth-dashboard/specs/003-org-switcher-ui` to respect
that repo's spec convention.

### Per-repo breakdown

- **skills-transfer-hub**: `sth org` command group; `UserSettings.active_org`; selector header on org-scoped requests; multi-org hint in `login`.
- **sth-back**: `resolveAuth` honors the selector against `memberships(assignee_id)`; new `GET /v1/orgs`; `multi_org` capability.
- **sth-dashboard**: mirror `multi_org` in `plans.ts` (this spec); switcher UI + "create org" gating in spec 003.

## Complexity Tracking

> Fill ONLY if Constitution Check has violations that must be justified.

No violations detected — complexity table not required.
