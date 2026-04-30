# STH — Skills Transfer Hub

STH est un CLI Go, distribué en binaire unique, pour connecter un projet à un dépôt hébergé de ressources IA, lire un catalogue `SKILLS.md`, installer des ressources dans le projet courant, puis les maintenir à jour.

Le flux actuel est centré sur le projet courant, pas sur une configuration partagée entre tous les dépôts. STH écrit donc un profil local dans `.sth/project.json` et ne stocke jamais de secret privé dans ce fichier.

## Commandes visibles

- `sth` ouvre le menu guidé en terminal interactif, y affiche l'état du projet et l'offre de mise à jour stable en cache, puis affiche la prochaine étape utile en contexte non interactif
- `sth init` configure de manière interactive un dépôt hébergé supporté pour le projet courant
- `sth login [<api-key>]` authentifie le CLI auprès de **LoreSkills Cloud** (provider SaaS) — clé stockée dans le keychain OS
- `sth logout` supprime la clé LoreSkills du keychain
- `sth list` lit `SKILLS.md` ou détecte les bundles compatibles et liste les ressources installables
- `sth install <resource-ref>` installe une ressource du catalogue distant
- `sth update` met à jour les ressources suivies dans le projet courant
- `sth update self` vérifie et applique la dernière release GitHub stable compatible avec la plateforme courante
- `sth buddy` affiche l'état du projet courant sous forme de compagnon visuel
- `sth help [commande]` affiche l'aide générale ou ciblée
- `sth version` affiche la même signature minimale que `sth`

## Provider LoreSkills Cloud (SaaS)

En plus des providers Git classiques (GitHub / GitLab / Azure DevOps / Bitbucket), STH peut
maintenant pointer sur le SaaS **LoreSkills Cloud** : pas de PAT Git à gérer, l'auth passe par
une API key hashée côté serveur.

### Workflow

1. Sur le dashboard (`sth-dashboard`), génère une licence dans `/dashboard/licenses`.
   La clé `lsk_live_…` n'est affichée **qu'une seule fois** — copie-la immédiatement.
2. Sur ta machine :
   ```bash
   sth login                       # demande la clé interactivement
   # ou
   sth login lsk_live_xxxxxxxxx    # passe la clé en argument (CI / scripting)
   ```
3. La clé est validée via `/v1/access-check`, puis stockée dans le keychain OS
   (Windows Credential Manager / macOS Keychain / Linux libsecret).
4. STH écrit automatiquement un provider `loreskills-cloud` dans `.sth/project.json`.
5. Toutes les commandes suivantes (`sth list`, `sth install`, …) tapent l'API LoreSkills.
6. Après chaque commande réussie, un record d'usage est envoyé à `/v1/usage`
   (best-effort, ne bloque jamais le CLI).

### Variables d'environnement

| Variable | Rôle |
|---|---|
| `LORESKILLS_API_KEY` | Fallback de la clé si le keychain n'est pas dispo (CI headless) |
| `STH_LORESKILLS_BASE_URL` | Override l'URL de base (default : prod). Ex. dev local : `http://127.0.0.1:54321/functions/v1/cli` |

### Quotas (Phase 1)

| Plan | Skills installés max |
|---|---|
| starter | 3 |
| pro | 10 |
| team | ∞ |

Au-delà du quota, `sth install` renvoie 402 `quota_exceeded`. Réinstaller un skill déjà
présent est idempotent (ne consomme pas de quota).

## Installation

### Via un gestionnaire de paquets

Sous macOS ou Linux (Homebrew tap personnel) :

```bash
brew install Grandpied33/sth/sth
```

Sous Windows (winget) :

```powershell
winget install Grandpied33.sth
```

Sous Windows (Chocolatey) :

```powershell
choco install sth
```

> Si vous installez STH via un gestionnaire de paquets, utilisez ce gestionnaire pour les mises à jour (`brew upgrade sth`, `winget upgrade Grandpied33.sth`, `choco upgrade sth`) plutôt que `sth update self`. La commande `sth update self` reste fonctionnelle mais sera écrasée à la prochaine mise à jour du paquet.

### Depuis GitHub Releases

STH est publié sur GitHub Releases avec les archives suivantes :

- `sth-windows-amd64.zip`
- `sth-linux-amd64.tar.gz`
- `sth-darwin-amd64.tar.gz`
- `sth-darwin-arm64.tar.gz`
- `SHA256SUMS`

