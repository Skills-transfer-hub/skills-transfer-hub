# Feature Specification: Private Repo Skills

**Feature Branch**: `[004-private-repo-skills]`  
**Created**: 2026-04-22  
**Status**: Draft  
**Input**: User description: "My skills management tool needs an update. We’ll need to manage private repositories and explain how to connect to them. Additionally, the skills management file should not be shared across all projects but should be specific to the project where it is installed. We need to provide user guidance on how to set up a private Git repository, supporting GitHub, GitLab, Azure DevOps, and Bitbucket; the installation help command is interactive. We must be able to read the project remotely to list available skills, agents, and other resources. Note: we must rely on the SKILLS.md file and report the folder name to account for the installation skills. The ASCII output when running the LORESKILLS command should only display the version number; we’re removing the animal. We’ll add the developers’ names to the version, along with a message: “dev with 🤖 by Hugo MR and Jeremy, Lead Tech.” A hidden Stockholm command that displays an ASCII version of this image: https://em-content.zobj.net/source/microsoft/74/rolling-on-the-floor-laughing_1f923.png, which will appear throughout the site. I want the app to update automatically, so we’ll need to check the version on GitHub—specifically the release, not the pre-releases"

## Clarifications

### Session 2026-04-22

- Q: How should LoreSkills handle credentials for private repository access? → A: Store project repository settings locally, but obtain credentials from provider login, environment variables, or OS credential storage.

## User Scenarios & Testing _(mandatory)_

### User Story 1 - Connect a Project to a Private Repository (Priority: P1)

As a developer setting up LoreSkills inside a project, I want an interactive setup flow for a private repository so that this project can use its own secure source of skills and resources without inheriting another project's configuration.

**Why this priority**: Project-level isolation and private repository support are the core business changes in this request. Without them, teams cannot safely adopt LoreSkills across multiple repositories.

**Independent Test**: Start from a project with no local LoreSkills setup, walk through the interactive installation help flow, choose a supported provider, complete project setup, and verify that the project now has its own repository connection details without reusing another project's settings.

**Acceptance Scenarios**:

1. **Given** a project that has no LoreSkills repository setup, **When** a user starts the interactive installation help flow, **Then** the tool explains the supported private repository providers and captures the connection information needed for that project.
2. **Given** a project that is already connected to a private repository, **When** the user reviews or updates the setup, **Then** only that project's management settings are shown and modified.
3. **Given** a user selects GitHub, GitLab, Azure DevOps, or Bitbucket during setup, **When** the flow completes, **Then** the tool explains how to grant LoreSkills access to the chosen private repository and what the next step is.

---

### User Story 2 - Discover Installable Resources from the Remote Catalog (Priority: P1)

As a developer connected to a repository, I want LoreSkills to read the remote project and show me its available skills, agents, and related resources so that I can decide what to install without browsing the repository manually.

**Why this priority**: Private repository support is only useful if users can discover what the repository offers. The remote catalog is the decision point before installation.

**Independent Test**: Connect a project to a repository that publishes a `SKILLS.md` catalog, request a listing from LoreSkills, and verify that the returned catalog matches the remote file and includes the folder name needed to understand installation placement.

**Acceptance Scenarios**:

1. **Given** a project is connected to a reachable repository with a valid `SKILLS.md`, **When** the user asks to view available resources, **Then** LoreSkills reads the remote project and lists the skills, agents, and other resources declared in that catalog.
2. **Given** the catalog contains installable resources in multiple folders, **When** LoreSkills lists those resources, **Then** each entry includes the folder name associated with its installation source.
3. **Given** the remote repository is unreachable, access is denied, or `SKILLS.md` cannot be interpreted, **When** the user requests the catalog, **Then** LoreSkills returns a clear error that explains what blocked discovery and what the user should check.

---

### User Story 3 - Keep the CLI on the Latest Stable Release (Priority: P2)

As a LoreSkills user, I want the CLI to detect and upgrade itself using only stable published releases so that I stay current without being moved onto a pre-release unintentionally.

**Why this priority**: Automatic product updates reduce maintenance friction, but stable-only selection is necessary to preserve trust in the tool.

