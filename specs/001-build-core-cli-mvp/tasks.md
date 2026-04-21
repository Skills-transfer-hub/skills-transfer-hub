# Tasks: Core CLI et MVP

**Input**: Design documents from `/specs/001-build-core-cli-mvp/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/

**Tests**: Tests are required by the constitution for every new command and helper. Write tests first, confirm they fail, then implement.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Align the repository layout and test scaffolding with the implementation plan before story work begins.

- [X] T001 Create the constitution-aligned network package target in `internal/http/client.go` and plan the retirement of `internal/fetch/fetch.go`
- [X] T002 [P] Create adjacent Go test file scaffolding in `cmd/root_test.go`, `cmd/init_test.go`, `cmd/install_test.go`, `cmd/update_test.go`, `internal/fs/fs_test.go`, and `internal/http/client_test.go`
- [X] T003 [P] Refresh the validation commands and manual verification steps in `specs/001-build-core-cli-mvp/quickstart.md`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure that MUST be complete before ANY user story can be implemented

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [X] T004 Implement shared CLI execution and output-capture helpers in `cmd/root_test.go` for command-level tests
- [X] T005 [P] Implement reusable temp home/project fixtures for config and state files in `internal/fs/fs_test.go`
- [X] T006 [P] Implement reusable `httptest` response fixtures for success, `404`, and unexpected HTTP status cases in `internal/http/client_test.go`

**Checkpoint**: Foundation ready - user story implementation can now begin in parallel

---

## Phase 3: User Story 1 - Initialiser la configuration locale (Priority: P1) 🎯 MVP

**Goal**: Let a new user create a valid `.loreskillsrc` in the home directory and receive usable CLI help and guidance.

**Independent Test**: Run `loreskills` with no arguments and `loreskills init` in an isolated temp home directory; verify help output, config creation, default JSON values, and refusal to overwrite an existing config.

### Tests for User Story 1 ⚠️

- [X] T007 [P] [US1] Add tests for no-argument help output and unknown-command handling in `cmd/root_test.go`
- [X] T008 [P] [US1] Add tests for init success and existing-config refusal in `cmd/init_test.go` and `internal/fs/fs_test.go`

### Implementation for User Story 1

- [X] T009 [US1] Refactor CLI output paths in `main.go` and `cmd/root.go` so help and errors are deterministic and testable
- [X] T010 [US1] Implement `loreskills init` behavior and user-facing success/error messages in `cmd/init.go`
- [X] T011 [US1] Implement config creation, placeholder defaults, and no-overwrite behavior in `internal/fs/fs.go`

**Checkpoint**: At this point, User Story 1 should be fully functional and testable independently

---

## Phase 4: User Story 2 - Installer un skill dans le projet courant (Priority: P2)

**Goal**: Download a named remote skill, write it into the current project, and track it in local state.

**Independent Test**: Prepare a valid `.loreskillsrc`, run `loreskills install <skill>` from a temp project, and verify target directory creation, managed header insertion, state persistence, and explicit `404` failure behavior.

### Tests for User Story 2 ⚠️

- [X] T012 [P] [US2] Add install command tests for successful install, target directory creation, state update, and `404` failure in `cmd/install_test.go`
- [X] T013 [P] [US2] Add filesystem tests for skill file writing and state persistence in `internal/fs/fs_test.go`
- [X] T014 [P] [US2] Add HTTP client tests for download success, `404`, and unexpected status handling in `internal/http/client_test.go`

### Implementation for User Story 2

- [X] T015 [US2] Move the download implementation from `internal/fetch/fetch.go` into `internal/http/client.go` and remove the obsolete `internal/fetch/fetch.go`
- [X] T016 [US2] Implement install orchestration and error propagation in `cmd/install.go` using `internal/http/client.go` and `internal/fs/fs.go`
- [X] T017 [US2] Finalize config validation, target-directory creation, managed-header writing, and state updates in `internal/fs/fs.go`

**Checkpoint**: At this point, User Stories 1 and 2 should both work independently

---

## Phase 5: User Story 3 - Mettre à jour les skills déjà installés (Priority: P3)

**Goal**: Refresh all tracked skills in the current project and no-op cleanly when none are registered.

**Independent Test**: Seed `.loreskillsrc` and `lore-state.json` in a temp project, run `loreskills update`, and verify the no-state message, per-skill refresh behavior, and failure reporting for download errors.

### Tests for User Story 3 ⚠️

- [X] T018 [P] [US3] Add update command tests for empty state, multi-skill refresh, and partial failure reporting in `cmd/update_test.go`
- [X] T019 [P] [US3] Add state edge-case tests for missing and malformed `lore-state.json` in `internal/fs/fs_test.go`

### Implementation for User Story 3

- [X] T020 [US3] Implement update orchestration, no-op messaging, and per-skill refresh reporting in `cmd/update.go`
- [X] T021 [US3] Adjust state read/write helpers for update semantics in `internal/fs/fs.go`

**Checkpoint**: All user stories should now be independently functional

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Improvements that affect multiple user stories

- [X] T022 [P] Update the final command contract and examples in `specs/001-build-core-cli-mvp/contracts/cli-contract.md` and `specs/001-build-core-cli-mvp/quickstart.md`
- [X] T023 [P] Perform consistency cleanup across `cmd/root.go`, `cmd/init.go`, `cmd/install.go`, `cmd/update.go`, `internal/fs/fs.go`, and `internal/http/client.go`
- [X] T024 Run `go test ./...` and `CGO_ENABLED=0 go build -o loreskills .` from `c:/SOURCE/LoreSkills`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion - BLOCKS all user stories
- **User Stories (Phase 3+)**: All depend on Foundational phase completion
  - User stories can then proceed in parallel if multiple contributors are available
  - Or sequentially in priority order (P1 → P2 → P3)
- **Polish (Phase 6)**: Depends on all desired user stories being complete

### User Story Dependencies

- **User Story 1 (P1)**: Can start after Foundational (Phase 2) - no dependencies on other stories
- **User Story 2 (P2)**: Can start after Foundational (Phase 2) - independently testable by seeding config without relying on `init`
- **User Story 3 (P3)**: Can start after Foundational (Phase 2) - independently testable by seeding config and state without relying on `install`

### Within Each User Story

- Tests MUST be written and fail before implementation
- Shared helpers before command logic
- Filesystem and HTTP support before final orchestration where applicable
- Story complete before marking the checkpoint reached

### Parallel Opportunities

- `T002` and `T003` can run in parallel after `T001`
- `T005` and `T006` can run in parallel after `T004`
- In **US1**, `T007` and `T008` can run in parallel
- In **US2**, `T012`, `T013`, and `T014` can run in parallel
- In **US3**, `T018` and `T019` can run in parallel
- `T022` and `T023` can run in parallel before final validation `T024`

---

## Parallel Example: User Story 1

```bash
Task: "Add tests for no-argument help output and unknown-command handling in cmd/root_test.go"
Task: "Add tests for init success and existing-config refusal in cmd/init_test.go and internal/fs/fs_test.go"
```

## Parallel Example: User Story 2

```bash
Task: "Add install command tests for successful install, target directory creation, state update, and 404 failure in cmd/install_test.go"
Task: "Add filesystem tests for skill file writing and state persistence in internal/fs/fs_test.go"
Task: "Add HTTP client tests for download success, 404, and unexpected status handling in internal/http/client_test.go"
```

## Parallel Example: User Story 3

```bash
Task: "Add update command tests for empty state, multi-skill refresh, and partial failure reporting in cmd/update_test.go"
Task: "Add state edge-case tests for missing and malformed lore-state.json in internal/fs/fs_test.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational
3. Complete Phase 3: User Story 1
4. **STOP and VALIDATE**: Run the US1 tests and verify `loreskills init` plus the default help flow independently
5. Demo the initial configuration experience before moving to install/update

### Incremental Delivery

1. Deliver **US1** to establish CLI help and config creation safely
2. Deliver **US2** to provide the core install value of the MVP
3. Deliver **US3** to complete the maintenance flow for already installed skills
4. Finish with cross-cutting cleanup and repository-wide validation