Chaque build stable embarque aussi les métadonnées de release nécessaires à `sth update self`.

### Vérifier une archive

Sous macOS ou Linux :

```bash
sha256sum -c SHA256SUMS --ignore-missing
```

Sous Windows PowerShell :

```powershell
$expected = (Select-String -Path .\SHA256SUMS -Pattern 'sth-windows-amd64.zip').ToString().Split(' ')[0]
$actual = (Get-FileHash .\sth-windows-amd64.zip -Algorithm SHA256).Hash.ToLower()
if ($expected -ne $actual) { throw 'Checksum mismatch' }
```

### Installation globale

Sous macOS ou Linux :

```bash
tar -xzf sth-linux-amd64.tar.gz
sudo install sth-linux-amd64/sth /usr/local/bin/sth
sth version
```

Sous Windows PowerShell :

```powershell
Expand-Archive .\sth-windows-amd64.zip -DestinationPath .\sth-windows-amd64 -Force
New-Item -ItemType Directory -Path "$env:USERPROFILE\bin" -Force | Out-Null
Copy-Item .\sth-windows-amd64\sth.exe "$env:USERPROFILE\bin\sth.exe" -Force
sth.exe version
```

## Sortir une release Alpha

Le pipeline GoReleaser et la détection brew/scoop sont déjà en place ; il reste à finaliser
les canaux de distribution. Cette section sert de checklist quand on s'y remettra.

### Validation locale (à faire au moins une fois avant le premier tag)

À exécuter sur le poste de release, sans rien pousser sur GitHub :

1. **Installer GoReleaser** (`brew install goreleaser` ou `scoop install goreleaser`).
2. **Smoke build** :
   ```bash
   goreleaser release --snapshot --clean --skip=publish
   ls dist/                 # binaires + SHA256SUMS + sth.rb + sth.json
   ```
3. **Inspecter** `dist/sth.rb` et `dist/sth.json` : URLs cohérentes, SHA256 réels, structure conforme.
4. **Tester un binaire localement** :
   ```bash
   ./dist/sth-windows-amd64_windows_amd64_v1/sth.exe version
   STH_EXECUTABLE_PATH="/opt/homebrew/Cellar/sth/0.1.0/bin/sth" \
     ./dist/sth-darwin-arm64_darwin_arm64/sth update self   # doit renvoyer le hint brew
   ```
5. **E2E contre le back** (avec une clé de test générée via le dashboard) :
   ```bash
   export STH_LORESKILLS_BASE_URL=https://jcdadtdkgnwobejkmpgv.supabase.co/functions/v1/cli
   ./dist/.../sth login lsk_live_…
   ./dist/.../sth list
   ./dist/.../sth install <skill>
   ```
6. **Remplacer les placeholders** dans `release-artifacts/homebrew-sth/Formula/sth.rb` et
   `release-artifacts/scoop-sth/bucket/sth.json` par le contenu généré dans `dist/` (les hash
   `0000…` doivent disparaître). Commit le résultat.

### Mise en place des repos de distribution (one-shot)

```bash
# Repos publics vides
gh repo create Grandpied33/homebrew-sth --public --description "Homebrew tap for sth"
gh repo create Grandpied33/scoop-sth    --public --description "Scoop bucket for sth"

# Bootstrap depuis les templates
git clone https://github.com/Grandpied33/homebrew-sth.git ../homebrew-sth
cp -r release-artifacts/homebrew-sth/* ../homebrew-sth/
(cd ../homebrew-sth && git add -A && git commit -m "bootstrap tap" && git push)

git clone https://github.com/Grandpied33/scoop-sth.git ../scoop-sth
cp -r release-artifacts/scoop-sth/* ../scoop-sth/
(cd ../scoop-sth && git add -A && git commit -m "bootstrap bucket" && git push)
```

### Release + promotion par tag

1. Tagger : `git tag v0.1.0-alpha.1 && git push --tags` puis créer la GitHub Release
   (ou `workflow_dispatch` sur le workflow Release pour validation).
2. GoReleaser produit binaires, archives, `SHA256SUMS`, `sth.rb`, `sth.json` et les
   attache à la release.
