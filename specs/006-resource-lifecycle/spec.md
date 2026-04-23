# Feature Specification: Resource Lifecycle

**Feature Branch**: `006-cli-product-roadmap`  
**Created**: 2026-04-23  
**Status**: Draft  
**Depends On**: [005-state-config-foundation](../005-state-config-foundation/spec.md)  
**Blocks**: 008-ecosystem-tooling  

## Context

With the new state schema in place (spec 005), loreskills can implement a proper
resource lifecycle: users can see what is installed and its health (`status`), safely
preview changes before they happen (`--dry-run`), cleanly remove resources they no longer
need (`remove`), lock resources to a known-good version (`pinning`), and exclude unwanted
files from a bundle (`.loreskillsignore`).

---

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Inspect Resource Health with `loreskills status` (Priority: P1)

A developer runs `loreskills status` and immediately sees, for every installed resource,
whether it is current, outdated, or has been locally modified — without needing to
cross-reference SKILLS.md or compare commit SHAs manually.

**Why this priority**: Status is the entry point to all lifecycle decisions. Without it,
users cannot know whether to update, remove, or investigate a resource.

**Independent Test**: Install a resource, manually edit one managed file, run
`loreskills status`, verify the resource appears as `modified-locally` with the edited
file listed.

**Acceptance Scenarios**:

1. **Given** all installed resources match their upstream version and no managed files
   have been edited, **When** the user runs `loreskills status`, **Then** each resource
   is listed as `up-to-date`.
2. **Given** a newer version is available for a resource, **When** the user runs
   `loreskills status`, **Then** that resource is listed as `outdated` with the current
   and available versions shown.
3. **Given** one or more managed files for a resource have been edited since install,
   **When** the user runs `loreskills status`, **Then** that resource is listed as
   `modified-locally` and the specific modified files are listed.
4. **Given** a resource was installed before spec 005 (no SHA256 baseline), **When** the
   user runs `loreskills status`, **Then** that resource is shown as `unknown-baseline`
   with a suggestion to re-install to establish integrity tracking.
5. **Given** no resources are installed, **When** the user runs `loreskills status`,
   **Then** the tool outputs "No resources installed" and exits with code 0.

---

### User Story 2 — Remove a Resource Cleanly with `loreskills remove` (Priority: P1)

A developer no longer needs a skill bundle. They run `loreskills remove <resource-name>`
and the tool deletes every file that was installed by that resource and removes its
entry from `lore-state.json`. They do not need to know which files to delete manually.

**Why this priority**: Without `remove`, users must manually find and delete managed
files, risking orphaned state entries or incomplete cleanup.

**Independent Test**: Install a resource, record the files it created, run
`loreskills remove <name>`, verify all those files are gone and the state entry is
removed.

**Acceptance Scenarios**:

1. **Given** an installed resource with no locally modified files, **When** the user
   runs `loreskills remove <name>`, **Then** all files in `managed_files` are deleted
   and the resource entry is removed from `lore-state.json`.
2. **Given** an installed resource where one or more managed files have been locally
   modified, **When** the user runs `loreskills remove <name>`, **Then** the tool
   displays a warning listing the modified files and asks for explicit confirmation
   before proceeding.
3. **Given** the user confirms removal of a resource with modified files, **When**
   the confirmation is accepted, **Then** all managed files are deleted regardless of
   modification status and the state entry is removed.
4. **Given** the user runs `loreskills remove <name>` for a resource that is not
   installed, **Then** the tool returns a clear "resource not found" error and exits
   with a non-zero code.
5. **Given** a partial deletion failure (e.g., one file is locked by another process),
   **When** the failure occurs, **Then** successfully deleted files stay deleted, the
   remaining files are listed in the error output, and the state entry is NOT removed
   (since the removal is incomplete).
6. **Given** the `--yes` flag is passed, **When** running `loreskills remove <name>`
   even with modified files, **Then** the confirmation prompt is skipped and removal
   proceeds immediately.

---

### User Story 3 — Preview Changes Before Committing with `--dry-run` (Priority: P1)

A developer wants to know exactly which files will be created, updated, or deleted by
`loreskills install` or `loreskills update` before any changes are written to disk.
They pass `--dry-run` and get a full diff-style summary with no side effects.

**Why this priority**: Unreviewed file changes in a shared codebase can cause conflicts.
Dry-run is a safety gate before committing to an action.

**Independent Test**: Run `loreskills install <name> --dry-run` for a new resource,
verify no files are written to disk and a full list of "would create" files is printed.

**Acceptance Scenarios**:

1. **Given** `--dry-run` is passed to `loreskills install`, **When** the command runs,
   **Then** each file that would be created is listed with `[+]` prefix; no files are
   written to disk and `lore-state.json` is not modified.
2. **Given** `--dry-run` is passed to `loreskills update`, **When** the command runs,
   **Then** files that would be added, modified, or deleted are listed with `[+]`,
   `[~]`, and `[-]` prefixes respectively; no files are written to disk.
