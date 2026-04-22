# Tasks: Private Repo Skills

**Input**: Design documents from `/specs/003-private-repo-skills/`  
**Prerequisites**: plan.md (required), spec.md (required), research.md, data-model.md, contracts/, quickstart.md

**Tests**: Tests are required by the constitution for every new command, helper, and filesystem/network flow. Add or update package-local Go tests before implementing the corresponding behavior.

**Organization**: Tasks are grouped by user story so each story can be implemented and validated independently.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no unfinished dependencies)
- **[Story]**: Which user story this belongs to (`[US1]`, `[US2]`, `[US3]`, `[US4]`)
- Every task includes exact file paths

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Prepare the repository for the new CLI surfaces and richer test coverage.

- [x] T001 Create command and test scaffolding for remote discovery and hidden artwork in `cmd/list.go`, `cmd/list_test.go`, `cmd/stockholm.go`, and `cmd/stockholm_test.go`
- [x] T002 [P] Extend the shared CLI test harness for interactive stdin, environment overrides, and hidden-command assertions in `cmd/root_test.go`
- [x] T003 [P] Add reusable provider/profile fixture helpers for filesystem and HTTP tests in `internal/fs/fs_test.go` and `internal/http/client_test.go`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Build the shared structures that all user stories depend on.

**⚠️ CRITICAL**: No user story is complete until this phase is finished.

- [x] T004 [P] Introduce shared project-profile and catalog-resource data structures in `internal/fs/fs.go`
- [x] T005 [P] Introduce provider-aware request models, catalog models, and release metadata models in `internal/http/client.go`
- [x] T006 [P] Refactor shared command dispatch to accommodate `list`, `update self`, and hidden commands in `cmd/root.go`
- [x] T007 Implement foundational coverage for shared profile/state helpers and command dispatch in `internal/fs/fs_test.go`, `internal/http/client_test.go`, and `cmd/root_test.go`

**Checkpoint**: Foundation ready. User stories can now proceed independently.

---

## Phase 3: User Story 1 - Connect a Project to a Private Repository (Priority: P1) 🎯 MVP

**Goal**: Let a project configure its own private repository connection through an interactive `init` flow without sharing repository settings across projects.

**Independent Test**: From a project with no local setup, run `loreskills init`, choose a provider, complete setup, and verify that the project has its own `.loreskills/project.json` without relying on another project's configuration.

### Tests for User Story 1 ⚠️

> **NOTE**: Write these tests first and confirm they fail before implementation.

- [x] T008 [P] [US1] Add interactive init coverage for provider selection, legacy config import, and project-profile creation in `cmd/init_test.go`
- [x] T009 [P] [US1] Add project-profile persistence and repository-validation coverage in `internal/fs/fs_test.go` and `internal/http/client_test.go`

### Implementation for User Story 1

- [x] T010 [US1] Implement project-local profile path, read/write helpers, and legacy `~/.loreskillsrc` migration in `internal/fs/fs.go`
- [x] T011 [US1] Implement provider-specific reachability checks and setup-guidance data sources in `internal/http/client.go`
- [x] T012 [US1] Implement the interactive `loreskills init` flow for project-scoped repositories in `cmd/init.go` and `cmd/root.go`
- [x] T013 [US1] Document project-local setup and private-provider connection guidance in `README.md`

**Checkpoint**: A project can connect to its own private repository and validate access without using shared configuration.

---

## Phase 4: User Story 2 - Discover Installable Resources from the Remote Catalog (Priority: P1)

**Goal**: Read `SKILLS.md` remotely, list installable resources with folder names, and install/update resources using catalog identity instead of flat skill names.

**Independent Test**: With a prepared project profile pointing at a valid private repo, run `loreskills list`, then install and update a resource to confirm folder-aware resolution and catalog-based state tracking.

### Tests for User Story 2 ⚠️

> **NOTE**: Write these tests first and confirm they fail before implementation.

- [x] T014 [P] [US2] Add `SKILLS.md` parsing and `loreskills list` coverage in `internal/http/client_test.go` and `cmd/list_test.go`
- [x] T015 [P] [US2] Add folder-aware install/update and rich `lore-state.json` coverage in `cmd/install_test.go`, `cmd/update_test.go`, and `internal/fs/fs_test.go`

### Implementation for User Story 2

