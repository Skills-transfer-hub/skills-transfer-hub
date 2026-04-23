# Implementation Plan: Guided Repository Onboarding

**Branch**: `[005-rework-repo-onboarding]` | **Date**: 2026-04-23 | **Spec**: `/specs/004-rework-repo-onboarding/spec.md`
**Input**: Feature specification from `/specs/004-rework-repo-onboarding/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/plan-template.md` for the execution workflow.

## Summary

Transformer LoreSkills d'un point d'entrée CLI minimal vers une expérience guidée orientée TTY, capable d'onboarder les quatre providers hébergés supportés, d'expliquer précisément la création des tokens/API keys, de découvrir des bundles compatibles via `SKILLS.md` ou un scan de structure `category/skill-folder/SKILL.md`, de synchroniser des bundles complets sous contrôle strict des chemins gérés, et de signaler les mises à jour stables via un cache machine-local de 24 heures. L'implémentation reste strictement stdlib-only et s'appuie sur un renforcement ciblé des couches existantes `cmd/`, `internal/http/` et `internal/fs/` plutôt que sur une refonte globale.

## Technical Context

**Language/Version**: Go 1.22  
**Primary Dependencies**: Bibliothèque standard Go uniquement (`bufio`, `encoding/json`, `net/http`, `os`, `os/exec`, `path`, `path/filepath`, `sort`, `strings`, `time`, `archive/zip`, `archive/tar`, `compress/gzip`, `crypto/sha256`, `testing`, `net/http/httptest`)  
**Storage**: profil projet `.loreskills/project.json`; état versionné des bundles gérés dans `<target_dir>/lore-state.json`; cache machine-local des checks de mise à jour dans `os.UserCacheDir()/loreskills/update-check.json`; sources distantes via `SKILLS.md` ou scan provider-aware  
**Testing**: `go test ./...`, `go vet ./...`, tests package-local `*_test.go`, `httptest`, temp dirs, tests CLI via `newApp` et entrées simulées  
**Target Platform**: CLI statique Windows, macOS et Linux, avec dépôts privés hébergés sur `github.com`, `gitlab.com`, `dev.azure.com` et `bitbucket.org`  
**Project Type**: application CLI mono-binaire  
**Performance Goals**: démarrage interactif sans réseau quand le cache update est frais en <150 ms localement; au plus un check réseau de release stable par fenêtre de 24 h et par machine; `init` valide l'accès en une tentative ciblée; `list` fait au plus une lecture de catalogue ou une séquence de listing provider-aware avant téléchargement de contenu; la synchronisation d'un bundle reste linéaire au nombre de fichiers suivis  
**Constraints**: stdlib only; aucun secret persisté dans les fichiers projet ou état; providers hébergés uniquement; `loreskills` ouvre le menu seulement en TTY; `SKILLS.md` garde la priorité sur le scan; toutes les écritures gérées restent confinées sous `<target_dir>`; les contenus gérés sont source-of-truth côté dépôt et écrasent les modifications locales dans la zone gérée; `update self` reste le seul chemin qui remplace explicitement le binaire; couverture de tests obligatoire sur chaque nouveau comportement  
**Scale/Scope**: 4 providers hébergés; 3 états principaux de menu guidé (non configuré, partiel/invalide, prêt); quelques dizaines à quelques centaines de bundles compatibles par dépôt; un profil projet par workspace; un cache de mise à jour par profil utilisateur local

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **Gate 1 - Zéro dépendance externe**: PASS. Tous les choix Phase 0/1 restent dans la stdlib Go et dans les packages existants du dépôt.
- **Gate 2 - Aucune valeur utilisateur codée en dur**: PASS WITH PRODUCT-METADATA BOUNDARY. Les URLs de dépôt, refs et dossiers cible restent lus au runtime depuis le profil projet. Les URLs de documentation provider et domaines hébergés supportés sont traités comme métadonnées produit centralisées, non comme configuration de dépôt utilisateur.
- **Gate 3 - Cross-platform statique**: PASS. Les nouveaux besoins utilisent `os.UserCacheDir`, `filepath`, `net/http` et les primitives d'archive déjà compatibles Windows/macOS/Linux sans CGO.
- **Gate 4 - Découpage modulaire strict**: PASS WITH CONTROLLED FILE SPLIT. `cmd/` garde le menu guidé, l'orchestration et le rendu; `internal/http/` étend la découverte distante, le scan provider-aware et les releases; `internal/fs/` centralise profil projet, manifestes de bundles, cache de mise à jour et garde-fous d'écriture.
- **Gate 5 - TDD strict**: PASS IF IMPLEMENTED. Le menu TTY, les guides providers, le fallback scan, la synchro de bundles, la politique d'écrasement et le cache 24 h exigent tous des tests ciblés dans `cmd/`, `internal/http/` et `internal/fs/`.

