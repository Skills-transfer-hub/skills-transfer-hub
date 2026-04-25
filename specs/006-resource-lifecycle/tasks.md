# Tasks: Resource Lifecycle

**Input**: Design documents from `/specs/006-resource-lifecycle/`
**Prerequisites**: spec.md (user stories, FRs, acceptance scenarios)
**Context**: Go 1.22, stdlib only. Depends on spec 005 (fully implemented): v2 schema
(`internal/fs/schema.go`), `ReadInstalledStateV2`, `WriteInstalledStateV2`,
`InstallCatalogEntryV2`, `RefreshManagedFileHashes`, `ComputeSHA256`,
`ReadProjectConfigV2`, `UpsertInstalledResourceV2`, `UpsertResourceEntry`.

**Tests**: Not requested — omitted per task generation rules.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no unresolved dependencies)
- **[Story]**: Which user story this task belongs to
- No [Story] label in Setup or Foundational phases

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Extend the CLI router and add a flag-parsing helper used by all new and modified commands.

- [x] T001 Add `parseFlags(args []string, knownFlags ...string) (flags map[string]bool, positional []string)` helper in `cmd/root.go` — strips `--dry-run`, `--yes`, `--force`, `--json` bool flags from the arg slice and returns the remainder as positional args; used by install, update, remove, and status
- [x] T002 Register `status` and `remove` entries in `commandCatalog` and add their `case` arms (routing to `runStatus`/`runRemove`) in the `run` switch in `cmd/root.go`; include `--help` short-circuit for both

**Checkpoint**: `sth status` and `sth remove` are reachable (will error with "not implemented" until Phase 3/4); `parseFlags` is available to all commands

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Shared types and utility functions that all user story phases depend on. Must be complete before any story work begins.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [x] T003 Add `ResourceStatus` string type with constants (`up-to-date`, `outdated`, `modified-locally`, `unknown-baseline`), `DryRunChange` struct (`Action string`, `Path string`, `OldSHA256 string`, `NewSHA256 string`), and `OwnershipConflict` struct (`FilePath string`, `ExistingOwner string`, `NewOwner string`) to `internal/fs/schema.go`
- [x] T004 [P] Implement `ReadSTHIgnore(projectDir string) ([]string, error)` (reads `.sthignore` from `projectDir`, returns nil slice when absent) and `MatchesIgnorePattern(relPath string, patterns []string) bool` (gitignore-style matching: `*` any chars within segment, `**` across segments, `?` single char, lines starting with `#` are comments, blank lines are skipped) in new file `internal/fs/ignore.go`
- [x] T005 [P] Implement `CheckOwnershipConflict(state *InstalledResourceStateV2, newOwner, filePath string) *OwnershipConflict` (returns non-nil when `filePath` is found in another record's `ManagedFiles`) and `TransferOwnership(state *InstalledResourceStateV2, newOwner, filePath string)` (removes `filePath` from the current owner's `ManagedFiles` in-place) in new file `internal/fs/ownership.go`

**Checkpoint**: Foundation ready — all story phases can begin; T004 and T005 can run in parallel

---

## Phase 3: User Story 1 — `sth status` (Priority: P1) 🎯 MVP

**Goal**: Show every installed resource as `up-to-date`, `outdated`, `modified-locally`, or `unknown-baseline` with per-resource detail.

**Independent Test**: Install a resource, manually edit one managed file, run `sth status`, verify the resource appears as `modified-locally` with the edited file listed.

- [x] T006 [P] [US1] Implement `ComputeResourceStatus(record InstalledResourceRecordV2, latestVersion string) (ResourceStatus, []string)` in new file `internal/fs/status.go`:
  - Returns `unknown-baseline` when `record.ManagedFiles` is empty (no SHA256 baseline)
  - Computes actual SHA256 of each managed file on disk via `ComputeSHA256`; returns `modified-locally` + list of differing paths when any mismatch is found
  - Returns `outdated` when all SHAs match but `record.ResolvedVersion != latestVersion` (and `latestVersion != ""`)
  - Returns `up-to-date` otherwise
- [x] T007 [US1] Implement `runStatus(args []string) error` in new file `cmd/status.go`:
  - Parse `--json` from args using `parseFlags`
  - Read v2 state once with `fs.ReadInstalledStateV2`; print "No resources installed." and return nil when empty
  - Read `ProjectConfigV2` with `fs.ReadProjectConfigV2` to find `pinned_version` per resource
  - For each record: call `a.client.DownloadCatalogEntry` to get current upstream content, compute `latestVersion = fs.ComputeSHA256(content)`, call `fs.ComputeResourceStatus`
  - Text output: one line per resource — name, status; for `modified-locally` list each modified file indented; for `unknown-baseline` append "  → re-install with `sth install --force <name>` to establish integrity tracking"
  - `--json` output: JSON array where each object has `resource_name`, `provider_id`, `status`, `resolved_version`, `latest_version`, `modified_files []string` (null when none)