- [x] T016 [US2] Implement canonical `SKILLS.md` catalog parsing and catalog entry lookup in `internal/http/client.go`
- [x] T017 [US2] Implement the `loreskills list` command and help registration in `cmd/list.go` and `cmd/root.go`
- [x] T018 [US2] Implement catalog-aware resource installation, folder destination resolution, and rich state persistence in `cmd/install.go` and `internal/fs/fs.go`
- [x] T019 [US2] Implement catalog-aware refresh logic for tracked resources in `cmd/update.go` and `internal/fs/fs.go`
- [x] T020 [US2] Document catalog structure, folder names, and `list`/`install` usage in `README.md` and `specs/003-private-repo-skills/contracts/cli-contract.md`

**Checkpoint**: Users can discover remote resources from `SKILLS.md`, see folder names, and install/update tracked resources independently of other stories.

---

## Phase 5: User Story 3 - Keep the CLI on the Latest Stable Release (Priority: P2)

**Goal**: Add stable-only self-update support through `loreskills update self`, backed by build-injected release metadata and GitHub release filtering.

**Independent Test**: With release metadata injected into a build, run `loreskills update self` against a feed containing stable and pre-release entries and verify that only the latest stable release is selected and applied.

### Tests for User Story 3 ⚠️

> **NOTE**: Write these tests first and confirm they fail before implementation.

- [x] T021 [P] [US3] Add stable-release selection and prerelease-filtering coverage in `internal/http/client_test.go`
- [x] T022 [P] [US3] Add `update self` routing, asset verification, and replacement-recovery coverage in `cmd/update_test.go` and `internal/fs/fs_test.go`

### Implementation for User Story 3

- [x] T023 [US3] Implement build-injected release metadata fields and `update self` dispatch in `cmd/version.go`, `cmd/update.go`, and `cmd/root.go`
- [x] T024 [US3] Implement stable GitHub release lookup, prerelease filtering, and platform asset/checksum selection in `internal/http/client.go`
- [x] T025 [US3] Implement downloaded asset verification and executable replacement helpers in `internal/fs/fs.go`
- [x] T026 [US3] Inject self-update linker flags and platform asset names in `.github/workflows/release.yml` and `.github/workflows/ci.yml`
- [x] T027 [US3] Document stable-only self-update behavior in `README.md` and `specs/003-private-repo-skills/quickstart.md`

**Checkpoint**: The CLI can check only stable releases, ignore pre-releases, and guide the user through self-update success or failure.

---

## Phase 6: User Story 4 - Show a Minimal Version Identity and Hide the Stockholm Artwork Behind an Easter Egg (Priority: P3)

**Goal**: Make `loreskills` and `loreskills version` render the same minimal version signature, remove the old animal guide from standard flows, and add the hidden `stockholm` command.

**Independent Test**: Run `loreskills`, `loreskills version`, `loreskills help`, and `loreskills stockholm` to verify that standard commands are concise, the old animal output is gone, and the hidden command renders its artwork without appearing in help.

### Tests for User Story 4 ⚠️

> **NOTE**: Write these tests first and confirm they fail before implementation.

- [x] T028 [P] [US4] Add minimal root/version signature coverage and removal of animal ASCII output in `cmd/root_test.go` and `cmd/version_test.go`
- [x] T029 [P] [US4] Add hidden `stockholm` coverage and help-exclusion assertions in `cmd/stockholm_test.go` and `cmd/help_test.go`

### Implementation for User Story 4

- [x] T030 [US4] Implement the shared minimal version signature for `loreskills` and `loreskills version` in `cmd/root.go` and `cmd/version.go`
- [x] T031 [US4] Remove animal-based standard rendering from visible command flows in `cmd/init.go`, `cmd/install.go`, `cmd/update.go`, and `cmd/root.go`
- [x] T032 [US4] Implement hidden `stockholm` rendering and dispatch exclusion from standard help in `cmd/stockholm.go` and `cmd/root.go`
- [x] T033 [US4] Update visible help text and product copy for the final command surface in `cmd/root.go` and `README.md`

**Checkpoint**: The public CLI surface is minimal and consistent, while the Stockholm easter egg remains available only by explicit invocation.

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Final consistency, documentation alignment, and full validation.