**Post-Design Check**: PASS. Les artefacts de recherche et de design gardent l'architecture à trois couches, n'ajoutent aucune dépendance externe, maintiennent la configuration dépôt côté projet, et introduisent seulement des fichiers d'état/cache machine-local gérés par `internal/fs/`.

## Project Structure

### Documentation (this feature)

```text
specs/004-rework-repo-onboarding/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   └── cli-contract.md
└── tasks.md
```

### Source Code (repository root)

```text
.github/
└── copilot-instructions.md

cmd/
├── root.go
├── root_test.go
├── init.go
├── init_test.go
├── list.go
├── list_test.go
├── install.go
├── install_test.go
├── update.go
├── update_test.go
├── version.go
├── version_test.go
├── stockholm.go
├── stockholm_test.go
├── menu.go                # new: guided root entry and fallback guidance
├── menu_test.go           # new
├── providers.go           # new: hosted provider guidance catalog
└── providers_test.go      # new

internal/
├── fs/
│   ├── fs.go
│   ├── fs_test.go
│   ├── state_cache.go     # new: bundle manifest + update cache helpers
│   └── state_cache_test.go
└── http/
    ├── client.go
    ├── client_test.go
    ├── discovery.go       # new: tree scan fallback and bundle enumeration
    └── discovery_test.go

README.md
main.go
go.mod
```

**Structure Decision**: Conserver la structure mono-binaire existante et étendre localement les packages qui possèdent déjà le comportement. Ajouter des fichiers ciblés pour le menu guidé, le catalogue provider, la découverte fallback et le cache d'état plutôt que d'alourdir davantage `root.go`, `init.go`, `client.go` et `fs.go`. Aucun nouveau package top-level n'est nécessaire.

## Complexity Tracking

No constitution violations are expected for this feature.

*** Add File: c:\SOURCE\LoreSkills\specs\004-rework-repo-onboarding\research.md
# Research: Guided Repository Onboarding

## Decision 1: Faire de `loreskills` un point d'entrée guidé dépendant du TTY

- **Decision**: `loreskills` sans argument ouvre un menu guidé state-aware quand `stdin` et `stdout` sont interactifs; hors TTY, la commande imprime une guidance courte indiquant l'état courant et la prochaine action utile.
- **Rationale**: Cela respecte la clarification validée, améliore l'onboarding sans casser les usages scriptés, et reste simple à implémenter en stdlib via `bufio.Reader` et un routeur d'actions dans `cmd/`.
- **Alternatives considered**: Conserver la sortie minimale historique a été rejeté car cela laisse l'utilisateur sans guidance. Ajouter une commande séparée `guide` a été rejeté car l'utilisateur a explicitement choisi `loreskills` comme point d'entrée. Rendre le menu obligatoire hors TTY a été rejeté car cela casserait les pipelines et les tests non interactifs.

## Decision 2: Centraliser la guidance provider dans un catalogue structuré aligné sur les docs officielles

- **Decision**: Définir un catalogue provider structuré pour les quatre hosts supportés (`github.com`, `gitlab.com`, `dev.azure.com`, `bitbucket.org`) contenant le label, le host, l'URL de documentation officielle, l'URL ou le chemin d'action à ouvrir, la permission minimale de lecture, le format attendu du `repository_id`, et les variables d'environnement recommandées. Pour Bitbucket Cloud, migrer la guidance d'App Password vers les API tokens avec permission `Repositories: Read`.
- **Rationale**: La guidance est aujourd'hui dispersée dans `cmd/init.go` et Bitbucket pointe encore vers un modèle ancien. Un catalogue structuré réduit la dérive, permet de réutiliser les mêmes données dans `init`, le menu racine, `help` et les tests, et ancre les futures mises à jour sur des pages officielles vérifiables.
- **Alternatives considered**: Laisser des chaînes inline dans `init.go` a été rejeté car cela complique la maintenance et la validation. Déplacer toute la guidance dans le README uniquement a été rejeté car le besoin porte sur une guidance in-product. Ouvrir maintenant le support self-hosted a été rejeté car l'utilisateur a limité la portée aux hosts hébergés.

## Decision 3: Garder `SKILLS.md` prioritaire, avec fallback de scan provider-aware si le catalogue est absent

