# Implementation Plan: Shared Configuration — Team CLI Config

**Branch**: `010-shared-configuration` | **Date**: 2026-06-12 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `specs/010-shared-configuration/spec.md`

## Summary

Add an org-level **shared configuration**: a server-stored, org-scoped subset of `ProjectConfigV2`
(providers, category selection, pinned versions) that owners/admins publish and members apply. A new
table `org_shared_configs` (one row per org, migration 0030) backs two Edge routes — `GET /v1/org-config`
(member read) and `PUT /v1/org-config` (owner/admin write, gated by the existing `fleet_sync` Team
capability, optimistic-concurrency by version). A new CLI command group `sth config pull|push|show`
synchronises the local `.sth/project.json`: pull makes the org config authoritative for the managed
fields while preserving purely-local resources, with `--dry-run` preview. The org targeted is always
the active org from spec 009. Secrets are never stored or transmitted.

CLI logic is pure Go stdlib; the backend reuses `resolveAuth`/`getServiceClient`/`jsonOk` and follows
the `access_request.ts` precedent for an authenticated org-scoped write.

## Technical Context

**Language/Version**: Go 1.22 (CLI), Deno/TypeScript (sth-back), SQL migration (sth-dashboard)
**Primary Dependencies**: CLI stdlib only — reuses `internal/fs` (`ReadProjectConfigV2`/`WriteProjectConfigV2`) and `internal/http` cloud client; backend reuses `shared/auth.ts`, `shared/supabase.ts`, `shared/errors.ts`.
**Storage**: new `org_shared_configs` table (Postgres, `sth-dashboard/supabase/migrations/0030_*`); local `.sth/project.json` is the merge target (no new local file).
**Testing**: `go test ./internal/fs/... ./cmd/...` (stdlib `testing`; merge logic unit-tested in `internal/fs`); Deno tests for the two routes (auth, role, capability, version conflict).
**Target Platform**: Windows, macOS, Linux — `CGO_ENABLED=0`, pure Go.
**Project Type**: CLI tool + Supabase Edge backend + one DB migration (+ admin UI in dashboard spec 004).
**Performance Goals**: `pull`/`push` one HTTP round-trip each.
**Constraints**: Zero external Go deps; no hardcoded URLs/paths; strict Go style; secrets never serialised.
**Scale/Scope**: One config blob per org, small JSON.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Zéro Dépendance Externe | ✅ PASS | CLI uses existing `internal/fs`/`internal/http`; no new module. |
| II. Agnostique — Aucune Valeur Codée en Dur | ✅ PASS | Org id from the active-org selector (spec 009); cloud base URL from env/default; nothing org-specific compiled in. |
| III. Cross-Platform — CGO Désactivé | ✅ PASS | Pure Go; merge is in-memory JSON over the existing v2 schema. |
| IV. Découpage Modulaire Strict | ✅ PASS | `cmd/config.go` routes only; merge + provenance in `internal/fs/`; transport in `internal/http/`; no inversion. |
| Tests (TDD strict) | ✅ PASS | Merge precedence, dry-run, and degrade-when-empty covered by `*_test.go`; routes covered by Deno tests. |

**Post-Phase-1 re-check**: ✅ PASS — one table, two routes, one command group; no dependency added.

## Project Structure

### Documentation (this feature)

```text
specs/010-shared-configuration/
├── plan.md                          # This file
├── spec.md                          # Feature spec
├── data-model.md                    # org_shared_configs table + RLS + merge algorithm
└── contracts/
    └── org-config-endpoints.md      # GET/PUT /v1/org-config + sth config commands
```

### Source Code (per repo)

```text
# skills-transfer-hub (CLI)
cmd/
├── root.go                  # MODIFY: register "config" in commandCatalog + run() switch
├── config.go                # NEW: runConfig() dispatch (pull|push|show); --dry-run
└── config_test.go           # NEW
internal/fs/
├── schema.go                # NO CHANGE (reuse ProjectConfigV2)
├── shared_config.go         # NEW: ShareableSubset(), MergeOrgConfig(), provenance result
└── shared_config_test.go    # NEW
internal/http/
└── cloud.go                 # MODIFY: GetOrgConfig() / PutOrgConfig() against CloudBaseURL()

# sth-back (backend)
functions/cli/
├── index.ts                 # MODIFY: route GET/PUT /v1/org-config (after resolveAuth)
└── routes/org_config.ts     # NEW: handleOrgConfigGet / handleOrgConfigPut (role + fleet_sync + version)

# sth-dashboard (schema + admin UI in spec 004)
supabase/migrations/
└── 0030_org_shared_config.sql   # NEW: org_shared_configs table + member-read/admin-write RLS
```

**Structure Decision**: CLI-primary with the schema owned where all migrations live
(`sth-dashboard/supabase/migrations/`). The admin editor UI is specified separately in
`sth-dashboard/specs/004-shared-config-admin-ui`; this spec owns the table, the wire contract, and the
CLI behavior.

### Per-repo breakdown

- **skills-transfer-hub**: `sth config pull|push|show`; shareable-subset extraction + org-priority merge with provenance; cloud client methods.
- **sth-back**: `GET /v1/org-config` (member read) and `PUT /v1/org-config` (owner/admin + `fleet_sync` + optimistic version) in `routes/org_config.ts`, registered in `index.ts`.
- **sth-dashboard**: migration 0030 (`org_shared_configs` + RLS); admin editor UI in spec 004.

## Complexity Tracking

> Fill ONLY if Constitution Check has violations that must be justified.

No violations detected — complexity table not required.
