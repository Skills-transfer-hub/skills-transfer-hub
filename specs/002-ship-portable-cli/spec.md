# Feature Specification: Ship Portable CLI

**Feature Branch**: `[003-ship-portable-cli]`  
**Created**: 2026-04-22  
**Status**: Draft  
**Input**: User description: "J'ai besoin de liver un build fonctionnel et qu'on peut installer sur Mac, Windows, Linux. On installe et l'app est dispo au globale on précise que cela doit être fait dans le github actions. J'ai besoin qu'on crée des commandes help et autres pour aider l'utilsiateur Et ce qui serait top c'est qu'on ait un avatar en ASCIII qui guide le user dans le terminal"

## Clarifications

### Session 2026-04-22

- Q: Quel canal d'installation officiel doit définir le MVP pour rendre `loreskills` disponible globalement ? → A: GitHub release artifacts with documented per-OS global install steps.
- Q: Quelle surface de commandes utilisateur doit être explicitement incluse dans ce MVP ? → A: `init`, `install`, `update`, `help`, and `version`.
- Q: Quand l'avatar ASCII doit-il apparaître dans l'expérience CLI ? → A: On every command output.
- Q: Quelle matrice minimale d'artifacts doit être publiée dans ce MVP ? → A: `windows/amd64`, `linux/amd64`, `darwin/amd64`, and `darwin/arm64`.
- Q: Quelle vérification d'intégrité doit accompagner les artifacts de release du MVP ? → A: Publish a SHA256 checksum file and document verification steps.

## User Scenarios & Testing _(mandatory)_

### User Story 1 - Installer LoreSkills globalement sur les trois OS (Priority: P1)

En tant qu'utilisateur qui découvre LoreSkills, je veux pouvoir l'installer sur macOS, Windows ou Linux et lancer ensuite `loreskills` depuis n'importe quel terminal afin de commencer à l'utiliser sans build manuel ni chemin local.

**Why this priority**: Tant que le CLI n'est pas distribuable et accessible globalement, l'adoption reste limitée aux développeurs capables de compiler le projet eux-mêmes.

**Independent Test**: Peut être testé sur chaque système d'exploitation pris en charge en suivant le flux d'installation officiel, puis en ouvrant un nouveau terminal et en vérifiant que `loreskills` répond sans chemin relatif.

**Acceptance Scenarios**:

1. **Given** qu'un utilisateur est sur un système d'exploitation pris en charge, **When** il suit le flux d'installation officiel, **Then** LoreSkills est installé et la commande `loreskills` est reconnue globalement dans un nouveau terminal.
2. **Given** qu'un utilisateur vient d'installer LoreSkills, **When** il exécute `loreskills --help`, **Then** il voit immédiatement la liste des commandes disponibles et la façon de démarrer.
3. **Given** qu'un utilisateur télécharge un artifact officiel pour `windows/amd64`, `linux/amd64`, `darwin/amd64`, ou `darwin/arm64`, **When** il suit le flux d'installation officiel, **Then** le paquet correspond clairement à sa plateforme cible.
4. **Given** qu'un utilisateur télécharge un artifact officiel et son fichier de checksum, **When** il suit la procédure de vérification documentée, **Then** il peut confirmer l'intégrité du paquet avant installation.
5. **Given** qu'un utilisateur tente d'installer LoreSkills dans un contexte où l'enregistrement global ne peut pas être finalisé automatiquement, **When** l'installation se termine, **Then** le flux officiel fournit une explication claire et la prochaine action à réaliser pour rendre la commande disponible globalement.

---

### User Story 2 - Se repérer grâce à l'aide intégrée (Priority: P2)

En tant qu'utilisateur novice ou occasionnel, je veux des commandes `help` et `version` ainsi qu'une aide claire avec syntaxe, exemples et messages d'erreur exploitables afin de comprendre rapidement quoi faire sans consulter la documentation du dépôt.

**Why this priority**: Une distribution plus large augmente le besoin d'autonomie utilisateur; l'aide intégrée réduit les erreurs et les demandes de support.

**Independent Test**: Peut être testé en exécutant l'aide générale, l'aide de chaque commande principale et plusieurs commandes invalides pour vérifier la présence d'explications, d'exemples et de messages correctifs.