- **Decision**: Si `SKILLS.md` existe et se parse correctement, LoreSkills s'appuie exclusivement sur ce catalogue. Si `SKILLS.md` est absent, LoreSkills bascule sur un scan provider-aware qui énumère les chemins correspondant à `category/skill-folder/SKILL.md`. Si `SKILLS.md` existe mais contient des entrées invalides, LoreSkills retourne l'erreur sans fusion silencieuse avec le scan.
- **Rationale**: Cette stratégie respecte la clarification `SKILLS.md` first, préserve la compatibilité avec les dépôts déjà catalogués, et ajoute une voie de compatibilité réaliste pour les dépôts structurés sans manifeste explicite.
- **Alternatives considered**: `SKILLS.md` obligatoire a été rejeté car le besoin impose une compatibilité plus souple. Le scan seul a été rejeté car `SKILLS.md` doit rester la source explicite quand il est présent. Fusionner catalogue et scan a été rejeté car cela créerait des ambiguïtés difficiles à expliquer et à tester.

## Decision 4: Synchroniser des bundles complets ancrés par `SKILL.md`, pas seulement un fichier markdown

- **Decision**: Normaliser chaque ressource découverte en bundle racine `category/skill-folder/`, avec `SKILL.md` comme point d'entrée requis. `install` et `update` synchronisent tous les fichiers de ce dossier racine, pas seulement `SKILL.md`, et stockent un manifeste précis des fichiers gérés pour permettre l'écrasement et le nettoyage ciblé.
- **Rationale**: Le besoin de protéger le code/plugin géré n'est pas satisfaisable avec le modèle actuel single-file. Le sync par bundle permet de suivre les fichiers compagnons, d'écraser proprement les fichiers gérés, et de supprimer les fichiers obsolètes qui disparaissent du dépôt source.
- **Alternatives considered**: Continuer à installer seulement `SKILL.md` a été rejeté car cela ne couvre ni le code compagnon ni la protection demandée. Copier récursivement sans manifeste a été rejeté car cela empêcherait de distinguer fichiers gérés et non gérés lors des updates.

## Decision 5: Appliquer une politique de frontière gérée stricte avec écrasement des contenus gérés

- **Decision**: Toute écriture gérée doit se résoudre sous `<target_dir>`. Les fichiers suivis dans le manifeste de bundle sont considérés comme propriété du dépôt source et sont écrasés lors d'un `install` ou `update`; toute tentative d'écriture hors frontière gérée est bloquée; les fichiers obsolètes encore présents dans le manifeste sont supprimés pendant la resynchronisation du bundle.
- **Rationale**: Cela aligne l'implémentation sur la clarification explicite de l'utilisateur: les modifications locales dans la zone gérée ne bloquent pas l'opération. La frontière de chemin reste le vrai mécanisme de protection.
- **Alternatives considered**: Demander une confirmation à chaque conflit a été rejeté par clarification. Tolérer des chemins hors frontière a été rejeté pour raison de sécurité. Ne jamais supprimer les fichiers obsolètes a été rejeté car cela laisserait dériver le contenu géré.

## Decision 6: Cacher l'état de mise à jour stable par machine via `os.UserCacheDir`

- **Decision**: Ajouter un cache machine-local `update-check.json` sous `os.UserCacheDir()/loreskills/` contenant `last_checked_at`, la dernière version stable connue, l'état (`no-update`, `update-available`, `error`) et les URLs d'asset/checksum nécessaires à l'offre d'update. Le menu racine le consulte; si l'état est frais (<24 h), aucun appel réseau n'est refait au démarrage interactif.
- **Rationale**: Le check doit être automatique mais borné dans le temps. Un cache machine-local évite d'alourdir le profil projet avec une donnée purement locale et garde un démarrage réactif.
- **Alternatives considered**: Checker à chaque lancement a été rejeté car trop coûteux et bavard. Ne checker que sur action explicite a été rejeté car cela ne correspond plus au comportement automatique demandé. Stocker cet état dans `.loreskills/project.json` a été rejeté car la donnée est machine-locale, pas projet.

## Decision 7: Étendre `internal/http/` avec les APIs de listing des providers hébergés pour le scan et la synchro de bundles

- **Decision**: Ajouter des endpoints provider-aware de listing d'arborescence dans `internal/http/` pour les quatre hosts supportés, en réutilisant le modèle d'authentification déjà en place. Les chemins dérivés servent à détecter les bundles compatibles, à lister les fichiers d'un bundle, puis à télécharger chaque fichier utile pendant `install` et `update`.
- **Rationale**: Les URLs raw actuelles permettent de lire un fichier, pas d'énumérer un dépôt. Le fallback scan et la synchro de bundle exigent une vue arborescente du dépôt source.
- **Alternatives considered**: Sheller vers `git clone` a été rejeté car cela élargit la surface d'exécution, alourdit les tests et complique Windows. Exiger un clone local préalable a été rejeté car l'outil doit rester autonome. Abandonner le fallback scan a été rejeté car ce point fait partie du besoin clarifié.