- [x] T034 [P] Refresh final command examples and manual validation steps in `specs/003-private-repo-skills/quickstart.md` and `specs/003-private-repo-skills/contracts/cli-contract.md`
- [x] T035 [P] Clean up migration notes and legacy/shared-config references for `~/.loreskillsrc`, `.loreskills/project.json`, and `SKILLS.md` in `README.md`
- [x] T036 Run `go test ./...`, `go vet ./...`, and a static build with release metadata from `c:/SOURCE/LoreSkills` against `cmd/root.go`, `cmd/update.go`, `cmd/version.go`, `internal/fs/fs.go`, `internal/http/client.go`, and `.github/workflows/release.yml`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion - BLOCKS all user stories
- **User Stories (Phases 3-6)**: Depend on Foundational completion
- **Polish (Phase 7)**: Depends on all desired user stories being complete

### User Story Dependencies

- **User Story 1 (P1)**: Can start after Foundational - no dependency on other user stories
- **User Story 2 (P1)**: Can start after Foundational - independently testable with a prepared project profile
- **User Story 3 (P2)**: Can start after Foundational - no functional dependency on US1 or US2, but it shares `cmd/update.go` and `cmd/root.go`
- **User Story 4 (P3)**: Can start after Foundational - no functional dependency on earlier stories, but it shares `cmd/root.go` and `cmd/version.go`

### Within Each User Story

- Tests MUST be written and fail before implementation
- Filesystem and HTTP helpers before CLI orchestration
- CLI orchestration before documentation updates
- Story-specific validation before moving to the next priority

### Parallel Opportunities

- `T002` and `T003` can run in parallel after `T001`
- `T004`, `T005`, and `T006` can run in parallel before `T007`
- In **US1**, `T008` and `T009` can run in parallel
- In **US2**, `T014` and `T015` can run in parallel
- In **US3**, `T021` and `T022` can run in parallel
- In **US4**, `T028` and `T029` can run in parallel
- In **Polish**, `T034` and `T035` can run in parallel before final validation `T036`

---

## Parallel Example: User Story 1

```bash
Task: "Add interactive init coverage for provider selection, legacy config import, and project-profile creation in cmd/init_test.go"
Task: "Add project-profile persistence and repository-validation coverage in internal/fs/fs_test.go and internal/http/client_test.go"
```

## Parallel Example: User Story 2

```bash
Task: "Add SKILLS.md parsing and loreskills list coverage in internal/http/client_test.go and cmd/list_test.go"
Task: "Add folder-aware install/update and rich lore-state.json coverage in cmd/install_test.go, cmd/update_test.go, and internal/fs/fs_test.go"
```

## Parallel Example: User Story 3

```bash
Task: "Add stable-release selection and prerelease-filtering coverage in internal/http/client_test.go"
Task: "Add update self routing, asset verification, and replacement-recovery coverage in cmd/update_test.go and internal/fs/fs_test.go"
```

## Parallel Example: User Story 4

```bash
Task: "Add minimal root/version signature coverage and removal of animal ASCII output in cmd/root_test.go and cmd/version_test.go"
Task: "Add hidden stockholm coverage and help-exclusion assertions in cmd/stockholm_test.go and cmd/help_test.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational
3. Complete Phase 3: User Story 1
4. **STOP and VALIDATE**: Confirm a project can complete private-repo setup without shared configuration
5. Demo the project-local onboarding flow before adding remote discovery and self-update

### Incremental Delivery

1. Deliver **US1** to establish project-scoped private repository setup
2. Deliver **US2** to make remote discovery and folder-aware installs usable
3. Deliver **US3** to add stable-only CLI self-update
4. Deliver **US4** to finalize the minimal version identity and hidden artwork behavior
5. Finish with documentation alignment and full validation

### Parallel Team Strategy

1. Complete Setup + Foundational together
2. Once Foundational is done:
   - Developer A: User Story 1
   - Developer B: User Story 2
   - Developer C: User Story 3
   - Developer D: User Story 4
3. Rejoin for Polish and repository-wide validation

---

## Notes

- `[P]` tasks touch different files and avoid unfinished dependencies
- `[US1]` through `[US4]` map directly to the feature specification stories
- Each story remains independently testable when using the documented fixtures and validation flow
- Keep implementation stdlib-only and preserve the `cmd/` → `internal/http/` / `internal/fs/` layering
