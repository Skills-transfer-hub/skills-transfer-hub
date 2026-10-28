<p align="center">
  <img src="assets/brand/icon-256.png" alt="STH" width="112" height="112">
</p>

# STH — Skills Transfer Hub

STH est un moteur et CLI Go sous [licence MIT](LICENSE), distribué en binaire unique, pour connecter un projet à un dépôt Git de ressources IA, lire un catalogue `SKILLS.md`, matérialiser des bundles multi-fichiers pour plusieurs agents, puis les maintenir à jour.

Ce dépôt contient le CLI. STH Code, le dashboard et le backend STH Cloud sont des projets séparés ; leur code n'est pas couvert par la licence de ce dépôt. Voir [CONTRIBUTING.md](CONTRIBUTING.md) pour contribuer et [SECURITY.md](SECURITY.md) pour signaler une vulnérabilité.

Le flux actuel est centré sur le projet courant, pas sur une configuration partagée entre tous les dépôts. STH écrit donc un profil local dans `.sth/project.json` et ne stocke jamais de secret privé dans ce fichier.

## Commandes visibles

- `sth` ouvre le menu guidé en terminal interactif, y affiche l'état du projet et l'offre de mise à jour stable en cache, puis affiche la prochaine étape utile en contexte non interactif
- `sth init` configure de manière interactive un dépôt hébergé supporté pour le projet courant
- `sth login [<api-key>]` authentifie le CLI auprès de **STH Cloud** (provider SaaS) — clé stockée dans le keychain OS
- `sth logout` supprime la clé STH Cloud du keychain
- `sth list` agrège le catalogue de **tous** les providers configurés (`SKILLS.md` ou bundles détectés) et préfixe chaque ressource de sa source quand il y en a plusieurs ; une source injoignable est signalée sur `stderr` sans faire échouer la commande ; avec `--json`, retourne l'état des ressources installées (sémantique différente du mode texte)
- `sth install <resource-ref> [--provider <id>]` installe une ressource du catalogue distant ; avec plusieurs sources configurées, la référence peut être qualifiée (`internal:skills/analyse-risque`) et STH demande la source en terminal interactif quand un nom existe dans plusieurs catalogues
- `sth update [--fail-on-changes]` met à jour les ressources suivies dans le projet courant ; affiche les entrées de `CHANGELOG.md` ajoutées depuis la version installée quand un bundle mis à jour en contient un ; `--fail-on-changes` exit 1 (sans rien modifier) si une ressource gérée a été éditée localement, utile comme gate CI
- `sth new <category>/<name>` scaffolde un nouveau skill conforme (`SKILL.md` pré-rempli) — pour les mainteneurs de dépôts de skills
- `sth catalog init [--force]` génère `SKILLS.md` à partir de la structure `category/skill/SKILL.md` du dépôt courant ; le catalogue produit est directement consommable par `sth list`
- `sth pack init` initialise gratuitement et hors ligne un dépôt de packs Git
- `sth pack promote <library-id> [--output skills/<slug>] [--dry-run] [--yes]` transforme volontairement un élément de `.sth/local/code/library.json` en bundle Git multi-fichiers, sans commit ni push
- `sth mcp` expose `library_search`, `library_get`, `library_save`, `asset_search` et `asset_get`. Sans compte, les données restent sous `.sth/local/code/`; `STH_MCP_LOCAL_ONLY=1` force ce mode
- `sth bridge --stdio` expose à STH Code le protocole JSON-RPC versionné (`status`, catalogue, preview/install/update/remove et `promote.preview`/`promote`). Toute mutation exige une confirmation explicite
- `sth update self` vérifie et applique la dernière release GitHub stable compatible avec la plateforme courante
- `sth status [--json]` montre l'état (à jour, obsolète, modifié-localement) de chaque ressource installée, avec sa source quand plusieurs providers sont configurés
- `sth provider <list|add|remove>` inspecte et gère les sources de catalogue du projet ; `list` affiche la santé de chaque source et le nombre de ressources qu'elle possède, `add` valide l'accès avant d'écrire, `remove` refuse de laisser des ressources orphelines sans `--force`
- `sth doctor` diagnostique la configuration STH du projet courant (project.json, sth-state, credentials, accès provider) avec une remédiation par check
- `sth completion <bash|zsh|powershell>` génère un script de complétion shell ; à `source` dans votre shell
- `sth buddy` affiche l'état du projet courant sous forme de compagnon visuel
- `sth help [commande]` affiche l'aide générale ou ciblée
- `sth version [--json]` affiche la signature minimale (ou `{"version", "commit"}` en JSON)

