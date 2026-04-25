# Feature Specification: State & Config Foundation

**Feature Branch**: `006-cli-product-roadmap`  
**Created**: 2026-04-23  
**Status**: Draft  
**Depends On**: — (foundational, no upstream dependency)  
**Blocks**: 006-resource-lifecycle, 007-ci-auth-observability, 008-ecosystem-tooling  

## Context

This is the foundational spec that enables all other roadmap features. The current
`sth-state.json` and `project.json` schemas are insufficient to support resource lifecycle
management, integrity verification, version pinning, or multi-provider setups. Before any
of those features can be built, the data model must be extended and the terminology
clarified.

Three concrete problems this spec resolves:

1. **`ref` is overloaded**: in `project.json`, `ref` currently means the provider's
   catalog branch/tag. Resource identity and version pinning need separate fields.
2. **State tracks no file inventory**: `sth-state.json` stores one record per resource
   but does not list which files were installed, their paths, or their checksums.
3. **Config is single-provider**: `project.json` supports one provider; multi-repo
   support requires a `providers` array.

---

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Migrate an Existing Project Without Losing Data (Priority: P1)

A developer already has a configured sth project with installed resources tracked
in `sth-state.json`. After upgrading sth, they run any command for the first time.
The tool detects the old schema version, migrates the config and state files in-place,
and completes the original command as if nothing happened.

**Why this priority**: Without a migration path, upgrading sth would break every
existing project. This must ship before any feature that reads the new schema.

**Independent Test**: Create an old-format `project.json` + `sth-state.json`, run
`sth list`, verify both files are rewritten to the new schema and the output
is correct.

**Acceptance Scenarios**:

1. **Given** a `project.json` in the v1 (single-provider) format, **When** any command
   runs, **Then** the file is rewritten to v2 (multi-provider) format with a single
   entry in the `providers` array and the schema version bumped to `2`.
2. **Given** a `sth-state.json` with entries that have no `managed_files` or `sha256`
   fields, **When** any command runs, **Then** the state file gains the new fields
   with empty/null values for legacy entries and the schema version is bumped.
3. **Given** a migration succeeds, **When** the developer inspects the migrated files,
   **Then** no information from the original file is lost and a backup of the original
   file is created alongside it (e.g., `project.json.bak`).
4. **Given** a migration fails (e.g., disk write error), **When** the error occurs,
   **Then** the original files are untouched, the error is reported with a clear
   remediation step, and the command exits with a non-zero code.

---

### User Story 2 — Configure Multiple Providers in a New Project (Priority: P1)

A developer initialises a new project and wants to pull resources from two different
skill repositories: one internal company repo and one public community repo. During
`sth init`, they can add both providers. Each provider is identified by a unique
short name they choose.

**Why this priority**: Multi-provider config must be expressible in `project.json`
before any feature can consume it. The schema must exist before the UX.

**Independent Test**: Run `sth init` twice (once per provider), verify
`project.json` contains both entries, then run `sth list` and verify resources
from both providers are shown.

**Acceptance Scenarios**:

1. **Given** an empty project, **When** the developer runs `sth init` and adds
   two providers, **Then** `project.json` contains a `providers` array with two entries,
   each with a unique `id`, `provider`, `repository_id`, `catalog_ref`, and `catalog_path`.
2. **Given** a project with one provider, **When** the developer runs `sth init`
   again and provides details for a second provider, **Then** the second provider is
   appended to the `providers` array without altering the first entry.
3. **Given** the developer provides a duplicate provider `id`, **When** confirming the
   init, **Then** the tool rejects the duplicate with a clear error message.
4. **Given** a `project.json` v2 file, **When** it is parsed, **Then** the tool
   resolves the correct provider for each resource using the resource's `provider_id` field.

---

### User Story 3 — Pin a Resource to a Specific Version (Priority: P1)

A developer wants to lock a specific installed resource to the version currently in
production, so future `sth update` runs skip it. They edit `project.json` to
add a `pinned_version` field to the resource entry.

**Why this priority**: The pin field must be part of the schema before the lifecycle
commands (006) can implement the skip-if-pinned behaviour.

**Independent Test**: Add `"pinned_version": "abc123"` to a resource entry in
`project.json`, run `sth update`, verify the resource entry in `sth-state.json`
is not updated.

**Acceptance Scenarios**:

1. **Given** a resource entry in `project.json` has a `pinned_version` set, **When**
   the state is read, **Then** `pinned_version` is available to all commands that check
   resource state.
2. **Given** a resource entry has no `pinned_version`, **When** the state is read,
   **Then** the resource is treated as unpinned and eligible for update.
3. **Given** a `pinned_version` refers to a non-existent commit/tag on the provider,
   **When** any command resolves the resource, **Then** the tool emits a clear error
   identifying the invalid ref and exits with a non-zero code.

