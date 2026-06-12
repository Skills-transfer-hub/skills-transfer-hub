# Feature Specification: Multiple Organizations — CLI Org Context

**Feature Branch**: `009-multi-organizations`
**Created**: 2026-06-12
**Status**: Draft
**Input**: Dashboard roadmap teaser — "Plusieurs organisations : Un seul compte, plusieurs organisations." Currently shown as a paid-plan ("Hors Alpha") teaser. This spec designs the **CLI + backend** half; the dashboard org-switcher UI is specified in `sth-dashboard/specs/003-org-switcher-ui`.

**Depends On**: 005 (state/config foundation), 007 (CI/auth)
**Blocks**: 010 (shared configuration), `sth-dashboard/003-org-switcher-ui`

## Context

The data layer already models multi-org fully: `memberships(user_id, organization_id, role)` is many-to-many and `users.active_organization_id` tracks a single active org. The dashboard exposes `getUserOrganizations()` and `switchActiveOrganization()`. What is missing is the **CLI** side: an `lsk_live_` key resolves to exactly one org via `licenses.organization_id`, `resolveAuth` is wired to that single org, and `sth login` receives the org name at login but discards it. The CLI therefore has no notion of "which org am I acting as", and there is no way to see, list, or switch organizations from the terminal.

This feature gives the CLI a persistent, switchable **active organization**, gated behind a new paid capability `multi_org`, while remaining fully backward compatible for single-org users and CI keys.

## User Scenarios & Testing *(mandatory)*

### User Story 1 — See the active org from the CLI (Priority: P1)

A developer who works across several organizations wants to confirm, before running a command, which org their CLI is currently acting as.

**Why this priority**: Acting in the wrong org (installing into, or reporting usage for, the wrong team) is the core risk multi-org introduces. Visibility is the prerequisite for trust.

**Independent Test**: With a logged-in CLI, run `sth org current` and verify it prints the active org's name, id, the caller's role, and the resolved plan — served from cache when offline.

**Acceptance Scenarios**:

1. **Given** a logged-in user, **When** they run `sth org current`, **Then** the CLI prints the active org id, name, role, and plan tier.
2. **Given** no org has been explicitly selected, **When** they run `sth org current`, **Then** the CLI reports the key's home org (the org the license resolves to) and indicates it is the default.
3. **Given** the network is unavailable, **When** they run `sth org current`, **Then** the CLI serves the last-known values from cache within the stale window and marks them as cached.

---

### User Story 2 — List the organizations the account can target (Priority: P1)

A developer who belongs to multiple organizations wants to see all orgs they can act as from the CLI, with the active one marked.

**Why this priority**: Switching is meaningless without discovery; the user must know the valid targets and their identifiers.

**Independent Test**: Run `sth org list` with a key whose identity is a member of two orgs and verify both appear, the active one is marked, and `--json` emits a stable schema.

**Acceptance Scenarios**:

1. **Given** the caller's identity is a member of two or more orgs, **When** they run `sth org list`, **Then** every targetable org is listed with id, name, role, and plan, and the active org is marked.
2. **Given** `--json` is passed, **When** `sth org list` runs, **Then** output is valid JSON matching the documented schema.
3. **Given** the caller's active org is not on a `multi_org`-entitled plan, **When** they run `sth org list`, **Then** the command is refused with a clear "requires multi_org (Pro/Team)" message and a non-zero exit code.

---

### User Story 3 — Switch the active organization (Priority: P1)

A developer changes which organization subsequent CLI commands act against.

**Why this priority**: This is the feature. Everything else is discovery and safety around it.

**Independent Test**: Run `sth org use <id|name>` targeting a second org the identity belongs to, then run `sth org current` and verify the active org changed and persisted across processes.

**Acceptance Scenarios**:

