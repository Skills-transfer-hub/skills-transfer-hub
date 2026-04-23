# Feature Specification: Guided Repository Onboarding

**Feature Branch**: `[005-rework-repo-onboarding]`  
**Created**: 2026-04-23  
**Status**: Draft  
**Input**: User description: "We need to rework entirely how LoreSkills helps users configure a repository. The tool must guide users through provider-specific API key setup, include the right documentation URLs, validate a compatible repository structure based on `<category>/<skill-folder>/SKILL.md` without depending on a rigid list of category names, protect managed plugin code, provide a more interactive menu when users start LoreSkills, and set up automatic updates for the tool."

## Clarifications

### Session 2026-04-23

- Q: How should the main `loreskills` entry point behave? → A: `loreskills` opens the interactive menu in a TTY; in non-interactive contexts it prints concise next-step guidance.
- Q: How should LoreSkills handle local modifications in managed plugin content during install or update? → A: Ignore local modifications in managed content and always overwrite with the repository version.
- Q: How should LoreSkills discover compatible resources in a repository? → A: Support both: use `SKILLS.md` when present, otherwise fall back to scanning `<category>/<skill-folder>/SKILL.md` automatically.
- Q: Which provider hosts are in scope for this feature? → A: Support only the hosted provider URLs in this feature: `github.com`, `gitlab.com`, `dev.azure.com`, and `bitbucket.org`.
- Q: How often should LoreSkills check for updates automatically? → A: Check for updates when `loreskills` starts in interactive mode, but no more than once per 24 hours per machine.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Configure a Provider Without External Guesswork (Priority: P1)

As a first-time LoreSkills user, I want a guided setup flow for each supported provider so that I can generate the right API key, open the right provider page, fill the right repository fields, and finish configuration without leaving the product confused.

**Why this priority**: Repository configuration is the first blocker for adoption. If users cannot complete setup confidently, every later capability becomes irrelevant.

**Independent Test**: Start from a project with no LoreSkills configuration, choose each supported provider in turn, and verify that the guided flow explains the required credentials, opens or displays the correct provider URL, captures the needed repository details, validates access, and tells the user what to do next.

**Acceptance Scenarios**:

1. **Given** a user starts LoreSkills in a project that is not configured, **When** the setup guide begins, **Then** the tool explains the supported providers and the information the user must gather before configuration.
2. **Given** a user selects a supported provider, **When** the provider guide is shown, **Then** the tool displays step-by-step instructions for creating the required API key or token, the exact provider URL to visit, the minimum access needed, and which values must be entered into LoreSkills.
3. **Given** a user enters incomplete or invalid repository access details, **When** validation runs, **Then** the tool identifies which step failed and tells the user how to correct it.

---

### User Story 2 - Publish Skills in a Compatible and Safe Repository Layout (Priority: P1)

As a repository maintainer, I want LoreSkills to recognize compatible skill repositories even when category names vary, while still enforcing a safe structure for managed content, so that teams can organize repositories flexibly without breaking installation or risking managed plugin files.

**Why this priority**: The repository contract determines whether LoreSkills can reliably discover and install resources. Flexibility matters, but only if safety and predictability remain intact.

**Independent Test**: Validate repositories that use categories such as `.net`, `agents`, `angular`, `vendredi`, `commands`, `common`, `react`, and additional custom category names, then confirm that LoreSkills accepts every resource that follows the compatible layout, can discover resources through `SKILLS.md` or automatic structure scanning, rejects unsafe layouts, and overwrites managed tracked files with the repository version when the destination remains inside the safe managed boundary.

**Acceptance Scenarios**:

1. **Given** a repository contains resources organized as `<category>/<skill-folder>/SKILL.md`, **When** LoreSkills validates compatibility, **Then** it accepts the repository even if the category name is not on a predefined list.
2. **Given** a repository contains a valid `SKILLS.md`, **When** LoreSkills discovers resources, **Then** it uses that catalog as the primary source of repository entries.
3. **Given** a repository has no `SKILLS.md` but does contain resources organized as `<category>/<skill-folder>/SKILL.md`, **When** LoreSkills discovers resources, **Then** it falls back to automatic repository scanning.
4. **Given** a repository entry points outside the expected layout or toward an unsafe destination, **When** LoreSkills prepares installation or update, **Then** it blocks the action and explains how the repository must be corrected.
5. **Given** LoreSkills is about to replace managed plugin files that were locally modified but still resolve inside the managed install boundary, **When** the install or update runs, **Then** the tool overwrites those files with the repository version.