*** Add File: c:\SOURCE\LoreSkills\specs\004-rework-repo-onboarding\data-model.md
# Data Model: Guided Repository Onboarding

## ProjectRepositoryConfiguration

- **Purpose**: Représente la configuration LoreSkills propre au projet courant.
- **Fields**:
  - `provider` (`string`, required): clé provider (`github`, `gitlab`, `azure-devops`, `bitbucket`).
  - `host` (`string`, derived): host hébergé attendu pour le provider (`github.com`, `gitlab.com`, `dev.azure.com`, `bitbucket.org`).
  - `repository_id` (`string`, required): identifiant canonique du dépôt pour le provider choisi.
  - `ref` (`string`, required): branche, tag ou ref Git à lire.
  - `catalog_path` (`string`, required): chemin vers `SKILLS.md`, par défaut `SKILLS.md`.
  - `target_dir` (`string`, required): racine relative du contenu géré dans le projet courant.
  - `migrated_from_legacy` (`bool`, optional): indique si le profil vient d'un import legacy.
- **Validation rules**:
  - `provider` doit appartenir à la liste des providers supportés.
  - `host` doit correspondre au domaine hébergé attendu pour `provider`.
  - `repository_id`, `ref`, `catalog_path` et `target_dir` doivent être non vides après normalisation.
  - `target_dir` doit rester relatif au projet courant.
  - Le profil ne contient aucun secret; tout token/session reste hors persistence.
- **State transitions**:
  - `absent` → aucun profil projet.
  - `configured` → profil saisi ou migré.
  - `validated` → accès dépôt + découverte initiale confirmés.
  - `updated` → profil remplacé par un nouveau setup.

## ProviderSetupGuide

- **Purpose**: Porte la guidance officielle nécessaire pour configurer un provider supporté.
- **Fields**:
  - `provider` (`string`, required): identifiant LoreSkills du provider.
  - `display_name` (`string`, required): nom lisible pour l'UI.
  - `host` (`string`, required): domaine hébergé supporté.
  - `documentation_url` (`string`, required): page officielle à citer dans la guidance.
  - `action_url` (`string`, required): URL directe ou chemin d'écran à ouvrir pour créer le token.
  - `credential_kind` (`string`, required): `fine-grained-pat`, `pat`, `api-token`.
  - `minimum_permission` (`string`, required): permission minimale de lecture dépôt.
  - `repository_id_format` (`string`, required): format attendu côté LoreSkills.
  - `repository_id_example` (`string`, required): exemple concret de valeur attendue.
  - `env_vars` (`[]string`, required): variables d'environnement recommandées.
  - `notes` (`[]string`, optional): contraintes ou précisions provider-specific.
- **Validation rules**:
  - Une seule entrée par provider supporté.
  - `host`, `documentation_url`, `action_url` et `minimum_permission` doivent être renseignés.
  - Les URLs doivent viser le provider correspondant.

## RepositoryDiscoverySource

- **Purpose**: Décrit d'où LoreSkills a dérivé la liste des bundles compatibles.
- **Fields**:
  - `kind` (`string`, required): `skills-md` ou `structure-scan`.
  - `catalog_path` (`string`, optional): chemin du manifeste utilisé quand `kind = skills-md`.
  - `scan_root` (`string`, optional): racine logique scannée quand `kind = structure-scan`.
  - `precedence` (`int`, required): ordre de priorité (`1` pour `skills-md`, `2` pour le fallback scan).
  - `status` (`string`, required): `used`, `missing`, `invalid`, `skipped`.
  - `detail` (`string`, optional): message utilisateur ou technique résumant le choix.
- **Validation rules**:
  - `kind = skills-md` ne peut être `used` que si le manifeste existe et se parse.
  - `kind = structure-scan` ne peut être `used` que si `skills-md` est absent.

## ResourceBundle

- **Purpose**: Représente une ressource installable complète, ancrée par un `SKILL.md` dans un dossier compatible.
- **Fields**:
  - `bundle_id` (`string`, derived): identifiant stable dérivé de `category_name` et `folder_name`.
  - `category_name` (`string`, required): dossier de premier niveau du dépôt compatible (`react`, `commands`, `.net`, etc.).
  - `folder_name` (`string`, required): nom du dossier bundle sous la catégorie.
  - `display_name` (`string`, required): nom affiché dans LoreSkills.
  - `description` (`string`, optional): description courte affichée par `list`.
  - `entrypoint_path` (`string`, required): chemin vers `category/folder/SKILL.md`.
  - `source_root` (`string`, required): racine du bundle, parent de `entrypoint_path`.
  - `discovery_source` (`RepositoryDiscoverySource`, required): source ayant permis de découvrir le bundle.
  - `install_refs` (`[]string`, derived): alias résolus côté CLI, par exemple `folder` ou `category/folder`.
