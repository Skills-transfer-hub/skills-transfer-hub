---

description: "Task list for feature 005 — State & Config Foundation"
---

# Tasks: State & Config Foundation

**Input**: Design documents from `specs/005-state-config-foundation/`
**Prerequisites**: spec.md, plan.md, data-model.md, research.md, quickstart.md, contracts/

**Go version**: 1.22 (stdlib only — `encoding/json`, `crypto/sha256`, `os`, `path/filepath`)  
**Test framework**: stdlib `testing` only — no external test libraries

---

## Phase 1: Setup

**Purpose**: Confirm a clean baseline before any schema work begins.

- [x] T001 Run `go test ./...` and confirm all existing tests pass (baseline — `cmd/`, `internal/fs/`, `internal/http/`)
- [x] T002 [P] Confirm `crypto/sha256` is already imported in `internal/fs/fs.go` (line 8) — no new import needed

---

## Phase 2: Foundation (Blocking Prerequisites)

**Purpose**: Define v2 types and low-level helpers that every user story depends on.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [x] T003 [P] Create `internal/fs/schema.go` — define `ProjectConfigV2` struct (`schema_version int`, `providers []ProviderEntry`, `resources []ResourceEntry`), `ProviderEntry` struct (`id`, `provider`, `repository_id`, `catalog_ref`, `catalog_path`, `target_dir` — all `string`), `ResourceEntry` struct (`resource_name`, `provider_id`, `pinned_version` — all `string`), `InstalledResourceStateV2` struct (`schema_version int`, `entries []InstalledResourceRecordV2`), `InstalledResourceRecordV2` struct (`resource_name`, `provider_id`, `resolved_version`, `previous_version`, `installed_at` — all `string`, `managed_files []ManagedFile`), `ManagedFile` struct (`path string`, `sha256 string`) — all with JSON tags matching data-model.md
- [x] T004 [P] Create `internal/fs/schema_test.go` — write round-trip marshal/unmarshal tests for all six v2 types: marshal to JSON then unmarshal back and compare; test that `pinned_version` survives round-trip as empty string when absent; test that `managed_files` survives as `[]` when empty (not `null`); ensure these tests FAIL before T003 is implemented
- [x] T005 Add `ComputeSHA256(data []byte) string` in `internal/fs/fs.go` — uses `crypto/sha256` already imported; returns lowercase 64-char hex string via `encoding/hex`; no error return (pure computation)
- [x] T006 [P] Create `internal/fs/migrate.go` — add `backupFile(path string) error` that copies the file at `path` to `path + ".bak"`, overwriting any existing `.bak`; use `os.ReadFile` + `os.WriteFile`; keep same file permissions (`0o644`)
- [x] T007 [P] In `internal/fs/migrate.go` — add `validateProviderID(id string) error` that returns an error if `id` is empty or does not match `^[a-zA-Z0-9_-]+$` (use `regexp.MustCompile` at package level); used by `cmd/init.go` to reject invalid provider IDs at input time

**Checkpoint**: v2 types defined and helpers in place — user story phases can now begin.

---

## Phase 3: User Story 1 — Migrate Existing Project Without Losing Data (Priority: P1) 🎯 MVP

**Goal**: Auto-detect v1 `project.json` / `sth-state.json` on any command run, rewrite to v2 in-place with a `.bak` safety copy, leave originals untouched on failure.

**Independent Test**: Create a v1 `project.json` + `sth-state.json`, call `ReadProjectConfigV2()` + `ReadInstalledStateV2()`, verify files are rewritten to v2 and original data is preserved.

### Tests for User Story 1 (write first — ensure FAIL before implementation)