### Flags globaux

- `--verbose` / `--debug` (alias) — émet sur `stderr` une trace `[verbose]` pour chaque requête HTTP (URL → status) et chaque chemin résolu (`project.json`, `sth-state-v2.json`). `stdout` reste propre.
- `--json` — supporté par `list`, `status` et `version`. En cas d'erreur, le payload stdout devient `{"error": "<message>", "code": <int>}` et le process exit non-zero (FR-022).

## Plusieurs sources dans un même projet

`.sth/project.json` accepte plusieurs entrées dans `providers` (par exemple un dépôt public et un dépôt interne, ou un dépôt Git et STH Cloud). Chaque source est traitée indépendamment :

- `sth list` interroge toutes les sources et étiquette chaque ressource ; une source injoignable est signalée sans interrompre les autres
- `sth install` résout la ressource sur l'ensemble des catalogues. Si un nom existe dans plusieurs sources, STH demande laquelle utiliser en terminal interactif, et sinon exige `--provider <id>` ou une référence qualifiée `<provider>:<folder>/<name>`
- l'état d'installation mémorise la source d'origine (`provider_id`) ; `sth update` regroupe les ressources par source et rafraîchit chacune depuis son dépôt d'origine
- la résolution de version (dernier commit du chemin) fonctionne pour GitHub, GitLab, Azure DevOps et Bitbucket, avec authentification et cache par `(source, chemin)`

```bash
sth provider add internal --type gitlab --repo group/skills
sth list
sth install internal:agents/reviewer
sth provider list
```

## Provider STH Cloud (SaaS)

En plus des providers Git classiques (GitHub / GitLab / Azure DevOps / Bitbucket), STH peut
maintenant pointer sur le SaaS **STH Cloud** : pas de PAT Git à gérer, l'auth passe par
une API key hashée côté serveur.

### Frontière freemium

- L'accès direct à un dépôt Git public ou privé avec les credentials du développeur reste local, illimité et gratuit.
- Les packs locaux, les cibles multiples, la bibliothèque locale et le MCP local fonctionnent sans compte ni réseau.
- STH Cloud monétise le service : catalogues privés hébergés, synchronisation entre personnes, mémoire d'organisation, MCP distant et gouvernance.
- Les anciennes clés `/v1` restent acceptées pendant la migration. STH Code utilise le flux device-code `/v2`, avec access token court et refresh token dans le coffre système.

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
4. STH écrit automatiquement un provider `sth-cloud` dans `.sth/project.json`.
5. Toutes les commandes suivantes (`sth list`, `sth install`, …) tapent l'API STH Cloud.
6. Après chaque commande réussie, un record d'usage est envoyé à `/v1/usage`
   (best-effort, ne bloque jamais le CLI).

### Variables d'environnement

| Variable | Rôle |
|---|---|
| `STH_CLOUD_API_KEY` | Fallback de la clé si le keychain n'est pas dispo (CI headless) |
| `STH_CLOUD_BASE_URL` | Override l'URL de base (default : prod). Ex. dev local : `http://127.0.0.1:54321/functions/v1/cli` |

### Quotas cloud