3. **Promouvoir** vers tap + bucket :
   ```bash
   TAG=v0.1.0-alpha.1
   gh release download "$TAG" -p sth.rb -p sth.json -D /tmp/sth-release
   cp /tmp/sth-release/sth.rb   ../homebrew-sth/Formula/sth.rb
   (cd ../homebrew-sth && git commit -am "sth $TAG" && git push)
   cp /tmp/sth-release/sth.json ../scoop-sth/bucket/sth.json
   (cd ../scoop-sth && git commit -am "sth $TAG" && git push)
   ```
4. **Vérifier sur 2 machines** :
   - macOS : `brew install grandpied33/sth/sth && sth version`
   - Windows : `scoop bucket add sth https://github.com/Grandpied33/scoop-sth && scoop install sth && sth version`
   - Sur les deux : `sth update self` doit renvoyer le hint brew/scoop, **pas** essayer de remplacer le binaire.

### Pièges à éviter

- **Ne pas tagger sans la validation locale** ci-dessus. Si une release Alpha plante côté
  login ou contrat backend, le gate Alpha bloque les utilisateurs jusqu'à la release suivante.
- **`sth update self`** est le seul filet pour les installs directes. S'il est cassé,
  les utilisateurs sont coincés à re-télécharger l'archive à la main. Le tester avant
  chaque release en simulant un binaire obsolète.
- **Pre-release vs stable** : le gate filtre `prerelease: true` côté GitHub. Si tu tagges
  comme prerelease, le gate ne le verra pas comme "latest stable" et passera. Pour l'Alpha
  on tagge en release stable mais avec `-alpha.N` dans la version.

### Reportés à la Beta

- **Winget** (manifest YAML, PR vers `microsoft/winget-pkgs`, modération 1-3 jours par release).
- **Chocolatey** (modération communautaire qui peut prendre des semaines).
- **Auto-PR vers tap/bucket** : passer `skip_upload: false` dans `.goreleaser.yaml` et
  fournir un `HOMEBREW_TAP_GITHUB_TOKEN`, une fois la cadence release stabilisée.
- **Linux arm64 / Windows arm64** : ajouter aux `goarch` du `.goreleaser.yaml` quand
  un utilisateur le demande.

## Démarrage rapide

### 0. Utiliser l'entrée guidée

Dans un terminal interactif, lancez simplement :

```bash
sth
```

STH détecte l'état du projet courant et propose l'action prioritaire suivante. Dans un contexte non interactif, la même commande affiche uniquement un résumé de l'état projet et la prochaine action recommandée sans ouvrir de prompt.

Si les métadonnées de release sont embarquées dans le binaire, le démarrage interactif vérifie aussi la dernière version stable disponible, puis met ce résultat en cache pendant 24 heures dans un cache machine-local. Quand une stable est disponible, le menu ajoute l'action `update-self`.

### 1. Configurer le projet courant

Depuis le projet où vous souhaitez installer des ressources :

```bash
sth init
```

Le flux interactif vous demande :

- le provider : `github`, `gitlab`, `azure-devops`, ou `bitbucket`
- affiche la documentation officielle du provider, l'URL d'action pour créer le token/API token, et la permission minimale attendue
- le `repository_id` du dépôt privé
- la référence Git à lire (`main` par défaut)
- le chemin du catalogue (`SKILLS.md` par défaut)
- le dossier cible du projet (par défaut `.claude/skills`)

Le `repository_id` peut être saisi sous forme canonique (`owner/repo`) ou sous forme d'URL hébergée supportée (`https://github.com/owner/repo`). Les hôtes self-hosted sont rejetés explicitement. Avant de continuer, le wizard propose aussi de relire la guidance provider sans relancer `sth init`.

Si un ancien `~/.sthrc` existe, STH propose de l'importer pour migrer vers le nouveau format projet.

Si un dossier `.loreskills/` est détecté dans le projet courant sans `.sth/`, STH propose aussi une migration automatique vers `.sth/`.

### 2. Authentification privée

STH ne persiste pas les secrets. Il s'appuie sur une authentification externe déjà disponible et rappelle dans `sth init` les URLs officielles pour créer le token/API token :

