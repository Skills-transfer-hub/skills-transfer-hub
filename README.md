# LoreSkills

LoreSkills est un CLI Go, distribué en binaire unique, pour connecter un projet à un dépôt hébergé de ressources IA, lire un catalogue `SKILLS.md`, installer des ressources dans le projet courant, puis les maintenir à jour.

Le flux actuel est centré sur le projet courant, pas sur une configuration partagée entre tous les dépôts. LoreSkills écrit donc un profil local dans `.loreskills/project.json` et ne stocke jamais de secret privé dans ce fichier.

## Commandes visibles

- `loreskills` ouvre le menu guidé en terminal interactif, y affiche l'état du projet et l'offre de mise à jour stable en cache, puis affiche la prochaine étape utile en contexte non interactif
- `loreskills init` configure de manière interactive un dépôt hébergé supporté pour le projet courant
- `loreskills list` lit `SKILLS.md` ou détecte les bundles compatibles et liste les ressources installables
- `loreskills install <resource-ref>` installe une ressource du catalogue distant
- `loreskills update` met à jour les ressources suivies dans le projet courant
- `loreskills update self` vérifie et applique la dernière release GitHub stable compatible avec la plateforme courante
- `loreskills help [commande]` affiche l'aide générale ou ciblée
- `loreskills version` affiche la même signature minimale que `loreskills`

## Installation

LoreSkills est publié sur GitHub Releases avec les archives suivantes :

- `loreskills-windows-amd64.zip`
- `loreskills-linux-amd64.tar.gz`
- `loreskills-darwin-amd64.tar.gz`
- `loreskills-darwin-arm64.tar.gz`
- `SHA256SUMS`

Chaque build stable embarque aussi les métadonnées de release nécessaires à `loreskills update self`.

### Vérifier une archive

Sous macOS ou Linux :

```bash
sha256sum -c SHA256SUMS --ignore-missing
```

Sous Windows PowerShell :

```powershell
$expected = (Select-String -Path .\SHA256SUMS -Pattern 'loreskills-windows-amd64.zip').ToString().Split(' ')[0]
$actual = (Get-FileHash .\loreskills-windows-amd64.zip -Algorithm SHA256).Hash.ToLower()
if ($expected -ne $actual) { throw 'Checksum mismatch' }
```

### Installation globale

Sous macOS ou Linux :

```bash
tar -xzf loreskills-linux-amd64.tar.gz
sudo install loreskills-linux-amd64/loreskills /usr/local/bin/loreskills
loreskills version
```

Sous Windows PowerShell :

```powershell
Expand-Archive .\loreskills-windows-amd64.zip -DestinationPath .\loreskills-windows-amd64 -Force
New-Item -ItemType Directory -Path "$env:USERPROFILE\bin" -Force | Out-Null
Copy-Item .\loreskills-windows-amd64\loreskills.exe "$env:USERPROFILE\bin\loreskills.exe" -Force
loreskills.exe version
```

## Démarrage rapide

### 0. Utiliser l'entrée guidée

Dans un terminal interactif, lancez simplement :

```bash
loreskills
```

LoreSkills détecte l'état du projet courant et propose l'action prioritaire suivante. Dans un contexte non interactif, la même commande affiche uniquement un résumé de l'état projet et la prochaine action recommandée sans ouvrir de prompt.

Si les métadonnées de release sont embarquées dans le binaire, le démarrage interactif vérifie aussi la dernière version stable disponible, puis met ce résultat en cache pendant 24 heures dans un cache machine-local. Quand une stable est disponible, le menu ajoute l'action `update-self`.

### 1. Configurer le projet courant

Depuis le projet où vous souhaitez installer des ressources :

```bash
loreskills init
```

Le flux interactif vous demande :

- le provider : `github`, `gitlab`, `azure-devops`, ou `bitbucket`
- affiche la documentation officielle du provider, l'URL d'action pour créer le token/API token, et la permission minimale attendue
- le `repository_id` du dépôt privé
- la référence Git à lire (`main` par défaut)
- le chemin du catalogue (`SKILLS.md` par défaut)
- le dossier cible du projet (par défaut `.claude/skills`)

Le `repository_id` peut être saisi sous forme canonique (`owner/repo`) ou sous forme d'URL hébergée supportée (`https://github.com/owner/repo`). Les hôtes self-hosted sont rejetés explicitement. Avant de continuer, le wizard propose aussi de relire la guidance provider sans relancer `loreskills init`.

Si un ancien `~/.loreskillsrc` existe, LoreSkills propose de l'importer pour migrer vers le nouveau format projet.

### 2. Authentification privée

LoreSkills ne persiste pas les secrets. Il s'appuie sur une authentification externe déjà disponible et rappelle dans `loreskills init` les URLs officielles pour créer le token/API token :

- GitHub : PAT via `https://github.com/settings/personal-access-tokens/new`, permission minimale `Contents: Read`, variables `GITHUB_TOKEN` ou `LORESKILLS_TOKEN`
- GitLab : PAT via `https://gitlab.com/-/user_settings/personal_access_tokens`, permission minimale `read_repository`, variables `GITLAB_TOKEN` ou `LORESKILLS_TOKEN`
- Azure DevOps : PAT via `https://dev.azure.com/<organisation>/_usersSettings/tokens`, permission minimale `Code (Read)`, variables `AZURE_DEVOPS_EXT_PAT`, `AZURE_DEVOPS_TOKEN`, ou `LORESKILLS_TOKEN`
- Bitbucket Cloud : API token via `https://id.atlassian.com/manage-profile/security/api-tokens`, permission minimale `Repositories: Read`, variable `BITBUCKET_TOKEN` ou `LORESKILLS_TOKEN`