---

### User Story 4 — View Complete Managed-File Inventory (Priority: P1)

After installing a resource, a developer can inspect `sth-state.json` and see every
file that was written to disk by that installation: its relative path and its SHA256
checksum at install time.

**Why this priority**: The managed-file inventory is the prerequisite for `remove`
(know what to delete), `status` (detect modifications), and integrity checks.

**Independent Test**: Install a resource, open `sth-state.json`, verify each installed
file appears with `path` and `sha256` fields.

**Acceptance Scenarios**:

1. **Given** a fresh resource installation, **When** `sth-state.json` is written,
   **Then** each file installed by that resource is listed under `managed_files` with
   its relative path and SHA256 hash.
2. **Given** a resource installation produces zero files (empty bundle), **When** the
   state is written, **Then** `managed_files` is an empty array (not absent).
3. **Given** a resource is updated to a newer version, **When** the state is written,
   **Then** `managed_files` reflects the new inventory (added/removed files updated),
   `previous_version` is set to the prior resolved commit, and `resolved_version` is
   updated.

---

### Edge Cases

- What happens when a `project.json` has no `schema_version` field? → Treat as v1, migrate.
- What if two resources in the same project list the same `managed_file` path? → Recorded in state with both owning resource IDs; conflict is surfaced at install time (spec 006 handles resolution policy).
- What if the backup file (`.bak`) already exists from a previous failed migration? → Overwrite the backup; never fail migration due to a pre-existing backup.
- What if `sth-state.json` is corrupt (invalid JSON)? → Report clearly, do not auto-migrate; require user to delete the state file and re-install.
- What if a provider `id` contains special characters? → Validated at `init` time; only alphanumeric, hyphens, and underscores allowed.
- What if `.sth-state.lock` is stale (left behind by a crashed process)? → The lock file is deleted automatically after every successful operation; a stale lock requires manual removal; the error message instructs the user to delete `.sth-state.lock` if no other `sth` process is running.

---

## Requirements *(mandatory)*

### Functional Requirements

**Schema Versioning**

- **FR-001**: `project.json` MUST include a `schema_version` field (integer). The current format is version `1`; the new format introduced by this spec is version `2`.
- **FR-002**: `sth-state.json` MUST include a `schema_version` field (integer). Existing files without the field are treated as version `1`; this spec introduces version `2`.

**Migration**

- **FR-003**: On first run after upgrade, the tool MUST detect v1 files and automatically migrate them to v2 before executing the requested command.
- **FR-004**: Before rewriting any config or state file during migration, the tool MUST create a `.bak` copy of the original file in the same directory.
- **FR-005**: If migration fails, the tool MUST leave the original files untouched and report the failure with a remediation step.

**Multi-Provider Config (project.json v2)**

- **FR-006**: `project.json` v2 MUST replace the single-provider flat fields with a `providers` array. Each provider entry MUST have: `id` (unique short name), `provider` (e.g., `github`), `repository_id`, `catalog_ref` (branch/tag of the catalog — formerly `ref`), `catalog_path`, and `target_dir`.
- **FR-007**: Each resource entry in `project.json` v2 MUST include a `provider_id` field referencing a `providers` entry, and an optional `pinned_version` field (commit SHA or tag).
- **FR-008**: Provider `id` values MUST be unique within a project and MUST contain only alphanumeric characters, hyphens, or underscores.
- **FR-009**: `sth init` MUST support configuring multiple providers by allowing the user to add provider entries sequentially.

**State Schema (sth-state.json v2)**

- **FR-010**: Each resource record in `sth-state.json` v2 MUST include: `resource_name`, `provider_id`, `resolved_version` (commit SHA), `previous_version` (commit SHA of the prior install, or empty for first install), `installed_at` (ISO 8601 timestamp), and `managed_files` (array). Records are uniquely identified by the compound key `(resource_name, provider_id)`; the same resource name sourced from two different providers constitutes two distinct records.
- **FR-011**: Each entry in `managed_files` MUST include: `path` (relative to project root) and `sha256` (hex-encoded hash of the file content at install time). SHA256 values are stored for consumption by spec 006 (resource lifecycle); integrity verification logic (comparing stored hashes to on-disk files) is out of scope for this spec.
- **FR-012**: When a resource is updated, the tool MUST set `previous_version` to the current `resolved_version` before overwriting it.

**Terminology**

- **FR-013**: The term `ref` MUST only be used for `catalog_ref` (the provider branch/tag). Resource-level versioning MUST use `pinned_version` and `resolved_version`. All internal code and user-facing messages MUST use this terminology consistently.

**Reliability**

