# Tasks: Guided Repository Onboarding

**Input**: Design documents from `/specs/004-rework-repo-onboarding/`  
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/cli-contract.md, quickstart.md

**Tests**: Tests are required by the constitution and quickstart for every new CLI behavior, provider guide, discovery flow, filesystem guard, and update-cache path. Write or update package-local Go tests before implementing the corresponding behavior.

**Organization**: Tasks are grouped by user story so each story can be implemented and validated independently.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no unfinished dependencies)
- **[Story]**: Which user story this belongs to (`[US1]`, `[US2]`, `[US3]`, `[US4]`)
- Every task includes exact file paths

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Prepare the repository for new guided-entry, provider-catalog, discovery, and cache work.

- [x] T001 Create the new feature file scaffolds in `cmd/menu.go`, `cmd/menu_test.go`, `cmd/providers.go`, `cmd/providers_test.go`, `internal/http/discovery.go`, `internal/http/discovery_test.go`, `internal/fs/state_cache.go`, and `internal/fs/state_cache_test.go`
- [x] T002 [P] Extend the shared CLI test harness for fake TTY input/output, machine-cache directories, and startup-state assertions in `cmd/root_test.go` and `cmd/menu_test.go`
- [x] T003 [P] Add reusable provider, repository-tree, and bundle fixture helpers for network and filesystem tests in `internal/http/client_test.go`, `internal/http/discovery_test.go`, and `internal/fs/fs_test.go`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Build the cross-story primitives that every later slice depends on.

**⚠️ CRITICAL**: No user story work should be considered complete until this phase is done.

- [x] T004 Implement shared supported-provider metadata types, hosted-domain validation helpers, and provider lookup functions in `cmd/providers.go` and `cmd/providers_test.go`
- [x] T005 [P] Implement shared root-command state loading, TTY detection hooks, and common next-step message helpers in `cmd/root.go` and `cmd/menu.go`
- [x] T006 [P] Implement common managed-path boundary, bundle-identity, and target-root helper functions in `internal/fs/fs.go` and `internal/fs/fs_test.go`
- [x] T007 Implement shared repository-tree and discovery-source model helpers in `internal/http/discovery.go` and `internal/http/discovery_test.go`
- [x] T008 Integrate the shared provider, state, and path helpers into command wiring entry points in `cmd/root.go`, `cmd/init.go`, `cmd/list.go`, `cmd/install.go`, and `cmd/update.go`

**Checkpoint**: Shared infrastructure is ready. User story slices can proceed independently.

---

## Phase 3: User Story 1 - Configure a Provider Without External Guesswork (Priority: P1) 🎯 MVP

**Goal**: Let a project complete guided repository onboarding for supported hosted providers with precise credential instructions and validation.

**Independent Test**: Start in a project with no LoreSkills configuration, run `loreskills init`, choose each supported provider in turn, and verify that the flow shows the correct documentation URL, minimum permission, repository identifier format, and successful reachability validation before writing `.loreskills/project.json`.

### Tests for User Story 1 ⚠️

> **NOTE**: Write these tests first and confirm they fail before implementation.

- [x] T009 [P] [US1] Add interactive init wizard coverage for hosted-provider selection, unsupported-host rejection, and repository-id guidance in `cmd/init_test.go`
- [x] T010 [P] [US1] Add configuration migration and access-validation coverage for supported providers in `internal/fs/fs_test.go` and `internal/http/client_test.go`

### Implementation for User Story 1

- [x] T011 [US1] Implement the provider guidance catalog with official documentation URLs, action URLs, minimum permissions, and environment variable hints in `cmd/providers.go`
- [x] T012 [US1] Implement hosted-provider-only onboarding prompts and provider-guidance review flow in `cmd/init.go`
- [x] T013 [US1] Implement project-profile validation updates for supported hosted providers and repository-id normalization in `internal/fs/fs.go`
- [x] T014 [US1] Implement configuration reachability validation that accepts either `SKILLS.md` presence or scan-detectable repository layout in `internal/http/client.go` and `internal/http/discovery.go`
- [x] T015 [US1] Update onboarding help and operator documentation for hosted-provider setup in `cmd/root.go` and `README.md`

**Checkpoint**: A user can configure a supported hosted provider with reliable guidance and project-local validation.

---

## Phase 4: User Story 2 - Publish Skills in a Compatible and Safe Repository Layout (Priority: P1)

**Goal**: Discover compatible bundles from `SKILLS.md` or repository scan fallback, then install and update them safely inside the managed boundary.

**Independent Test**: Point LoreSkills at one repository with a valid `SKILLS.md` and one repository without `SKILLS.md` but with valid `category/folder/SKILL.md` bundles, then verify `list`, `install`, and `update` respect discovery precedence, write only under `<target_dir>`, overwrite managed files, and clean up obsolete tracked files.

### Tests for User Story 2 ⚠️