- **Validation rules**:
  - `entrypoint_path` doit finir par `/SKILL.md`.
  - `source_root` doit être le parent direct de `entrypoint_path`.
  - `category_name` et `folder_name` doivent être non vides et sans remontée de chemin.
  - `bundle_id` doit être unique dans la découverte courante.
- **State transitions**:
  - `discovered` → bundle listé via catalogue ou scan.
  - `selected` → bundle choisi pour installation.
  - `synced` → bundle copié localement avec manifeste à jour.
  - `refreshed` → bundle re-synchronisé par `update`.

## ManagedBundleState

- **Purpose**: Représente le contenu de `<target_dir>/lore-state.json` pour suivre les bundles gérés et leurs fichiers.
- **Fields**:
  - `version` (`int`, required): version du schéma de manifeste.
  - `entries` (`[]ManagedBundleRecord`, required): bundles suivis.
- **ManagedBundleRecord fields**:
  - `bundle_id` (`string`, required): identifiant stable du bundle.
  - `category_name` (`string`, required): catégorie du bundle.
  - `folder_name` (`string`, required): nom du dossier bundle.
  - `display_name` (`string`, required): nom d'affichage.
  - `source_root` (`string`, required): racine distante logique du bundle.
  - `entrypoint_path` (`string`, required): chemin du `SKILL.md` source.
  - `managed_root` (`string`, required): racine locale du bundle sous `<target_dir>`.
  - `managed_files` (`[]string`, required): liste relative des fichiers gérés sous `managed_root`.
  - `ref` (`string`, required): ref distante utilisée au dernier sync.
  - `status` (`string`, required): `installed`, `updated`, `error`.
- **Validation rules**:
  - `managed_root` et chaque entrée de `managed_files` doivent rester sous `<target_dir>`.
  - `bundle_id` doit être unique dans le manifeste.
  - `managed_files` ne doit pas contenir de chemins absolus ni de `..`.
- **State transitions**:
  - `missing` → aucun bundle suivi.
  - `installed` → premier sync réussi.
  - `updated` → sync de rafraîchissement réussi.
  - `removed` → manifeste nettoyé si bundle désinstallé ou supprimé.

## GuidedActionMenuState

- **Purpose**: Décrit l'état courant de l'expérience racine `loreskills` en TTY.
- **Fields**:
  - `interaction_mode` (`string`, required): `interactive` ou `non-interactive`.
  - `project_state` (`string`, required): `unconfigured`, `configured-invalid`, `configured-ready`.
  - `primary_action` (`string`, required): action prioritaire proposée.
  - `available_actions` (`[]string`, required): actions affichées dans le menu.
  - `provider_hint` (`string`, optional): provider détecté/configuré si disponible.
  - `update_offer` (`StableUpdateOffer`, optional): statut de mise à jour à afficher.
- **Validation rules**:
  - `interaction_mode = non-interactive` interdit l'affichage d'un prompt bloquant.
  - `project_state` pilote l'ensemble des `available_actions`.

## StableUpdateOffer

- **Purpose**: Représente l'offre de mise à jour affichable à l'utilisateur.
- **Fields**:
  - `current_version` (`string`, required): version courante du binaire.
  - `latest_stable_version` (`string`, optional): dernière version stable connue.
  - `status` (`string`, required): `fresh-no-update`, `fresh-update-available`, `stale`, `error`.
  - `message` (`string`, required): texte prêt à afficher dans le menu ou la guidance fallback.
  - `download_ready` (`bool`, required): indique si les métadonnées nécessaires à `update self` sont disponibles.
- **Validation rules**:
  - Une pré-release ne peut jamais devenir `latest_stable_version`.

## UpdateCheckState

- **Purpose**: Persiste le dernier check automatique de mise à jour stable pour limiter les appels réseau.
- **Fields**:
  - `last_checked_at` (`string`, required): timestamp UTC ISO-8601 du dernier check automatique.
  - `ttl_hours` (`int`, required): valeur attendue `24`.
  - `current_version` (`string`, required): version utilisée lors du check.
  - `latest_stable_version` (`string`, optional): dernière stable connue.
  - `download_url` (`string`, optional): asset stable retenu.
  - `checksum_url` (`string`, optional): manifeste checksum retenu.
  - `asset_name` (`string`, optional): artefact plateforme retenu.
  - `status` (`string`, required): `no-update`, `update-available`, `error`.
  - `error_message` (`string`, optional): détail du dernier échec non bloquant.