- [x] T008 [P] Create `internal/fs/migrate_test.go` — write test `TestMigrateProjectConfig_V1ToV2`: given a v1 `project.json` (flat fields: `provider`, `repository_id`, `ref`, `catalog_path`, `target_dir`), call `MigrateProjectConfig(path)`, assert `schema_version == 2`, `providers[0].id == "default"`, `providers[0].catalog_ref == original ref value`, `resources == []`, original field `ref` absent from output
- [x] T009 [P] In `internal/fs/migrate_test.go` — write test `TestMigrateInstalledState_V1ToV2`: given a v1 `sth-state.json` with entries having `catalog_id`, `resource_type`, `name`, `folder_name`, `installed_path`, `source_path`, `status`, call `MigrateInstalledState(path)`, assert `schema_version == 2`, each entry maps `catalog_id` → `resource_name`, `provider_id == "default"`, `managed_files == []`, legacy fields absent
- [x] T010 [P] In `internal/fs/migrate_test.go` — write test `TestMigrateProjectConfig_CreatesBackup`: verify `project.json.bak` is created with original content before the file is rewritten; use `os.ReadFile` on `.bak` and compare to original bytes
- [x] T011 [P] In `internal/fs/migrate_test.go` — write test `TestMigrateProjectConfig_WriteFailure_LeavesOriginalUntouched`: simulate a write failure (write to a read-only path or use a path in a non-existent directory) and assert original file content is unchanged after the error
- [x] T012 [P] In `internal/fs/migrate_test.go` — write test `TestMigrateProjectConfig_AlreadyV2_ReturnsAsIs`: given a `project.json` with `schema_version == 2`, call `MigrateProjectConfig(path)`, assert it returns the parsed v2 struct without rewriting the file (file mod time unchanged)
- [x] T013 [P] In `internal/fs/migrate_test.go` — write test `TestMigrateProjectConfig_CorruptJSON_ReturnsError`: given a `project.json` with invalid JSON content, call `MigrateProjectConfig(path)`, assert a non-nil error is returned and no panic occurs

### Implementation for User Story 1

- [x] T014 In `internal/fs/migrate.go` — implement `MigrateProjectConfig(path string) (ProjectConfigV2, error)`: read file bytes; unmarshal into a `map[string]any` to detect `schema_version`; if absent or `!= 2`, call `backupFile(path)`, map v1 fields to `ProjectConfigV2` (`ref` → `catalog_ref`, flat fields → single `ProviderEntry{id: "default"}`), marshal with `json.MarshalIndent`, write back; if write fails return error (original was backed up but not overwritten); if already v2, unmarshal directly into `ProjectConfigV2` and return
- [x] T015 In `internal/fs/migrate.go` — implement `MigrateInstalledState(path string) (InstalledResourceStateV2, error)`: same pattern as T014; detect `schema_version`; for v1 entries map `catalog_id` → `resource_name`, set `provider_id = "default"`, set `resolved_version`, `previous_version`, `installed_at` to `""`, set `managed_files = []ManagedFile{}`; drop legacy fields; write back v2; return error on write failure without modifying original
- [x] T016 In `internal/fs/fs.go` — implement `ReadProjectConfigV2() (ProjectConfigV2, error)`: locate `.sth/project.json` using existing `projectConfigDir()` logic; if not found return empty `ProjectConfigV2{}` with no error; read bytes; call `MigrateProjectConfig(path)` which handles both v1 and v2; return result
- [x] T017 In `internal/fs/fs.go` — implement `ReadInstalledStateV2(targetDir string) (InstalledResourceStateV2, error)`: validate `targetDir`; locate `sth-state.json`; if not found return empty `InstalledResourceStateV2{SchemaVersion: 2, Entries: []InstalledResourceRecordV2{}}` with no error; call `MigrateInstalledState(path)` and return result; return wrapped error on corrupt JSON (do not auto-migrate corrupt files per spec edge case)
- [x] T018 In `internal/fs/fs.go` — implement `WriteProjectConfigV2(cfg ProjectConfigV2) (string, error)`: ensure `cfg.SchemaVersion = 2`; marshal with `json.MarshalIndent`; use existing `projectConfigDir()` to get path; call `os.MkdirAll` on dir; write with `os.WriteFile(..., 0o644)`; return absolute path and nil on success
- [x] T019 In `internal/fs/fs.go` — implement `WriteInstalledStateV2(targetDir string, state InstalledResourceStateV2) error`: validate `targetDir`; ensure `state.SchemaVersion = 2`; marshal with `json.MarshalIndent`; write to `filepath.Join(targetDir, stateFileName)`; return wrapped error on failure