**Acceptance Scenarios**:

1. **Given** qu'un utilisateur exécute `loreskills --help`, **When** l'aide s'affiche, **Then** elle présente les commandes principales, leur rôle et au moins un exemple de démarrage.
2. **Given** qu'un utilisateur demande l'aide d'une commande principale, **When** l'aide contextuelle s'affiche, **Then** elle décrit la syntaxe de cette commande, ses arguments attendus et un exemple d'utilisation.
3. **Given** qu'un utilisateur exécute `loreskills version`, **When** la commande répond, **Then** elle affiche clairement la version du CLI installée.
4. **Given** qu'un utilisateur saisit une commande inconnue ou incomplète, **When** le CLI refuse l'entrée, **Then** il affiche une erreur compréhensible suivie d'une action de correction ou d'une référence vers l'aide pertinente.

---

### User Story 3 - Être guidé dans le terminal par un avatar ASCII (Priority: P3)

En tant qu'utilisateur, je veux qu'un avatar ASCII accompagne chaque sortie du CLI afin que le terminal reste accueillant, reconnaissable et que les prochaines étapes soient toujours plus visibles.

**Why this priority**: Cet élément n'est pas nécessaire au fonctionnement du CLI, mais il améliore la perception du produit et rend les messages de guidance plus mémorables.

**Independent Test**: Peut être testé en exécutant l'aide, une commande réussie et une commande en échec afin de vérifier que chaque sortie affiche l'avatar avec un message lisible et utile.

**Acceptance Scenarios**:

1. **Given** qu'un utilisateur exécute une commande LoreSkills, **When** le CLI produit une sortie visible, **Then** un avatar ASCII cohérent accompagne le message affiché.
2. **Given** qu'une commande se termine avec succès, **When** le CLI affiche le message de confirmation, **Then** l'avatar ASCII met en avant la prochaine étape utile sans masquer le résultat fonctionnel.
3. **Given** qu'une commande échoue, **When** le CLI affiche le message d'échec, **Then** l'avatar ASCII reste présent sans masquer l'erreur ni la guidance associée.

### Edge Cases

- Que se passe-t-il si un utilisateur télécharge le mauvais paquet pour son environnement ? Le flux officiel doit lui permettre d'identifier clairement le paquet attendu et d'éviter d'exécuter un binaire incompatible.
- Que se passe-t-il si un utilisateur est sur une architecture non couverte par le MVP ? Le flux officiel doit indiquer explicitement que la plateforme n'est pas encore publiée plutôt que de laisser croire qu'un artifact compatible existe.
- Que se passe-t-il si la vérification SHA256 échoue pour un artifact téléchargé ? Le flux officiel doit indiquer que l'installation doit être interrompue et que l'artifact ne doit pas être exécuté.
- Que se passe-t-il si l'installation réussit mais que la commande n'est pas encore visible dans le terminal courant ? La guidance doit expliquer comment vérifier la disponibilité globale et quelle action minimale effectuer.
- Que se passe-t-il si l'utilisateur appelle une commande sans argument obligatoire ? Le CLI doit signaler l'information manquante et proposer la syntaxe correcte.
- Que se passe-t-il si l'utilisateur demande de l'aide sur une commande inconnue ? Le CLI doit indiquer que la commande n'existe pas et rediriger vers la liste des commandes valides.
- Que se passe-t-il si l'avatar ASCII est affiché dans un terminal étroit ? Le rendu doit rester lisible et ne pas empêcher la lecture des messages fonctionnels, y compris lorsqu'il apparaît sur chaque commande.

## Requirements _(mandatory)_

### Functional Requirements

