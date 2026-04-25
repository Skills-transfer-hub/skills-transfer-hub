# Feature Specification: STH CLI Product Roadmap

**Feature Branch**: `006-cli-product-roadmap`  
**Created**: 2026-04-23  
**Status**: Draft  

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Graceful Auth Recovery on Expired Token (Priority: P1)

A developer runs `sth update` mid-session, but their Personal Access Token has expired. Instead of receiving a cryptic `401 Unauthorized` error and having to figure out where to re-enter their token, the tool detects the failed auth, informs them clearly, and surfaces the token input prompt directly in the interactive menu — allowing them to continue without restarting.

**Why this priority**: Auth failures are the most common blocking issue for users. A hard error with no recovery path creates frustration and erodes trust. This directly reduces support burden.

**Independent Test**: Can be fully tested by configuring an expired/invalid token and running any authenticated command, verifying the interactive re-auth prompt appears.

**Acceptance Scenarios**:

1. **Given** a user has a stored token that returns `401`, **When** they run any network command, **Then** the tool displays a clear "Token expired or invalid" message and opens the token re-entry prompt.
2. **Given** a user has a stored token that returns `403`, **When** they run any network command, **Then** the tool explains the permission issue and invites them to update the token.
3. **Given** a user re-enters a valid token at the recovery prompt, **When** they confirm, **Then** the original command resumes successfully without requiring a re-run.
4. **Given** a user dismisses the recovery prompt, **When** they exit, **Then** the tool exits cleanly with a descriptive error code and message.

---

### User Story 2 - Remove and Status Commands for Resource Lifecycle (Priority: P1)

A developer has installed several skill bundles over time. They want to see at a glance which resources are up to date, which have been modified locally, and which are outdated — and then cleanly uninstall a resource they no longer need, knowing all managed files and state entries will be cleaned up.

**Why this priority**: Without `status` and `remove`, users have no lifecycle visibility and must manually inspect and delete files. This is a fundamental gap in resource management.

**Independent Test**: Install a resource, run `sth status` to confirm it appears with correct state, then run `sth remove <ref>` and verify files are gone and state is cleaned.

**Acceptance Scenarios**:

1. **Given** resources are installed, **When** the user runs `sth status`, **Then** each resource is listed with one of: `up-to-date`, `outdated`, or `modified-locally`.
2. **Given** a resource has been manually edited, **When** the user runs `sth status`, **Then** that resource is marked `modified-locally` with the affected files highlighted.
3. **Given** a valid installed resource ref, **When** the user runs `sth remove <ref>`, **Then** all managed files for that resource are deleted and its entry is removed from state.
4. **Given** the user runs `sth remove <ref>` on a resource with locally modified files, **When** the command executes, **Then** the tool warns about local modifications and requires explicit confirmation before removing.
5. **Given** an unknown ref, **When** the user runs `sth remove <unknown>`, **Then** the tool returns a clear "resource not found" error.

---

### User Story 3 - Dry Run Mode for Install and Update (Priority: P1)

A developer wants to preview exactly which files would be created, updated, or deleted by `sth install` or `sth update` before committing to changes — especially useful in a team context where unexpected file changes could conflict with in-progress work.

**Why this priority**: Irreversible disk changes without preview is a source of anxiety. Dry-run is a foundational safety feature expected in modern CLI tools.

**Independent Test**: Run `sth install --dry-run` on a new resource and verify no files are written to disk while a complete diff-like summary is printed.

**Acceptance Scenarios**:

1. **Given** `--dry-run` is passed, **When** `sth install` is run, **Then** a list of files that would be created is displayed and no files are written.
2. **Given** `--dry-run` is passed, **When** `sth update` is run, **Then** a diff-style summary shows files that would be added, modified, or removed, without touching the disk.
3. **Given** `--dry-run` is passed, **When** no changes would occur, **Then** the tool outputs "No changes would be made."
4. **Given** `--dry-run` is combined with `--json`, **When** the command runs, **Then** the dry-run output is emitted as structured JSON.

---

### User Story 4 - Debugging, Diagnostics, and Observability (Priority: P1)

A developer encounters unexpected behavior (wrong files installed, auth failing, cache stale). They want to enable verbose output to trace what the tool is doing — HTTP calls, resolved paths, cache decisions — and also run a self-diagnostic command that validates the full configuration and reports any issues with corrective steps.

**Why this priority**: Without diagnostic tooling, debugging configuration issues requires source-level investigation. `--verbose` and `sth doctor` are table-stakes for a production CLI tool.

**Independent Test**: Run any command with `--verbose` and verify HTTP request details and path resolution are printed. Run `sth doctor` with a broken config and verify it reports the specific failure with a suggested fix.

