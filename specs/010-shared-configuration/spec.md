# Feature Specification: Shared Configuration — Team CLI Config

**Feature Branch**: `010-shared-configuration`
**Created**: 2026-06-12
**Status**: Draft
**Input**: Dashboard roadmap teaser — "Configuration partagée : Une même config CLI pour toute l'équipe." Description: "versions épinglées, fournisseurs et catégories synchronisés automatiquement." Currently shown as a paid-plan ("Hors Alpha") teaser. This spec designs the **CLI + backend**; the dashboard admin editor is specified in `sth-dashboard/specs/004-shared-config-admin-ui`.

**Depends On**: 005 (state/config foundation), 006 (resource lifecycle, `--dry-run` & pinning), **009 (active org)**
**Blocks**: `sth-dashboard/004-shared-config-admin-ui`

## Context

Today every team member configures their CLI independently: `.sth/project.json` (`ProjectConfigV2`:
providers, resources, targets, pinned versions) is purely local, and nothing is stored server-side.
Teams want a single org-level baseline — the same providers, the same pinned versions, the same
category selection — applied automatically across all members.

This feature adds an **org-level shared configuration**: a server-stored, org-scoped subset of
`ProjectConfigV2` that owners/admins publish (`sth config push`) and members apply (`sth config
pull`). It is a **Team** feature, gated by the existing `fleet_sync` capability for writes. Per the
product promise ("synchronisés automatiquement"), the org config is **authoritative for the managed
fields** (providers + pinned versions): a pull overwrites those locally while preserving purely-local
resources. Secrets are never stored or transmitted.

The org whose config is read/written is always the **active org** from spec 009.

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Apply the team config (Priority: P1)

A team member runs one command and their CLI adopts the team's providers and pinned versions.

**Why this priority**: This is the value proposition — "une même config CLI pour toute l'équipe".
Without pull, there is no sharing.

**Independent Test**: With an org that has a published config, run `sth config pull` in a fresh
project and verify `.sth/project.json` now contains the org's providers and pinned versions, while
any resource that existed only locally is preserved.

**Acceptance Scenarios**:

1. **Given** the active org has a published shared config, **When** a member runs `sth config pull`,
   **Then** the managed fields (providers, pinned versions, category selection) in
   `.sth/project.json` are replaced by the org's, and purely-local resources are kept.
2. **Given** a member runs `sth config pull --dry-run`, **When** the command executes, **Then** the
   planned changes are printed and nothing is written to disk.
3. **Given** the active org has no shared config, **When** a member runs `sth config pull`, **Then**
   the CLI prints "no shared configuration for this organization" and exits 0 (no error).
4. **Given** the shared config conflicts with a locally-overridden managed field, **When** `pull`
   runs, **Then** the org value wins (the field is overwritten) and the change is reported in the
   summary.

---

### User Story 2 — Publish the team config (Priority: P1)

An owner or admin publishes the current project's shareable config as the org baseline.

**Why this priority**: Someone must set the baseline before anyone can pull it.

**Independent Test**: As an owner/admin on a Team org, run `sth config push` from a configured
project and verify the org config is stored, then have a second member pull it.

**Acceptance Scenarios**:

1. **Given** an owner/admin on a Team-entitled org, **When** they run `sth config push`, **Then** the
   shareable subset of the local `.sth/project.json` (providers, category selection, pins) is stored
   as the org config and a new version number is returned.
2. **Given** a caller who lacks `fleet_sync` (not Team), **When** they run `sth config push`, **Then**
   the command is refused with the standard not-entitled message naming `fleet_sync`.
3. **Given** a caller with `fleet_sync` but role `member`, **When** they run `sth config push`, **Then**
   the backend rejects the write with a clear "owner/admin only" authorization error.
4. **Given** secrets or tokens exist in the local environment, **When** `push` runs, **Then** no
   secret is ever included in the stored config.

---

### User Story 3 — Inspect the effective config (Priority: P1)

A developer sees what their effective config is and where each part comes from (org vs local).

**Independent Test**: Run `sth config show` and verify it lists providers and pins with a provenance
tag per field.

**Acceptance Scenarios**:

1. **Given** an org config has been pulled, **When** the user runs `sth config show`, **Then** each
   managed field is displayed with a provenance marker (`org` or `local`).
2. **Given** no org config exists, **When** `sth config show` runs, **Then** all fields are marked
   `local`.

---

### User Story 4 — Safe concurrent publishing (Priority: P2)

Two admins editing the org config do not silently clobber each other.

**Independent Test**: Push a config, then push again from a stale base version and verify the second
push is rejected with a re-pull instruction.

**Acceptance Scenarios**:

1. **Given** an admin pushed version N, **When** a second admin pushes against version N−1, **Then**
   the push is rejected with "config changed, re-pull" and a non-zero exit code.
2. **Given** an admin pulls the latest, edits, and pushes against the current version, **When** the
   push runs, **Then** it succeeds and the version is incremented.

---

### Edge Cases