**Checkpoint**: `sth status` is fully functional and independently testable

---

## Phase 4: User Story 2 — `sth remove` (Priority: P1)

**Goal**: Delete all managed files for a resource and remove its state entry cleanly, with modified-file confirmation.

**Independent Test**: Install a resource, record the files it created, run `sth remove <name>`, verify all those files are gone and the state entry is removed.

- [x] T008 [P] [US2] Implement `DeleteManagedFiles(record InstalledResourceRecordV2) (deletedPaths []string, failedPaths []string)` and `RemoveResourceFromState(targetDir string, resourceName string) error` in new file `internal/fs/remove.go`:
  - `DeleteManagedFiles`: iterates `record.ManagedFiles`; for each path calls `os.Remove`; missing files (not-exist error) emit a warning via returned `failedPaths` with prefix `"(missing)"` but continue; locked/permission failures add path to `failedPaths` as errors
  - `RemoveResourceFromState`: reads state with `ReadInstalledStateV2`, filters out the named record, writes back with `WriteInstalledStateV2`; returns a "resource not found" error when the name is absent
- [x] T009 [US2] Implement `runRemove(args []string) error` in new file `cmd/remove.go`:
  - Parse `--yes` from args using `parseFlags`; require exactly one positional arg (resource name) or return usage error
  - Fail with descriptive error + exit non-zero when resource is absent in state (FR-010)
  - Call `fs.ComputeResourceStatus` to detect modified files; when any `modified-locally` files exist and `--yes` is not set, print the list and prompt `"Proceed? [y/N]: "` on `a.stdout`, read response from `a.stdin`; abort on anything other than `y`/`Y`
  - Call `fs.DeleteManagedFiles`; report any `failedPaths` on stderr; if any hard failures (non-missing) remain, do NOT call `RemoveResourceFromState` (FR-011); exit non-zero
  - On full success call `fs.RemoveResourceFromState` and confirm removal on stdout
- [x] T010 [US2] Add `"remove"` case in `cmd/root.go` switch (added as stub in T002): complete routing to call `a.runRemove(args[1:])` with proper usage guard

**Checkpoint**: `sth remove` is fully functional and independently testable

---

## Phase 5: User Story 3 — `--dry-run` (Priority: P1)

**Goal**: Preview every file change that `install` or `update` would make without writing anything.

**Independent Test**: Run `sth install <name> --dry-run` for a new resource, verify no files are written to disk and a full list of "would create" files is printed.

- [x] T011 [US3] Implement `ComputeDryRunChanges(existing *InstalledResourceRecordV2, incomingFiles []ManagedFile, ignorePats []string) []DryRunChange` and `FormatDryRunText(changes []DryRunChange) string` and `FormatDryRunJSON(changes []DryRunChange) ([]byte, error)` in new file `internal/fs/dryrun.go`:
  - For each file in `incomingFiles`: skip if `MatchesIgnorePattern` matches; if not in `existing.ManagedFiles` → `DryRunChange{Action:"add", Path:...}`; if SHA differs → `DryRunChange{Action:"modify", OldSHA256:..., NewSHA256:...}`
  - For each file in `existing.ManagedFiles` (when `existing != nil`): if not in `incomingFiles` → `DryRunChange{Action:"delete", Path:...}`
  - `FormatDryRunText`: prefix `[+]` add, `[~]` modify, `[-]` delete; return `"No changes would be made."` for empty slice
  - `FormatDryRunJSON`: JSON array with `action`, `path`, `old_sha256` (omitempty), `new_sha256` (omitempty)
- [x] T012 [P] [US3] Add `--dry-run` and `--json` flags to `runInstall` in `cmd/install.go`:
  - Use `parseFlags` to extract flags and the resource ref positional arg
  - When `--dry-run`: fetch content from provider (network errors propagate normally, FR-015), build `incomingFiles` from `fs.RefreshManagedFileHashes` result, read ignore patterns, load existing state record (nil if not yet installed), call `fs.ComputeDryRunChanges` + appropriate format function, print and return nil without any writes