- GitHub : PAT via `https://github.com/settings/personal-access-tokens/new`, permission minimale `Contents: Read`, variables `GITHUB_TOKEN` ou `STH_TOKEN`
- GitLab : PAT via `https://gitlab.com/-/user_settings/personal_access_tokens`, permission minimale `read_repository`, variables `GITLAB_TOKEN` ou `STH_TOKEN`
- Azure DevOps : PAT via `https://dev.azure.com/<organisation>/_usersSettings/tokens`, permission minimale `Code (Read)`, variables `AZURE_DEVOPS_EXT_PAT`, `AZURE_DEVOPS_TOKEN`, ou `STH_TOKEN`
- Bitbucket Cloud : API token via `https://id.atlassian.com/manage-profile/security/api-tokens`, permission minimale `Repositories: Read`, variable `BITBUCKET_TOKEN` ou `STH_TOKEN`

STH accepte encore `BITBUCKET_USERNAME` et `BITBUCKET_APP_PASSWORD` pour la compatibilité Bitbucket, mais la guidance produit recommande désormais l'API token Atlassian.

Le flux `init` vérifie l'accès au dépôt et au catalogue avant d'enregistrer la configuration du projet.

### 3. Déclarer `SKILLS.md` ou utiliser la structure compatible

STH commence toujours par lire un tableau Markdown depuis `SKILLS.md` quand il existe. Exemple minimal :

```md
| Type  | Name           | Folder | Source                   | Description          |
| ----- | -------------- | ------ | ------------------------ | -------------------- |
| skill | analyse-risque | skills | skills/analyse-risque.md | Check-list d'analyse |
| agent | reviewer       | agents | agents/reviewer.md       | Agent de revue       |
```

Règles importantes :

- `Type`, `Name`, `Folder`, et `Source` sont requis
- `Folder` est le dossier affiché par `sth list` et utilisé pour l'installation locale
- `Source` est le chemin distant utilisé pour `install` et `update`
- le format de référence utilisateur est soit un nom unique, soit `folder/name` en cas d'ambiguïté

Si `SKILLS.md` est absent, STH bascule automatiquement sur un scan compatible de l'arborescence distante. Toute entrée `category/folder/SKILL.md` devient alors un bundle installable avec une référence canonique `category/folder` affichée par `sth list`.

Exemple de structure compatible sans `SKILLS.md` :

```text
react/form-wizard/SKILL.md
commands/release-helper/SKILL.md
.net/azure-bootstrap/SKILL.md
```

### 4. Lister puis installer une ressource

```bash
sth list
sth install react/form-wizard
```

`sth list` affiche les ressources distantes avec leur type et, pour les bundles détectés par scan, leur référence canonique `category/folder`. Quand `SKILLS.md` existe, il reste la source unique de vérité; un `SKILLS.md` invalide provoque une erreur explicite et n'est pas fusionné avec le scan.

`sth install` :

- relit `.sth/project.json`
- relit `SKILLS.md` ou le fallback scan compatible
- résout la ressource demandée
- télécharge soit un `source_path` unique, soit tous les fichiers du bundle détecté
- écrit un bundle complet dans `<target_dir>/<category>/<folder>/...`
- met à jour `<target_dir>/sth-state.json`

### 5. Mettre à jour les ressources suivies

```bash
sth update
```

STH recharge chaque ressource installée à partir de son `source_path` d'origine. Pour un bundle suivi, tous les fichiers gérés sont resynchronisés, les fichiers supprimés côté dépôt sont retirés localement, et les modifications locales dans la zone gérée sont écrasées. Si une ressource échoue, les autres continuent à être rafraîchies et la commande retourne ensuite une erreur globale.

### 6. Mettre à jour le CLI lui-même

```bash
sth update self
```

Le self-update :

- force un check réseau stable immédiat même si le cache de démarrage est encore frais
- consulte le canal GitHub injecté au build
- ignore explicitement les pré-releases
- sélectionne l'artefact correspondant à la plateforme courante
- vérifie le checksum SHA256 avant tout remplacement local

Si aucune version stable plus récente n'existe, STH affiche `Aucune mise à jour stable disponible.`

### 7. Buddy — compagnon visuel

```bash
sth buddy
sth buddy --watch
```

`sth buddy` lit l'état du projet courant et affiche un compagnon visuel compact :

```
    💧

  ╭────╮
  │>/< │
  ╰────╯
Tout est à jour.
```

Les états possibles sont : à jour (💧 `>/<`), mise à jour disponible (⚠️ `o.O`), pas de config (😶 `._. `), erreur (❌ `x x`), opération en cours (⏳ `>_<`), et opération terminée (✅ `^.^`).

