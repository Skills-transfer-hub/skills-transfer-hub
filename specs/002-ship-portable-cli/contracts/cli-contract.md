# CLI Contract: Ship Portable CLI

## Shared Output Contract

- **Applies to**: toutes les commandes visibles par l'utilisateur.
- **Behavior**:
  - chaque sortie visible du CLI inclut le même avatar ASCII
  - les messages de succès et d'information sont écrits sur `stdout`
  - les messages d'erreur sont écrits sur `stderr`
  - l'avatar ne remplace jamais le message fonctionnel principal
- **Exit code**:
  - `0` sur succès ou no-op attendu
  - non-zéro sur erreur utilisateur ou erreur d'exécution

## Command: `loreskills`

- **Purpose**: Afficher l'aide générale par défaut.
- **Input**: aucun argument.
- **Success output**:
  - avatar ASCII
  - liste des commandes `init`, `install`, `update`, `help`, `version`
  - exemple de démarrage
- **Exit code**: `0`.

## Command: `loreskills help [commande]`

- **Purpose**: Afficher l'aide générale ou l'aide ciblée d'une commande.
- **Input**:
  - argument optionnel : `commande`
- **Success output**:
  - avatar ASCII
  - si aucun argument : aide générale
  - si une commande valide est fournie : usage, résumé, arguments et exemple de cette commande
- **Failure cases**:
  - commande cible inconnue → erreur explicite et redirection vers la liste des commandes valides
- **Exit code**: `0` sur succès, non-zéro sur erreur.

## Command: `loreskills version`

- **Purpose**: Afficher la version installée du CLI.
- **Input**: aucun argument.
- **Success output**:
  - avatar ASCII
  - version lisible du build
  - métadonnées de build disponibles si elles existent (commit, date, cible)
- **Exit code**: `0`.

## Command: `loreskills init`

- **Purpose**: Créer la configuration utilisateur initiale dans le répertoire personnel.
- **Input**: aucun argument.
- **Side effects**:
  - crée `.loreskillsrc` dans le home utilisateur si absent
  - écrit les clés JSON `repo_raw_url` et `target_dir`
- **Success output**:
  - avatar ASCII
  - confirmation de création
  - prochaine étape indiquant de renseigner `repo_raw_url`
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
- **Success output**:
  - avatar ASCII
  - confirmation d'installation
  - prochaine étape utile (`update`, `help`, ou usage du skill)
- **Failure cases**:
  - configuration absente ou invalide
  - argument manquant
  - `404` distant ou autre erreur HTTP
  - erreur de création de dossier ou d'écriture de fichier
- **Exit code**: `0` sur succès, non-zéro sur erreur.

## Command: `loreskills update`

- **Purpose**: Retélécharger tous les skills déjà suivis dans le projet courant.
- **Input**: aucun argument.
- **Side effects**:
  - lit `.loreskillsrc`
  - lit `<target_dir>/lore-state.json`
  - pour chaque skill enregistré, rejoue le flux de téléchargement et d'écriture
- **Success output**:
  - avatar ASCII
  - message de mise à jour pour chaque skill rafraîchi, ou
  - `Aucun skill à mettre à jour.` si aucun skill n'est enregistré
- **Failure cases**:
  - configuration absente ou invalide
  - état illisible
  - échec de téléchargement ou d'écriture pour un skill donné
- **Exit code**: `0` si rien à mettre à jour ou si tous les rafraîchissements réussissent, non-zéro si au moins un skill échoue après tentative de traitement de tous les skills enregistrés.