---

### User Story 3 - Know What To Do From the Main LoreSkills Experience (Priority: P2)

As a user opening LoreSkills, I want `loreskills` to open an interactive, state-aware menu in a terminal session and fall back to concise next-step guidance in non-interactive contexts so that I do not need prior command knowledge to configure a provider, browse skills, install resources, or troubleshoot problems.

**Why this priority**: The current entry experience does not guide users toward the next step. Improving discoverability reduces setup friction and support overhead.

**Independent Test**: Launch LoreSkills in an unconfigured project, a partially configured project, and a ready-to-use project, and confirm that the main experience changes its suggested actions and lets the user start the next relevant workflow directly.

**Acceptance Scenarios**:

1. **Given** a project has no LoreSkills configuration, **When** the user starts LoreSkills, **Then** the main experience highlights provider setup as the primary next step.
2. **Given** a project is configured and reachable, **When** the user starts LoreSkills, **Then** the main experience highlights actions such as browse resources, install, update, and review configuration.
3. **Given** LoreSkills is launched in an interactive terminal session, **When** the user runs `loreskills`, **Then** the tool opens the guided menu instead of requiring a separate command.
4. **Given** LoreSkills is launched in a non-interactive environment, **When** an interactive menu cannot be shown, **Then** the tool falls back to concise guidance that still tells the user the next available action.

---

### User Story 4 - Stay Current Through Guided Automatic Updates (Priority: P2)

As a LoreSkills user, I want the tool to detect stable updates automatically when I start `loreskills` interactively, without checking more than once every 24 hours per machine, and let me apply them from the same guided experience so that I stay on a current release without unnecessary network checks or manual GitHub monitoring.

**Why this priority**: Keeping the CLI current protects users from stale setup guidance and outdated behavior, especially when provider requirements change.

**Independent Test**: Use release feeds containing current, newer stable, and newer pre-release versions, then confirm that LoreSkills automatically checks for updates at interactive startup, skips redundant checks within 24 hours on the same machine, ignores pre-releases, offers the correct stable update, and preserves the current installation if the update cannot be completed.

**Acceptance Scenarios**:

1. **Given** a newer stable release exists, **When** LoreSkills performs its automatic update check, **Then** the user is informed that an update is available and can start it from the guided experience.
2. **Given** only a newer pre-release exists, **When** LoreSkills performs its automatic update check, **Then** it keeps the current stable installation and reports that no newer stable update is available.
3. **Given** LoreSkills has already completed an automatic update check on the same machine within the last 24 hours, **When** the user starts `loreskills` interactively again, **Then** the tool skips the network update check and reuses the existing update status until the interval expires.
4. **Given** an automatic update cannot be completed because of permissions, connectivity, or integrity checks, **When** the update attempt ends, **Then** LoreSkills leaves the current installation intact and provides the next recovery step.

### Edge Cases

