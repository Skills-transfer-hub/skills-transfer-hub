# LoreSkills

LoreSkills est un CLI Go pour installer et maintenir des skills Markdown dans un projet local.

Le MVP couvre trois commandes :

- `loreskills init` pour créer la configuration utilisateur
- `loreskills install <skill>` pour télécharger un skill distant dans le projet courant
- `loreskills update` pour mettre à jour tous les skills déjà suivis

## Présentation du projet

LoreSkills vise un flux simple et portable :

- binaire unique en Go
- dépendances limitées à la bibliothèque standard
- fonctionnement sur Windows, macOS et Linux
- configuration utilisateur locale via `~/.loreskillsrc`
- état du projet courant via `lore-state.json`

Lorsqu'un skill est installé, LoreSkills :

1. lit la configuration utilisateur
2. télécharge `<repo_raw_url>/<nom-du-skill>.md`
3. écrit le fichier dans le dossier cible du projet
4. ajoute un en-tête signalant que le skill est géré par LoreSkills
5. enregistre le skill dans l'état local pour les futures mises à jour

## Installation

### Prérequis

- Go 1.22 ou plus récent
- accès au dépôt Git

### Depuis les sources

Depuis la racine du dépôt :

```bash
go build -o loreskills .
```

Pour une vérification proche du build CI/documenté :

```bash
CGO_ENABLED=0 go build -o loreskills .
```

Sous Windows, exécutez ensuite le binaire avec :

```powershell
.\loreskills.exe
```

Sous macOS ou Linux :

```bash
./loreskills
```

## Comment tester

### Tests automatisés

```bash
go test ./...
```

### Validation build

```bash
CGO_ENABLED=0 go build -o loreskills .
```

### Vérification manuelle rapide

1. Lancez `loreskills init`
2. Renseignez `repo_raw_url` dans `~/.loreskillsrc`
3. Exécutez `loreskills install exemple-skill` depuis un projet de test
4. Vérifiez la création du dossier cible, du fichier Markdown et de `lore-state.json`
5. Exécutez `loreskills update`

## Comment configurer

La commande suivante crée le fichier de configuration utilisateur :

```bash
loreskills init
```

Le fichier créé est `~/.loreskillsrc` avec cette structure :

```json
{
  "repo_raw_url": "https://raw.githubusercontent.com/votre-organisation/votre-repo/main",
  "target_dir": ".claude/skills"
}
```

### Champs de configuration

- `repo_raw_url` : URL de base brute de votre dépôt de skills
- `target_dir` : chemin relatif au projet courant où écrire les skills

### Exemple de configuration

Si vos fichiers de skills sont stockés dans `skills/` sur la branche `main` :

```json
{
  "repo_raw_url": "https://raw.githubusercontent.com/mon-organisation/mon-repo/main/skills",
  "target_dir": ".claude/skills"
}
```

Avec cette configuration :

- `loreskills install analyse-risque` télécharge `https://raw.githubusercontent.com/mon-organisation/mon-repo/main/skills/analyse-risque.md`
- le fichier est écrit dans `.claude/skills/analyse-risque.md`
- l'état est enregistré dans `.claude/skills/lore-state.json`

## Utilisation

### Aide

```bash
loreskills
```

### Initialiser la configuration

```bash
loreskills init
```

### Installer un skill

```bash
loreskills install mon-skill
```

### Mettre à jour les skills installés

```bash
loreskills update
```

## Fichiers générés

### Configuration utilisateur

- emplacement : `~/.loreskillsrc`
- rôle : stocke l'URL distante et le dossier cible

### Fichiers du projet

- `<target_dir>/<skill>.md` : contenu du skill avec en-tête LoreSkills
- `<target_dir>/lore-state.json` : état des skills installés

Exemple d'état local :

```json
{
  "analyse-risque": "installed",
  "memo-compliance": "installed"
}
```

## Comportements utiles à connaître

- `target_dir` doit rester un chemin relatif au projet courant
- un nom de skill ne peut pas contenir de séparateurs de chemin
- `init` refuse d'écraser une configuration existante
- `install` échoue explicitement sur une configuration invalide ou un `404`
- `update` continue les autres mises à jour même si un skill échoue, puis retourne une erreur globale si au moins un refresh a échoué
- les fichiers de skills générés ne doivent pas être modifiés manuellement

## Structure du dépôt

```text
cmd/                Routage CLI et orchestration des commandes
internal/fs/        Configuration, écriture des skills et état local
internal/http/      Téléchargement HTTP des skills
specs/              Artefacts Speckit de la feature
main.go             Point d'entrée du binaire
```

## Limites actuelles du MVP

- pas de gestion de versions de skills
- pas de registre central ou d'index de skills
- pas de fusion intelligente si un fichier géré a été modifié à la main
- pas d'automatisation native du cycle commit/push/PR dans le binaire CLI
