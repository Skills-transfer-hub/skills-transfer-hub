# Quickstart: Ship Portable CLI

## Prérequis

- Go 1.22 disponible localement
- Accès à GitHub Actions pour valider le workflow de release
- Aucun package Go externe requis

## Vérification locale rapide

Depuis la racine du dépôt :

```bash
go test ./...
go vet ./...
CGO_ENABLED=0 go build -ldflags "-X loreskills/cmd.version=dev-local" -o loreskills .
./loreskills help
./loreskills version
```

Sous Windows PowerShell, remplacez l'exécution finale par `./loreskills.exe help` et `./loreskills.exe version`.

## Flux de validation manuelle

### 1. Vérifier l'expérience CLI enrichie

Valider localement :

- `loreskills` sans argument affiche l'aide générale avec l'avatar ASCII
- `loreskills help install` affiche une aide contextuelle avec syntaxe et exemple
- `loreskills version` affiche une version exploitable
- une commande invalide affiche l'avatar, l'erreur et une action corrective

### 2. Vérifier les commandes métier existantes avec la nouvelle présentation

Utiliser un home temporaire et un projet de test pour confirmer :

- `loreskills init` crée toujours `.loreskillsrc`
- `loreskills install <skill>` conserve les comportements de téléchargement et d'état local
- `loreskills update` conserve le flux de mise à jour multiple et les erreurs partielles

### 3. Vérifier la préparation de release

Le workflow GitHub Actions doit produire :

- un package `windows/amd64`
- un package `linux/amd64`
- un package `darwin/amd64`
- un package `darwin/arm64`
- un fichier `SHA256SUMS` ou équivalent listant les checksums de ces artefacts

Validation manuelle attendue :

```bash
CGO_ENABLED=0 go build -ldflags "-X loreskills/cmd.version=dev-local -X loreskills/cmd.commit=local -X loreskills/cmd.buildDate=2026-04-22T00:00:00Z -X loreskills/cmd.target=$(go env GOOS)/$(go env GOARCH)" -o loreskills .
```

Puis verifier que le workflow `Release` publie ou prepare les fichiers suivants :

- `loreskills-windows-amd64.zip`
- `loreskills-linux-amd64.tar.gz`
- `loreskills-darwin-amd64.tar.gz`
- `loreskills-darwin-arm64.tar.gz`
- `SHA256SUMS`

### 4. Vérifier la guidance d'installation globale

Dans `README.md` et/ou les notes de release :

- chaque OS cible a une procédure d'installation globale à partir des artifacts publiés
- les commandes de vérification SHA256 sont documentées
- les étapes pour rendre `loreskills` disponible dans le `PATH` sont décrites pour Windows, macOS et Linux

Exemples de verification attendus :

```bash
sha256sum -c SHA256SUMS --ignore-missing
```

```powershell
$expected = (Select-String -Path .\SHA256SUMS -Pattern 'loreskills-windows-amd64.zip').ToString().Split(' ')[0]
$actual = (Get-FileHash .\loreskills-windows-amd64.zip -Algorithm SHA256).Hash.ToLower()
if ($expected -ne $actual) { throw 'Checksum mismatch' }
```

## Cibles de test attendues

- `cmd/`: routage CLI, aide racine, aide contextuelle, `version`, erreurs, avatar ASCII, stabilité des sorties
- `internal/fs/`: pas de régression sur config et état local
- `internal/http/`: pas de régression sur téléchargement et erreurs HTTP
- `.github/workflows/`: validation de la matrice de build/release et de la génération du manifeste SHA256