`sth buddy --watch` prend le contrôle du terminal et se met à jour automatiquement dès qu'un changement est détecté dans `.sth/` ou le dossier cible. Quitter avec Ctrl+C.

## Fichiers générés

### Profil projet

STH crée `.sth/project.json` dans le projet courant (format v2) :

```json
{
  "schema_version": 2,
  "providers": [
    {
      "id": "default",
      "provider": "github",
      "repository_id": "mon-org/mon-repo",
      "catalog_ref": "main",
      "catalog_path": "SKILLS.md",
      "target_dir": ".claude/skills"
    }
  ],
  "resources": []
}
```

Aucun secret n'est stocké dans ce fichier.

### État local des ressources

STH suit les ressources installées dans `<target_dir>/sth-state.json` :

```json
{
  "entries": [
    {
      "catalog_id": "bundle::react::form-wizard",
      "resource_type": "bundle",
      "name": "form-wizard",
      "folder_name": "react",
      "installed_path": ".claude/skills/react/form-wizard",
      "source_path": "react/form-wizard/SKILL.md",
      "managed_root": ".claude/skills/react/form-wizard",
      "source_root": "react/form-wizard",
      "managed_files": [
        {
          "relative_path": "SKILL.md",
          "installed_path": ".claude/skills/react/form-wizard/SKILL.md",
          "source_path": "react/form-wizard/SKILL.md"
        },
        {
          "relative_path": "templates/prompt.md",
          "installed_path": ".claude/skills/react/form-wizard/templates/prompt.md",
          "source_path": "react/form-wizard/templates/prompt.md"
        }
      ],
      "status": "installed"
    }
  ]
}
```

### Cache machine-local de mise à jour

STH peut aussi créer un cache machine-local pour les checks automatiques de mise à jour stable dans `os.UserCacheDir()/sth/update-check.json`.

- le cache est relu uniquement par le démarrage interactif
- sa durée de vie est de 24 heures
- `sth update self` ignore cette fraîcheur et force un check stable immédiat
- ce fichier ne contient aucun secret et n'est jamais écrit dans `.sth/project.json`

## Variables d'environnement

| Variable | Usage |
|----------|-------|
| `STH_TOKEN` | Token d'authentification (tous providers) |
| `STH_GITHUB_RAW_BASE_URL` | Override base URL GitHub raw content |
| `STH_GITHUB_API_BASE_URL` | Override base URL GitHub API |
| `STH_GITLAB_API_BASE_URL` | Override base URL GitLab API |
| `STH_AZURE_DEVOPS_BASE_URL` | Override base URL Azure DevOps |
| `STH_BITBUCKET_API_BASE_URL` | Override base URL Bitbucket API |
| `STH_USER_CACHE_DIR` | Override du dossier de cache machine-local |
| `STH_EXECUTABLE_PATH` | Override du chemin de l'exécutable (self-update) |

## Validation locale

Tests et lint Go :

```bash
go test ./...
go vet ./...
```

Build local avec métadonnées de release :

```bash
go build -trimpath -ldflags "-X sth/cmd.version=0.0.0-dev -X sth/cmd.releaseAPIURL=https://api.github.com/repos/<owner>/<repo>/releases -X sth/cmd.releaseAssetName=sth-<os>-<arch>.tar.gz -X sth/cmd.releaseChecksumName=SHA256SUMS" -o sth .
```

Sous Windows, remplacez `releaseAssetName` par l'archive `.zip` attendue pour la plateforme.

## Remarques utiles

- `target_dir` doit rester relatif au projet courant
- le profil projet remplace la configuration partagée historique, mais `init` peut importer `~/.sthrc`
- `sth help` documente uniquement les commandes visibles
- `sth stockholm` existe toujours comme easter egg, mais n'apparaît pas dans l'aide standard
- les ressources Markdown générées reçoivent un en-tête STH et ne doivent pas être modifiées manuellement

## Structure du dépôt

```text
cmd/                Routage CLI et orchestration des commandes
internal/fs/        Profil projet, état local, écriture des ressources, self-update local
internal/http/      Accès provider-aware, lecture de SKILLS.md, téléchargement, releases GitHub
specs/              Spécifications actives des fonctionnalités en cours ou de référence
```