**Acceptance Scenarios**:

1. **Given** `--verbose` or `--debug` is passed, **When** any command runs, **Then** HTTP requests (URL, status code), resolved file paths, and cache decisions are printed to stderr.
2. **Given** a valid project, **When** the user runs `sth doctor`, **Then** it outputs a pass/fail checklist covering: token validity, project config structure, remote repository accessibility, and state file integrity.
3. **Given** a misconfigured project (e.g., missing token, invalid provider URL), **When** the user runs `sth doctor`, **Then** failing checks are clearly identified with actionable remediation steps.
4. **Given** all checks pass, **When** the user runs `sth doctor`, **Then** the output ends with a green "All checks passed" summary.

---

### User Story 5 - CI/CD Integration Without Interactive Auth (Priority: P2)

A CI pipeline (e.g., GitHub Actions) runs `sth update` as part of a build. The environment is non-interactive (no TTY), has no stored PAT, but does have access to GitHub Actions OIDC tokens. The tool detects the CI context automatically and authenticates using the OIDC token without requiring manual configuration. Additionally, a `--fail-on-changes` flag allows the pipeline to act as a gate that fails if managed files have drifted from their expected state.

**Why this priority**: CI support is critical for teams using sth at scale. OIDC removes the need to store long-lived secrets in CI environments.

**Independent Test**: Run in a simulated non-TTY environment with `ACTIONS_ID_TOKEN_REQUEST_URL` set and verify authentication succeeds without a PAT.

**Acceptance Scenarios**:

1. **Given** a non-TTY environment with `ACTIONS_ID_TOKEN_REQUEST_URL` present, **When** any authenticated command runs, **Then** the tool exchanges the OIDC token automatically and proceeds without prompting.
2. **Given** a CI environment without OIDC and without a stored token, **When** an authenticated command runs, **Then** the tool exits with a clear non-interactive error and a non-zero exit code.
3. **Given** managed files have been modified locally, **When** `sth update --fail-on-changes` is run, **Then** the tool exits with a non-zero code and lists the modified files.
4. **Given** no managed files have been modified, **When** `sth update --fail-on-changes` is run, **Then** the tool exits with code 0.

---

### User Story 6 - Version Pinning for Resources (Priority: P2)

A developer wants to lock a specific resource to a known-good version (commit SHA or tag) to avoid unexpected breakage from upstream changes. When a newer version is available, the tool informs them without forcing an upgrade.

**Why this priority**: Without pinning, any `sth update` can silently introduce breaking changes. Pinning is essential for production-grade reliability.

**Independent Test**: Pin a resource to a specific ref in `project.json`, run `sth update`, and verify the pinned version is not upgraded while the tool reports that a newer version exists.

**Acceptance Scenarios**:

1. **Given** a resource is pinned to a specific ref in `project.json`, **When** `sth update` runs, **Then** the pinned resource is not upgraded and is listed as "pinned" in the output.
2. **Given** a pinned resource has a newer version upstream, **When** `sth status` or `sth update` runs, **Then** the tool displays a notice: "Newer version available for `<ref>` (pinned at `<version>`)."
3. **Given** a user removes the pin from `project.json`, **When** `sth update` runs next, **Then** the resource is upgraded to the latest version.
4. **Given** a pin points to a non-existent ref (deleted tag/commit), **When** any command tries to resolve it, **Then** the tool reports a clear error identifying the invalid ref.

---

### User Story 7 - Multi-Repository Resource Sources (Priority: P2)

A developer works at a company that maintains both an internal skill repository and uses a public one. They want to install resources from both sources in the same project without needing separate config files or tool invocations.

**Why this priority**: Single-repo limitation is a blocker for teams with mixed public/private skill libraries.

**Independent Test**: Configure two providers in `project.json` and install a resource from each. Verify both appear in `sth list` with their respective source tagged.

**Acceptance Scenarios**:

1. **Given** multiple providers are configured in `project.json`, **When** `sth list` runs, **Then** all available resources from all providers are listed, with source provider indicated.
2. **Given** resources from two different providers, **When** `sth install <ref>` is run, **Then** the tool resolves the resource from the correct provider.
3. **Given** a resource ref exists in multiple providers, **When** installing without disambiguation, **Then** the tool prompts the user to choose the source or accepts a `--provider` flag to disambiguate.
4. **Given** one provider is unavailable, **When** commands run against other providers, **Then** only the unavailable provider's resources fail — others are unaffected.

---

### User Story 8 - Shell Completion and Machine-Readable Output (Priority: P2)

A power user wants tab completion for `sth` commands in their shell, and a CI engineer needs structured JSON output from `sth list` and `sth status` to pipe into automation scripts.