Starter Cloud comprend une organisation personnelle, trois packages hébergés et 25 éléments de bibliothèque. Pro et Team lèvent ces plafonds de contenu selon le contrat canonique de `sth-back`. Ces quotas ne limitent jamais un dépôt Git lu directement ni une ressource locale ; les mutations cloud renvoient une décision structurée avant toute écriture.

## Installation

### Depuis les sources

Prérequis : Git et Go 1.25.0 ou plus récent (version déclarée dans `go.mod`). Aucun compte STH ni secret de publication n'est requis pour compiler et tester ; le premier téléchargement des modules Go nécessite un accès réseau.

```bash
git clone https://github.com/Skills-transfer-hub/skills-transfer-hub.git
cd skills-transfer-hub
go build -o sth .
./sth version
go test ./...
go vet ./...
```

Sous Windows, utilisez `go build -o sth.exe .` puis `.\sth.exe version`. Lancez ensuite le binaire depuis votre projet ; les packs et le MCP local fonctionnent sans compte. Les binaires distribués et leurs mises à jour restent disponibles sur [sth-releases](https://github.com/Skills-transfer-hub/sth-releases/releases).

### Via un gestionnaire de paquets

Sous macOS ou Linux (Homebrew tap personnel) :

```bash
brew install skills-transfer-hub/sth/sth
```

Sous Windows (Scoop) :

```powershell
scoop bucket add sth https://github.com/Skills-transfer-hub/scoop-sth
scoop install sth
```

Sous Windows (winget) :

```powershell
winget install STH.STH
```

Sous Windows (Chocolatey) :

```powershell
choco install sth
```

> Si vous installez STH via un gestionnaire de paquets, utilisez ce gestionnaire pour les mises à jour (`brew upgrade sth`, `scoop update sth`, `winget upgrade STH.STH`, `choco upgrade sth`) plutôt que `sth update self`. La commande `sth update self` reste fonctionnelle mais sera écrasée à la prochaine mise à jour du paquet.

### Depuis GitHub Releases

STH est publié sur GitHub Releases avec les archives suivantes :

- `sth-windows-amd64.zip`
- `sth-linux-amd64.tar.gz`
- `sth-darwin-amd64.tar.gz`
- `sth-darwin-arm64.tar.gz`
- `sth-engine-{linux-amd64,darwin-amd64,darwin-arm64,windows-amd64.exe}`
- `sth-code-engine.json`
- `LICENSE`
- `THIRD_PARTY_NOTICES.md`
- `SHA256SUMS`
- `SHA256SUMS.sig`

Chaque archive inclut la licence MIT et les notices des dépendances. Les mêmes fichiers sont aussi distribués séparément pour les moteurs bruts ; ils sont couverts par `SHA256SUMS` et sa signature. Chaque build stable embarque aussi les métadonnées de release nécessaires à `sth update self`.

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

### Skills globaux pour les agents

Sans `sth init`, depuis n’importe quel dossier :

```sh
sth install https://github.com/DietrichGebert/ponytail --skill ponytail --global --all-agents
sth list --global
sth status --global --json
sth update ponytail --global
sth remove ponytail --global
```

La portée utilisateur copie les skills dans les emplacements documentés, avec suivi, sauvegarde et restauration. Elle n’installe ni hooks ni plugins et ne certifie pas leur chargement par les agents. Voir [le guide des skills globaux](docs/global-skills.md) et [les preuves de validation](docs/architecture/global-validation.md).

### Installation globale du binaire

Sous macOS ou Linux :

```bash
tar -xzf sth-linux-amd64.tar.gz
sudo install sth /usr/local/bin/sth
sth version
```

Sous Windows PowerShell :

```powershell
Expand-Archive .\sth-windows-amd64.zip -DestinationPath . -Force
New-Item -ItemType Directory -Path "$env:USERPROFILE\bin" -Force | Out-Null
Copy-Item .\sth.exe "$env:USERPROFILE\bin\sth.exe" -Force
sth.exe version
```

## Pipeline de release

Le packaging est orchestré par GoReleaser + un workflow Chocolatey dédié. Après un push sur `main` dont le CI réussit et dont le SHA est toujours la tête de `main` :

1. `auto-release.yml`, déclenché par la fin du workflow `CI`, vérifie le `head_sha` validé, calcule le prochain tag stable (`vX.Y.Z`) ou pré-release (`vX.Y.Z-rc.N` si la PR a le label `prerelease`) et crée la GitHub Release. Un résultat CI devenu obsolète est ignoré. La release est authentifiée via `RELEASE_PAT` (pas le `GITHUB_TOKEN` par défaut, sinon l'événement `release.published` ne déclencherait pas le workflow en aval).
2. `release.yml` (Linux) lance `goreleaser release --clean` qui :
   - construit les binaires (`linux-amd64`, `darwin-amd64`, `darwin-arm64`, `windows-amd64`)
   - exécute le contrat consommateur `sth-code-bridge-v1`, chargé depuis la révision immuable `1435656322b212376e70e105873de364f905c9a4` de `sth-code-private-archive`, sur le binaire candidat
   - attache les archives CLI, les binaires moteur bruts et `sth-code-engine.json` à la release publique sur `Skills-transfer-hub/sth-releases`
   - inclut le manifeste moteur et tous les binaires dans `SHA256SUMS`, puis signe ce manifeste en Ed25519
   - pousse `Formula/sth.rb` sur `Skills-transfer-hub/homebrew-sth`
   - pousse `bucket/sth.json` sur `Skills-transfer-hub/scoop-sth`
   - ouvre une PR sur `microsoft/winget-pkgs` depuis `Skills-transfer-hub/winget-pkgs`
3. Après le succès complet de `release.yml` et seulement pour un tag stable, le workflow réutilisable `publish-chocolatey.yml` (Windows) vérifie le manifeste signé, rend le nuspec depuis `packaging/chocolatey/`, puis exécute `choco pack` et `choco push` vers `chocolatey.org`. Son lancement manuel exige un tag stable exact et checkout ce tag.

Les relances manuelles de `release.yml` exigent elles aussi un tag stable ou RC signé exact ; le checkout est fait depuis le commit immuable de ce tag, jamais depuis la tête courante de `main`. Les tags antérieurs à l'introduction du manifeste moteur signé ne sont pas rebuildables par ce chemin. La publication est sérialisée, mais la file `concurrency` GitHub n'est pas FIFO : si un run en attente est remplacé, relancer explicitement `release.yml` avec son tag.

### Secrets requis

Ces secrets sont réservés aux mainteneurs du dépôt canonique. Ils vivent dans les secrets Actions du dépôt ; les appels vers les workflows réutilisables transmettent uniquement les secrets dont ils ont besoin. Les PR et les forks n'en ont pas besoin. `STH_CODE_READ_TOKEN`, en lecture seule, sert au contrôle consommateur privé sur les pushes mainteneurs et les releases, jamais dans les jobs de pull request.

| Secret | Usage | Type / Permissions |
|---|---|---|
| `RELEASE_PAT` | `auto-release.yml` crée la release sur `skills-transfer-hub` ET GoReleaser la publie cross-repo sur `sth-releases` | Fine-grained PAT `Contents: R/W` sur `skills-transfer-hub` + `sth-releases` |
| `RELEASE_SSH_SIGNING_KEY` | Création du tag Git signé dans `auto-release.yml` | Clé privée SSH enregistrée comme signing key sur GitHub |
| `STH_CODE_READ_TOKEN` | Le CI et la release chargent le contrat consommateur officiel depuis la révision épinglée de `sth-code-private-archive` | Fine-grained PAT `Contents: R` limité à `sth-code-private-archive` |
| `HOMEBREW_TAP_TOKEN` | Push Formula sur `homebrew-sth` | Fine-grained PAT `Contents: R/W` sur `homebrew-sth` |
| `SCOOP_BUCKET_TOKEN` | Push manifest sur `scoop-sth` | Fine-grained PAT `Contents: R/W` sur `scoop-sth` |
| `WINGET_TOKEN` | Push branche `sth-X.Y.Z` sur le fork + créer PR cross-fork vers `microsoft/winget-pkgs` | **PAT classique** avec scope `public_repo` |
| `CHOCOLATEY_API_KEY` | `choco push` vers `push.chocolatey.org` | API key Chocolatey |
| `STH_RELEASE_SIGNING_KEY` | Signature Ed25519 détachée de `SHA256SUMS` | Clé privée Ed25519 encodée en base64 |

Conserver des tokens distincts et limités à chaque dépôt réduit l'exposition : `RELEASE_PAT` n'a pas besoin d'accès en écriture aux dépôts Homebrew ou Scoop, qui utilisent leurs secrets dédiés.

Le fingerprint SSH de la clé de tag release est épinglé dans les workflows (`SHA256:Y/USCQxn6KoP3GBU6Kr/3/POw4pqWgi/kukMAq1vwI0`). Une rotation de `RELEASE_SSH_SIGNING_KEY` doit mettre à jour ce fingerprint dans les trois workflows dans la même PR.

⚠️ **`WINGET_TOKEN` doit être un PAT classique** (pas fine-grained). Les fine-grained PATs avec "Only select repositories" ne peuvent pas créer de PR cross-fork vers `microsoft/winget-pkgs` (qui n'est pas dans la liste, et ne peut pas y être ajouté puisque vous ne possédez pas ce repo). Le PAT classique avec scope `public_repo` permet à GitHub de créer une PR depuis votre fork vers n'importe quel repo public upstream. Alternative : un fine-grained PAT en mode "All public repositories" avec `Pull requests: Write` + `Contents: Write` (le `Contents: Write` est requis pour pousser la branche `sth-X.Y.Z` sur le fork avant l'ouverture de PR).

### Smoke test local avant tag

```bash
# Pré-requis : GoReleaser v2, jq, zip et un shell POSIX
key_output="$(go run ./tools/release-sign -genkey)"
export STH_RELEASE_SIGNING_KEY="$(printf '%s\n' "$key_output" | sed -n 's/^STH_RELEASE_SIGNING_KEY=//p')"
export STH_RELEASE_SIGNING_PUBKEY="$(printf '%s\n' "$key_output" | sed -n 's/^releaseSigningPublicKeyBase64=//p')"
goreleaser release --snapshot --clean
snapshot_version="$(jq -r '.engine_version' .release/sth-code-engine.json)"
sh scripts/verify-release-assets.sh dist "$snapshot_version"
```

### Première soumission Winget (manuelle, une fois)

Le `PackageIdentifier` `STH.STH` doit être validé manuellement par Microsoft pour la première version. Après publication d'un tag stable :

```powershell
winget install Microsoft.WingetCreate
wingetcreate new --urls https://github.com/Skills-transfer-hub/sth-releases/releases/download/<tag>/sth-windows-amd64.zip
```

Microsoft review la PR (~3–7 jours). Les versions suivantes sont ouvertes automatiquement par GoReleaser via `WINGET_TOKEN`.

## Démarrage rapide

### 0. Utiliser l'entrée guidée

Dans un terminal interactif, lancez simplement :

```bash
sth
```

STH détecte l'état du projet courant et propose l'action prioritaire suivante. Dans un contexte non interactif, la même commande affiche uniquement un résumé de l'état projet et la prochaine action recommandée sans ouvrir de prompt.

Si les métadonnées de release sont embarquées dans le binaire, le démarrage interactif vérifie aussi la dernière version stable disponible, puis met ce résultat en cache pendant 72 heures dans un cache machine-local. Une erreur réseau est retentée après une heure. Quand une stable est disponible, le menu ajoute l'action `update-self`.

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

### 2. Authentification privée

STH ne persiste pas les secrets. Il s'appuie sur une authentification externe déjà disponible et rappelle dans `sth init` les URLs officielles pour créer le token/API token :

- GitHub : PAT via `https://github.com/settings/personal-access-tokens/new`, permission minimale `Contents: Read`, variables `GITHUB_TOKEN` ou `STH_TOKEN`
- GitLab : PAT via `https://gitlab.com/-/user_settings/personal_access_tokens`, permission minimale `read_repository`, variables `GITLAB_TOKEN` ou `STH_TOKEN`
- Azure DevOps : PAT via `https://dev.azure.com/<organisation>/_usersSettings/tokens`, permission minimale `Code (Read)`, variables `AZURE_DEVOPS_EXT_PAT`, `AZURE_DEVOPS_TOKEN`, ou `STH_TOKEN`
- Bitbucket Cloud : API token via `https://id.atlassian.com/manage-profile/security/api-tokens`, permission minimale `Repositories: Read`, variable `BITBUCKET_TOKEN` ou `STH_TOKEN`

#### Modèle de credential et auth recovery (spec 007)

STH résout les credentials dans cet ordre, par requête réseau :

1. **`profile.APIToken`** — token saisi à `sth init` (jamais persisté disque, session courante uniquement)
2. **Keychain `sth-cloud`** — pour le provider SaaS uniquement, via `sth login`
3. **`STH_TOKEN`** — variable d'environnement universelle (tous providers Git)
4. **GitHub Actions OIDC** — quand `ACTIONS_ID_TOKEN_REQUEST_URL` et `ACTIONS_ID_TOKEN_REQUEST_TOKEN` sont injectés par l'action runner. Le JWT est demandé avec l'audience `sth-cli`. **Précondition** : le service cible doit valider cette claim — utile pour STH Cloud, mais `api.github.com` ne l'accepte pas. En CI sur un repo Git privé, préférez `STH_TOKEN`.
5. **Variables d'env spécifiques au provider** : `GITHUB_TOKEN`, `GITLAB_TOKEN`, `AZURE_DEVOPS_EXT_PAT`, `AZURE_DEVOPS_TOKEN`, `BITBUCKET_TOKEN`, `BITBUCKET_USERNAME`+`BITBUCKET_APP_PASSWORD`
6. **`git credential fill`** — réutilise les identifiants déjà gérés par votre git CLI
7. **Prompt TTY** — uniquement dans une session interactive ; le token est exporté en `STH_TOKEN` pour le retry et n'est pas persisté

Sur un `401`/`403` en TTY, STH affiche `Authentication failed (HTTP 4xx) — your token may have expired.` puis demande un token à coller. Une seule retry par invocation (FR-006). En CI, le `401` propage immédiatement avec un exit code non-zero.

Aucun secret n'est jamais écrit sur disque (FR-002).

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
- sa durée de vie est de 72 heures pour un résultat valide et d'une heure pour une erreur
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

Compilation, tests et lint Go, sans compte ni accès au dépôt privé STH Code :

```bash
go build ./...
go test ./...
go vet ./...
```

La CI des PR exécute les tests bridge locaux. Les pushes mainteneurs et les releases ajoutent un contrat consommateur épinglé provenant de `sth-code-private-archive` ; ce contrôle utilise Node.js et un token de lecture réservé aux mainteneurs. Il n'est pas requis pour contribuer.

Les contrôles de compatibilité ci-dessous sont complémentaires. Les tests hermétiques utilisent des fixtures locales ; la certification réelle nécessite les CLI, IDE et accès fournisseurs indiqués, et ne fait pas partie du démarrage contributeur.

Certification de compatibilité Claude, Codex, Copilot, Antigravity et Gemini :

```bash
for script in scripts/compat-*.sh testdata/runtime/fake-cli/*.sh; do
  sh -n "$script"
done
sh scripts/compat-runtime-test.sh
sh scripts/compat-sth-lifecycle-test.sh
RUN_MODEL_SMOKES=0 sh scripts/compat-runtime.sh
sh scripts/compat-runtime.sh

go build -o /abs/cert/sth .
STH_BIN=/abs/cert/sth \
COMPAT_FIXTURE_EXPORT_ROOT=/abs/cert/fixtures \
sh scripts/compat-sth-lifecycle-test.sh

CLAUDE_BIN=/abs/cli/claude \
CLAUDE_COMPAT_EXPECTED_VERSION=2.1.201 \
CLAUDE_COMPAT_EXPECTED_SHA256='<sha256-claude-resolu>' \
CODEX_BIN=/abs/cli/codex \
CODEX_COMPAT_EXPECTED_VERSION=0.147.0-alpha.1.2 \
CODEX_COMPAT_EXPECTED_SHA256='<sha256-codex-resolu>' \
COPILOT_BIN=/abs/cli/copilot \
COPILOT_COMPAT_EXPECTED_VERSION=1.0.78 \
COPILOT_COMPAT_EXPECTED_SHA256='<sha256-copilot-resolu>' \
GEMINI_BIN=/abs/cli/gemini \
GEMINI_COMPAT_EXPECTED_VERSION=0.53.1 \
GEMINI_COMPAT_EXPECTED_SHA256='<sha256-gemini-resolu>' \
ANTIGRAVITY_BIN=/abs/cli/agy \
ANTIGRAVITY_COMPAT_EXPECTED_VERSION=1.1.10 \
ANTIGRAVITY_COMPAT_EXPECTED_SHA256='<sha256-antigravity-resolu>' \
COMPAT_STH_BIN=/abs/cert/sth \
COMPAT_FIXTURE_ROOT=/abs/cert/fixtures \
STH_RUNTIME_SENTINEL_SUFFIX=UPDATED \
COMPAT_HERMETIC=0 \
COPILOT_IDE_TRANSCRIPT=/abs/copilot-transcript.jsonl \
COPILOT_IDE_EXTENSION_VERSION=0.59.0 \
ANTIGRAVITY_IDE_TRANSCRIPT=/abs/transcript_full.jsonl \
ANTIGRAVITY_IDE_VERSION=2.1.1 \
ANTIGRAVITY_IDE_APP='/Applications/Antigravity IDE.app' \
STRICT_RUNTIME_CERTIFICATION=1 \
sh scripts/compat-runtime.sh \
  >/abs/cert/runtime-results.tsv \
  2>/abs/cert/runtime-identities.log
```

Le test hermétique construit un vrai binaire `sth`, exporte ses fixtures `UPDATED`, vérifie leur provenance puis exige une matrice stricte de 38 preuves avec les cinq faux CLI et les deux transcripts IDE. Il valide le runner, pas les services externes. Chaque ligne TSV porte un champ `mode` égal à `hermetic` ou `real`; la recette réelle force `COMPAT_HERMETIC=0`. Le test lifecycle génère les vingt contrats via `install`, puis valide `update --dry-run`, `update`, `status` et `remove` contre un fournisseur local isolé.

Le mode normal signale les surfaces indisponibles par `NON_EXECUTE` sans faire échouer l'audit. Le mode strict réel exige en plus une version et un SHA-256 préalablement enregistrés pour chacun des cinq CLI. Il résout chaque chemin canonique, vérifie la sortie `--version` et le contenu exact du fichier avant le premier appel runtime, puis écrit cinq lignes `Identité runtime vérifiée: ...` sur stderr; conserver ce log avec le TSV. Les valeurs `<sha256-...>` de l'exemple sont volontairement invalides jusqu'à leur remplacement par 64 chiffres hexadécimaux issus d'un relevé indépendant. Cette empreinte atteste le fichier local résolu au moment du contrôle, pas sa provenance éditeur, sa signature ni les dépendances transitives d'un lanceur script.

Le mode strict exige aussi les fixtures générées par le même `COMPAT_STH_BIN` depuis moins de six heures, les deux transcripts IDE, les versions IDE, la co-présence d'une app Antigravity signée par l'équipe Google attendue et acceptée par Gatekeeper, exactement 38 contrats et aucun statut non certifiant. L'export affiche un challenge aléatoire : il doit apparaître dans la dernière invocation IDE de chaque preuve, ce qui interdit de réutiliser un transcript provenant d'un export antérieur. Le transcript natif Antigravity n'exposant pas l'identifiant de build de l'app, cette preuve atteste une co-présence de la version annoncée et non un lien cryptographique entre ce processus et le transcript. Une préparation invalide sort `2`; une matrice exécutée mais non certifiante sort `1`; une certification complète sort `0`. Seul `copilot-cli/instruction-discovery` accepte `INFO`, car Copilot CLI 1.0.78 n'expose pas cette inspection. Le rapport détaillé et daté se trouve dans [`docs/compatibility-report-2026-08-05.md`](docs/compatibility-report-2026-08-05.md).

Les captures IDE fraîches peuvent être préparées avec `scripts/compat-copilot-ide-launch.sh` et `scripts/compat-antigravity-ide-launch.sh`. Leurs frontières d'isolation, les requêtes exactes et la collecte des transcripts sont détaillées dans [`docs/copilot-ide-runtime.md`](docs/copilot-ide-runtime.md) et [`docs/antigravity-ide-runtime.md`](docs/antigravity-ide-runtime.md). Ces helpers ne terminent aucun OAuth automatiquement.

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


### First skill from the menu

Run `sth` in your project, choose **Installer un skill**, select a skill and its assistant, inspect the file preview, then confirm. In an empty folder, the short setup asks for a public GitHub repository (default: `anthropics/skills`). Other repositories, credentials and configuration remain under **Réglages avancés**. After installation, the menu explains how to open a fresh assistant conversation and invoke the skill.

**Mes skills** lists installed resources without network access. **Supprimer** previews the managed files before confirmation; personal modifications must be copied aside and the original restored before removal. **Diagnostic** checks configuration and access. A failed check leaves **connexion non vérifiée** visible. GitHub quota exhaustion is reported separately from authentication failure, without demanding a token for public access.

`sth list` and `sth list --json` now return the same remote catalog. Scripts that previously used JSON to inspect installed resources must use `sth list --installed --json`; the text equivalent is `sth list --installed`. `sth status --local` inspects local state without fetching remote versions. A no-change skill install or update returns `unchanged` in JSON and **Déjà à jour** in text. Updating retains the assistant selection recorded at installation.

Removal accepts `name`, `skills/name`, canonical IDs and source-qualified references; use `--provider <id>` when the name exists in more than one source. Both `sth remove` and `sth doctor` accept `--json`.

### Contrôler les mises à jour des dépôts

Le menu vérifie automatiquement les versions des skills installés, sans modifier leurs fichiers. Le résultat est conservé cinq minutes pendant la session ; « Vérifier les mises à jour » force un nouveau contrôle. « Mettre à jour » applique les changements à votre demande, en protégeant les éditions personnelles et les versions épinglées.

`sth check-updates` donne le résultat par source ; `sth check-updates --json` fournit le même détail aux scripts. Une source inaccessible reste « non vérifiée » et ne masque pas les autres résultats.

« Mes dépôts » permet d’ajouter une source publique ou privée et de retrouver le guide de création de clé. Pour plusieurs providers, configurez leurs variables dédiées (`GITHUB_TOKEN`, `GITLAB_TOKEN`, etc.) plutôt qu’une clé globale `STH_TOKEN`. Pour un dépôt GitLab privé, `read_api` permet la lecture du catalogue, de l’arborescence et des versions.
