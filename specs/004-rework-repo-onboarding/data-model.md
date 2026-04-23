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
