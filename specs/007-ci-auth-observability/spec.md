# Feature Specification: CI, Auth & Observability

**Feature Branch**: `006-cli-product-roadmap`  
**Created**: 2026-04-23  
**Status**: Draft  
**Depends On**: [005-state-config-foundation](../005-state-config-foundation/spec.md)  
**Blocks**: —  

## Context

This spec addresses three related areas that all centre on how loreskills handles
credentials and communicates with users — in interactive terminals, in CI pipelines,
and when things go wrong:

1. **Credential model**: The current tool accepts a token once during `init` but never
   re-prompts on subsequent auth failures. In CI environments there is no interactive
   session at all. This spec defines where tokens come from and what happens when they
   expire.
2. **Observability**: Debugging a misconfigured project today requires source-level
   investigation. `--verbose`, `loreskills doctor`, and `--json` output provide
   structured, actionable information without reading source code.
3. **CI integration**: `--fail-on-changes` enables loreskills to act as a gate in
   automated pipelines.
4. **Shell completion**: Makes the tool feel first-class in any developer shell.

---

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Recover from an Expired Token Without Restarting (Priority: P1)

A developer runs `loreskills update` mid-session. The request returns `401` because
their token has expired. Instead of a bare error message, the tool detects the auth
failure, explains it clearly, and opens the token re-entry prompt in the same terminal
session. After entering a valid token, the original command continues.

**Why this priority**: Auth recovery in the same session is the most common friction
point. A dead end at a `401` drives users away.

**Independent Test**: Configure an invalid token, run any authenticated command,
verify the interactive re-entry prompt appears; enter a valid token, verify the
command completes successfully.

**Acceptance Scenarios**:

1. **Given** a stored token returns `401`, **When** any network command runs,
   **Then** the tool displays "Authentication failed (401) — your token may have expired"
   and opens the token input prompt.
2. **Given** a stored token returns `403`, **When** any network command runs,
   **Then** the tool displays "Access denied (403) — check that your token has
   the required permissions" and opens the token input prompt.
3. **Given** the user enters a valid token at the recovery prompt, **When** the input
   is confirmed, **Then** the tool retries the original request with the new token
   and continues to completion.
4. **Given** the user dismisses the recovery prompt (Ctrl+C or empty input), **When**
   they exit, **Then** the tool exits cleanly with code 1 and a message explaining
   the command did not complete.
5. **Given** the replacement token is also invalid, **When** the retry also returns
   `401`/`403`, **Then** the tool exits with a non-zero code instead of looping
   indefinitely.

---

### User Story 2 — Credential Model: Where Tokens Come From (Priority: P1)

A developer and their CI system both use loreskills but through different credential
paths. The developer has a token in the `LORESKILLS_TOKEN` environment variable; CI
uses GitHub Actions OIDC. The tool resolves the right credential for each context
automatically.

**Why this priority**: Without a clear, documented credential precedence, every team
invents a different workaround. This unblocks all authenticated use cases.

**Independent Test**: Set `LORESKILLS_TOKEN` in the environment and run any authenticated
command without going through `init`; verify the env var token is used. Then unset it
and verify the interactive prompt appears.

**Acceptance Scenarios**:

1. **Given** `LORESKILLS_TOKEN` is set in the environment, **When** any authenticated
   command runs, **Then** the tool uses that token without prompting.
2. **Given** `LORESKILLS_TOKEN` is not set and the tool is in an interactive (TTY)
   session, **When** an authenticated command needs a token, **Then** the tool prompts
   the user to enter one (session-only, not persisted to disk).
3. **Given** `LORESKILLS_TOKEN` is not set and `ACTIONS_ID_TOKEN_REQUEST_URL` is
   present in the environment, **When** any authenticated command runs, **Then** the
   tool exchanges the GitHub Actions OIDC token automatically and uses it — no prompt
   is shown.
4. **Given** none of the above credential sources are available in a non-TTY
   environment, **When** any authenticated command runs, **Then** the tool exits with
   a non-zero code and a message explaining how to provide credentials (env var or
   OIDC).
5. **Given** a credential is sourced from any path, **Then** it MUST NOT be written
   to any file on disk — all credentials are session-only.

**Credential resolution order** (highest to lowest priority):
1. `LORESKILLS_TOKEN` environment variable
2. GitHub Actions OIDC (when `ACTIONS_ID_TOKEN_REQUEST_URL` is set)
3. Interactive TTY prompt (session-only)
4. Fail with error in non-TTY environments with no other source

---

### User Story 3 — Debug Any Problem with `--verbose` (Priority: P1)

