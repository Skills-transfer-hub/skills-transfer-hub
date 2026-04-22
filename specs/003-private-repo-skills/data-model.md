# Data Model: Private Repo Skills

## ProjectRepositoryProfile

- **Purpose**: Représente la configuration LoreSkills propre au projet courant.
- **Fields**:
  - `provider` (`string`, required): fournisseur de dépôt (`github`, `gitlab`, `azure-devops`, `bitbucket`).
  - `repository_id` (`string`, required): identifiant canonique du dépôt pour le provider choisi.
  - `ref` (`string`, required): branche ou tag utilisé pour lire le catalogue et les ressources.
  - `catalog_path` (`string`, required): chemin vers `SKILLS.md` dans le dépôt distant.
  - `target_dir` (`string`, required): chemin relatif au projet courant où installer les ressources.
  - `credential_mode` (`string`, derived): mode d'accès externe (`env`, `git-credential`, `provider-session`).
  - `migrated_from_legacy` (`bool`, optional): indique si le profil provient d'un import depuis `~/.loreskillsrc`.
- **Validation rules**:
  - `provider` doit appartenir à la liste des providers pris en charge.
  - `catalog_path` doit cibler `SKILLS.md` et rester un chemin relatif dans le dépôt distant.
  - `target_dir` doit rester un chemin relatif au projet courant.
  - Le profil ne doit contenir aucun secret privé.
- **State transitions**:
  - `absent` → créé par `init`.
  - `initialized` → renseigné par l'utilisateur ou importé depuis l'ancien fichier home.
  - `validated` → reachability du dépôt et lecture du catalogue confirmées.
  - `updated` → modifié par une nouvelle exécution de `init` ou un réglage équivalent.

## AccessSource

- **Purpose**: Représente la source externe d'authentification utilisée pour accéder au dépôt privé.
- **Fields**:
  - `kind` (`string`, required): type de source (`env`, `git-credential`, `provider-session`).
  - `hint` (`string`, optional): nom de variable d'environnement, hôte Git ou indication utilisateur.
  - `resolved` (`bool`, required): indique si LoreSkills a trouvé une source exploitable.
- **Validation rules**:
  - Une tentative d'accès distant doit utiliser exactement une source résolue à la fois.
  - Les informations stockées dans cette structure ne doivent pas exposer la valeur brute d'un secret.

## CatalogEntry

- **Purpose**: Représente une ressource installable déclarée dans le catalogue `SKILLS.md`.
- **Fields**:
  - `catalog_id` (`string`, derived): identifiant stable dérivé de `resource_type`, `folder_name` et `name`.
  - `resource_type` (`string`, required): type logique (`skill`, `agent`, ou autre ressource supportée).
  - `name` (`string`, required): nom de la ressource tel qu'exposé à l'utilisateur.
  - `folder_name` (`string`, required): dossier logique ou physique associé à l'installation.
  - `source_path` (`string`, required): chemin relatif de la ressource dans le dépôt distant.
  - `description` (`string`, optional): description courte affichée dans `list`.
- **Validation rules**:
  - Le triplet `resource_type` + `folder_name` + `name` doit être unique dans un catalogue.
  - `folder_name` et `source_path` doivent être non vides.
  - `source_path` doit rester relatif au dépôt distant.
- **State transitions**:
  - `discovered` → extrait de `SKILLS.md`.
  - `selected` → ciblé par `install`.
  - `installed` → enregistré dans l'état local.
  - `updated` → rafraîchi par `update`.

## InstalledResourceState

- **Purpose**: Représente le contenu de `<target_dir>/lore-state.json` pour suivre les ressources installées depuis le catalogue.
- **Fields**:
  - `entries` (`[]InstalledResourceRecord`, required): ensemble des ressources suivies.
- **InstalledResourceRecord fields**:
  - `catalog_id` (`string`, required): référence stable vers l'entrée du catalogue.
  - `resource_type` (`string`, required): type de ressource installée.
  - `name` (`string`, required): nom de la ressource.
  - `folder_name` (`string`, required): dossier d'installation déclaré par le catalogue.
  - `installed_path` (`string`, required): chemin relatif écrit dans le projet courant.
  - `source_path` (`string`, required): chemin source distant utilisé pour les mises à jour.
  - `status` (`string`, required): valeur attendue `installed`.
- **Validation rules**:
  - `catalog_id` doit être unique dans le fichier d'état.
  - `installed_path` doit rester sous `target_dir`.
  - Une ressource installée doit conserver son `source_path` d'origine pour rester réactualisable.
- **State transitions**:
  - `missing` → créé lors de la première installation.
  - `tracked` → relu par `list`, `install` et `update`.
  - `refreshed` → mis à jour après un `update` réussi.

## ReleaseChannelMetadata

- **Purpose**: Représente les métadonnées injectées au build pour vérifier et télécharger les releases stables de LoreSkills.
- **Fields**:
  - `current_version` (`string`, required): version embarquée dans le binaire courant.
  - `releases_api_url` (`string`, required): URL API des releases injectée au build.
  - `asset_name` (`string`, required): nom attendu de l'artefact pour la plateforme courante.
  - `latest_version` (`string`, optional): version stable la plus récente détectée.
  - `download_url` (`string`, optional): URL de téléchargement de l'artefact stable sélectionné.
  - `checksum_url` (`string`, optional): URL du manifeste de checksum associé.
- **Validation rules**:
  - `releases_api_url` doit être non vide dans une build distribuée.
  - Une release pré-release ne peut jamais devenir `latest_version` retenue.
  - `asset_name` doit correspondre à la plateforme de build courante.

## VersionSignature

- **Purpose**: Représente l'identité textuelle minimale exposée par `loreskills` et `loreskills version`.
- **Fields**:
  - `version` (`string`, required): version lisible du binaire.
  - `byline` (`string`, required): texte exact `dev with 🤖 by Hugo MR and Jeremy, Lead Tech.`.
  - `rendered` (`string`, derived): rendu final concis imprimé par les commandes.
- **Validation rules**:
  - Le rendu doit toujours inclure la version et la byline.
  - Le rendu ne doit pas réintroduire l'ancien guide ASCII animal.

## StockholmArtwork

- **Purpose**: Représente l'easter egg ASCII exposé par la commande cachée `stockholm`.
- **Fields**:
  - `command_name` (`string`, required): nom de la commande cachée (`stockholm`).
  - `art_lines` (`[]string`, required): lignes ASCII composant le rendu.
  - `visibility` (`string`, required): valeur attendue `hidden`.
- **Validation rules**:
  - La commande doit rester absente de l'aide standard.
  - Le rendu doit rester suffisamment compact pour un terminal courant.
