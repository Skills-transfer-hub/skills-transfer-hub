# Tasks: Ship Portable CLI

**Input**: Design documents from `/specs/002-ship-portable-cli/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/, quickstart.md

**Tests**: Tests are required by the constitution for every new command and helper. Add or update package-local Go tests before implementing the corresponding CLI behavior.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this belongs to (`[US1]`, `[US2]`, `[US3]`)
- Every task includes exact file paths

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Create the implementation scaffolding needed for release automation and the new CLI surfaces.

- [X] T001 Create the release workflow scaffold and MVP artifact naming baseline in `.github/workflows/release.yml`
- [X] T002 [P] Create new CLI test file scaffolding for `help` and `version` in `cmd/help_test.go` and `cmd/version_test.go`
- [X] T003 [P] Extend shared CLI runner/assertion helpers for richer stdout/stderr verification in `cmd/root_test.go`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core structures that MUST be complete before user story work can proceed cleanly.

**⚠️ CRITICAL**: No user story work should be considered complete until this phase is done.

- [X] T004 Implement a shared command catalog and command lookup helpers in `cmd/root.go`
- [X] T005 [P] Implement shared avatar-aware stdout/stderr rendering helpers in `cmd/root.go`
- [X] T006 [P] Implement build metadata defaults and formatting helpers in `cmd/version.go`

**Checkpoint**: Foundation ready - release work and CLI story work can now proceed.

---

## Phase 3: User Story 1 - Installer LoreSkills globalement sur les trois OS (Priority: P1) 🎯 MVP

**Goal**: Publish cross-platform GitHub release artifacts with checksum verification and documented global install steps for Windows, macOS, and Linux.

**Independent Test**: Run the release workflow for the MVP matrix, verify the four artifacts and checksum manifest are published, then follow the README install steps on a target OS until `loreskills --help` is callable globally.

### Implementation for User Story 1

- [X] T007 [US1] Implement the GitHub Actions release matrix, archive packaging, and SHA256 manifest publication in `.github/workflows/release.yml`
- [X] T008 [P] [US1] Document Windows, macOS, and Linux global installation plus SHA256 verification steps in `README.md`
- [X] T009 [US1] Add release validation expectations and manual verification commands to `specs/002-ship-portable-cli/quickstart.md`

**Checkpoint**: At this point, the repository can publish and document an installable MVP release independently of the later CLI UX improvements.

---

## Phase 4: User Story 2 - Se repérer grâce à l'aide intégrée (Priority: P2)

**Goal**: Provide explicit `help` and `version` commands with consistent syntax, examples, and actionable usage errors.

**Independent Test**: Run `loreskills`, `loreskills help`, `loreskills help install`, `loreskills version`, and an invalid command; verify each output is specific, actionable, and matches the documented command surface.

### Tests for User Story 2 ⚠️

- [X] T010 [P] [US2] Add root and contextual help coverage in `cmd/root_test.go` and `cmd/help_test.go`
- [X] T011 [P] [US2] Add build metadata and `version` command coverage in `cmd/version_test.go`

### Implementation for User Story 2

- [X] T012 [US2] Implement `help` command routing and command-specific help output in `cmd/root.go`
- [X] T013 [US2] Implement `loreskills version` output using build metadata helpers in `cmd/version.go`
- [X] T014 [US2] Refresh command usage strings and corrective argument errors in `cmd/init.go`, `cmd/install.go`, and `cmd/update.go`

**Checkpoint**: User Story 2 is independently functional when users can discover and understand the command surface without opening repository files.

---

## Phase 5: User Story 3 - Être guidé dans le terminal par un avatar ASCII (Priority: P3)

**Goal**: Show a consistent ASCII guide on every visible CLI output without hiding results, errors, or next actions.

**Independent Test**: Run help, success, no-op, and failure flows across the CLI and verify that every visible stdout/stderr message contains the same compact ASCII guide and still remains readable.

### Tests for User Story 3 ⚠️

- [X] T015 [P] [US3] Add ASCII guide coverage for root help and unknown-command output in `cmd/root_test.go` and `cmd/help_test.go`
- [X] T016 [P] [US3] Add ASCII guide coverage for `init`, `install`, `update`, and `version` outputs in `cmd/init_test.go`, `cmd/install_test.go`, `cmd/update_test.go`, and `cmd/version_test.go`

### Implementation for User Story 3

- [X] T017 [US3] Implement a shared ASCII guide renderer for visible stdout/stderr output in `cmd/root.go`
- [X] T018 [US3] Apply ASCII-guided success and no-op messaging in `cmd/init.go`, `cmd/install.go`, `cmd/update.go`, and `cmd/version.go`
- [X] T019 [US3] Apply ASCII-guided error and fallback help output in `cmd/root.go`

**Checkpoint**: All visible LoreSkills command outputs now include the same guide and remain independently testable.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Final consistency, validation, and repo-wide quality checks.

- [X] T020 [P] Synchronize final command examples and release/install guidance in `README.md`, `specs/002-ship-portable-cli/contracts/cli-contract.md`, and `specs/002-ship-portable-cli/quickstart.md`
- [X] T021 [P] Add `go vet` and static build validation coverage to `.github/workflows/ci.yml` and `.github/workflows/release.yml`
- [X] T022 Run `go test ./...`, `go vet ./...`, and `CGO_ENABLED=0 go build -ldflags "-X loreskills/cmd.version=dev" -o loreskills .` from `c:/SOURCE/LoreSkills` against `cmd/root.go`, `cmd/version.go`, `.github/workflows/release.yml`, and `README.md`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion - BLOCKS clean user story completion
- **User Stories (Phase 3+)**: Depend on Foundational completion
- **Polish (Phase 6)**: Depends on all desired user stories being complete

### User Story Dependencies

- **User Story 1 (P1)**: Can start after Foundational - no dependency on later stories
- **User Story 2 (P2)**: Can start after Foundational - independently testable from local CLI execution
- **User Story 3 (P3)**: Can start after Foundational, but merges most cleanly after User Story 2 because both touch `cmd/root.go` and `cmd/version.go`

### Within Each User Story

- Add or update tests before changing the corresponding command behavior
- Shared helpers before command orchestration
- Documentation updates after the underlying behavior or workflow is stable
- Validate each story independently before moving to the next one

### Parallel Opportunities

- `T002` and `T003` can run in parallel after `T001`
- `T005` and `T006` can run in parallel after `T004`
- In **US1**, `T008` can run in parallel with `T007`; `T009` follows once artifact names and workflow behavior are fixed
- In **US2**, `T010` and `T011` can run in parallel
- In **US3**, `T015` and `T016` can run in parallel
- In **Polish**, `T020` and `T021` can run in parallel before final validation `T022`

---

## Parallel Example: User Story 1

```bash
Task: "Document Windows, macOS, and Linux global installation plus SHA256 verification steps in README.md"
Task: "Implement the GitHub Actions release matrix, archive packaging, and SHA256 manifest publication in .github/workflows/release.yml"
```

## Parallel Example: User Story 2

```bash
Task: "Add root and contextual help coverage in cmd/root_test.go and cmd/help_test.go"
Task: "Add build metadata and version command coverage in cmd/version_test.go"
```

## Parallel Example: User Story 3

```bash
Task: "Add ASCII guide coverage for root help and unknown-command output in cmd/root_test.go and cmd/help_test.go"
Task: "Add ASCII guide coverage for init, install, update, and version outputs in cmd/init_test.go, cmd/install_test.go, cmd/update_test.go, and cmd/version_test.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational
3. Complete Phase 3: User Story 1
4. **STOP and VALIDATE**: Run the release workflow or a local dry run and verify the documented install path on one target platform
5. Demo the installable release flow before continuing to richer CLI UX work

### Incremental Delivery

1. Deliver **US1** to make LoreSkills publishable and installable
2. Deliver **US2** to make the shipped CLI self-explanatory through `help` and `version`
3. Deliver **US3** to add the consistent ASCII-guided terminal identity
4. Finish with repository-wide validation and documentation synchronization

### Parallel Team Strategy

1. Complete Setup + Foundational together
2. Then split the work:
   - Developer A: `US1` release workflow and install docs
   - Developer B: `US2` help/version tests and command implementation
   - Developer C: `US3` avatar rendering tests and presentation integration
3. Rejoin for Polish and full validation

---

## Notes

- `[P]` tasks touch different files and avoid incomplete dependencies
- `[US1]`, `[US2]`, and `[US3]` map directly to the clarified specification stories
- Each story remains independently testable even when later stories improve the same CLI surface
- Keep Go code stdlib-only and preserve the `cmd/` → `internal/http/` / `internal/fs/` layering