> **NOTE**: Write these tests first and confirm they fail before implementation.

- [x] T016 [P] [US2] Add `SKILLS.md`-first versus structure-scan discovery coverage in `internal/http/discovery_test.go` and `cmd/list_test.go`
- [x] T017 [P] [US2] Add managed bundle install, overwrite, cleanup, and update-manifest coverage in `cmd/install_test.go`, `cmd/update_test.go`, and `internal/fs/state_cache_test.go`

### Implementation for User Story 2

- [x] T018 [US2] Implement provider-aware repository tree listing and `SKILLS.md`/scan precedence resolution in `internal/http/discovery.go` and `internal/http/client.go`
- [x] T019 [US2] Implement bundle discovery parsing and canonical `category/folder` listing output in `cmd/list.go` and `cmd/root.go`
- [x] T020 [US2] Implement bundle manifest persistence, managed-file tracking, and obsolete-file cleanup in `internal/fs/state_cache.go` and `internal/fs/fs.go`
- [x] T021 [US2] Implement full bundle installation sync under `<target_dir>/<category>/<folder>/...` in `cmd/install.go`, `internal/http/client.go`, and `internal/fs/fs.go`
- [x] T022 [US2] Implement full bundle resynchronization and repository-overwrites-local semantics in `cmd/update.go`, `internal/http/client.go`, and `internal/fs/state_cache.go`
- [x] T023 [US2] Update repository compatibility, discovery precedence, and managed overwrite documentation in `README.md` and `specs/004-rework-repo-onboarding/quickstart.md`

**Checkpoint**: Users can discover, install, and refresh compatible bundles safely from either discovery source.

---

## Phase 5: User Story 3 - Know What To Do From the Main LoreSkills Experience (Priority: P2)

**Goal**: Make `loreskills` itself a guided, state-aware CLI entrypoint in TTY while preserving a concise non-interactive fallback.

**Independent Test**: Run `loreskills` in an unconfigured project, a configured-but-invalid project, and a configured-ready project, and verify that the interactive menu shows the correct primary action and routes to the right workflows; then run it in a non-interactive context and verify it prints next-step guidance without prompting.

### Tests for User Story 3 ⚠️

> **NOTE**: Write these tests first and confirm they fail before implementation.

- [x] T024 [P] [US3] Add guided menu coverage for `unconfigured`, `configured-invalid`, and `configured-ready` project states in `cmd/menu_test.go`
- [x] T025 [P] [US3] Add bare `loreskills` routing and non-interactive fallback guidance coverage in `cmd/root_test.go`

### Implementation for User Story 3

- [x] T026 [US3] Implement guided menu rendering, selection loop, and primary-action prioritization in `cmd/menu.go`
- [x] T027 [US3] Implement bare-root TTY routing to the guided menu and non-interactive summary output in `cmd/root.go`
- [x] T028 [US3] Wire menu actions to onboarding, discovery, install, update, configuration review, and provider-guidance review flows in `cmd/menu.go`, `cmd/init.go`, `cmd/list.go`, and `cmd/update.go`
- [x] T029 [US3] Update visible help and README entrypoint guidance for the guided root experience in `cmd/root.go` and `README.md`

**Checkpoint**: `loreskills` tells the user the next useful action without requiring prior command knowledge.

---

## Phase 6: User Story 4 - Stay Current Through Guided Automatic Updates (Priority: P2)

**Goal**: Check for stable updates automatically at interactive startup, cache the result for 24 hours per machine, and let users trigger the stable self-update path from the guided experience.

**Independent Test**: Use a release feed containing stable and pre-release entries, run `loreskills` twice interactively on the same machine, and verify that the first run performs a stable-only check, the second run reuses the cached status within 24 hours, and `loreskills update self` still forces the explicit stable update path.

### Tests for User Story 4 ⚠️

> **NOTE**: Write these tests first and confirm they fail before implementation.

- [x] T030 [P] [US4] Add stable release selection and prerelease-ignore coverage in `internal/http/client_test.go`
- [x] T031 [P] [US4] Add 24-hour update-cache reuse and startup surfacing coverage in `internal/fs/state_cache_test.go`, `cmd/menu_test.go`, and `cmd/update_test.go`

### Implementation for User Story 4

- [x] T032 [US4] Implement machine-local update-check cache persistence and freshness evaluation in `internal/fs/state_cache.go` and `internal/fs/fs.go`
- [x] T033 [US4] Implement interactive-startup automatic stable update check orchestration and stale-cache refresh in `cmd/menu.go` and `cmd/root.go`
- [x] T034 [US4] Implement stable update offer rendering and explicit `update self` forced-refresh behavior in `cmd/update.go` and `internal/http/client.go`
- [x] T035 [US4] Update startup-check and self-update documentation in `README.md` and `specs/004-rework-repo-onboarding/quickstart.md`