**Independent Test**: Validate the release check with a feed that contains both stable and pre-release versions, confirm that LoreSkills chooses only the latest stable release, and confirm that the upgrade flow handles blocked upgrades with guidance.

**Acceptance Scenarios**:

1. **Given** a newer stable release and a newer pre-release both exist, **When** LoreSkills checks for updates, **Then** it selects the newest stable release and ignores the pre-release.
2. **Given** only a newer pre-release exists, **When** LoreSkills checks for updates, **Then** it keeps the current stable installation and reports that no newer stable update is available.
3. **Given** a newer stable release is available but the environment cannot complete the upgrade automatically, **When** LoreSkills attempts the update, **Then** it explains what prevented the upgrade and what the user must do next.

---

### User Story 4 - Show a Minimal Version Identity and Hide the Stockholm Artwork Behind an Easter Egg (Priority: P3)

As a user, I want the default LoreSkills output to be concise while still preserving a recognizable hidden brand element so that routine CLI use stays minimal and the themed artwork remains available on demand.

**Why this priority**: This request changes the first impression of the product but does not block private repository support or resource discovery.

**Independent Test**: Run the bare `loreskills` command, run the version experience, and invoke the hidden Stockholm command to verify that the default output is version-only, the new byline appears, and the hidden artwork is available without appearing in standard guidance.

**Acceptance Scenarios**:

1. **Given** a user runs `loreskills` with no subcommand, **When** the default output appears, **Then** it shows only the current version signature and does not show the current animal ASCII guide.
2. **Given** a user requests version information, **When** LoreSkills responds, **Then** the output includes the installed version together with the message `dev with 🤖 by Hugo MR and Jeremy, Lead Tech.`
3. **Given** a user intentionally invokes the hidden Stockholm command, **When** LoreSkills responds, **Then** it renders an ASCII interpretation of the provided rolling-on-the-floor-laughing artwork without advertising that command in the standard help flow.

### Edge Cases

- What happens if a project still has an older shared LoreSkills management file but no project-specific setup yet? LoreSkills must make it clear which project settings are active and avoid silently using another project's source.
- What happens if a user selects a supported provider but the repository permissions are incomplete or expire later? The tool must explain why the private repository cannot be reached and how to restore access.
- What happens if the remote `SKILLS.md` file exists but an entry is missing a folder name or points to a missing location? LoreSkills must identify the broken entry instead of returning a misleading install option.
- What happens if two resources in the catalog share the same name across different folders? LoreSkills must distinguish them clearly enough that users can choose the intended item.
- What happens if the user requests catalog discovery from outside a project that has been configured? LoreSkills must explain that no project-specific repository setup is available in the current location.
- What happens if a newer pre-release exists but no newer stable release exists? LoreSkills must keep the current installation and avoid treating the pre-release as the preferred update target.
- What happens if the CLI cannot replace its installed binary because of permissions, path ownership, or a locked executable? LoreSkills must leave the current version intact and give a recovery path.

## Requirements _(mandatory)_

### Functional Requirements