- **Validation rules**:
  - Le cache est considéré frais tant que `now - last_checked_at < 24h`.
  - Les URLs de téléchargement ne sont présentes que si `status = update-available`.
  - Le cache est machine-local et ne doit jamais être stocké dans `.loreskills/project.json`.

*** Add File: c:\SOURCE\LoreSkills\specs\004-rework-repo-onboarding\contracts\cli-contract.md
# CLI Contract: Guided Repository Onboarding

## Shared Output And Interaction Contract

- **Applies to**: toutes les commandes visibles et au point d'entrée racine `loreskills`.
- **Behavior**:
  - les sorties de succès et d'information vont sur `stdout`
  - les erreurs utilisateur et erreurs d'exécution vont sur `stderr`
  - `loreskills` décide du mode `interactive` ou `non-interactive` à partir du contexte TTY
  - `stockholm` reste cachée de l'aide standard
  - les checks automatiques de mise à jour au démarrage utilisent le cache machine-local quand il est frais
- **Exit code**:
  - `0` sur succès ou no-op attendu
  - non-zéro sur erreur utilisateur ou d'exécution

## Command: `loreskills`

- **Purpose**: Ouvrir l'expérience guidée principale du CLI.
- **Input**: aucun argument.
- **Interactive contract (TTY)**:
  - affiche l'état projet (`non configuré`, `configuré mais invalide`, `configuré et prêt`)
  - affiche l'état de mise à jour stable connu (`aucune`, `disponible`, `stale`, `erreur non bloquante`)
  - propose des actions adaptées à l'état courant
- **Expected action sets**:
  - `unconfigured`: configurer le dépôt, relire la guidance provider, quitter
  - `configured-invalid`: revalider l'accès, rééditer la configuration, relire la guidance provider, quitter
  - `configured-ready`: lister les bundles, installer un bundle, mettre à jour les bundles gérés, revoir la configuration, lancer `update self` si une stable est disponible, quitter
- **Non-interactive contract**:
  - n'ouvre jamais de prompt
  - imprime un résumé concis de l'état courant et au moins une prochaine commande utile (`init`, `list`, `update`, `version`)
- **Failure cases**:
  - erreurs de lecture de profil projet, d'état local ou du cache de mise à jour
  - erreurs terminal I/O empêchant l'affichage du menu

## Command: `loreskills init`