1. **Given** the caller is a member of the target org, **When** they run `sth org use <id|name>`, **Then** the active org is persisted and a confirmation is printed.
2. **Given** the caller is NOT a member of the target org, **When** they run `sth org use <target>`, **Then** the command fails with a clear error and a non-zero exit code, and the active org is unchanged.
3. **Given** a successful switch, **When** the user runs any subsequent org-scoped command in a new process, **Then** that command acts within the newly selected org.
4. **Given** the active org is not on a `multi_org`-entitled plan, **When** they run `sth org use`, **Then** the command is refused with the capability message.

---

### User Story 4 — Org-scoped commands act on the active org (Priority: P1)

Catalog, install, usage reporting, and entitlements all act within the selected active org rather than always the key's home org.

**Why this priority**: A switch that does not actually re-scope downstream commands is a cosmetic illusion and a correctness hazard.

**Independent Test**: Select org B, run `sth list`, and verify the catalog returned is org B's (public + org B private), and that a usage record is attributed to org B.

**Acceptance Scenarios**:

1. **Given** an active org selected with `sth org use`, **When** an org-scoped command runs, **Then** the request carries the active-org selector and the backend resolves the org from it.
2. **Given** the backend receives a selector for an org the key's identity is a member of, **When** it authenticates the request, **Then** it resolves org and effective plan from the selected org.
3. **Given** the backend receives a selector the identity is NOT a member of, **When** it authenticates, **Then** it rejects the request without revealing whether the org exists.
4. **Given** no selector is sent (older CLI, or no selection made), **When** a command runs, **Then** the backend falls back to the key's home org — identical to today's behavior.

---

### User Story 5 — Login surfaces multi-org (Priority: P2)

When a user logs in with a key whose identity belongs to several orgs, the CLI tells them and points to `sth org use`.

**Acceptance Scenarios**:

1. **Given** a key whose identity is a member of more than one org, **When** `sth login` succeeds, **Then** the CLI reports the count and how to switch.
2. **Given** a single-org identity, **When** `sth login` succeeds, **Then** no multi-org hint is shown (no behavior change).

---

### User Story 6 — Graceful handling of a stale active org (Priority: P2)

If the persisted active org is no longer valid (membership lost, org scheduled for deletion), the CLI recovers instead of failing cryptically.

**Acceptance Scenarios**:

1. **Given** a persisted active org the identity is no longer a member of, **When** any command runs, **Then** the CLI emits a one-time warning and falls back to the home org.
2. **Given** the active org is scheduled for deletion, **When** `sth org current` runs, **Then** the CLI surfaces that status.

---

### Edge Cases

- A CI key with no `assignee_id` (no human owner) — cannot enumerate memberships; remains single-org (home org only).
- The active org's plan downgrades from Pro/Team to starter between sessions — `org list`/`org use` become gated; `org current` still works.
- `sth org use` to the org that is already active — succeeds idempotently.
- A name passed to `sth org use` that is ambiguous (two orgs with the same name) — the CLI requires the id to disambiguate.
- Offline `sth org use` — rejected (membership cannot be validated server-side) with a clear "requires network" message.

## Requirements *(mandatory)*

### Functional Requirements

**CLI commands**

- **FR-001**: The CLI MUST provide `sth org current` printing the active org id, name, caller role, and plan tier; it MUST serve cached values when offline within the entitlements stale window and mark them as cached. `org current` MUST NOT be capability-gated.
- **FR-002**: The CLI MUST provide `sth org list` showing every org the caller's identity can target, marking the active one, supporting `--json` with a stable documented schema.
- **FR-003**: The CLI MUST provide `sth org use <id|name>` that validates membership server-side, persists the selection on success, confirms, and on failure leaves the previous selection unchanged with a non-zero exit code.
- **FR-004**: `sth org list` and `sth org use` MUST be gated by the `multi_org` capability via `enforceCapability`; refusal MUST use the standard not-entitled message naming the `multi_org` capability.

**Persistence & selection**