**Why this priority**: Shell completion and JSON output are quality-of-life multipliers that make the tool feel polished and CI-friendly.

**Independent Test**: Generate completion script for zsh, source it, and verify `sth <TAB>` lists commands. Run `sth list --json` and verify valid JSON is emitted.

**Acceptance Scenarios**:

1. **Given** the user runs `sth completion bash`, **When** the output is sourced, **Then** `sth <TAB>` completes available commands and known flags.
2. **Given** completion scripts for bash, zsh, and PowerShell are generated, **When** sourced in their respective shells, **Then** command and flag completion works for all top-level commands.
3. **Given** `--json` is passed to `sth list`, `sth status`, or `sth version`, **When** the command runs, **Then** output is valid JSON with a stable, documented schema.
4. **Given** an error occurs with `--json` enabled, **When** the command fails, **Then** the error is also emitted as JSON (not plain text) with a non-zero exit code.

---

### User Story 9 - File Integrity Verification (Priority: P2)

A developer suspects that a managed skill file has been corrupted or altered outside of sth. They want the tool to detect this automatically during status checks and warn them clearly.

**Why this priority**: Without integrity checks, silent corruption or accidental edits go unnoticed. This protects teams from divergence between the declared state and disk reality.

**Independent Test**: Install a resource, manually alter a managed file, run `sth status`, and verify the integrity warning appears with the affected file listed.

**Acceptance Scenarios**:

1. **Given** a managed file has been altered externally, **When** `sth status` runs, **Then** the file is flagged as `modified-locally` with a hash mismatch indicator.
2. **Given** a managed file is intact, **When** `sth status` runs, **Then** the file passes integrity check silently.
3. **Given** a state file is missing SHA256 entries (legacy install), **When** `sth status` runs, **Then** the tool notes the missing baseline and suggests re-installing to establish integrity tracking.

---

### User Story 10 - Ecosystem Tooling: Catalog, Templates, and Changelogs (Priority: P3)

A skills repository maintainer wants to: (a) auto-generate a `SKILLS.md` catalog from their directory structure instead of writing it by hand, (b) scaffold new skill entries with consistent structure using a template command, and (c) surface a `CHANGELOG.md` to end-users during updates so they know what changed.

**Why this priority**: These features improve the experience for skill authors and create a richer consumer experience, but they depend on the core tool being stable first.

**Independent Test**: Run `sth catalog init` in a skills repo and verify `SKILLS.md` is generated. Run `sth new category/name` and verify a pre-structured skill folder is created.

**Acceptance Scenarios**:

1. **Given** a skills repository with the expected `category/skill/SKILL.md` structure, **When** the maintainer runs `sth catalog init`, **Then** a `SKILLS.md` is generated listing all skills grouped by category.
2. **Given** the user runs `sth new <category>/<name>`, **When** the command executes, **Then** a scaffold folder is created with the required `SKILL.md` template structure.
3. **Given** a bundle contains a `CHANGELOG.md`, **When** `sth update` installs a newer version, **Then** the relevant changelog entries since the last installed version are displayed in the terminal.
4. **Given** no `CHANGELOG.md` exists in a bundle, **When** updating, **Then** no changelog section is shown (silent omission, not an error).

---

### Edge Cases

- What happens when a `sth remove` is interrupted mid-deletion (partial removal)?
- How does multi-repo handle a naming conflict where two providers export a resource with the same ref?
- What happens when `--dry-run` is run with no network access (e.g., offline)?
- How does OIDC auth behave when the token exchange endpoint is unreachable?
- What happens when `sth doctor` is run in a project with no `project.json`?
- How does shell completion behave when sth is installed in a non-standard path?
- What if a `CHANGELOG.md` is malformed or very large — does it truncate or fail?

## Requirements *(mandatory)*

### Functional Requirements

**Authentication & Security**

- **FR-001**: When an authenticated command receives a `401` or `403` response, the tool MUST display a human-readable error and surface the token re-entry prompt without exiting.
- **FR-002**: In non-TTY environments where `ACTIONS_ID_TOKEN_REQUEST_URL` is set, the tool MUST use GitHub Actions OIDC to authenticate without requiring a stored PAT.
- **FR-003**: The tool MUST store a SHA256 hash for each managed file in state and compare it on `status` checks, warning on mismatch.

**Resource Lifecycle**

