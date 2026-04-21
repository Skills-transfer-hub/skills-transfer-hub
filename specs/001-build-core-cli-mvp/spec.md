# Feature Specification: Core CLI et MVP

**Feature Branch**: `[001-build-core-cli-mvp]`  
**Created**: 2026-04-19  
**Status**: Draft  
**Input**: User description: "Implement the feature specification based on the updated constitution. I want to build... # Spécification 01 : Le Core CLI et le MVP"

## Clarifications

### Session 2026-04-19

- Q: Si `.loreskillsrc` existe déjà, que doit faire `loreskills init` ? → A: `init` échoue avec un message clair.

## User Scenarios & Testing _(mandatory)_

### User Story 1 - Initialiser la configuration locale (Priority: P1)

En tant qu'utilisateur qui découvre LoreSkills, je veux créer un fichier de configuration par défaut dans mon répertoire personnel afin de pouvoir renseigner l'URL de mon dépôt de skills et préparer les commandes suivantes.

**Why this priority**: Sans configuration locale, l'utilisateur ne peut pas relier le CLI à une source distante ni utiliser le flux principal d'installation.

**Independent Test**: Peut être testé entièrement en exécutant `loreskills init` dans un environnement sans configuration existante et en vérifiant la création du fichier, son contenu JSON et le message de guidance affiché.

**Acceptance Scenarios**:

1. **Given** qu'aucun fichier de configuration LoreSkills n'existe dans le répertoire personnel, **When** l'utilisateur exécute `loreskills init`, **Then** un fichier `.loreskillsrc` est créé avec les clés `repo_raw_url` et `target_dir` et un message invite l'utilisateur à personnaliser l'URL du dépôt.
2. **Given** qu'un utilisateur exécute `loreskills` sans argument, **When** le programme démarre, **Then** un message d'aide présente les commandes `init`, `install` et `update`.
3. **Given** qu'un fichier `.loreskillsrc` existe déjà dans le répertoire personnel, **When** l'utilisateur exécute `loreskills init`, **Then** la commande échoue avec un message clair et ne modifie pas le fichier existant.

---

### User Story 2 - Installer un skill dans le projet courant (Priority: P2)

En tant qu'utilisateur ayant configuré LoreSkills, je veux télécharger un skill nommé depuis mon dépôt distant et l'ajouter au projet courant afin de l'utiliser immédiatement sans copier manuellement des fichiers.

**Why this priority**: C'est la valeur métier centrale du MVP : récupérer un skill distant et l'intégrer proprement dans un projet local.

**Independent Test**: Peut être testé en préparant une configuration valide, en exécutant `loreskills install <nom-du-skill>` depuis un répertoire de projet vide, puis en vérifiant la présence du fichier du skill, du texte d'avertissement et de l'état local mis à jour.

**Acceptance Scenarios**:

1. **Given** qu'une configuration valide existe et que le skill distant est disponible, **When** l'utilisateur exécute `loreskills install mon-skill`, **Then** le dossier cible est créé dans le projet courant si nécessaire, le fichier `mon-skill.md` est enregistré avec l'avertissement LoreSkills en tête et `lore-state.json` marque `mon-skill` comme installé.
2. **Given** qu'une configuration valide existe mais que le skill distant n'existe pas, **When** l'utilisateur exécute `loreskills install skill-inexistant`, **Then** la commande échoue avec un message d'erreur clair mentionnant l'absence du fichier distant et aucun fichier de skill vide n'est créé localement.

---

### User Story 3 - Mettre à jour les skills déjà installés (Priority: P3)

En tant qu'utilisateur ayant déjà installé plusieurs skills dans un projet, je veux relancer leur téléchargement en une seule commande afin d'actualiser rapidement mon environnement local.

**Why this priority**: Cette commande réduit le coût de maintenance du projet après la première installation et complète le cycle de vie minimal du produit.

**Independent Test**: Peut être testé en préparant un `lore-state.json` avec un ou plusieurs noms de skills, en exécutant `loreskills update`, puis en vérifiant que chaque fichier installé est réécrit à partir de la source distante ou qu'un message explicite est affiché quand aucun skill n'est enregistré.

**Acceptance Scenarios**:

1. **Given** qu'un projet contient un `lore-state.json` avec des skills installés, **When** l'utilisateur exécute `loreskills update`, **Then** chaque skill listé est retéléchargé et son fichier local est remplacé par la version distante la plus récente avec l'avertissement LoreSkills conservé.
2. **Given** qu'aucun fichier d'état n'existe ou qu'il ne contient aucun skill, **When** l'utilisateur exécute `loreskills update`, **Then** la commande affiche `Aucun skill à mettre à jour.` et se termine sans modifier le projet.

### Edge Cases