A developer encounters unexpected behaviour (wrong files installed, network errors,
stale data). They re-run the failing command with `--verbose` and immediately see each
HTTP request, its status code, every resolved file path, and each decision the tool
makes — all on stderr, leaving stdout clean.

**Why this priority**: Without verbose output, debugging requires adding print
statements to the binary. This is a zero-friction diagnostic that every developer
expects from a production CLI.

**Independent Test**: Run any network command with `--verbose`, verify that at minimum
the request URL and response status code appear on stderr; verify stdout contains only
the normal command output.

**Acceptance Scenarios**:

1. **Given** `--verbose` is passed, **When** the tool makes an HTTP request, **Then**
   the request URL and response HTTP status code are printed to stderr.
2. **Given** `--verbose` is passed, **When** the tool resolves a file path (config,
   state, install target), **Then** each resolved absolute path is printed to stderr.
3. **Given** `--verbose` is not passed, **When** any command runs, **Then** no
   diagnostic information appears on stderr.
4. **Given** `--debug` is passed (alias for `--verbose`), **When** any command runs,
   **Then** behaviour is identical to `--verbose`.
5. **Given** `--verbose` is active and an error occurs, **When** the error is printed,
   **Then** it appears on stderr with the normal error formatting.

---

### User Story 4 — Diagnose Configuration with `loreskills doctor` (Priority: P1)

A developer onboards a new machine and suspects loreskills is misconfigured. They run
`loreskills doctor` and get a pass/fail checklist covering every aspect of the setup:
token validity, config file integrity, provider reachability, and state file health.
Failing items include a specific remediation step.

**Why this priority**: Configuration errors today require manual investigation of JSON
files and HTTP endpoints. `doctor` surfaces and solves them without documentation lookup.

**Independent Test**: Break one item (e.g., invalid token, missing config file), run
`loreskills doctor`, verify the specific failing check appears with a remediation step.

**Acceptance Scenarios**:

1. **Given** all configuration is valid, **When** the user runs `loreskills doctor`,
   **Then** each check is listed with `[✓]` and the command ends with "All checks passed."
2. **Given** `LORESKILLS_TOKEN` (or OIDC) resolves to an invalid credential,
   **When** `loreskills doctor` runs, **Then** the "Credential validity" check fails
   with `[✗]` and the remediation reads: "Set a valid LORESKILLS_TOKEN or run
   `loreskills init`."
3. **Given** `project.json` is missing or corrupt, **When** `loreskills doctor` runs,
   **Then** the "Project configuration" check fails with a message pointing to the
   exact file path and "Run `loreskills init` to create it."
4. **Given** a configured provider is unreachable (network timeout or 404),
   **When** `loreskills doctor` runs, **Then** the "Provider reachability" check fails
   for that provider with the URL and "Check network access and provider credentials."
5. **Given** `lore-state.json` is corrupt (invalid JSON), **When** `loreskills doctor`
   runs, **Then** the "State file integrity" check fails with "Delete
   `.loreskills/lore-state.json` and re-install your resources."
6. **Given** `loreskills doctor` is run in a directory with no `project.json`,
   **When** the command runs, **Then** the "Project configuration" check fails immediately
   and all provider-specific checks are skipped with "[SKIP] No project configured."

---

### User Story 5 — CI Pipeline Gate with `--fail-on-changes` (Priority: P2)

A CI pipeline runs `loreskills update --fail-on-changes` as a quality gate. If any
managed file has been locally modified (SHA256 mismatch), the command exits with a
non-zero code, causing the pipeline to fail and alerting the team that a managed skill
was hand-edited.

**Why this priority**: Teams maintaining shared skill libraries need a way to enforce
that managed files are not manually edited. This is the CI equivalent of `git status --porcelain`.

**Independent Test**: Modify a managed file, run `loreskills update --fail-on-changes`,
verify exit code is non-zero and the modified file path is printed.

**Acceptance Scenarios**:

1. **Given** one or more managed files have been locally modified, **When**
   `loreskills update --fail-on-changes` runs, **Then** the tool exits with code 1 and
   lists the modified files.
2. **Given** no managed files have been modified, **When**
   `loreskills update --fail-on-changes` runs, **Then** the tool performs the update
   normally and exits with code 0.
3. **Given** `--fail-on-changes` is combined with `--dry-run`, **When** modified files
   exist, **Then** the tool exits with code 1 and lists the modified files without
   making any changes.

---

### User Story 6 — Machine-Readable Output with `--json` (Priority: P2)