- **FR-001**: The system MUST provide an official installation path for Windows, macOS, and Linux based on downloadable GitHub release artifacts.
- **FR-001a**: The initial release matrix for this MVP MUST include artifacts for `windows/amd64`, `linux/amd64`, `darwin/amd64`, and `darwin/arm64`.
- **FR-002**: The system MUST make LoreSkills executable as a globally available command after a successful installation flow on each supported operating system.
- **FR-003**: The system MUST provide a verification step that lets users confirm the CLI is correctly installed and callable after installation.
- **FR-004**: The system MUST provide a repository-hosted automated release workflow in GitHub Actions that publishes the installable LoreSkills artifacts and the related per-OS installation guidance.
- **FR-004a**: The release workflow MUST publish a SHA256 checksum file covering the MVP release artifacts.
- **FR-005**: The system MUST expose `init`, `install`, `update`, `help`, and `version` as the primary end-user command surface for this MVP.
- **FR-006**: The system MUST provide a default help experience when users run `loreskills` without arguments or request global help explicitly.
- **FR-007**: The system MUST provide help output for each primary command exposed to end users.
- **FR-008**: The help output MUST describe the purpose of the command, the expected syntax, and at least one usage example.
- **FR-009**: The system MUST provide a `version` command that reports the installed LoreSkills CLI version.
- **FR-010**: The system MUST return actionable guidance when users enter an unknown command, omit a required argument, or invoke help for an invalid target.
- **FR-011**: The system MUST keep the built-in guidance aligned with the commands currently available in the CLI.
- **FR-012**: The system MUST display a consistent ASCII avatar on every visible CLI command output.
- **FR-013**: The ASCII avatar MUST accompany the message without hiding critical command output, errors, or next steps.
- **FR-014**: The system MUST use the avatar-guided messages to point users toward the next relevant action after help, successful setup, recoverable errors, and routine command responses.
- **FR-015**: Users MUST be able to discover how to start with LoreSkills from the CLI itself without opening repository source files.
- **FR-016**: The official installation guidance MUST explain how users verify the SHA256 checksum of a downloaded release artifact before installation.

### Key Entities _(include if feature involves data)_

- **Release Package**: A downloadable LoreSkills artifact published from GitHub releases for one supported operating system and ready to be installed by an end user.
- **Checksum Manifest**: A published SHA256 file that lets users verify the integrity of the release artifacts before installation.
- **Supported Platform Matrix**: The set of operating system and architecture pairs officially published in the MVP release workflow.
- **Installation Flow**: The official set of user-facing steps that makes LoreSkills available as a global command on a supported operating system.
- **Command Help Entry**: The built-in guidance associated with a command, including its purpose, syntax, examples, and corrective hints.
- **ASCII Guide**: The terminal persona made of ASCII characters that appears alongside every visible LoreSkills command output.

## Success Criteria _(mandatory)_

### Measurable Outcomes

- **SC-001**: On Windows, macOS, and Linux, a first-time user can complete the official installation flow and invoke `loreskills` globally in 10 minutes or less.
- **SC-002**: In release validation, 100% of the MVP target platforms (`windows/amd64`, `linux/amd64`, `darwin/amd64`, and `darwin/arm64`) have an installable LoreSkills package and an accompanying installation path published at the same time.
- **SC-002a**: In release validation, 100% of published MVP releases include a SHA256 checksum file and user-facing verification steps.
- **SC-003**: 95% of users can identify the correct next command from the built-in help in under 60 seconds during onboarding validation.
- **SC-004**: 100% of primary commands return help content that includes purpose, syntax, and an example.
- **SC-005**: In tested guidance scenarios, 100% of common user input errors produce a corrective message that points to a next action or relevant help command.

## Assumptions

- Les utilisateurs finaux installent LoreSkills à partir des artifacts officiels publiés dans les GitHub releases plutôt qu'en compilant le code source eux-mêmes.
- Le périmètre de cette feature couvre la distribution sur les systèmes d'exploitation majeurs de bureau et serveur visés par le projet, sans engagement sur toutes les variantes exotiques.
- Le MVP publie uniquement les quatre couples plateforme/architecture retenus (`windows/amd64`, `linux/amd64`, `darwin/amd64`, `darwin/arm64`) ; les autres variantes restent hors périmètre initial.
- Les commandes cœur existantes de LoreSkills restent la base du produit et doivent devenir plus faciles à découvrir plutôt qu'être remplacées.
- L'avatar ASCII apparaît sur chaque sortie visible du CLI et doit donc rester compact, lisible et compatible avec les terminaux courants.
- L'automatisation de publication attendue par les parties prenantes est hébergée dans GitHub Actions et produit les artifacts de release ainsi que la guidance d'installation globale associée.