- An org config that pins a resource version that no longer exists upstream — surfaced by
  `sth status` as an unresolved pin (reuses spec 006 pinning notices), not a hard failure of `pull`.
- A member of a non-Team org — there is simply no org config to pull (read returns empty); they are
  never blocked by a capability error on `pull`/`show`.
- First `push` for an org — creates version 1.
- `pull` with no local `.sth/project.json` — creates one from the org config.
- A provider present only locally (not in the org config) — preserved by `pull` (it is not a managed
  override of an org field; it is an addition). Providers present in BOTH are taken from the org.

## Requirements *(mandatory)*

### Functional Requirements

**CLI commands**

- **FR-001**: The CLI MUST provide `sth config pull`, `sth config push`, and `sth config show`, wired
  through `commandCatalog` + the `run()` switch with a `runConfig()` method on `*app`.
- **FR-002**: `sth config pull` MUST apply the active org's managed fields (providers, pinned versions,
  category selection) **over** the local `.sth/project.json`, MUST preserve purely-local resources,
  and MUST support `--dry-run` to preview without writing.
- **FR-003**: `sth config push` MUST require the `fleet_sync` capability via `enforceCapability`, and
  the backend MUST additionally enforce owner/admin role.
- **FR-004**: `sth config show` MUST display the effective config with per-field provenance (`org` vs
  `local`) and MUST NOT be capability-gated.
- **FR-005**: When the active org has no shared config, `pull` and `show` MUST degrade gracefully
  (message + exit 0); the first `push` MUST create version 1.

**Content & safety**

- **FR-006**: The shared config MUST contain only the non-secret shareable subset of `ProjectConfigV2`
  — providers, resource/category selection, and pinned versions. Secrets/tokens MUST NEVER be stored
  or transmitted.
- **FR-007**: On a managed-field conflict, the **org value MUST win** (overwrite local) and the change
  MUST be reported in the pull summary.

**Backend (sth-back)**

- **FR-008**: The backend MUST expose `GET /v1/org-config` returning the active org's `{ config,
  config_version }` (or an empty result), readable by any member of the org.
- **FR-009**: The backend MUST expose `PUT /v1/org-config` that upserts the active org's config; it
  MUST require `fleet_sync` AND owner/admin role (looked up via `memberships` for the key's
  `assignee_id`, since `resolveAuth` does not expose role), and MUST reject keys with no assignee.
- **FR-010**: `PUT /v1/org-config` MUST implement optimistic concurrency: the caller supplies the base
  version (`If-Match` / body field); a stale push MUST be rejected with a conflict error and a non-zero
  outcome, and a successful push MUST increment `config_version`.

**Data**

- **FR-011**: Migration **0030** in `sth-dashboard/supabase/migrations/` MUST create
  `org_shared_configs` (one row per org) with member-read / owner-admin-write RLS using the
  `(SELECT requesting_user_id())` pattern.

**Gating data**

- **FR-012**: Shared configuration writes MUST reuse the existing `fleet_sync` (Team) capability; no
  new capability and no schema change to the capability maps are required.

**Code quality**

- **FR-013**: All new Go code MUST follow the strict repo style (no single-letter vars; no comments,
  godoc included) and ship `*_test.go` unit tests using stdlib `testing` only. Backend routes MUST
  reuse `resolveAuth`, `getServiceClient`, and `jsonOk/jsonError`.

### Key Entities

- **OrgSharedConfig** (DB + wire): `organization_id` (unique), `config` (the shareable
  `ProjectConfigV2` subset, internally schema-versioned), `config_version` (monotonic), `updated_by`,
  `updated_at`.
- **ConfigMergeResult** (CLI): per managed field, the provenance (`org` | `local`) and the action a
  pull would take (`apply` | `keep` | `overwrite`), used by `show` and by `pull --dry-run`.

## Success Criteria *(mandatory)*

- **SC-001**: After one member `push` and another `pull`, both CLIs resolve identical providers and
  pinned versions for the managed fields.
- **SC-002**: A non-Team caller can never publish a shared config (100% refused), and a member-role
  caller can never publish (100% refused server-side).
- **SC-003**: `pull --dry-run` lists every change it would make and writes nothing to disk.
- **SC-004**: No secret value ever appears in the stored or transmitted shared config.
- **SC-005**: Concurrent stale pushes are rejected 100% of the time via the version check.

## Assumptions

- **Precedence**: org-priority on managed fields (providers + pins) with local-only resources
  preserved — chosen to honor "synchronisé automatiquement". A purely additive/local-wins merge was
  considered and rejected as it would let team drift persist.
- **One config per org** (`UNIQUE(organization_id)`); named profiles are deferred to a future spec.
- **Read access**: `pull`/`show` are not capability-gated client-side; non-Team orgs simply have no
  config to return. Only `push` is gated (`fleet_sync`).
- **Active org**: the org targeted by every `config` command is the active org from spec 009; this
  spec does not introduce its own org-resolution path.
- **Secrets**: provider tokens are resolved at runtime from env/keychain/credential helpers (spec 007)
  and are out of the shared config by construction, consistent with `init` never persisting secrets.