A CI engineer pipes `loreskills list --json` into a script that builds a dashboard
of installed resources. They also use `loreskills status --json` to check for drift
in a monitoring workflow. The JSON output is stable across patch versions.

**Why this priority**: Scripting against human-readable output is fragile. A documented
JSON schema makes loreskills a composable component in any automation pipeline.

**Independent Test**: Run `loreskills list --json`, pipe to a JSON validator, verify
it parses without error. Run again after a patch upgrade and verify field names have
not changed.

**Acceptance Scenarios**:

1. **Given** `--json` is passed to `loreskills list`, **When** the command runs,
   **Then** stdout is a JSON array of resource objects with fields: `resource_name`,
   `provider_id`, `status`, `resolved_version`, `pinned_version` (null if absent).
2. **Given** `--json` is passed to `loreskills status`, **When** the command runs,
   **Then** stdout is a JSON array of status objects with fields: `resource_name`,
   `state` (one of the status enum values), `modified_files` (array, may be empty).
3. **Given** `--json` is passed to `loreskills version`, **When** the command runs,
   **Then** stdout is a JSON object with fields: `version`, `commit` (build SHA if
   available, otherwise `"unknown"`).
4. **Given** an error occurs with `--json` enabled, **When** the command fails,
   **Then** stdout is a JSON object with fields `error` (string message) and
   `code` (integer), and the process exits with a non-zero code.
5. **Given** `--json` is not passed, **When** an error occurs, **Then** the error
   is printed in human-readable format to stderr (existing behaviour, unchanged).

---

### User Story 7 — Shell Tab Completion with `loreskills completion` (Priority: P2)

A developer adds loreskills tab completion to their shell once and then enjoys
`loreskills <TAB>` completing commands and flags for all future sessions. An install
hint is shown during `loreskills init`.

**Why this priority**: Tab completion is a force-multiplier for CLI usability. It also
serves as live documentation for available commands and flags.

**Independent Test**: Generate a bash completion script, source it, type
`loreskills <TAB>`, verify the full list of top-level commands appears.

**Acceptance Scenarios**:

1. **Given** the user runs `loreskills completion bash`, **When** the output is
   sourced in bash, **Then** `loreskills <TAB>` lists all top-level commands.
2. **Given** the user runs `loreskills completion zsh`, **When** the output is
   sourced in zsh, **Then** `loreskills <TAB>` lists all top-level commands.
3. **Given** the user runs `loreskills completion powershell`, **When** the output
   is dot-sourced in PowerShell, **Then** `loreskills <TAB>` lists all top-level
   commands.
4. **Given** a top-level command is selected via completion, **When** the user presses
   `<TAB>` again, **Then** the flags for that command are listed.
5. **Given** `loreskills init` completes, **When** the output is shown, **Then** a
   hint is printed: "Run `loreskills completion <shell>` to enable tab completion."
6. **Given** hidden commands exist in the binary, **When** tab completion runs,
   **Then** those hidden commands MUST NOT appear in completion suggestions.

---

### Edge Cases

- What if the OIDC token exchange endpoint (`ACTIONS_ID_TOKEN_REQUEST_URL`) is unreachable? → The tool falls back to interactive prompt if TTY is available; otherwise exits non-zero with "OIDC exchange failed: <reason>".
- What if a token retry also fails (second `401`)? → The tool exits with code 1, does not loop. Maximum one retry per command invocation.
- What if `loreskills doctor` is run with no network access? → Provider reachability checks fail with "unreachable (timeout)"; all other local checks still run.
- What if a `--json` command produces no results (empty list)? → Output is an empty JSON array `[]`, not `null` or an error.
- What if completion is requested for an unsupported shell? → The tool exits with "Unsupported shell: <name>. Supported: bash, zsh, powershell."
- What if `--verbose` and `--json` are both passed? → Verbose output goes to stderr only; stdout still contains clean JSON.

---

## Requirements *(mandatory)*

### Functional Requirements

**Credential Model**

- **FR-001**: The tool MUST resolve credentials in this priority order: (1) `LORESKILLS_TOKEN` env var, (2) GitHub Actions OIDC when `ACTIONS_ID_TOKEN_REQUEST_URL` is set, (3) interactive TTY prompt, (4) error in non-TTY with no other source.
- **FR-002**: Credentials MUST NEVER be written to any file on disk.
- **FR-003**: The interactive token prompt MUST NOT appear in non-TTY environments.

**Auth Recovery**

- **FR-004**: When a network request returns `401` or `403` in an interactive (TTY) session, the tool MUST display the status and a human-readable explanation, then open the token re-entry prompt.
- **FR-005**: After re-entering a valid token, the tool MUST retry the original request once.
- **FR-006**: If the retry also returns `401`/`403`, the tool MUST exit with code 1 without looping.