**Checkpoint**: US1 complete — `MigrateProjectConfig`, `MigrateInstalledState`, `ReadProjectConfigV2`, `ReadInstalledStateV2`, `WriteProjectConfigV2`, `WriteInstalledStateV2` all implemented and tested.

---

## Phase 4: User Story 2 — Configure Multiple Providers in a New Project (Priority: P1)

**Goal**: `sth init` appends a new `ProviderEntry` to `providers` array in a v2 `project.json`; rejects duplicate IDs.

**Independent Test**: Run init twice (different provider IDs), verify `project.json` has two entries in `providers`; run init with duplicate ID, verify error with message.

### Tests for User Story 2 (write first — ensure FAIL before implementation)

- [x] T020 [P] In `cmd/init_test.go` (extend existing file) — write test `TestRunInit_EmptyProject_CreatesV2WithOneProvider`: simulate fresh project (no `project.json`), run `runInit()` with stub prompts, assert `project.json` written with `schema_version == 2`, `providers` array has exactly one entry with the provided `id`, `provider`, `repository_id`, `catalog_ref`, `catalog_path`, `target_dir`
- [x] T021 [P] In `cmd/init_test.go` — write test `TestRunInit_ExistingV2_AppendsSecondProvider`: start with a v2 `project.json` having one provider, run `runInit()` with stub for a second provider ID, assert `providers` array has two entries and first entry is unchanged
- [x] T022 [P] In `cmd/init_test.go` — write test `TestRunInit_DuplicateProviderID_ReturnsError`: start with a v2 `project.json` having provider `id = "default"`, run `runInit()` with stub providing `id = "default"` again, assert error contains message indicating the duplicate provider ID
- [x] T023 [P] In `cmd/init_test.go` — write test `TestRunInit_V2WrittenCorrectly_ProvidersArray`: after a successful init, call `ReadProjectConfigV2()`, assert `providers` has correct content (all fields match prompts) and `resources` is an empty array (not null)

### Implementation for User Story 2

- [x] T024 In `cmd/init.go` — replace `fs.ReadProjectProfileIfPresent()` call with `fs.ReadProjectConfigV2()`; store result as `existingCfg ProjectConfigV2`; if `len(existingCfg.Providers) > 0`, print existing providers list and ask "Add a new provider?" (yes/no); if no, print "Configuration conservée" and return nil; if yes, continue to collect new provider details
- [x] T025 In `cmd/init.go` — add prompt for provider `id` (new step before provider type): call `promptValue(reader, stdout, "Provider ID (unique short name)", "default")`; call `fs.ValidateProviderID(id)` (export `validateProviderID` from `migrate.go` by capitalising); on error, reprint validation message and re-prompt; check `id` is not already in `existingCfg.Providers` and return error if duplicate
- [x] T026 In `cmd/init.go` — after collecting all provider fields (provider, repositoryID, refValue, catalogPath, targetDir), construct `fs.ProviderEntry{ID: id, Provider: provider, RepositoryID: repositoryID, CatalogRef: refValue, CatalogPath: catalogPath, TargetDir: targetDir}`, call `fs.CheckProjectAccess` with the new provider's data, append entry to `existingCfg.Providers`, set `existingCfg.SchemaVersion = 2`, call `fs.WriteProjectConfigV2(existingCfg)` and print success message including provider ID
- [x] T027 In `cmd/init.go` — update success output lines to reflect v2 terminology: replace `ref:` with `catalog_ref:`, include provider `id` in output; remove references to old `profile.Ref` field; ensure backward-compatible output for single-provider projects

**Checkpoint**: US2 complete — init appends providers to v2 config and rejects duplicates.