**Checkpoint**: The guided experience surfaces a stable-only update offer with a 24-hour machine-local cache, and explicit self-update still works.

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Final alignment, regression protection, and repository-wide validation.

- [x] T036 [P] Refresh the final CLI contract and manual validation examples in `specs/004-rework-repo-onboarding/contracts/cli-contract.md` and `specs/004-rework-repo-onboarding/quickstart.md`
- [x] T037 [P] Add final regression coverage for unsupported hosts, path escape blocking, and managed overwrite behavior in `cmd/init_test.go`, `internal/fs/fs_test.go`, and `cmd/update_test.go`
- [x] T038 Run repository-wide validation from `c:/SOURCE/LoreSkills` with `go test ./...`, `go vet ./...`, and a release-metadata build covering `cmd/`, `internal/http/`, `internal/fs/`, and `README.md`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies, can start immediately.
- **Foundational (Phase 2)**: Depends on Setup completion and blocks story work.
- **User Stories (Phases 3-6)**: Depend on Foundational completion.
- **Polish (Phase 7)**: Depends on all desired user stories being complete.

### User Story Dependencies

- **User Story 1 (P1)**: Can start after Foundational completion and is the recommended MVP.
- **User Story 2 (P1)**: Can start after Foundational completion; it does not depend on User Story 1 being complete, but it reuses provider/profile primitives from the foundation.
- **User Story 3 (P2)**: Can start after Foundational completion; it depends only on shared root/menu primitives, not on discovery or update completion.
- **User Story 4 (P2)**: Depends on Foundational completion and benefits from User Story 3 because the update offer is surfaced through the guided menu.

### Within Each User Story

- Tests MUST be written and fail before implementation.
- Shared models/helpers before command orchestration.
- Command orchestration before documentation updates.
- Story-specific validation before moving to the next story.

### Parallel Opportunities

- `T002` and `T003` can run in parallel after `T001`.
- `T005` and `T006` can run in parallel after `T004`.
- In **US1**, `T009` and `T010` can run in parallel.
- In **US2**, `T016` and `T017` can run in parallel.
- In **US3**, `T024` and `T025` can run in parallel.
- In **US4**, `T030` and `T031` can run in parallel.
- In **Polish**, `T036` and `T037` can run in parallel before `T038`.

---

## Parallel Example: User Story 1

```bash
Task: "Add interactive init wizard coverage for hosted-provider selection, unsupported-host rejection, and repository-id guidance in cmd/init_test.go"
Task: "Add configuration migration and access-validation coverage for supported providers in internal/fs/fs_test.go and internal/http/client_test.go"
```

## Parallel Example: User Story 2

```bash
Task: "Add SKILLS.md-first versus structure-scan discovery coverage in internal/http/discovery_test.go and cmd/list_test.go"
Task: "Add managed bundle install, overwrite, cleanup, and update-manifest coverage in cmd/install_test.go, cmd/update_test.go, and internal/fs/state_cache_test.go"
```

## Parallel Example: User Story 3

```bash
Task: "Add guided menu coverage for unconfigured, configured-invalid, and configured-ready project states in cmd/menu_test.go"
Task: "Add bare loreskills routing and non-interactive fallback guidance coverage in cmd/root_test.go"
```

## Parallel Example: User Story 4

```bash
Task: "Add stable release selection and prerelease-ignore coverage in internal/http/client_test.go"
Task: "Add 24-hour update-cache reuse and startup surfacing coverage in internal/fs/state_cache_test.go, cmd/menu_test.go, and cmd/update_test.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup.
2. Complete Phase 2: Foundational.
3. Complete Phase 3: User Story 1.
4. **STOP and VALIDATE**: confirm a project can complete hosted-provider onboarding and validation independently.
5. Demo the guided onboarding flow before adding bundle discovery or automatic updates.

### Incremental Delivery

1. Deliver **US1** to establish reliable hosted-provider onboarding.
2. Deliver **US2** to make repository compatibility and bundle synchronization usable.
3. Deliver **US3** to turn `loreskills` into the guided state-aware entrypoint.
4. Deliver **US4** to surface stable-only startup update checks and explicit self-update from the guided experience.
5. Finish with polish and full repository validation.

### Parallel Team Strategy

1. Complete Setup + Foundational together.
2. Once Foundational is done:
   - Developer A: User Story 1
   - Developer B: User Story 2
   - Developer C: User Story 3
3. After User Story 3 stabilizes, Developer D (or Developer C) takes User Story 4 because it builds on the guided menu surface.
4. Rejoin for Polish and repository-wide validation.

---

## Notes

- `[P]` tasks touch different files and avoid unfinished dependencies.
- `[US1]` through `[US4]` map directly to the feature specification stories.
- Each user story remains independently testable when using the documented quickstart flows.
- Keep implementation stdlib-only and preserve the `cmd/` → `internal/http/` / `internal/fs/` layering.