3. **Given** `--dry-run` is passed and no changes would result, **When** the command
   runs, **Then** the tool outputs "No changes would be made." and exits with code 0.
4. **Given** `--dry-run` is combined with `--json`, **When** the command runs, **Then**
   the dry-run output is a JSON array where each entry has `action` (`add`/`modify`/`delete`),
   `path`, and for `modify` the `old_sha256` and `new_sha256`.
5. **Given** `--dry-run` is run without network access, **When** the tool cannot fetch
   the remote catalog, **Then** the tool exits with a clear "offline" error and a
   non-zero code (dry-run does not suppress network errors).

---

### User Story 4 — Pin a Resource to a Specific Version (Priority: P1)

A developer locks a production resource to its current commit SHA by adding
`"pinned_version": "abc1234"` to its entry in `project.json`. Future `loreskills update`
runs skip this resource entirely and display it as "pinned". When a newer version is
available, a notice is shown but no action is taken.

**Why this priority**: Unpinned auto-updates can silently break working setups. Pinning
is a contract: "do not change this until I say so."

**Independent Test**: Pin a resource, run `loreskills update`, verify the resource's
`resolved_version` in `lore-state.json` is unchanged and the output shows "(pinned)".

**Acceptance Scenarios**:

1. **Given** a resource has `pinned_version` set, **When** `loreskills update` runs,
   **Then** the resource is skipped and shown as `[pinned at <version>]` in the output.
2. **Given** a pinned resource has a newer version available upstream, **When**
   `loreskills update` or `loreskills status` runs, **Then** a notice "Newer version
   available for <name> (pinned at <version>)" is shown but no update is performed.
3. **Given** a `pinned_version` refers to a commit/tag that no longer exists on the
   provider, **When** any command resolves that resource, **Then** the tool emits a
   clear error "Pinned version <version> not found for <name>" and exits non-zero.
4. **Given** the user removes `pinned_version` from a resource entry, **When** the next
   `loreskills update` runs, **Then** the resource is treated as unpinned and updated
   to the latest available version.

---

### User Story 5 — Exclude Unwanted Files from a Bundle with `.loreskillsignore` (Priority: P2)

A skill bundle contains test fixtures and example files that a developer does not want
installed in their project. They create a `.loreskillsignore` file in the project root
with gitignore-style patterns to exclude those files.

**Why this priority**: Bloated installs with unnecessary files degrade the user's
workspace. Exclusion rules give users control without requiring changes to the upstream bundle.

**Independent Test**: Add `*.test.md` to `.loreskillsignore`, install a bundle that
includes `.test.md` files, verify those files are not present in the target directory.

**Acceptance Scenarios**:

1. **Given** a `.loreskillsignore` file with a pattern that matches certain bundle files,
   **When** `loreskills install` runs, **Then** matching files are not written to disk
   and are not recorded in `managed_files`.
2. **Given** a `.loreskillsignore` file, **When** `loreskills update` runs, **Then**
   the same exclusion rules apply; files previously installed that now match a new
   pattern are removed from disk and from `managed_files`.
3. **Given** no `.loreskillsignore` file exists, **When** `loreskills install` runs,
   **Then** all bundle files are installed (current default behaviour, unchanged).
4. **Given** a `.loreskillsignore` entry uses a comment (`#`), **When** the file is
   parsed, **Then** the comment line is ignored.

---

### User Story 6 — File Ownership Conflict Detection (Priority: P2)

Two resources in the same project attempt to install a file to the same target path.
The tool detects this conflict and refuses to install the second resource without
explicit override, protecting the first resource's file from silent overwrite.

**Why this priority**: Silent overwrites in multi-resource projects are a data integrity
risk. Detection at install time prevents hard-to-debug divergence.

**Independent Test**: Create two mock resources that install a file to the same path,
attempt to install both, verify the second install fails with a conflict error.

**Acceptance Scenarios**:

1. **Given** resource B would install a file already owned by resource A, **When**
   `loreskills install B` runs, **Then** the tool halts with a conflict error listing
   the contested file path and the two resource names.
2. **Given** the `--force` flag is passed to `loreskills install B`, **When** the
   conflict is detected, **Then** the tool overwrites the file, transfers ownership to
   resource B in `managed_files`, and logs a warning.
3. **Given** the conflict is during `loreskills update` (not a new install), **When**
   the conflict is detected, **Then** the update for the conflicting resource is skipped
   and the conflict is reported; other resources update normally.

---

### Edge Cases

- What if a managed file was deleted externally before `loreskills remove` runs? → Log a warning for each missing file, continue removal, still clean up the state entry.
- What if `--dry-run` is run on a resource that has never been fetched? → The tool must still perform a network fetch to determine what files would be installed (dry-run affects writes, not reads).
- What if a `.loreskillsignore` pattern excludes all files in a bundle? → The install proceeds with zero files; `managed_files` is empty; a warning is shown ("no files installed — all excluded by .loreskillsignore").
- What if `loreskills remove` is interrupted (SIGINT) after partial deletion? → Partial state: remaining files + state entry persist. Next `loreskills remove` re-runs from current state and cleans up remaining files.