- **FR-005**: User settings MUST gain an `active_org` block recording the selected org (id, name, role, plan, selection timestamp). Absence of this block MUST mean "use the key's home org" (backward compatible).
- **FR-006**: Org-scoped commands (catalog, install, usage, entitlements) MUST carry the active-org selector to the backend when an active org is set.
- **FR-007**: `sth login` MUST associate the validated key with the org it resolves to and MUST report when the identity belongs to more than one org, pointing to `sth org use`.
- **FR-008**: A persisted active org that is no longer valid (lost membership, org deleting) MUST trigger a single warning and an automatic fallback to the home org.

**Backend (sth-back)**

- **FR-009**: The backend MUST accept an active-org selector on authenticated CLI requests and, when present and valid for the key's identity membership, MUST resolve org and effective plan from the selected org instead of the license's home org.
- **FR-010**: When the selector names an org the key's identity is not a member of, the backend MUST reject the request with an authorization error that does not disclose org existence.
- **FR-011**: When no selector is present, the backend MUST behave exactly as today (resolve the license's home org) — zero regression for existing callers.
- **FR-012**: Keys with no `assignee_id` MUST be treated as single-org (home org only); a selector on such keys MUST be ignored or rejected, never honored.

**Entitlements / gating data**

- **FR-013**: A new capability `multi_org` MUST be added to the `pro`, `team`, and `alpha` tiers in BOTH `sth-back/shared/capabilities.ts` and its mirror `sth-dashboard/src/lib/billing/plans.ts` (the maps are annotated "KEEP IN SYNC"). No database migration is required (capabilities derive from `plan_tier`).
- **FR-014**: The `multi_org` capability MUST be resolved from the **active org's** entitlements (i.e. `GET /v1/entitlements` for the selected org).

**Code quality**

- **FR-015**: All new Go code MUST follow the repo's strict style (no single-letter variables outside the listed idioms; no comments, godoc included) per `CLAUDE.md`, and MUST be wired through the existing `commandCatalog` + `run()` dispatcher with a `runOrg()` method on `*app`. New behavior MUST ship with `*_test.go` unit tests using the stdlib `testing` package only.

### Key Entities

- **ActiveOrgSelection** (CLI, persisted in user settings): `org_id`, `org_name`, `role`, `plan`, `selected_at`. Absent ⇒ home org.
- **TargetableOrg** (wire row for `sth org list`): `id`, `name`, `role`, `plan`, `active` (bool).
- **OrgSelector** (wire): the active-org identifier sent on authenticated requests; honored only against the key identity's memberships.

## Success Criteria *(mandatory)*

- **SC-001**: A user belonging to N orgs can enumerate all N and identify the active one in a single `sth org list`.
- **SC-002**: After `sth org use B`, 100% of subsequent org-scoped commands in fresh processes act on org B until changed.
- **SC-003**: Existing single-org users and CI keys observe zero behavior change when they never call `sth org use` (no selector sent, home org resolved).
- **SC-004**: Attempting to switch to a non-member org fails closed in 100% of cases without leaking org existence.
- **SC-005**: `sth org current` resolves in under 1s online and works offline from cache.

## Assumptions

- **Key→org model**: the recommended design is a **single key + active-org selector** (header) validated against `memberships` for the key's `assignee_id`. The simpler alternative (one key per org via repeated `sth login`) is documented in `contracts/` as a fallback but not the primary path.
- **Capability scope**: `multi_org` is resolved from the active org's plan. A documented alternative — gating on the *target* org's plan during `sth org use` — is left as a refinement; the default keeps parity with the existing `enforceCapability` flow.
- **Server-authoritative active org**: out of scope. The CLI active org lives only in user settings; the dashboard's `users.active_organization_id` stays Clerk-session-scoped and independent.
- **No schema migration**: 009 reuses `memberships` + `active_organization_id`; it adds no table or column.
- **Membership is free**: belonging to multiple orgs (via invitations) is not itself gated; only the CLI multi-org commands are. The per-org plan continues to gate per-org features.