- What happens if a provider changes the page or process used to create API keys? LoreSkills must detect outdated guidance during maintenance reviews and fall back to clear manual instructions instead of showing a broken path.
- What happens if a user attempts to configure a self-hosted or unsupported provider URL? LoreSkills must reject it clearly and explain that this feature supports only `github.com`, `gitlab.com`, `dev.azure.com`, and `bitbucket.org`.
- What happens if a token is created successfully but does not have enough permission to read the target repository? LoreSkills must fail validation with a specific explanation of the missing access level.
- What happens if a repository uses category names not seen before but still follows the compatible layout? LoreSkills must treat the repository as valid rather than rejecting it because of an unknown category label.
- What happens if a repository contains both a `SKILLS.md` catalog and a scan-detectable structure, but the two disagree? LoreSkills must treat `SKILLS.md` as the primary source and explain any invalid entries it contains.
- What happens if repository metadata points to paths that escape the managed install area or would overwrite protected LoreSkills-managed content? LoreSkills must block the action before any file is changed.
- What happens if a user manually edits a LoreSkills-managed plugin file and later runs install or update? LoreSkills must treat the repository version as the source of truth and overwrite the local edits as long as the target path remains inside the managed boundary.
- What happens if a user has an older project configuration from a previous LoreSkills flow? The guided experience must explain what can be reused, what must be updated, and what happens next.
- What happens if LoreSkills starts while offline or without permission to modify its own installed binary? The tool must keep working, explain the skipped or stale update state, and avoid leaving the installation partially updated.
- What happens if the machine already checked for updates recently but the user expects a new check? LoreSkills must make the 24-hour automatic check interval clear and still allow the guided update path to show the current known status.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST provide a guided repository onboarding experience from the main `loreskills` entry point and from the dedicated setup workflow.
- **FR-002**: When LoreSkills is started in an interactive terminal session, running `loreskills` with no subcommand MUST open the interactive, state-aware guided menu.
- **FR-003**: When the current project is not configured, the default LoreSkills experience MUST prioritize configuration guidance over expecting the user to know the command sequence in advance.
- **FR-004**: When LoreSkills is started in a non-interactive context, running `loreskills` with no subcommand MUST print concise next-step guidance instead of attempting to open an interactive menu.
- **FR-005**: The onboarding experience MUST list every repository provider currently supported by LoreSkills and explain the information users need before starting configuration.
- **FR-006**: For each supported provider, the onboarding experience MUST provide step-by-step instructions for obtaining the required API key or access token, including the exact provider URL to visit, the minimum access level required, and the LoreSkills fields the user must fill.
- **FR-007**: The onboarding experience MUST let users review provider guidance again after initial setup without forcing them to restart the entire configuration flow.
- **FR-008**: The system MUST validate repository reachability and catalog access before confirming a project configuration and MUST identify the failing step when validation does not succeed.
- **FR-009**: The system MUST keep repository connection details project-specific and MUST NOT require private secrets to be stored in project configuration files.
- **FR-009a**: This feature MUST support only the hosted provider domains `github.com`, `gitlab.com`, `dev.azure.com`, and `bitbucket.org`.
- **FR-009b**: The onboarding flow MUST reject self-hosted or unsupported provider URLs with guidance that states the supported hosted domains.
- **FR-010**: The system MUST discover repository resources from `SKILLS.md` when that catalog is present.
- **FR-011**: If `SKILLS.md` is absent, the system MUST fall back to automatic discovery by scanning resources organized as `<category>/<skill-folder>/SKILL.md`.
- **FR-012**: When both `SKILLS.md` and scan-detectable repository structure are present, the system MUST treat `SKILLS.md` as the primary source of repository entries.
- **FR-013**: The system MUST recognize a compatible LoreSkills repository when a managed skill or plugin resource is organized as `<category>/<skill-folder>/SKILL.md`.
- **FR-014**: Repository compatibility checks MUST NOT depend on a fixed, closed list of category names and MUST continue to accept known examples such as `.net`, `agents`, `angular`, `vendredi`, `commands`, `common`, and `react` alongside other category names that follow the same structure.
- **FR-015**: The system MUST explain repository compatibility failures in terms of the expected layout and the exact resource that failed validation.
- **FR-016**: The system MUST protect LoreSkills-managed plugin files and other managed installed content from unsafe paths and any writes outside the managed installation boundary during install and update operations.
- **FR-017**: When an install or update targets existing LoreSkills-managed content inside the managed installation boundary, the system MUST overwrite local modifications with the repository version.
- **FR-018**: The main LoreSkills guided menu MUST tell the user the next useful action based on the current project state.
- **FR-019**: The interactive menu MUST adapt to at least these states: unconfigured project, partially configured project, and configured project ready for browsing or installation.
- **FR-020**: The system MUST automatically check for newer LoreSkills stable releases when `loreskills` starts in interactive mode.
- **FR-021**: Automatic update checks MUST run no more than once per 24 hours per machine.
- **FR-022**: Automatic update checks MUST ignore pre-releases when determining the latest available update.
- **FR-023**: When a newer stable release is available, the system MUST let the user start the update from the same guided experience without requiring a separate manual discovery workflow.
- **FR-024**: If an update cannot be completed, the system MUST preserve the current installation and explain the manual recovery path.
- **FR-025**: The system MUST keep provider guidance content aligned with every provider that is exposed in the product so that setup instructions, URLs, and expected access levels do not drift out of date.