---

## Requirements *(mandatory)*

### Functional Requirements

**`loreskills status`**

- **FR-001**: The tool MUST provide a `loreskills status` command listing every installed resource with one of: `up-to-date`, `outdated`, `modified-locally`, or `unknown-baseline`.
- **FR-002**: For `outdated` resources, the command MUST display the installed version and the available version.
- **FR-003**: For `modified-locally` resources, the command MUST list each modified file.
- **FR-004**: For `unknown-baseline` resources (no SHA256 in state), the command MUST suggest running `loreskills install --force <name>` to re-establish integrity tracking.
- **FR-005**: `loreskills status` MUST support `--json` output (schema defined in spec 007).

**`loreskills remove`**

- **FR-006**: The tool MUST provide a `loreskills remove <resource-name>` command.
- **FR-007**: The command MUST delete all files listed in the resource's `managed_files` and remove the resource entry from `lore-state.json`.
- **FR-008**: If any managed file has been locally modified (SHA256 mismatch), the command MUST warn and require explicit confirmation before proceeding (unless `--yes` is passed).
- **FR-009**: If a managed file is missing from disk at removal time, the tool MUST log a warning and continue (not fail).
- **FR-010**: If the resource is not found in `lore-state.json`, the tool MUST exit with a non-zero code and a descriptive error.
- **FR-011**: If deletion of any managed file fails, the tool MUST report the failed files and NOT remove the state entry.

**`--dry-run`**

- **FR-012**: `loreskills install` and `loreskills update` MUST accept a `--dry-run` flag.
- **FR-013**: When `--dry-run` is active, no files MUST be written or deleted and `lore-state.json` MUST NOT be modified.
- **FR-014**: The dry-run output MUST label each file with `[+]` (add), `[~]` (modify), or `[-]` (delete).
- **FR-015**: When combined with `--json`, the dry-run output MUST be a JSON array (schema: `action`, `path`, optional `old_sha256`, `new_sha256`).

**Version Pinning**

- **FR-016**: When a resource's `pinned_version` is set, `loreskills update` MUST skip that resource and display it as pinned.
- **FR-017**: When a pinned resource has a newer upstream version, the tool MUST display a non-blocking informational notice.
- **FR-018**: If `pinned_version` cannot be resolved on the provider, the tool MUST exit non-zero with a clear error.

**`.loreskillsignore`**

- **FR-019**: The tool MUST read `.loreskillsignore` from the project root if it exists.
- **FR-020**: Files matching patterns in `.loreskillsignore` MUST NOT be written to disk and MUST NOT appear in `managed_files`.
- **FR-021**: `.loreskillsignore` MUST support standard gitignore pattern syntax including wildcards (`*`, `**`, `?`) and comments (`#`).

**File Ownership**

- **FR-022**: Before installing a file, the tool MUST check whether the target path is already owned by a different resource in `lore-state.json`.
- **FR-023**: A conflict MUST cause the install to halt with an error unless `--force` is passed.
- **FR-024**: With `--force`, the file is overwritten and ownership transferred to the new resource in `managed_files`.

### Key Entities

- **ResourceStatus**: The computed health of an installed resource — `up-to-date`, `outdated`, `modified-locally`, `unknown-baseline`.
- **DryRunChange**: A previewed change — `action` (`add`/`modify`/`delete`), `path`, optional hashes.
- **OwnershipConflict**: A detected collision where two resources claim the same file path.

---

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: `loreskills status` completes in under 3 seconds for a project with 20 installed resources across 2 providers.
- **SC-002**: `loreskills remove` leaves zero orphaned files on disk after a successful removal — verified by checking that none of the resource's `managed_files` paths exist.
- **SC-003**: Dry-run output matches actual install output 100% of the time — every file listed as `[+]` by dry-run is subsequently created by a live install.
- **SC-004**: A pinned resource is never updated by `loreskills update` — zero false updates across 100 test runs with a pinned resource present.
- **SC-005**: Files matching `.loreskillsignore` patterns are never written to disk — 100% exclusion rate.

---

## Assumptions

- The v2 state schema from spec 005 is in place before this spec is implemented.
- "Outdated" is determined by comparing `resolved_version` (SHA in state) against the latest available commit on the provider's `catalog_ref` branch.
- `loreskills remove` operates on a single resource at a time; bulk removal is out of scope.
- `.loreskillsignore` patterns are applied to the relative path of each file within the bundle, not the final install path.
- The `--yes` flag (skip confirmation) is a global flag available on all interactive commands, not specific to `remove`.
- Version pinning by tag vs commit SHA is treated identically by the tool; the provider must be able to resolve both.