LoreSkills accepte encore `BITBUCKET_USERNAME` et `BITBUCKET_APP_PASSWORD` pour la compatibilité Bitbucket, mais la guidance produit recommande désormais l'API token Atlassian.

Le flux `init` vérifie l'accès au dépôt et au catalogue avant d'enregistrer la configuration du projet.

### 3. Déclarer `SKILLS.md` ou utiliser la structure compatible

LoreSkills commence toujours par lire un tableau Markdown depuis `SKILLS.md` quand il existe. Exemple minimal :

```md
| Type  | Name           | Folder | Source                   | Description          |
| ----- | -------------- | ------ | ------------------------ | -------------------- |
| skill | analyse-risque | skills | skills/analyse-risque.md | Check-list d'analyse |
| agent | reviewer       | agents | agents/reviewer.md       | Agent de revue       |
```

Règles importantes :

- `Type`, `Name`, `Folder`, et `Source` sont requis
- `Folder` est le dossier affiché par `loreskills list` et utilisé pour l'installation locale
- `Source` est le chemin distant utilisé pour `install` et `update`
- le format de référence utilisateur est soit un nom unique, soit `folder/name` en cas d'ambiguïté

Si `SKILLS.md` est absent, LoreSkills bascule automatiquement sur un scan compatible de l'arborescence distante. Toute entrée `category/folder/SKILL.md` devient alors un bundle installable avec une référence canonique `category/folder` affichée par `loreskills list`.

Exemple de structure compatible sans `SKILLS.md` :

```text
react/form-wizard/SKILL.md
commands/release-helper/SKILL.md
.net/azure-bootstrap/SKILL.md
```

### 4. Lister puis installer une ressource

```bash
loreskills list
loreskills install react/form-wizard
```

`loreskills list` affiche les ressources distantes avec leur type et, pour les bundles détectés par scan, leur référence canonique `category/folder`. Quand `SKILLS.md` existe, il reste la source unique de vérité; un `SKILLS.md` invalide provoque une erreur explicite et n'est pas fusionné avec le scan.

`loreskills install` :

- relit `.loreskills/project.json`
- relit `SKILLS.md` ou le fallback scan compatible
- résout la ressource demandée
- télécharge soit un `source_path` unique, soit tous les fichiers du bundle détecté
- écrit un bundle complet dans `<target_dir>/<category>/<folder>/...`
- met à jour `<target_dir>/lore-state.json`

### 5. Mettre à jour les ressources suivies

```bash
loreskills update
```

LoreSkills recharge chaque ressource installée à partir de son `source_path` d'origine. Pour un bundle suivi, tous les fichiers gérés sont resynchronisés, les fichiers supprimés côté dépôt sont retirés localement, et les modifications locales dans la zone gérée sont écrasées. Si une ressource échoue, les autres continuent à être rafraîchies et la commande retourne ensuite une erreur globale.

### 6. Mettre à jour le CLI lui-même

```bash
loreskills update self
```

Le self-update :

- force un check réseau stable immédiat même si le cache de démarrage est encore frais
- consulte le canal GitHub injecté au build
- ignore explicitement les pré-releases
- sélectionne l'artefact correspondant à la plateforme courante
- vérifie le checksum SHA256 avant tout remplacement local

Si aucune version stable plus récente n'existe, LoreSkills affiche `Aucune mise à jour stable disponible.`

## Fichiers générés

### Profil projet

LoreSkills crée `.loreskills/project.json` dans le projet courant :

```json
{
  "provider": "github",
  "repository_id": "mon-org/mon-repo",
  "ref": "main",
  "catalog_path": "SKILLS.md",
  "target_dir": ".claude/skills"
}
```

Aucun secret n'est stocké dans ce fichier.

### État local des ressources

LoreSkills suit les ressources installées dans `<target_dir>/lore-state.json` :

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

LoreSkills peut aussi créer un cache machine-local pour les checks automatiques de mise à jour stable dans `os.UserCacheDir()/loreskills/update-check.json`.

- le cache est relu uniquement par le démarrage interactif
- sa durée de vie est de 24 heures
- `loreskills update self` ignore cette fraîcheur et force un check stable immédiat
- ce fichier ne contient aucun secret et n'est jamais écrit dans `.loreskills/project.json`

## Validation locale

Tests et lint Go :

```bash
go test ./...
go vet ./...
```

Build local avec métadonnées de release :

```bash
go build -trimpath -ldflags "-X loreskills/cmd.version=0.0.0-dev -X loreskills/cmd.releaseAPIURL=https://api.github.com/repos/<owner>/<repo>/releases -X loreskills/cmd.releaseAssetName=loreskills-<os>-<arch>.tar.gz -X loreskills/cmd.releaseChecksumName=SHA256SUMS" -o loreskills .
```

Sous Windows, remplacez `releaseAssetName` par l'archive `.zip` attendue pour la plateforme.

## Remarques utiles

- `target_dir` doit rester relatif au projet courant
- le profil projet remplace la configuration partagée historique, mais `init` peut importer `~/.loreskillsrc`
- `loreskills help` documente uniquement les commandes visibles
- `loreskills stockholm` existe toujours comme easter egg, mais n'apparaît pas dans l'aide standard
- les ressources Markdown générées reçoivent un en-tête LoreSkills et ne doivent pas être modifiées manuellement

## Structure du dépôt

```text
cmd/                Routage CLI et orchestration des commandes
internal/fs/        Profil projet, état local, écriture des ressources, self-update local
internal/http/      Accès provider-aware, lecture de SKILLS.md, téléchargement, releases GitHub
specs/003-private-repo-skills/
```
