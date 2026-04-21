# Data Model: Core CLI et MVP

## UserConfig

- **Purpose**: Représente la configuration utilisateur persistée dans `.loreskillsrc`.
- **Fields**:
  - `repo_raw_url` (`string`, required): URL racine distante utilisée pour construire l'adresse `<repo_raw_url>/<nom-du-skill>.md`.
  - `target_dir` (`string`, required): chemin relatif au projet courant dans lequel les skills sont installés.
- **Validation rules**:
  - Les deux champs doivent être non vides pour permettre `install` et `update`.
  - `target_dir` est interprété comme un chemin relatif et doit pouvoir être créé dans le projet courant.
- **State transitions**:
  - `absent` → créé par `init` avec valeurs par défaut.
  - `existing` → préservé si `init` est relancé, avec retour d'une erreur explicite.

## InstalledSkillFile

- **Purpose**: Représente le fichier Markdown matérialisé dans le projet local pour un skill donné.
- **Fields**:
  - `skill_name` (`string`, derived): nom logique du skill, utilisé aussi pour le nom de fichier `<nom-du-skill>.md`.
  - `managed_header` (`string`, constant): avertissement LoreSkills ajouté en préfixe.
  - `body` (`string`, required): contenu Markdown téléchargé depuis la source distante.
  - `destination_path` (`string`, derived): `<cwd>/<target_dir>/<nom-du-skill>.md`.
- **Validation rules**:
  - Le fichier ne doit jamais être écrit si le téléchargement échoue ou retourne `404`.
  - Le préfixe LoreSkills doit toujours être présent après un `install` ou `update` réussi.

## SkillStateMap

- **Purpose**: Représente le contenu de `<target_dir>/lore-state.json` pour savoir quels skills sont gérés.
- **Fields**:
  - `entries` (`map[string]string`): map dont la clé est `skill_name` et la valeur attendue pour le MVP est `installed`.
- **Validation rules**:
  - Une installation réussie doit créer la map si elle n'existe pas et enregistrer l'entrée du skill.
  - Un `update` sans fichier d'état ou avec map vide ne modifie rien et affiche un message explicite.
- **State transitions**:
  - `missing` → créé lors du premier `install` réussi.
  - `populated` → relu par `update` pour déclencher un rafraîchissement de chaque skill enregistré.

## CommandExecutionResult

- **Purpose**: Modélise le contrat observable de chaque commande CLI.
- **Fields**:
  - `stdout_message` (`string`): message humain de succès ou d'information.
  - `stderr_message` (`string`, optional): message d'erreur exploitable.
  - `exit_code` (`int`): `0` sur succès, non-zéro sur erreur.
- **Validation rules**:
  - Les erreurs de configuration et de téléchargement doivent sortir avec un code non-zéro.
  - `loreskills` sans argument et `update` sans état doivent retourner `0` avec un message utile.