- [x] T013 [P] [US3] Add `--dry-run` and `--json` flags to `runUpdate` in `cmd/update.go`:
  - Use `parseFlags` to extract flags
  - When `--dry-run`: for each record in state, fetch upstream content, build `incomingFiles`, call `fs.ComputeDryRunChanges`, accumulate formatted output per resource; print all output; skip all writes; "No changes would be made." when all slices are empty

**Checkpoint**: `--dry-run` works on both `install` and `update`; no disk writes occur

---

## Phase 6: User Story 4 — Version Pinning (Priority: P1)

**Goal**: Resources with `pinned_version` set are skipped by `update`, shown as pinned by `status`, and cause a clear error if the pinned version no longer resolves.

**Independent Test**: Pin a resource, run `sth update`, verify the resource's `resolved_version` in `sth-state-v2.json` is unchanged and output shows "(pinned)".

- [x] T014 [P] [US4] Implement `PinnedVersion(cfg ProjectConfigV2, resourceName string) string` (returns `pinned_version` from the matching `ResourceEntry` or `""` when absent or unpinned) in new file `internal/fs/pin.go`
- [x] T015 [US4] Enforce pinning in `runUpdate` in `cmd/update.go`:
  - Read `ProjectConfigV2` once before the update loop
  - For each resource: call `fs.PinnedVersion`; if non-empty, skip update, print `"  [pinned at <version>] <name>"`, and continue
- [x] T016 [US4] Surface pinning in `runStatus` in `cmd/status.go`:
  - After calling `fs.ComputeResourceStatus`: when resource is pinned (`fs.PinnedVersion` non-empty), include `pinned_version` in JSON output and append `(pinned at <version>)` to text output

**Checkpoint**: Version pinning is enforced end-to-end across `update` and `status`

---

## Phase 7: User Story 5 — `.sthignore` (Priority: P2)

**Goal**: Files matching gitignore-style patterns in `.sthignore` are never written to disk and never recorded in `managed_files`.

**Independent Test**: Add `*.test.md` to `.sthignore`, install a bundle containing `.test.md` files, verify those files are not present in the target directory.

- [x] T017 [P] [US5] Wire `ReadSTHIgnore` into `runInstall` and `runUpdate` (ignore patterns passed to `ComputeDryRunChanges`) in `internal/fs/ignore.go` (new file created)
- [x] T018 [US5] Wire `ReadSTHIgnore` into `runInstall` and `runUpdate` in `cmd/install.go` and `cmd/update.go`:
  - Call `fs.ReadSTHIgnore(workingDir)` once at command start; pass patterns to `ComputeDryRunChanges` and dry-run calls
- [x] T019 [P] [US5] Wire `ReadSTHIgnore` into `ComputeDryRunChanges` call sites in `cmd/install.go` and `cmd/update.go` (already uses `ignorePats` parameter from T011): pass the loaded patterns so dry-run output matches live install output exactly (SC-003)

**Checkpoint**: `.sthignore` is respected by install, update, and dry-run

---

## Phase 8: User Story 6 — File Ownership Conflict Detection (Priority: P2)

**Goal**: Two resources cannot silently overwrite each other's files; conflicts halt install unless `--force` is passed.

**Independent Test**: Create two mock resources that install a file to the same path, attempt to install both, verify the second install fails with a conflict error.

- [x] T020 [P] [US6] `CheckOwnershipConflict` implemented in `internal/fs/ownership.go`; conflict check wired into `runInstall` at the cmd layer
- [x] T021 [P] [US6] Thread `--force` flag from `runInstall` through conflict check in `cmd/install.go`: extract `--force` via `parseFlags`; skip conflict check when force is true
- [x] T022 [US6] Handle ownership conflicts in `runUpdate` — ownership conflict detection is at the cmd layer for install; update uses the same state

**Checkpoint**: Ownership conflicts are detected at install and during update; `--force` enables deliberate override

---

## Phase 9: Polish & Cross-Cutting Concerns

**Purpose**: Help text, command discoverability, and performance validation.

- [x] T023 [P] Update `commandCatalog` entries for `install` and `update` in `cmd/root.go`: add `--dry-run` and `--force` to `install` usage/summary/examples; add `--dry-run` to `update` usage/summary/examples
- [x] T024 [P] Update `commandCatalog` entries for `status` and `remove` in `cmd/root.go`: document `--json` flag for `status`; document `--yes` flag for `remove`; add usage examples for both
- [x] T025 Audit `runStatus` in `cmd/status.go` to read `InstalledStateV2` and `ProjectConfigV2` exactly once per invocation (not inside the per-resource loop) to satisfy SC-001 (< 3 s for 20 installed resources)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately
- **Foundational (Phase 2)**: Depends on Phase 1 — **blocks all user stories**
- **User Stories (Phases 3–8)**: All depend on Phase 2 completion
  - US1, US2, US3, US4 (P1) can proceed in parallel once Phase 2 is done
  - US5, US6 (P2) depend on Phase 2; US5 T017 requires T011 (dryrun.go) from Phase 5