- Que se passe-t-il si l'utilisateur exécute `init` alors qu'un fichier `.loreskillsrc` existe déjà ? La commande doit échouer explicitement et préserver le fichier existant.
- Que se passe-t-il si l'utilisateur exécute `install` ou `update` sans avoir créé ou renseigné un fichier `.loreskillsrc` valide ? La commande doit échouer avec une erreur exploitable indiquant qu'une configuration valide est requise.
- Que se passe-t-il si le `target_dir` configuré n'existe pas encore dans le projet courant ? Le dossier doit être créé automatiquement avant l'écriture du skill ou du fichier d'état.
- Que se passe-t-il si le fichier distant répond `404 Not Found` ? La commande `install` ou la mise à jour du skill concerné doit échouer explicitement sans créer de fichier local vide ni corrompre l'état existant.
- Que se passe-t-il si `lore-state.json` est absent ou vide lors d'un `update` ? Le système doit informer l'utilisateur qu'aucun skill n'est à mettre à jour.

## Requirements _(mandatory)_

### Functional Requirements

- **FR-001**: The system MUST provide a default help message when the user runs `loreskills` without arguments.
- **FR-002**: The system MUST support an `init` command that creates a `.loreskillsrc` file in the user's home directory.
- **FR-003**: The `init` command MUST write a JSON object containing the keys `repo_raw_url` and `target_dir` with starter values that the user can customize.
- **FR-004**: The `init` command MUST display a success message instructing the user to replace the placeholder repository URL with their own repository location.
- **FR-004a**: If `.loreskillsrc` already exists, the `init` command MUST fail with a clear message and MUST NOT overwrite the existing file.
- **FR-005**: The system MUST support an `install <nom-du-skill>` command that reads the local configuration before attempting any remote retrieval.
- **FR-006**: The `install` command MUST request the remote file located at `<repo_raw_url>/<nom-du-skill>.md`.
- **FR-007**: When installing a skill, the system MUST create the configured target directory inside the current working directory if it does not already exist.
- **FR-008**: The system MUST save installed skills using the filename `<nom-du-skill>.md` inside the configured target directory of the current project.
- **FR-009**: The system MUST prepend the warning text `> ⚠️ ATTENTION : Skill géré par LoreSkills. Ne pas modifier manuellement.` followed by a blank line before the downloaded skill content.
- **FR-010**: After a successful installation, the system MUST create or update `<target_dir>/lore-state.json` so that the installed skill name maps to the value `installed`.
- **FR-011**: If the remote skill file does not exist, the `install` command MUST return a clear error and MUST NOT create an empty local skill file.
- **FR-012**: The system MUST support an `update` command that reads the current project's `<target_dir>/lore-state.json` file.
- **FR-013**: If the state file is missing or contains no installed skills, the `update` command MUST display `Aucun skill à mettre à jour.` and stop without modifying the project.
- **FR-014**: For each skill listed in the state file, the `update` command MUST repeat the download and overwrite flow used by `install`.
- **FR-015**: The system MUST preserve a valid state entry for each successfully updated skill.
- **FR-016**: If the configuration file is missing, unreadable, or malformed, commands that depend on it MUST fail with an actionable error message.

### Key Entities _(include if feature involves data)_

- **Configuration utilisateur**: Fichier local `.loreskillsrc` stockant l'adresse du dépôt de skills et le chemin cible relatif au projet courant.
- **Skill installé**: Fichier Markdown téléchargé depuis le dépôt distant, enrichi d'un avertissement LoreSkills et écrit dans le projet local.
- **État local des skills**: Fichier `lore-state.json` du projet courant indiquant quels skills sont considérés comme installés et donc éligibles à la mise à jour.

## Success Criteria _(mandatory)_

### Measurable Outcomes

- **SC-001**: Un nouvel utilisateur peut initialiser LoreSkills et obtenir un fichier de configuration exploitable en moins d'une minute sans intervention manuelle sur d'autres fichiers.
- **SC-002**: Dans 100 % des cas d'installation réussie, le skill demandé est présent dans le dossier cible du projet courant avec l'avertissement LoreSkills en tête et une entrée correspondante dans l'état local.
- **SC-003**: Lorsqu'un skill distant n'existe pas, 100 % des tentatives d'installation échouent avec un message explicite sans créer de fichier Markdown vide dans le projet.
- **SC-004**: Lorsqu'un projet contient des skills enregistrés, l'utilisateur peut déclencher leur rafraîchissement complet en une seule commande.
- **SC-005**: Lorsqu'aucun skill n'est enregistré dans le projet, l'utilisateur reçoit immédiatement un message indiquant qu'aucune mise à jour n'est nécessaire.

## Assumptions

- Le fichier `.loreskillsrc` du MVP est stocké dans le répertoire personnel de l'utilisateur, même si d'autres emplacements de configuration pourront être envisagés plus tard.
- `target_dir` est interprété comme un chemin relatif à la racine du projet courant où la commande est exécutée.
- Chaque skill distant est exposé sous la forme d'un unique fichier Markdown nommé `<nom-du-skill>.md`.
- Le fichier `lore-state.json` du MVP se limite à suivre l'état `installed` par nom de skill, sans version ni métadonnée supplémentaire.
- La commande `update` réutilise le comportement fonctionnel de `install` pour le téléchargement et l'écrasement, sans interaction supplémentaire avec l'utilisateur.