---

## Phase 5: User Story 3 — Pin a Resource to a Specific Version (Priority: P1)

**Goal**: `pinned_version` field is fully supported in v2 schema and survives all read/write cycles; `UpsertResourceEntry` allows install (spec 006) to declare resources.

**Independent Test**: Write `project.json` with `pinned_version: "abc123"`, call `ReadProjectConfigV2()`, assert `resources[0].PinnedVersion == "abc123"`.

### Tests for User Story 3 (write first — ensure FAIL before implementation)

- [x] T028 [P] In `internal/fs/schema_test.go` (extend T004) — write test `TestResourceEntry_PinnedVersionRoundTrip`: marshal a `ResourceEntry` with `pinned_version = "abc123"` to JSON, unmarshal back, assert `PinnedVersion == "abc123"`
- [x] T029 [P] In `internal/fs/schema_test.go` — write test `TestResourceEntry_AbsentPinnedVersion_TreatedAsUnpinned`: unmarshal a JSON resource entry with no `pinned_version` key, assert `PinnedVersion == ""`
- [x] T030 [P] In `internal/fs/fs_test.go` (extend existing file) — write test `TestUpsertResourceEntry_AddNew`: call `UpsertResourceEntry` on an empty `ProjectConfigV2`, pass a `ResourceEntry`, assert it is appended to `resources`
- [x] T031 [P] In `internal/fs/fs_test.go` — write test `TestUpsertResourceEntry_UpdateExisting`: call `UpsertResourceEntry` twice with same `resource_name` but different `pinned_version`, assert `resources` has exactly one entry with the updated `pinned_version`

### Implementation for User Story 3

- [x] T032 Verify `ResourceEntry.PinnedVersion` field (JSON tag `pinned_version,omitempty`) is defined in `internal/fs/schema.go` as part of T003 — if `omitempty` was used, change to always-present empty string by removing `omitempty` so the field is always serialised (matches spec: absence and `""` both mean unpinned, but field should be explicit in JSON output for clarity; use `omitempty` only for `pinned_version` to keep JSON clean when unpinned — confirm with data-model.md)
- [x] T033 In `internal/fs/fs.go` — implement `UpsertResourceEntry(cfg *ProjectConfigV2, entry ResourceEntry) error`: validate `entry.ResourceName` is non-empty and `entry.ProviderID` references an existing provider in `cfg.Providers` (return error if not found); search `cfg.Resources` for existing entry with same `resource_name`; replace if found, append if not; sort `cfg.Resources` by `resource_name` for deterministic output

**Checkpoint**: US3 complete — `pinned_version` round-trips correctly and `UpsertResourceEntry` is available for spec 006.

---

## Phase 6: User Story 4 — View Complete Managed-File Inventory (Priority: P1)

**Goal**: Every `sth install` and `sth update` writes a `managed_files` array with per-file `path` and `sha256` to `sth-state.json`. `previous_version` is set before overwriting `resolved_version` on update.

**Independent Test**: Install a resource, open `sth-state.json`, verify `managed_files` has one entry with matching `sha256` (read the installed file, hash it, compare).

### Tests for User Story 4 (write first — ensure FAIL before implementation)

- [x] T034 [P] In `internal/fs/fs_test.go` — write test `TestInstallCatalogEntryV2_WritesManagedFiles`: call `InstallCatalogEntryV2(targetDir, entry, content)`, call `ReadInstalledStateV2(targetDir)`, assert `entries[0].managed_files` has one entry with correct relative path and `sha256` matching `ComputeSHA256(fileContent)` where `fileContent` is the bytes actually written (after `skillHeader`)
- [x] T035 [P] In `internal/fs/fs_test.go` — write test `TestInstallCatalogEntryV2_EmptyContent_ManagedFilesIsEmptyArray`: install with zero-length content, assert `managed_files == []` (not null) in state
- [x] T036 [P] In `cmd/update_test.go` (extend existing file) — write test `TestRunUpdate_SetsPreviousVersion`: set up state with `resolved_version = "old-sha"`, simulate update with new content, assert resulting state has `previous_version == "old-sha"` and `resolved_version == "new-sha"`
- [x] T037 [P] In `internal/fs/fs_test.go` — write test `TestInstallCatalogEntryV2_SHA256MatchesActualFileContent`: after install, read the file from disk, compute `ComputeSHA256` over its bytes, assert it equals the `sha256` stored in `managed_files[0].sha256`