- **FR-014**: ALL writes to the v2 state file `sth-state-v2.json` (install, update, and migration output) MUST use an atomic write-then-rename strategy: write the new content to a temporary file (e.g., `.sth-tmp-*`) in the same directory, then rename it into place using `os.Rename()`. Direct in-place overwrites are prohibited to prevent partial-write corruption on process interruption. The legacy file `sth-state.json` is migration input only and MUST NOT be treated as the canonical v2 state path.
- **FR-015**: Any operation that reads or writes `sth-state-v2.json` MUST first acquire an advisory file lock (`.sth-state.lock`, created with `O_CREATE|O_EXCL`) before proceeding. Migration flows that read legacy `sth-state.json` and then create or update `sth-state-v2.json` MUST also hold this same lock for the full operation. If the lock is already held by another process, the tool MUST fail fast with a clear error message identifying the lock file and exit with a non-zero code. No silent overwrites or wait/retry loops are permitted.

**Observability**

- **FR-016**: Migration events MUST emit one human-readable progress line to `stderr` per file migrated, using the format `[sth] migrating <filename> v<N> → v<M> … done` (or `… failed: <reason>` on error). No structured JSON output, external logging dependency, or metrics sink is required.

### Key Entities

- **Provider**: A configured source of resources. Fields: `id`, `provider`, `repository_id`, `catalog_ref`, `catalog_path`, `target_dir`.
- **ResourceEntry** (in project.json): Declares a desired resource. Fields: `resource_name`, `provider_id`, `pinned_version` (optional).
- **InstalledResource** (in sth-state.json): Tracks the installed state of a resource. Fields: `resource_name`, `provider_id`, `resolved_version`, `previous_version`, `installed_at`, `managed_files`. The unique identity key within `sth-state.json` is the compound key `(resource_name, provider_id)`; the same resource name from two different providers is permitted and treated as distinct records.
- **ManagedFile**: One file owned by an installed resource. Fields: `path`, `sha256`.

---

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of existing projects (v1 schema) are migrated without data loss on first command run after upgrade, verified by comparing resource counts before and after.
- **SC-002**: Migration completes in under 2 seconds for projects with up to 50 installed resources.
- **SC-003**: Every installed file is traceable to exactly one resource via `sth-state.json` — no installed files are unaccounted for after a successful install.
- **SC-004**: Zero breaking changes to existing `sth init` and `sth install` workflows from the user's perspective.
- **SC-005**: All commands that previously read `project.json` or `sth-state.json` continue to work correctly after schema migration.
- **SC-006**: A `sth-state.json` write interrupted at any point (e.g., process kill) leaves the prior valid file intact and never produces a partial/corrupt state file.
- **SC-007**: When two `sth` processes attempt concurrent state access, exactly one proceeds and the other exits immediately with a non-zero code and a message identifying `.sth-state.lock`.

---

## Assumptions

- The existing `provider` field values (`github`, `gitlab`, etc.) are preserved as-is during migration.
- `target_dir` is currently a single global setting; in v2 it moves to per-provider. During migration, the global value is copied to each migrated provider entry.
- SHA256 hashes are computed over the raw file bytes as written to disk (after applying the `skillHeader` prefix if applicable).
- `catalog_ref` replaces the current `ref` field semantically; the JSON key is renamed from `ref` to `catalog_ref` in v2.
- The `APIToken` field remains session-only (never persisted) — the credential model (how tokens are sourced) is out of scope for this spec and is addressed in spec 007.
- `sth init` UX for adding multiple providers can be sequential interactive prompts; a `sth provider add` subcommand is deferred to a future spec.

---

## Clarifications

### Session 2026-04-25

- Q: What atomicity model should govern all `sth-state.json` writes (install, update, migration)? → A: Write-then-rename — write to `.sth-state.json.tmp`, then atomically rename to final path via `os.Rename()`; direct in-place overwrites are prohibited.
- Q: When/how are stored SHA256 checksums used for verification, and is a `sth verify` command in scope? → A: Data-only — hashes are stored by this spec; integrity verification logic (hash comparison, drift detection) is spec 006's responsibility; no verify command or passive check is introduced here.
- Q: What concurrency/file-locking model should govern simultaneous `sth` invocations on the same project? → A: Advisory file lock — acquire `.sth-state.lock` (O_CREATE|O_EXCL) before any state read/write; fail fast with a clear error if already held; no wait/retry loop.
- Q: What observability is required for migration events? → A: Emit one human-readable `stderr` line per migration event (`[sth] migrating <file> v<N> → v<M> … done/failed`); no structured JSON or external logging dependency.
- Q: What is the unique identity key for a resource record in `sth-state.json`? → A: Compound key `(resource_name, provider_id)` — same resource name from two different providers is permitted and stored as two distinct records.
