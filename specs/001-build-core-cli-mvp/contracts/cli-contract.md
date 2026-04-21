# CLI Contract: LoreSkills Core MVP

## Command: `loreskills`

- **Purpose**: Afficher l'aide par défaut si aucun argument n'est fourni.
- **Input**: Aucun argument.
- **Success output**: Liste des commandes `init`, `install`, `update` et usage minimal.
- **Exit code**: `0`.

## Command: `loreskills init`

- **Purpose**: Créer la configuration utilisateur initiale dans le répertoire personnel.
- **Input**: Aucun argument.
- **Side effects**:
  - crée `.loreskillsrc` dans le home utilisateur si absent
  - écrit les clés JSON `repo_raw_url` et `target_dir`
- **Success output**: message confirmant la création et demandant de personnaliser `repo_raw_url`
- **Failure cases**:
  - fichier déjà présent → erreur claire, aucun écrasement
  - impossibilité de résoudre le home ou d'écrire le fichier → erreur claire
- **Exit code**: `0` sur succès, non-zéro sur erreur.

## Command: `loreskills install <nom-du-skill>`

- **Purpose**: Télécharger un skill distant et l'installer dans le projet courant.
- **Input**:
  - argument positionnel requis : `nom-du-skill`
- **Side effects**:
  - lit `.loreskillsrc`
  - télécharge `<repo_raw_url>/<nom-du-skill>.md`
  - crée `<target_dir>` dans le CWD si nécessaire
  - écrit `<target_dir>/<nom-du-skill>.md` avec l'en-tête LoreSkills
  - crée ou met à jour `<target_dir>/lore-state.json`
- **Success output**: message confirmant l'installation du skill.
- **Failure cases**:
  - configuration absente ou invalide
  - `404` distant ou autre erreur HTTP
  - erreur de création de dossier ou d'écriture de fichier
- **Exit code**: `0` sur succès, non-zéro sur erreur.

## Command: `loreskills update`

- **Purpose**: Retélécharger tous les skills déjà suivis dans le projet courant.
- **Input**: Aucun argument.
- **Side effects**:
  - lit `.loreskillsrc`
  - lit `<target_dir>/lore-state.json`
  - pour chaque skill enregistré, rejoue le flux de téléchargement et d'écriture
- **Success output**:
  - message de mise à jour pour chaque skill rafraîchi, ou
  - `Aucun skill à mettre à jour.` si aucun skill n'est enregistré
- **Failure cases**:
  - configuration absente ou invalide
  - état illisible
  - échec de téléchargement ou d'écriture pour un skill donné
- **Exit code**: `0` si rien à mettre à jour ou si tous les rafraîchissements réussissent, non-zéro si une erreur bloque l'opération globale selon la stratégie retenue à l'implémentation.