### Implementation for User Story 4

- [x] T038 In `internal/fs/fs.go` — update `SaveCatalogEntry()` to also return the `ManagedFile` it produced: change signature to `SaveCatalogEntry(targetDir string, entry CatalogEntry, content []byte) (InstalledResourceRecord, ManagedFile, error)`; after writing, compute `fileContent := addManagedHeaderIfMarkdown(installedPath, content)`, call `ComputeSHA256(fileContent)`, build `ManagedFile{Path: filepath.ToSlash(relPath), SHA256: hash}` where `relPath` is relative to project root; keep existing `InstalledResourceRecord` return for backward compat
- [x] T039 In `internal/fs/fs.go` — add `InstallCatalogEntryV2(targetDir string, entry CatalogEntry, content []byte) (InstalledResourceRecordV2, error)`: call updated `SaveCatalogEntry` to get `(record, managedFile, err)`; build `InstalledResourceRecordV2{ResourceName: entry.CatalogID, ProviderID: "default", ResolvedVersion: "", PreviousVersion: "", InstalledAt: time.Now().UTC().Format(time.RFC3339), ManagedFiles: []ManagedFile{managedFile}}`; call `UpsertInstalledResourceV2(targetDir, v2record)` and return
- [x] T040 In `internal/fs/fs.go` — add `UpsertInstalledResourceV2(targetDir string, record InstalledResourceRecordV2) error`: call `ReadInstalledStateV2(targetDir)`; search entries by `resource_name`; replace or append; sort by `resource_name`; call `WriteInstalledStateV2(targetDir, state)`
- [x] T041 In `internal/fs/fs.go` — add `SaveInstalledResourceV2(record InstalledResourceRecordV2, content []byte) (InstalledResourceRecordV2, error)`: write content to disk (using `record.ManagedFiles[0].Path` to locate the file, cleaned via `filepath.Clean(filepath.FromSlash(...))`); recompute SHA256 of bytes actually written; update `record.ManagedFiles[0].SHA256` to new hash; return updated record
- [x] T042 In `cmd/install.go` — add call to `fs.InstallCatalogEntryV2()` after existing `fs.InstallCatalogEntry()` succeeds (or replace it — confirm install.go still compiles and tests pass); update success message to reference `record.ResourceName` instead of `entry.Name` when using v2 path
- [x] T043 In `cmd/update.go` — in the per-record loop, before calling `fs.SaveInstalledResource(record, content)`, call `fs.ReadInstalledStateV2(profile.TargetDir)` to find the existing v2 record; set `v2record.PreviousVersion = v2record.ResolvedVersion`; after save, set `v2record.ResolvedVersion` to the commit SHA returned from download (or leave as empty string if unavailable); call `fs.UpsertInstalledResourceV2()` with the updated record

**Checkpoint**: US4 complete — every install writes `managed_files` with SHA256; every update preserves `previous_version`.

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Validate correctness across all stories; fix any issues surfaced by tools.

