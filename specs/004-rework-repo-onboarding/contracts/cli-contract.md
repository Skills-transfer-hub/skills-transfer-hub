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
  - affiche l'état de mise à jour stable connu (`aucune`, `disponible`, `erreur non bloquante`) avec un cache machine-local de 24 h
  - propose des actions adaptées à l'état courant
- **Expected action sets**:
  - `unconfigured`: configurer le dépôt, afficher l'aide, quitter
  - `configured-invalid`: rééditer la configuration, afficher l'aide, quitter
  - `configured-ready`: lister les bundles, installer un bundle, mettre à jour les bundles gérés, lancer `update-self` si une stable est disponible, revoir la configuration, relire la guidance provider, réexécuter `init`, afficher l'aide, quitter
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
  - une ligne par ressource avec au minimum son type, sa référence affichée et sa description si disponible
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
  - confirmation ressource par ressource, ou message `Aucune ressource à mettre à jour.`
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
  - réécrit le cache machine-local avec le dernier résultat stable connu
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
