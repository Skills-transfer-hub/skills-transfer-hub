# LoreSkills

LoreSkills est un CLI Go, distribué en binaire unique, pour connecter un projet à un dépôt privé de ressources IA, lire un catalogue `SKILLS.md`, installer des ressources dans le projet courant, puis les maintenir à jour.

Le flux actuel est centré sur le projet courant, pas sur une configuration partagée entre tous les dépôts. LoreSkills écrit donc un profil local dans `.loreskills/project.json` et ne stocke jamais de secret privé dans ce fichier.

## Commandes visibles

- `loreskills` affiche la signature minimale du binaire courant
- `loreskills init` configure le dépôt privé du projet courant de manière interactive
- `loreskills list` lit `SKILLS.md` et liste les ressources installables
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

### 1. Configurer le projet courant

Depuis le projet où vous souhaitez installer des ressources :

```bash
loreskills init
```

Le flux interactif vous demande :

- le provider : `github`, `gitlab`, `azure-devops`, ou `bitbucket`
- le `repository_id` du dépôt privé
- la référence Git à lire (`main` par défaut)
- le chemin du catalogue (`SKILLS.md` par défaut)
- le dossier cible du projet (par défaut `.claude/skills`)

Si un ancien `~/.loreskillsrc` existe, LoreSkills propose de l'importer pour migrer vers le nouveau format projet.

### 2. Authentification privée

LoreSkills ne persiste pas les secrets. Il s'appuie sur une authentification externe déjà disponible :

- GitHub : `GITHUB_TOKEN`, `LORESKILLS_TOKEN`, ou vos identifiants Git pour `github.com`
- GitLab : `GITLAB_TOKEN`, `LORESKILLS_TOKEN`, ou vos identifiants Git pour `gitlab.com`
- Azure DevOps : `AZURE_DEVOPS_EXT_PAT`, `AZURE_DEVOPS_TOKEN`, ou vos identifiants Git pour `dev.azure.com`
- Bitbucket : `BITBUCKET_TOKEN`, `BITBUCKET_USERNAME` + `BITBUCKET_APP_PASSWORD`, ou vos identifiants Git pour `bitbucket.org`

Le flux `init` vérifie l'accès au dépôt et au catalogue avant d'enregistrer la configuration du projet.

### 3. Déclarer le catalogue `SKILLS.md`

LoreSkills lit un tableau Markdown depuis `SKILLS.md`. Exemple minimal :

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

### 4. Lister puis installer une ressource

```bash
loreskills list
loreskills install skills/analyse-risque
```

`loreskills list` affiche les ressources distantes avec leur type, leur nom, leur dossier et, si présent, leur description.

`loreskills install` :

- relit `.loreskills/project.json`
- relit `SKILLS.md`
- résout la ressource demandée
- télécharge son `source_path`
- écrit la ressource dans `<target_dir>/<folder>/...`
- met à jour `<target_dir>/lore-state.json`

### 5. Mettre à jour les ressources suivies

```bash
loreskills update
```

LoreSkills recharge chaque ressource installée à partir de son `source_path` d'origine. Si une ressource échoue, les autres continuent à être rafraîchies et la commande retourne ensuite une erreur globale.

### 6. Mettre à jour le CLI lui-même

```bash
loreskills update self
```

Le self-update :

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
      "catalog_id": "skill::skills::analyse-risque",
      "resource_type": "skill",
      "name": "analyse-risque",
      "folder_name": "skills",
      "installed_path": ".claude/skills/skills/analyse-risque.md",
      "source_path": "skills/analyse-risque.md",
      "status": "installed"
    }
  ]
}
```

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