- [x] T044 Run `go vet ./...` from repo root and fix any reported issues (vet covers all packages: `sth`, `sth/cmd`, `sth/internal/fs`, `sth/internal/http`)
- [x] T045 Run `go test ./...` from repo root and confirm all tests pass — including existing tests in `cmd/` that must not be broken by v2 changes (SC-004, SC-005)
- [x] T046 [P] Verify backward compatibility: existing commands `sth list` and `sth install` still function correctly when `project.json` is v1 — confirm migration is triggered automatically on read and commands complete successfully
- [x] T047 [P] Update `README.md` if it references the `ref` field directly — replace any mention of `ref` with `catalog_ref` to match FR-013 terminology; check for references to the old single-provider flat config format and update examples to show v2 `providers` array

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately
- **Foundation (Phase 2)**: Depends on Setup — **BLOCKS all user stories**
- **US1 (Phase 3)**: Depends on Foundation (T003–T007)
- **US2 (Phase 4)**: Depends on Foundation + US1 (needs `ReadProjectConfigV2`, `WriteProjectConfigV2`)
- **US3 (Phase 5)**: Depends on Foundation (T003 defines `ResourceEntry.PinnedVersion`)
- **US4 (Phase 6)**: Depends on Foundation + US1 (needs `ReadInstalledStateV2`, `WriteInstalledStateV2`, `ComputeSHA256`)
- **Polish (Phase 7)**: Depends on all story phases complete

### Within Each Phase

- Test tasks (T008–T013, T020–T023, T028–T031, T034–T037) are written **first** and must **fail** before implementation begins (TDD)
- T003 (schema.go) and T004 (schema_test.go) are independent and can be written in parallel
- T006 (`backupFile`) and T007 (`validateProviderID`) are independent in `migrate.go` — parallelizable
- US3 (Phase 5) and US4 (Phase 6) can start in parallel once Foundation and US1 are complete
- US2 (Phase 4) can start in parallel with US3 and US4 once Foundation and US1 are complete

### Parallel Opportunities

Tasks marked `[P]` touch different files or independent sections — safe to execute concurrently within the same phase.

---

## Implementation Strategy

### MVP First

1. Complete Phase 1 (Setup) + Phase 2 (Foundation)
2. Complete Phase 3 (US1 — Migration) — **required before any other story**
3. Complete Phase 4 (US2), Phase 5 (US3), Phase 6 (US4) in parallel
4. Complete Phase 7 (Polish)

### Key Files Modified

| File | Change |
|------|--------|
| `internal/fs/schema.go` | **New** — v2 type definitions |
| `internal/fs/migrate.go` | **New** — `MigrateProjectConfig`, `MigrateInstalledState`, `backupFile`, `validateProviderID` |
| `internal/fs/schema_test.go` | **New** — round-trip tests for all v2 types |
| `internal/fs/migrate_test.go` | **New** — migration edge-case tests |
| `internal/fs/fs.go` | **Extend** — `ComputeSHA256`, `ReadProjectConfigV2`, `ReadInstalledStateV2`, `WriteProjectConfigV2`, `WriteInstalledStateV2`, `UpsertResourceEntry`, `InstallCatalogEntryV2`, `UpsertInstalledResourceV2`, `SaveInstalledResourceV2` |
| `internal/fs/fs_test.go` | **Extend** — US3 + US4 test scenarios |
| `cmd/init.go` | **Extend** — multi-provider append flow, provider ID prompt |
| `cmd/init_test.go` | **Extend** — US2 test scenarios |
| `cmd/install.go` | **Extend** — use `InstallCatalogEntryV2` |
| `cmd/update.go` | **Extend** — set `previous_version` before overwrite |
| `cmd/update_test.go` | **Extend** — US4 `previous_version` test |
| `README.md` | **Update** — terminology: `ref` → `catalog_ref` |

---

## Notes

- `[P]` = different files or fully independent sections — safe to work in parallel
- Test tasks have no `[US*]` label when in Setup/Foundation phases; they carry `[US1]`–`[US4]` only in story phases
- All new functions in `internal/fs/` require `_test.go` coverage (TDD per constitution)
- `ValidateProviderID` (exported) must be the public name used by `cmd/init.go`; `validateProviderID` (unexported) is the internal helper — export by capitalising for cmd layer use
- `time` package import will be needed in `internal/fs/fs.go` for `InstalledAt` timestamp in T039
- Do not remove `InstalledCatalogEntry` or `UpsertInstalledResource` v1 functions until all callers are migrated (spec 006 handles full cutover)
