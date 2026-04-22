# CLI Contract: Private Repo Skills

## Shared Output Contract

- **Applies to**: toutes les commandes visibles par l'utilisateur.
- **Behavior**:
  - les sorties de succès et d'information sont écrites sur `stdout`
  - les erreurs utilisateur et les erreurs d'exécution sont écrites sur `stderr`
  - l'ancien guide ASCII animal n'apparaît plus sur les commandes standard
  - la commande cachée `stockholm` est exclue de l'aide standard et du catalogue des commandes visibles
- **Exit code**:
  - `0` sur succès ou no-op attendu
  - non-zéro sur erreur utilisateur ou erreur d'exécution

## Command: `loreskills`

- **Purpose**: Afficher uniquement la signature de version minimale du binaire courant.
- **Input**: aucun argument.
- **Success output**:
  - version installée
  - message `dev with 🤖 by Hugo MR and Jeremy, Lead Tech.`
- **Exit code**: `0`.

## Command: `loreskills help [commande]`

- **Purpose**: Afficher l'aide générale ou l'aide ciblée d'une commande visible.
- **Input**:
  - argument optionnel : `commande`
- **Success output**:
  - si aucun argument : liste des commandes `init`, `list`, `install`, `update`, `help`, `version`
  - si une commande valide est fournie : usage, résumé et exemples
- **Failure cases**:
  - commande cible inconnue → erreur explicite et redirection vers l'aide générale
- **Exit code**: `0` sur succès, non-zéro sur erreur.

## Command: `loreskills init`

- **Purpose**: Guider l'utilisateur dans la configuration projet et la migration éventuelle depuis `~/.loreskillsrc`.
- **Input**: aucun argument.
- **Side effects**:
  - crée `.loreskills/project.json` dans le projet courant si absent
  - peut importer `target_dir` et les informations équivalentes de l'ancien fichier home après confirmation
  - vérifie que le dépôt distant et `SKILLS.md` sont joignables avec l'authentification externe disponible
- **Success output**:
  - confirmation du provider et du dépôt retenus
  - emplacement du profil projet
  - prochaine étape `loreskills list`
- **Failure cases**:
  - absence d'informations minimales sur le dépôt
  - authentification externe introuvable
  - dépôt ou catalogue non joignable
- **Exit code**: `0` sur succès, non-zéro sur erreur.

## Command: `loreskills list`

- **Purpose**: Lire `SKILLS.md` depuis le dépôt configuré et afficher les ressources installables.
- **Input**: aucun argument.
- **Success output**:
  - une entrée par ressource avec `type`, `name`, `folder` et description courte si disponible
- **Failure cases**:
  - projet non configuré
  - accès distant refusé ou indisponible
  - catalogue absent ou mal formé
- **Exit code**: `0` sur succès, non-zéro sur erreur.

## Command: `loreskills install <resource-ref>`

- **Purpose**: Installer une ressource déclarée dans le catalogue distant.
- **Input**:
  - argument positionnel requis : `resource-ref`
  - format attendu : nom unique ou forme désambiguïsée `folder/name`
- **Side effects**:
  - lit `.loreskills/project.json`
  - lit `SKILLS.md` pour résoudre la ressource choisie
  - télécharge la ressource distante à partir de son `source_path`
  - écrit la ressource dans `<target_dir>/<folder_name>/...`
  - crée ou met à jour `<target_dir>/lore-state.json`
- **Success output**:
  - confirmation de la ressource installée
  - type et dossier retenus
  - dossier d'installation retenu
  - prochaine étape utile `update`
- **Failure cases**:
  - argument manquant
  - ressource introuvable ou ambiguë dans le catalogue
  - erreur d'accès distant ou d'écriture locale
- **Exit code**: `0` sur succès, non-zéro sur erreur.

## Command: `loreskills update`

- **Purpose**: Rafraîchir toutes les ressources suivies dans le projet courant.
- **Input**: aucun argument.
- **Side effects**:
  - relit `.loreskills/project.json`
  - relit `<target_dir>/lore-state.json`
  - retélécharge chaque ressource suivie depuis son `source_path`
- **Success output**:
  - confirmation par ressource mise à jour ou message `Aucune ressource à mettre à jour.`
- **Failure cases**:
  - projet non configuré
  - état local illisible
  - échec partiel ou total de téléchargement/écriture
- **Exit code**: `0` sur succès ou no-op, non-zéro si au moins une ressource échoue.

## Command: `loreskills update self`

- **Purpose**: Vérifier les releases stables de LoreSkills et appliquer la plus récente disponible pour la plateforme courante.
- **Input**:
  - argument positionnel requis : `self`
- **Side effects**:
  - lit les métadonnées de canal de release injectées au build
  - consulte les releases stables distantes
  - télécharge et vérifie l'artefact de la plateforme courante si une version plus récente existe
  - tente le remplacement du binaire courant
- **Success output**:
  - version stable détectée
  - confirmation d'application ou message `Aucune mise à jour stable disponible.`
- **Failure cases**:
  - métadonnées de release absentes
  - pré-release uniquement disponible
  - téléchargement, checksum ou remplacement du binaire en échec
- **Exit code**: `0` sur succès ou absence de mise à jour stable, non-zéro sur erreur.

## Command: `loreskills version`

- **Purpose**: Afficher la même signature minimale que `loreskills`.
- **Input**: aucun argument.
- **Success output**:
  - version installée
  - message `dev with 🤖 by Hugo MR and Jeremy, Lead Tech.`
- **Exit code**: `0`.

## Command: `loreskills stockholm`

- **Purpose**: Afficher l'art ASCII caché inspiré du visuel fourni.
- **Input**: aucun argument.
- **Success output**:
  - rendu ASCII Stockholm complet
- **Failure cases**:
  - aucune spécifique hors erreur standard d'écriture sur sortie
- **Exit code**: `0`.
