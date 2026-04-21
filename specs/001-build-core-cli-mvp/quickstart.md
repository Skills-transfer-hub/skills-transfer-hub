# Quickstart: Core CLI et MVP

## Prérequis

- Go 1.22 installé localement
- Shell capable d'exécuter `go test` et `go build`
- Aucun package externe requis

## Vérification rapide

```bash
go test ./...
CGO_ENABLED=0 go build -o loreskills .
```

Sur Windows PowerShell, utilisez les mêmes commandes depuis `C:\SOURCE\LoreSkills`.

## Flux de validation manuelle

### 1. Initialiser la configuration utilisateur

```bash
./loreskills init
```

Résultat attendu :

- création de `.loreskillsrc` dans le répertoire personnel
- présence des clés JSON `repo_raw_url` et `target_dir`
- message demandant de personnaliser `repo_raw_url`
- si le fichier existe déjà, la commande échoue sans l'écraser

### 2. Installer un skill

1. Modifier `.loreskillsrc` pour pointer vers un dépôt de test valide.
2. Depuis un projet local vide, exécuter :

```bash
./loreskills install exemple-skill
```

Résultat attendu :

- création du dossier cible dans le répertoire courant
- écriture de `<target_dir>/exemple-skill.md` avec l'avertissement LoreSkills en tête
- création ou mise à jour de `<target_dir>/lore-state.json`
- si le fichier distant n'existe pas, la commande échoue explicitement sans créer de fichier Markdown vide

### 3. Mettre à jour les skills installés

```bash
./loreskills update
```

Résultat attendu :

- si des skills sont enregistrés, chacun est retéléchargé et réécrit
- sinon, le message `Aucun skill à mettre à jour.` est affiché
- en cas d'échec sur un skill, les autres mises à jour continuent et l'erreur est signalée clairement

## Cibles de tests unitaires

- `cmd/`: aide CLI, gestion des arguments, orchestration de `init`, `install`, `update`
- `internal/http/`: téléchargement réussi, erreur réseau, `404`, statut inattendu
- `internal/fs/`: création de config, préservation d'une config existante, écriture des skills, lecture/écriture de l'état