**OIDC (GitHub Actions)**

- **FR-007**: When `ACTIONS_ID_TOKEN_REQUEST_URL` is set, the tool MUST exchange it for a bearer token using the standard GitHub Actions OIDC exchange and use that token for all subsequent requests.
- **FR-008**: If the OIDC exchange fails, the tool MUST exit with a clear error message including the failure reason and code 1.

**`--verbose` / `--debug`**

- **FR-009**: Both `--verbose` and `--debug` MUST be accepted as equivalent flags on all commands.
- **FR-010**: When active, each outbound HTTP request URL and its response status code MUST be printed to stderr.
- **FR-011**: When active, each resolved absolute file path (config, state, install target) MUST be printed to stderr.
- **FR-012**: Verbose/debug output MUST go exclusively to stderr; stdout MUST contain only normal command output.

**`loreskills doctor`**

- **FR-013**: The command MUST check and report pass/fail for: credential validity, project config file existence and parseability, state file existence and parseability, and provider reachability (HTTP HEAD or catalog fetch) for each configured provider.
- **FR-014**: Each failing check MUST include a remediation step in the output.
- **FR-015**: Checks MUST run independently; a failure in one check MUST NOT prevent other checks from running.
- **FR-016**: If no `project.json` exists, provider and state checks MUST be skipped and marked `[SKIP]`.

**`--fail-on-changes`**

- **FR-017**: `loreskills update --fail-on-changes` MUST exit with code 1 when any managed file has a SHA256 mismatch vs the stored hash.
- **FR-018**: The list of modified files MUST be printed before exiting.
- **FR-019**: `--fail-on-changes` combined with `--dry-run` MUST check for modifications and exit non-zero if found, without making any changes.

**`--json`**

- **FR-020**: `loreskills list`, `loreskills status`, and `loreskills version` MUST support `--json`.
- **FR-021**: When `--json` is active, all output to stdout MUST be valid JSON following the schemas defined in the contracts.
- **FR-022**: When `--json` is active and an error occurs, stdout MUST be a JSON object `{"error": "<message>", "code": <int>}` and the exit code MUST be non-zero.

**`loreskills completion`**

- **FR-023**: The command MUST accept `bash`, `zsh`, and `powershell` as arguments and emit the corresponding completion script to stdout.
- **FR-024**: For unsupported shells, the command MUST exit with code 1 and a clear error.
- **FR-025**: Hidden commands MUST NOT appear in any generated completion script.
- **FR-026**: `loreskills init` completion MUST display a hint about running `loreskills completion <shell>`.

### Key Entities

- **CredentialSource**: The resolved origin of an auth token — `env_var`, `oidc`, `interactive`, or `none`.
- **DoctorCheck**: A named diagnostic check with a `status` (`pass`/`fail`/`skip`) and optional `remediation` string.

---

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A developer can recover from a `401` auth failure and complete the original command in a single terminal session — zero need to restart the tool.
- **SC-002**: In a GitHub Actions environment with OIDC configured, `loreskills update` runs to completion with zero manual credential setup steps.
- **SC-003**: `loreskills doctor` identifies the specific misconfigured item in 100% of the test scenarios defined in the acceptance criteria above, each with a remediation step.
- **SC-004**: `--json` output on `loreskills list`, `loreskills status`, and `loreskills version` passes JSON Schema validation with zero errors.
- **SC-005**: Tab completion covers 100% of documented top-level commands and their flags in bash, zsh, and PowerShell.
- **SC-006**: `--verbose` output contains at minimum the HTTP request URL and status code for every network call — verifiable by running with a mock server.

---

## Assumptions

- The v2 config/state schema from spec 005 is in place.
- `LORESKILLS_TOKEN` is the canonical environment variable for passing a token; no other env var names are supported.
- The GitHub Actions OIDC exchange targets the GitHub API only; other OIDC providers (GitLab, Bitbucket CI) are out of scope for this spec.
- "Interactive TTY" is detected using standard OS mechanisms (checking if stdin is a terminal); no third-party library is used.
- The JSON output schemas defined in this spec are versioned at `v1`; backward-incompatible schema changes require a major version bump and are out of scope.
- `loreskills doctor` performs live network checks (provider reachability), which may be slow on poor connections; no timeout configuration is required for v1 of this feature.
- Completion scripts are generated programmatically from the command registry — they do not need to be manually maintained.
- OSC 8 clickable terminal links are out of scope for this spec (deferred to a later UI polish pass).