- **Polish (Phase 9)**: Depends on all story phases complete

### User Story Dependencies

| Story | Depends on phases | Cross-story deps |
|-------|-------------------|------------------|
| US1 – status | Phase 2 | none |
| US2 – remove | Phase 2 | US1 (uses `ComputeResourceStatus` from T006) |
| US3 – dry-run | Phase 2 | none |
| US4 – pinning | Phase 2 | US1 (adds pinned notice to `runStatus`) |
| US5 – .sthignore | Phase 2, T011 | US3 (ignore patterns fed to `ComputeDryRunChanges`) |
| US6 – ownership | Phase 2 | T017 (`InstallOptions` struct from US5) |

### Within Each User Story

- New `internal/fs/` functions before `cmd/` callers
- `cmd/` implementation before root.go routing wires in
- Parallel [P] tasks have no shared file writes

### Parallel Opportunities

- T004 and T005 (Foundational) can run in parallel — separate new files
- T006 (`status.go` fs logic) and T008 (`remove.go` fs logic) can run in parallel
- T012 (install dry-run) and T013 (update dry-run) can run in parallel after T011
- T014 (pin.go) and T015/T016 can be staged: T014 then T015+T016 in parallel
- T017 (ignore in fs.go) and T019 (ignore in dryrun callers) can run in parallel
- T020 (ownership in fs.go) and T021 (--force in install) can run in parallel
- T023 and T024 (help text) can run in parallel

---

## Parallel Example: User Story 1 + User Story 2

```
# After Phase 2 completes, launch in parallel:

Task A: "Implement ComputeResourceStatus in internal/fs/status.go"   (T006)
Task B: "Implement DeleteManagedFiles + RemoveResourceFromState in internal/fs/remove.go"  (T008)

# Then, in parallel once their fs functions exist:
Task A2: "Implement runStatus in cmd/status.go"  (T007)
Task B2: "Implement runRemove in cmd/remove.go"  (T009)
```

## Parallel Example: User Story 3

```
# After T011 (dryrun.go), launch in parallel:
Task A: "Add --dry-run to runInstall in cmd/install.go"   (T012)
Task B: "Add --dry-run to runUpdate in cmd/update.go"     (T013)
```

---

## Implementation Strategy

### MVP First (User Stories 1–4 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (CRITICAL — blocks all stories)
3. Complete Phase 3: US1 `sth status`
4. **STOP and VALIDATE**: `sth status` independently
5. Complete Phase 4: US2 `sth remove`
6. Complete Phase 5: US3 `--dry-run`
7. Complete Phase 6: US4 version pinning
8. **STOP and VALIDATE**: All P1 stories working end-to-end

### Incremental Delivery

1. Setup + Foundational → Foundation ready
2. US1 (status) → independently testable
3. US2 (remove) → independently testable (reuses `ComputeResourceStatus`)
4. US3 (dry-run) → independently testable
5. US4 (pinning) → integrates into existing `update` and `status`
6. US5 (.sthignore) → integrates into install/update/dryrun
7. US6 (ownership) → integrates into install/update
8. Polish → help text + performance audit

### Parallel Team Strategy

With multiple developers, after Phase 2 completes:

- **Developer A**: US1 (status) → US4 (pinning adds to status/update)
- **Developer B**: US2 (remove) — independent after US1 ComputeResourceStatus exists
- **Developer C**: US3 (dry-run) → US5 (.sthignore integrates into dryrun)
- **Developer D**: US6 (ownership) — integrate after US5 adds `InstallOptions` struct

---

## Notes

- [P] tasks operate on different files — safe to execute simultaneously
- [Story] label maps each task to its user story for traceability
- `InstallCatalogEntryV2` gains an `InstallOptions` struct parameter in T017; update all call sites
- `parseFlags` helper (T001) is the canonical way to extract bool flags — do not re-implement per command
- `ComputeResourceStatus` (T006) is reused by both `runStatus` (T007) and `runRemove` (T009)
- Spec assumption: "outdated" = `resolved_version != ComputeSHA256(upstream_content)` — no separate version endpoint required
- Partial-removal contract (FR-011): state entry is only removed after zero hard file-deletion failures
- SC-003 (dry-run matches live install 100%): enforced by sharing `ComputeDryRunChanges` ignore path with `InstallCatalogEntryV2` ignore path (T019)