- **FR-004**: The tool MUST provide a `sth remove <ref>` command that deletes all managed files for a resource and removes its entry from state.
- **FR-005**: When removing a resource with locally modified files, the tool MUST warn the user and require explicit confirmation before proceeding.
- **FR-006**: The tool MUST provide a `sth status` command that reports each installed resource as `up-to-date`, `outdated`, or `modified-locally`.
- **FR-007**: Resources MUST support a `ref` field in `project.json` to pin them to a specific commit or tag; pinned resources MUST be skipped during `sth update`.
- **FR-008**: When a pinned resource has a newer upstream version, the tool MUST display an informational notice without blocking the command.
- **FR-009**: The tool MUST support multiple provider configurations in `project.json` and resolve resources from the correct provider.

**CLI Experience**

- **FR-010**: `sth install` and `sth update` MUST support a `--dry-run` flag that prints a preview of all changes without modifying the filesystem.
- **FR-011**: `sth completion <shell>` MUST generate completion scripts for `bash`, `zsh`, and `powershell`.
- **FR-012**: `sth list`, `sth status`, and `sth version` MUST support a `--json` flag emitting valid, stable JSON.
- **FR-013**: When `--json` is active, errors MUST also be emitted as JSON with a non-zero exit code.
- **FR-014**: All commands MUST support `--verbose` / `--debug` flags that output HTTP details, resolved paths, and cache decisions to stderr.

**CI/CD**

- **FR-015**: `sth update --fail-on-changes` MUST exit with a non-zero code when any managed file has been locally modified.
- **FR-016**: The tool MUST support a `.sthignore` file (gitignore syntax) to exclude files from a bundle during installation.

**Diagnostics**

- **FR-017**: `sth doctor` MUST check and report pass/fail for: token validity, project config structure, provider reachability, and state file integrity.
- **FR-018**: Each failing check in `sth doctor` MUST include a human-readable remediation step.

**Ecosystem**

- **FR-019**: `sth catalog init` MUST generate a `SKILLS.md` from the `category/skill/SKILL.md` directory structure of the current repository.
- **FR-020**: `sth new <category>/<name>` MUST scaffold a new skill directory with a structured `SKILL.md` template.
- **FR-021**: When updating a bundle that contains a `CHANGELOG.md`, the tool MUST display the changelog entries introduced since the previously installed version.

### Key Entities

- **Resource**: A named, versioned bundle of files managed by sth, sourced from a provider repository. Has attributes: ref (name), pinned version, install state, SHA256 hashes per file.
- **Provider**: A configured source repository from which resources are fetched. A project can have multiple providers.
- **State File** (`sth-state.json`): Tracks installed resources, their resolved versions, managed file paths, and SHA256 hashes.
- **Project Config** (`project.json`): Declares provider(s), installed resource refs, optional pins, and tool settings.
- **Bundle**: The set of files associated with a resource ref, optionally filtered by `.sthignore`.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Auth failures are recovered from within the same interactive session — zero need to restart the tool after a token error.
- **SC-002**: Users can identify the full state of all installed resources (up-to-date / outdated / modified) in under 5 seconds on a typical project.
- **SC-003**: Dry-run previews show a complete, accurate diff of what would change — 100% of files that would be modified are listed.
- **SC-004**: CI pipelines using OIDC auth require zero long-lived secrets stored in repository settings.
- **SC-005**: `sth doctor` reduces average time-to-resolution for configuration issues by surfacing actionable fixes without requiring documentation lookup.
- **SC-006**: Shell tab-completion covers 100% of top-level commands and their documented flags.
- **SC-007**: `--json` output passes schema validation in a JSON Schema linter with no errors.
- **SC-008**: All integrity violations (externally modified managed files) are detected and reported by `sth status` without false negatives.
- **SC-009**: `sth catalog init` produces a valid `SKILLS.md` for any conforming skills repository structure without manual editing.

## Assumptions

- Users are familiar with the existing sth CLI and have a working `project.json` already configured.
- The GitHub Actions OIDC implementation targets GitHub-hosted runners; other CI platforms (GitLab, Bitbucket) are out of scope for v1 CI auth.
- `.sthignore` follows standard gitignore pattern syntax; no custom extensions are introduced.
- Telemetry (opt-in anonymous usage counters) is explicitly out of scope for the initial roadmap implementation — it requires a separate privacy review before inclusion.
- Clickable terminal links (OSC 8 hyperlinks) are treated as a UI polish enhancement and are scoped as a low-priority addition after core features are stable.
- A "GitHub Action" (`sth-action`) for automated PR creation is considered infrastructure work and is tracked separately from the CLI feature set.
- Multi-repo disambiguation defaults to prompting the user interactively; a `--provider` flag handles CI disambiguation.
- SHA256 hashes are stored per file (not per bundle) in `sth-state.json` to allow fine-grained integrity tracking.
- Version pinning uses the same `ref` field in `project.json` that already identifies a resource; an additional `version` sub-field specifies the pin.
