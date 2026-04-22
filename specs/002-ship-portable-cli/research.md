# Research: Ship Portable CLI

## Decision 1: Publish release artifacts from a dedicated GitHub Actions workflow

- **Decision**: Add a dedicated `release.yml` workflow that builds and publishes LoreSkills release artifacts from GitHub Actions, separate from the existing `ci.yml` verification workflow.
- **Rationale**: The repository already has a lightweight cross-OS CI workflow. Keeping release publication separate avoids overloading CI semantics, preserves fast PR validation, and gives a clean place to assemble archives, checksums, and release attachments.
- **Alternatives considered**: Reusing `ci.yml` for publication was rejected because it couples validation and release side effects; manual local release creation was rejected because it does not satisfy the automated GitHub Actions requirement.

## Decision 2: Use an MVP artifact matrix of four static binaries with platform-native archive formats

- **Decision**: Publish `windows/amd64`, `linux/amd64`, `darwin/amd64`, and `darwin/arm64` artifacts, using `.zip` for Windows and `.tar.gz` for Linux/macOS, all built with `CGO_ENABLED=0`.
- **Rationale**: This matches the clarified scope, covers the most common install targets, and keeps archive handling familiar for users on each platform. Static builds align with the constitution and avoid OS-specific runtime dependencies.
- **Alternatives considered**: Publishing only raw binaries was rejected because archives package better with checksums and documentation; adding every `arm64` variant immediately was rejected as broader than the clarified MVP.

## Decision 3: Inject build metadata at compile time for the `version` command

- **Decision**: Add build metadata variables in `cmd/` populated through Go linker flags during build/release so `loreskills version` can report at least version, target, and commit/build context.
- **Rationale**: This keeps application code stdlib-only, avoids a new runtime configuration source, and makes release artifacts self-describing. The information is available both in local dev builds and in GitHub-published binaries.
- **Alternatives considered**: Hardcoding version strings in source was rejected because it is error-prone and conflicts with automated releases; omitting build metadata beyond a bare version was rejected because it weakens release traceability.

## Decision 4: Centralize CLI help data in a command catalog inside `cmd/`

- **Decision**: Represent the visible commands (`init`, `install`, `update`, `help`, `version`) and their help metadata from a single catalog in `cmd/`, then derive root help, contextual help, and invalid-command guidance from that catalog.
- **Rationale**: The current root help is hand-written and would drift once `help` and `version` are added. A single source of truth keeps the help contract aligned with the routed commands and makes tests straightforward.
- **Alternatives considered**: Maintaining separate hardcoded help strings per code path was rejected because it creates drift risk; introducing an external CLI framework was rejected by the constitution.

## Decision 5: Render the ASCII guide through a shared presentation helper applied to every visible output

- **Decision**: Add a compact presentation helper in `cmd/` that prepends the same ASCII guide block to all visible stdout/stderr user messages while keeping error text and next actions readable.
- **Rationale**: The spec explicitly requires the avatar on every visible CLI output. A shared renderer keeps the output consistent across success, help, no-op, and error flows without changing package boundaries.
- **Alternatives considered**: Printing the avatar manually in each command was rejected because it is brittle and hard to test; restricting the avatar to onboarding-only moments was rejected because clarification explicitly chose all command outputs.

## Decision 6: Document per-OS global installation and checksum verification in the README and release guidance

- **Decision**: Treat `README.md` plus release attachments/notes as the canonical MVP installation path, including OS-specific global install steps and SHA256 verification commands.
- **Rationale**: Users need one obvious source of truth from the repository and release page. This satisfies the feature without introducing package manager distribution in the same iteration.
- **Alternatives considered**: Adding `brew`, `winget`, or Linux package repositories now was rejected as larger operational scope; publishing checksums without verification guidance was rejected because the clarification requires documented verification steps.