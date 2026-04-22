# Research: Private Repo Skills

## Decision 1: Move repository setup into a project-local profile under `.loreskills/`

- **Decision**: Replace the home-scoped repository configuration for this feature with a project-local profile stored under `.loreskills/project.json`, and treat `~/.loreskillsrc` only as an explicit one-time migration source during `init`.
- **Rationale**: The current `internal/fs` configuration model applies one `repo_raw_url` and one `target_dir` to every project, which conflicts with the feature requirement for project-specific repository setup. A dedicated project-local profile keeps repository identity, target folder, and catalog context tied to the current workspace while preserving a clear migration path from the existing file.
- **Alternatives considered**: Keeping the home-scoped file as an automatic fallback was rejected because it would continue leaking settings across projects. Storing repository configuration inside the managed install directory was rejected because `target_dir` contains generated artifacts and should not become the authoritative project profile location.

## Decision 2: Resolve private repository access through external credentials, not secrets stored in the project profile

- **Decision**: Keep credentials outside `.loreskills/project.json`, with resolution ordered through environment variables first and existing Git credential helpers or OS-backed stores second.
- **Rationale**: The feature explicitly forbids storing private access secrets in the project file, and the constitution disallows third-party SDKs. Using external credentials preserves the stdlib-only rule, supports GitHub, GitLab, Azure DevOps, and Bitbucket, and aligns with environments where users already authenticate through provider login or Git Credential Manager.
- **Alternatives considered**: Storing PATs or tokens in the project profile was rejected because it creates secret-sprawl and violates the clarified credential model. Adding provider SDKs or external login libraries was rejected by the constitution's zero-dependency rule.

## Decision 3: Use `SKILLS.md` as the only remote catalog, with a dedicated machine-parsable catalog table

- **Decision**: Treat `SKILLS.md` as the sole source of truth for remote discovery and require a canonical catalog section that yields `type`, `name`, `folder`, `source_path`, and optional description for each installable resource.
- **Rationale**: The feature requires LoreSkills to list skills, agents, and other resources remotely while still reporting folder names. A dedicated catalog section inside `SKILLS.md` keeps discovery and human documentation in one file, remains stdlib-parsable, and gives enough metadata to disambiguate duplicate names across folders.
- **Alternatives considered**: Walking repository folders directly was rejected because it bypasses `SKILLS.md`, which the feature explicitly names as the catalog source. Introducing a second JSON or YAML catalog file was rejected because it would drift from the Markdown source of truth.

## Decision 4: Track installed resources by catalog identity, not by bare resource name

- **Decision**: Expand `lore-state.json` from a flat `skill_name -> installed` map to records keyed by catalog identity, combining resource type, folder name, and resource name.
- **Rationale**: The existing state model works only for flat skill names and cannot represent agents, other resource types, or duplicate names across folders. Catalog-aware state keeps updates deterministic and lets `update` refresh the exact remote artifact originally installed.
- **Alternatives considered**: Keeping the current flat map was rejected because folder-aware installs become ambiguous immediately. Storing only destination file paths was rejected because update logic still needs stable source identity from the catalog.

## Decision 5: Add an explicit `list` command and repurpose `init` as the interactive setup and migration wizard

- **Decision**: Keep `init`, `install`, `update`, `help`, and `version`, add a visible `list` command for remote discovery, and turn `init` into the interactive project setup and migration flow.
- **Rationale**: The current command set lacks a remote discovery step, yet the feature requires users to inspect available skills, agents, and other resources before installation. Reusing `init` for project onboarding minimizes command sprawl, while `list` provides a clear read-only catalog surface.
- **Alternatives considered**: Making `help` interactive was rejected because it blurs documentation and setup behavior. Creating a separate `setup` command was rejected because it duplicates the role of `init` without adding functional value.

## Decision 6: Implement stable-only self-update from build-injected release metadata, with upgrade execution under `update self`

- **Decision**: Inject the official LoreSkills release channel metadata at build time, use a stable-only GitHub release lookup that ignores pre-releases, and execute CLI binary upgrade through `loreskills update self` rather than silent replacement on arbitrary commands.
- **Rationale**: The feature requires stable-only release checks, but the constitution forbids hardcoded runtime repository identifiers. Build-time release metadata preserves that rule, while an explicit self-update subcommand avoids unsafe silent binary replacement and keeps the bare `loreskills` and `version` outputs limited to the requested signature.
- **Alternatives considered**: Hardcoding the GitHub repository or release URL in source was rejected by the constitution. Silently replacing the binary on every command was rejected because it is brittle across platforms, especially on Windows, and would conflict with the minimal root output requirement.

## Decision 7: Remove the old animal guide from standard flows and isolate the Stockholm artwork behind a hidden command

- **Decision**: Standard command output becomes plain text without the existing animal ASCII guide, while a hidden `stockholm` command renders the new artwork on demand.
- **Rationale**: The feature explicitly removes the existing mascot from standard command flows and narrows the bare `loreskills` output to a version signature. Isolating the new artwork behind a hidden command preserves the requested easter egg without reintroducing visual noise into the standard CLI.
- **Alternatives considered**: Keeping the old animal on help, install, or update flows was rejected because the request removes that mascot from standard LoreSkills output. Replacing every standard output with the Stockholm artwork was rejected because it would conflict with the requested minimal default presentation.
