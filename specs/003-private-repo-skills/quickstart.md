# Quickstart: Private Repo Skills

## Prérequis

- Go 1.22 disponible localement.
- Accès à un dépôt privé de test sur GitHub, GitLab, Azure DevOps ou Bitbucket.
- Une méthode d'authentification externe déjà disponible, via variables d'environnement ou gestionnaire d'identifiants Git/OS.
- Aucun module Go externe requis.

## Vérification locale rapide

Depuis la racine du dépôt :

```bash
go test ./...
go vet ./...
CGO_ENABLED=0 go build -trimpath -ldflags "-X loreskills/cmd.version=0.0.0-dev -X loreskills/cmd.releaseAPIURL=https://api.github.com/repos/<owner>/<repo>/releases -X loreskills/cmd.releaseAssetName=loreskills-<os>-<arch>.tar.gz -X loreskills/cmd.releaseChecksumName=SHA256SUMS" -o loreskills .
```

Sous Windows PowerShell, construisez `loreskills.exe` et adaptez `releaseAssetName` au paquet Windows.

Préparez aussi une authentification externe avant les vérifications manuelles, par exemple `GITHUB_TOKEN`, `GITLAB_TOKEN`, `AZURE_DEVOPS_EXT_PAT`, ou `BITBUCKET_TOKEN` selon le provider ciblé.

## Flux de validation manuelle

### 1. Initialiser le projet courant

Dans un répertoire de projet de test :

```bash
loreskills init
```

Valider que le flux :

- propose le provider ciblé (`github`, `gitlab`, `azure-devops`, `bitbucket`)
- permet d'importer l'ancien `~/.loreskillsrc` si présent
- écrit un profil projet dans `.loreskills/project.json`
- ne stocke aucun secret privé dans ce fichier

### 2. Vérifier la découverte distante

```bash
loreskills list
```

Valider que LoreSkills :

- lit `SKILLS.md` depuis le dépôt configuré
- affiche chaque ressource avec son type, son nom et son `folder_name`
- retourne une erreur exploitable si l'accès distant échoue ou si le catalogue est mal formé

### 3. Installer puis mettre à jour une ressource

```bash
loreskills install skills/analyse-risque
loreskills update
```

Valider que :

- la ressource choisie est écrite sous `<target_dir>/<folder>/...`
- `<target_dir>/lore-state.json` suit l'identité complète de la ressource installée
- `update` recharge les ressources suivies depuis leur `source_path` d'origine

### 4. Vérifier la présentation CLI minimale

```bash
loreskills
loreskills version
loreskills help
loreskills stockholm
```

Valider que :

- `loreskills` et `loreskills version` affichent la même signature minimale avec la byline demandée
- l'ancien guide ASCII animal n'apparaît plus sur les sorties standard
- `loreskills help` documente les commandes visibles mais n'expose pas `stockholm`
- `loreskills stockholm` rend l'art ASCII caché

### 5. Vérifier la mise à jour stable-only du CLI

```bash
loreskills update self
```

Valider que :

- LoreSkills consulte uniquement le canal des releases stables injecté au build
- les pré-releases sont ignorées lors de la sélection de version
- le checksum `SHA256SUMS` est consulté avant toute tentative de remplacement
- une erreur d'écriture ou de remplacement du binaire laisse la version courante intacte et fournit une guidance corrective

## Cibles de test attendues

- `cmd/`: `init`, `list`, `install`, `update`, `help`, `version`, `stockholm`, sortie racine minimale, erreurs et flux interactifs
- `internal/fs/`: profil projet, migration depuis `~/.loreskillsrc`, installation par dossier, évolution de `lore-state.json`, remplacement local du binaire
- `internal/http/`: lecture de `SKILLS.md`, téléchargement des ressources, résolution d'authentification externe, lecture des releases stables
- `README.md`: guidance de setup projet, providers privés pris en charge, usage de `list`, et flux de self-update
