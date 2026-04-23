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
- chaque bundle listé expose une référence canonique `category/folder` dans la sortie de `loreskills list`

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
- un fichier non géré hors manifeste n'est pas touché, même s'il reste sous `<target_dir>`

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
- quand une stable est disponible, le menu expose aussi l'action `update-self`

Puis vérifier le chemin explicite:

```bash
loreskills update self
```

Valider que:

- la commande force un check stable immédiat
- les pré-releases sont ignorées
- le checksum est validé avant remplacement
- un échec de remplacement laisse le binaire courant intact
- le cache machine-local est réécrit avec le résultat le plus récent

## Cibles de test attendues

- `cmd/root_test.go`, `cmd/menu_test.go`: détection TTY, menu racine, fallback non interactif
- `cmd/init_test.go`, `cmd/providers_test.go`: guidance provider, rejection self-hosted, migration legacy, validation reachability
- `cmd/list_test.go`, `cmd/install_test.go`, `cmd/update_test.go`: priorité `SKILLS.md`, fallback scan, synchro bundle complète, écrasement local
- `internal/http/client_test.go`, `internal/http/discovery_test.go`: parse catalogue, endpoints de listing, pagination et fetch provider-aware
- `internal/fs/fs_test.go`, `internal/fs/state_cache_test.go`: sécurité de chemins, manifeste de bundles, cache update 24 h, nettoyage des fichiers obsolètes
- `README.md`: documentation utilisateur alignée avec le menu guidé, les providers hébergés et la nouvelle politique Bitbucket API token