### Key Entities *(include if feature involves data)*

- **Project Repository Configuration**: The project-specific record that captures which provider the project uses, which repository the project reads from, and the non-secret values needed to validate and browse that repository.
- **Provider Setup Guide**: The provider-specific onboarding content that explains where users go to create an API key or token, what access they must grant, and which values they must bring back into LoreSkills.
- **Supported Hosted Provider**: One of the four in-scope hosted endpoints for this feature: `github.com`, `gitlab.com`, `dev.azure.com`, or `bitbucket.org`.
- **Repository Discovery Source**: The mechanism LoreSkills uses to enumerate compatible resources, either the explicit `SKILLS.md` catalog or the automatic repository structure scan fallback.
- **Compatible Repository Resource**: A skill or plugin resource that follows the LoreSkills repository layout contract and is therefore safe to validate, display, install, and update.
- **Repository Category**: The top-level grouping label that organizes compatible resources, such as `.net`, `agents`, `angular`, `vendredi`, `commands`, `common`, `react`, or another valid category name.
- **Managed Plugin Content**: Files installed or updated by LoreSkills that must remain within protected boundaries and are treated as repository-owned managed content during later installs and updates.
- **Guided Action Menu**: The state-aware main LoreSkills experience that presents the next recommended action and entry points into setup, browsing, installation, update, and troubleshooting flows.
- **Stable Update Offer**: The current update status shown to the user after automatic checks determine whether a newer stable LoreSkills release is available.
- **Update Check State**: The machine-local record of when LoreSkills last performed an automatic stable update check and what the latest known update status was.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 90% of first-time users can complete repository setup for a supported provider in 7 minutes or less without needing external documentation.
- **SC-002**: 100% of supported provider onboarding flows show a valid provider URL, the correct minimum access requirement, and the exact input values the user must provide to LoreSkills.
- **SC-003**: 95% of users can identify the next useful action within 15 seconds of starting LoreSkills in each tested project state.
- **SC-004**: In repository compatibility validation, 100% of repositories that follow the `<category>/<skill-folder>/SKILL.md` contract are accepted, and 100% of tested unsafe or incompatible layouts are rejected before any managed content is changed.
- **SC-005**: 100% of automatic update checks ignore pre-releases and surface the newest stable update when one exists.
- **SC-005**: 100% of automatic update checks ignore pre-releases and surface the newest stable update when one exists.
- **SC-006**: 90% of users who accept an available stable update can complete the guided update flow in under 3 minutes without manual release browsing.
- **SC-007**: In validation runs, LoreSkills performs at most one automatic update check per 24-hour window on the same machine during interactive startup.

## Assumptions

- The supported providers for this feature remain the hosted services GitHub (`github.com`), GitLab (`gitlab.com`), Azure DevOps (`dev.azure.com`), and Bitbucket (`bitbucket.org`) unless LoreSkills explicitly adds more providers later.
- The repository catalog and installation model remain project-scoped, even though the onboarding and menu experience are being reworked.
- `SKILLS.md` remains the preferred explicit catalog format when present, while repository structure scanning acts as a compatibility fallback for repositories that omit it.
- "Protect my plugin code" refers to protecting LoreSkills-managed installed plugin content and managed paths from unsafe replacement or writes outside the managed boundary, not to preserving local edits inside managed content or adding source control permissions or digital rights enforcement.
- The first release of this feature performs automatic stable update checks only during interactive startup, caches that check per machine for 24 hours, and still allows users to confirm the update rather than silently replacing the binary without notice.
- LoreSkills-managed plugin content is treated as generated or synced repository content, so local edits to that managed content are considered replaceable on the next install or update.
- Existing projects may contain older LoreSkills configuration that needs to be reviewed or migrated during onboarding.