- **FR-001**: The system MUST support project-specific connection to private repositories hosted on GitHub, GitLab, Azure DevOps, and Bitbucket.
- **FR-002**: The system MUST store repository management settings at the project level and MUST NOT share those settings across unrelated projects by default.
- **FR-002a**: The project-specific management settings MUST store repository identity and installation context, but MUST NOT require private access secrets to be stored in the project file.
- **FR-003**: The system MUST provide an interactive installation help flow for setting up a private repository connection inside the current project.
- **FR-004**: The interactive setup flow MUST explain how users authorize LoreSkills to access the selected private repository provider.
- **FR-004a**: The interactive setup flow MUST support access through provider login, environment variables, or OS credential storage.
- **FR-005**: The system MUST let users confirm whether the current project can reach its configured repository before installation begins.
- **FR-006**: The system MUST read the remote project so users can list available skills, agents, and other installable resources from the CLI.
- **FR-007**: The system MUST use `SKILLS.md` as the source of truth for the remote catalog of installable resources.
- **FR-008**: Each listed catalog entry MUST include the resource name, resource type, and the folder name associated with its installation source.
- **FR-009**: The system MUST return actionable guidance when the remote project is unreachable, access is denied, or `SKILLS.md` cannot be read or interpreted.
- **FR-010**: Users MUST be able to view the remote catalog before deciding what to install.
- **FR-011**: When installing or updating a resource, the system MUST resolve the resource location using the metadata declared in `SKILLS.md`, including the folder name when relevant.
- **FR-012**: When users run `loreskills` without a subcommand, the system MUST display only the current version signature and MUST NOT display the current animal ASCII output.
- **FR-012a**: Standard LoreSkills command outputs MUST stop using the current animal ASCII guide.
- **FR-013**: The version signature MUST include the installed version together with the exact message `dev with 🤖 by Hugo MR and Jeremy, Lead Tech.`
- **FR-014**: The system MUST provide the same version signature through the dedicated version experience.
- **FR-015**: The system MUST provide a hidden `stockholm` command that renders an ASCII interpretation of the provided rolling-on-the-floor-laughing reference artwork.
- **FR-016**: The hidden `stockholm` command MUST remain absent from the standard help and installation guidance unless the user explicitly asks for it.
- **FR-017**: The system MUST automatically check for newer LoreSkills versions against the official GitHub stable release channel.
- **FR-018**: The update check MUST ignore pre-releases when determining whether a newer version is available.
- **FR-019**: When a newer stable version is available, the system MUST support upgrading the installed CLI to that stable version from within the LoreSkills experience.
- **FR-020**: If LoreSkills cannot complete an upgrade automatically, the system MUST explain what blocked the update and what the user must do next.
- **FR-021**: The system MUST preserve project-to-project isolation so that one project's repository source, resource listings, and installation context do not leak into another project's flow.
- **FR-022**: The system MUST keep provider-specific setup guidance aligned with the supported providers and the project-local repository model.

### Key Entities _(include if feature involves data)_

- **Project Repository Profile**: The project-scoped management record that identifies the selected provider, the private repository to use, and the local installation context, without embedding private access secrets in the project file.
- **Provider Setup Guide**: The interactive guidance content that explains how a user connects LoreSkills to GitHub, GitLab, Azure DevOps, or Bitbucket for the current project.
- **Access Source**: The external credential mechanism LoreSkills uses to reach a private repository, such as provider login, environment variables, or OS credential storage.
- **Remote Catalog**: The list of installable skills, agents, and other resources declared by the remote project's `SKILLS.md` file.
- **Catalog Entry**: A single installable item from the remote catalog, including its name, resource type, folder name, and the information needed to locate it for installation.
- **Stable Release Channel**: The ordered set of official LoreSkills releases eligible for automatic updates, excluding pre-releases.
- **Version Signature**: The concise identity string shown by the base `loreskills` command and the version experience, combining the current version with the requested authorship message.
- **Stockholm Artwork**: The hidden ASCII rendering inspired by the provided rolling-on-the-floor-laughing reference image.

## Success Criteria _(mandatory)_

### Measurable Outcomes

- **SC-001**: 90% of first-time users can connect a project to a supported private repository provider in 5 minutes or less using the interactive setup flow.
- **SC-002**: In validation scenarios, 100% of catalog listings match the entries declared in the remote `SKILLS.md` file and include the correct folder name for each listed resource.
- **SC-003**: 95% of users can identify at least one relevant skill, agent, or other resource from the CLI catalog listing without opening the repository web interface.
- **SC-004**: In release-channel validation, 100% of update checks ignore pre-releases and select the newest stable release when a stable update exists.
- **SC-005**: Users can identify the installed version and authorship message from the bare `loreskills` command in under 10 seconds, and the default output no longer shows the previous animal mascot.

## Assumptions

- Each project has one active LoreSkills repository source at a time unless the user deliberately changes it.
- Project files store repository selection and installation context, while credentials are supplied externally through provider login, environment variables, or OS credential storage.
- Supported private repositories expose a path that can be reached with credentials or access mechanisms already available to the user.
- Repositories that want to be consumable by LoreSkills publish and maintain a valid `SKILLS.md` catalog.
- Automatic CLI updates target only official stable LoreSkills releases and may require the user's existing installation permissions to succeed.
- The hidden Stockholm command is part of the CLI experience; broader reuse of the artwork outside LoreSkills command surfaces is out of scope for this feature.