- **Purpose**: Configurer ou reconfigurer le dépôt du projet courant avec guidance provider détaillée.
- **Input**: aucun argument.
- **Side effects**:
  - lit le profil projet existant si présent
  - propose la migration depuis `~/.loreskillsrc` si applicable
  - présente la guidance du provider choisi (doc officielle, URL/action de création du token, permission minimale, format de `repository_id`, variables d'environnement)
  - écrit `.loreskills/project.json` après validation
- **Validation contract**:
  - rejette tout host self-hosted ou non supporté
  - valide l'accès en utilisant `SKILLS.md` si présent, sinon le fallback scan de structure
  - retourne l'étape précise ayant échoué (`format dépôt`, `authentification`, `catalogue invalide`, `scan vide`, etc.)
- **Success output**:
  - provider retenu
  - dépôt/`ref`/`target_dir` retenus
  - chemin du profil projet
  - prochaine étape recommandée

## Command: `loreskills list`

- **Purpose**: Découvrir et afficher les bundles compatibles du dépôt configuré.
- **Input**: aucun argument.
- **Discovery precedence**:
  - si `SKILLS.md` existe et est valide, l'utiliser comme source unique
  - si `SKILLS.md` est absent, basculer sur le scan `category/skill-folder/SKILL.md`
  - si `SKILLS.md` existe mais est invalide, échouer explicitement sans fallback silencieux
- **Success output**:
  - une ligne par bundle avec au minimum `category`, `folder`, nom affiché, source de découverte (`catalog` ou `scan`) et description si disponible
  - le format de référence installable doit être clairement déductible, avec `category/folder` comme référence canonique
- **Failure cases**:
  - projet non configuré
  - accès refusé au dépôt
  - manifeste invalide
  - scan ne trouvant aucun bundle compatible

## Command: `loreskills install <resource-ref>`

- **Purpose**: Synchroniser un bundle compatible du dépôt vers le projet courant.
- **Input**:
  - argument positionnel requis: `resource-ref`
  - format accepté: nom unique de bundle ou référence canonique `category/folder`
- **Side effects**:
  - résout le bundle depuis la source de découverte active
  - énumère tous les fichiers du bundle sous `source_root`
  - écrit les fichiers sous `<target_dir>/<category>/<folder>/...`
  - écrase les fichiers déjà suivis dans cette zone gérée
  - met à jour `<target_dir>/lore-state.json` avec le manifeste complet des fichiers gérés
- **Failure cases**:
  - bundle introuvable ou ambigu
  - chemin source ou destination sortant de la frontière gérée
  - erreur de listing distant, téléchargement, checksum local éventuel ou écriture disque

## Command: `loreskills update`

- **Purpose**: Resynchroniser tous les bundles gérés du projet courant.
- **Input**: aucun argument.
- **Side effects**:
  - relit `.loreskills/project.json`
  - relit `<target_dir>/lore-state.json`
  - retélécharge chaque bundle selon `source_root`
  - écrase les fichiers gérés existants
  - supprime les fichiers suivis devenus absents de la source distante
- **Success output**:
  - confirmation bundle par bundle, ou message `Aucun bundle à mettre à jour.`
- **Failure cases**:
  - manifeste local illisible ou incompatible
  - échec partiel/total de téléchargement
  - violation de frontière gérée

## Command: `loreskills update self`

- **Purpose**: Vérifier immédiatement la dernière release stable compatible et tenter le remplacement du binaire courant.
- **Input**:
  - argument positionnel requis: `self`
- **Behavior**:
  - ignore la fraîcheur du cache automatique et effectue un check réseau explicite
  - ignore les pré-releases
  - sélectionne l'artefact correspondant à la plateforme courante
  - vérifie le checksum avant remplacement
- **Success output**:
  - version stable détectée
  - résultat du remplacement, ou message `Aucune mise à jour stable disponible.`
- **Failure cases**:
  - métadonnées de release absentes
  - téléchargement ou checksum invalides
  - remplacement du binaire impossible

## Command: `loreskills help [commande]`

- **Purpose**: Afficher l'aide générale ou ciblée.
- **Input**: argument optionnel `commande`.
- **Success output**:
  - décrit les commandes visibles `init`, `list`, `install`, `update`, `help`, `version`
  - précise que `loreskills` sans argument ouvre l'expérience guidée en TTY
- **Failure cases**:
  - commande cible inconnue → erreur explicite et renvoi vers l'aide générale

## Command: `loreskills version`

- **Purpose**: Afficher la signature minimale de version du CLI.
- **Input**: aucun argument.
- **Success output**:
  - version installée
  - byline produit

## Command: `loreskills stockholm`

- **Purpose**: Afficher l'easter egg ASCII caché.
- **Input**: aucun argument.
- **Success output**:
  - rendu ASCII complet
- **Visibility rule**:
  - absente de l'aide standard et du menu guidé principal

*** Add File: c:\SOURCE\LoreSkills\specs\004-rework-repo-onboarding\quickstart.md
# Quickstart: Guided Repository Onboarding

## Prérequis

- Go 1.22 disponible localement.
- Un dépôt privé de test sur `github.com`, `gitlab.com`, `dev.azure.com` ou `bitbucket.org`.
- Une méthode d'authentification externe disponible pour chaque provider testé.
- Aucun module Go externe requis.

## Vérification locale de base

Depuis la racine du dépôt:

```bash
go test ./...
go vet ./...
CGO_ENABLED=0 go build -trimpath -ldflags "-X loreskills/cmd.version=0.0.0-dev -X loreskills/cmd.releaseAPIURL=https://api.github.com/repos/<owner>/<repo>/releases -X loreskills/cmd.releaseAssetName=loreskills-<os>-<arch>.tar.gz -X loreskills/cmd.releaseChecksumName=SHA256SUMS" -o loreskills .
```

Sous Windows PowerShell, construisez `loreskills.exe` et adaptez `releaseAssetName` à l'archive Windows.

## Matrice de guidance provider à valider

| Provider | Type de credential | Point d'entrée utilisateur à afficher | Permission minimale attendue | Variables d'environnement attendues |
| --- | --- | --- | --- | --- |
| GitHub | Fine-grained PAT recommandé | `https://github.com/settings/personal-access-tokens/new` | `Contents: Read` | `GITHUB_TOKEN`, `LORESKILLS_TOKEN` |
| GitLab | Personal access token | `https://gitlab.com/-/user_settings/personal_access_tokens` | `read_repository` | `GITLAB_TOKEN`, `LORESKILLS_TOKEN` |
| Azure DevOps | PAT | `https://dev.azure.com/<organisation>/_usersSettings/tokens` | `Code (Read)` | `AZURE_DEVOPS_EXT_PAT`, `AZURE_DEVOPS_TOKEN`, `LORESKILLS_TOKEN` |
| Bitbucket Cloud | API token | Atlassian Account > Security > Create and manage API tokens, avec doc `https://support.atlassian.com/bitbucket-cloud/docs/create-an-api-token/` | `Repositories: Read` | `BITBUCKET_TOKEN`, ou couple user/token si conservé pour compatibilité, plus `LORESKILLS_TOKEN` |

## Flux de validation manuelle

### 1. Vérifier le point d'entrée racine

Dans un terminal interactif:

```bash
loreskills
```

Valider que:

- un menu guidé s'ouvre
- l'état projet est affiché
- l'action primaire change selon que le projet soit configuré ou non
- le statut de mise à jour stable connu apparaît sans déclencher de prompt annexe

Puis vérifier le fallback non interactif:

```bash
loreskills < /dev/null
```

ou son équivalent PowerShell, puis valider que:

- aucun prompt n'est ouvert
- une guidance courte est imprimée
- au moins une prochaine commande utile est proposée

### 2. Vérifier le setup provider guidé

Pour chaque provider hébergé supporté:

```bash
loreskills init
```

Valider que le flux:

- n'accepte que le host hébergé supporté
- affiche l'URL/action correcte pour créer le token
- affiche la permission minimale correcte
- rappelle les variables d'environnement supportées
- valide l'accès avant d'écrire `.loreskills/project.json`
- ne persiste jamais le token saisi à la main

Valider aussi la migration legacy:

- créer un `~/.loreskillsrc` legacy GitHub
- lancer `loreskills init`
- vérifier l'import proposé puis l'écriture du nouveau profil projet

### 3. Vérifier la découverte `SKILLS.md` puis le fallback scan

Préparer deux dépôts de test:

- un dépôt avec `SKILLS.md` valide et plusieurs bundles `category/folder/SKILL.md`
- un dépôt sans `SKILLS.md`, mais avec une structure compatible comme:

```text
react/form-wizard/SKILL.md
commands/release-helper/SKILL.md
.net/azure-bootstrap/SKILL.md
```

Exécuter:

```bash
loreskills list
```

Valider que:

- `SKILLS.md` gagne quand il existe
- le scan est utilisé uniquement si `SKILLS.md` est absent
- un `SKILLS.md` présent mais invalide provoque une erreur explicite
- chaque bundle listé expose une référence canonique `category/folder`

### 4. Vérifier la synchro de bundle complet et la protection des chemins

Installer un bundle contenant `SKILL.md` et des fichiers compagnons:

```bash
loreskills install react/form-wizard
```

Valider que:

- le bundle complet est écrit sous `<target_dir>/react/form-wizard/...`
- `<target_dir>/lore-state.json` contient la liste complète des fichiers gérés
- toute tentative de chemin sortant de `<target_dir>` est rejetée

Modifier ensuite localement un fichier géré et relancer:

```bash
loreskills update
```

Valider que:

- le fichier modifié est écrasé par la version du dépôt
- un fichier supprimé côté dépôt disparaît aussi de la zone gérée locale
- un fichier non géré hors manifeste n'est pas touché

### 5. Vérifier le cache de mise à jour stable

Dans un terminal interactif:

```bash
loreskills
loreskills
```

Valider que:

- le premier démarrage peut déclencher un check réseau si le cache est absent/stale
- le second démarrage, à moins de 24 h, réutilise l'état du cache sans nouvel appel réseau
- l'état affiché distingue absence d'update, update disponible et erreur non bloquante

Puis vérifier le chemin explicite:

```bash
loreskills update self
```

Valider que:

- la commande force un check stable immédiat
- les pré-releases sont ignorées
- le checksum est validé avant remplacement
- un échec de remplacement laisse le binaire courant intact

## Cibles de test attendues

- `cmd/root_test.go`, `cmd/menu_test.go`: détection TTY, menu racine, fallback non interactif
- `cmd/init_test.go`, `cmd/providers_test.go`: guidance provider, rejection self-hosted, migration legacy, validation reachability
- `cmd/list_test.go`, `cmd/install_test.go`, `cmd/update_test.go`: priorité `SKILLS.md`, fallback scan, synchro bundle complète, écrasement local
- `internal/http/client_test.go`, `internal/http/discovery_test.go`: parse catalogue, endpoints de listing, pagination et fetch provider-aware
- `internal/fs/fs_test.go`, `internal/fs/state_cache_test.go`: sécurité de chemins, manifeste de bundles, cache update 24 h, nettoyage des fichiers obsolètes
- `README.md`: documentation utilisateur alignée avec le menu guidé, les providers hébergés et la nouvelle politique Bitbucket API token
