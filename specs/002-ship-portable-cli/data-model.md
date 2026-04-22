# Data Model: Ship Portable CLI

## ReleasePackage

- **Purpose**: Représente un livrable LoreSkills publié pour une plateforme cible précise.
- **Fields**:
  - `os` (`string`, required): système cible (`windows`, `linux`, `darwin`).
  - `arch` (`string`, required): architecture cible (`amd64`, `arm64`).
  - `archive_name` (`string`, required): nom du fichier publié dans la release.
  - `archive_format` (`string`, required): format d'archive (`zip` ou `tar.gz`).
  - `binary_name` (`string`, required): nom du binaire inclus (`loreskills` ou `loreskills.exe`).
  - `checksum` (`string`, derived): somme SHA256 du fichier publié.
  - `install_notes_ref` (`string`, required): référence à la guidance d'installation correspondante.
- **Validation rules**:
  - Le couple `os` + `arch` doit appartenir à la matrice MVP autorisée.
  - Le format d'archive doit être cohérent avec la plateforme publiée.
  - Chaque package publié doit avoir une entrée de checksum et une guidance d'installation.
- **State transitions**:
  - `planned` → défini dans le workflow release.
  - `built` → archive produite avec `CGO_ENABLED=0`.
  - `checksummed` → checksum SHA256 calculé et ajouté au manifeste.
  - `published` → archive attachée à la GitHub release.

## ChecksumManifest

- **Purpose**: Représente le fichier SHA256 publié avec les artefacts de release.
- **Fields**:
  - `entries` (`[]ChecksumEntry`, required): liste des checksums par archive.
- **ChecksumEntry fields**:
  - `archive_name` (`string`, required): nom exact du package publié.
  - `sha256` (`string`, required): empreinte SHA256 attendue.
- **Validation rules**:
  - Chaque artefact de la matrice MVP doit avoir exactement une entrée.
  - Le manifeste doit être assez stable pour être copié-collé dans les commandes de vérification documentées.

## BuildMetadata

- **Purpose**: Représente les informations injectées au build et affichées par `loreskills version`.
- **Fields**:
  - `version` (`string`, required): version lisible de la release ou valeur de développement par défaut.
  - `commit` (`string`, optional): identifiant court du commit source.
  - `build_date` (`string`, optional): date/horodatage de build.
  - `target` (`string`, optional): cible compilée (`os/arch`).
- **Validation rules**:
  - Une build locale sans injection doit rester exécutable avec des valeurs de repli explicites.
  - Une build de release doit renseigner une version non vide exploitable par l'utilisateur.

## CommandCatalogEntry

- **Purpose**: Représente la définition canonique d'une commande visible par l'utilisateur.
- **Fields**:
  - `name` (`string`, required): nom principal de la commande.
  - `usage` (`string`, required): syntaxe courte affichée dans l'aide.
  - `summary` (`string`, required): rôle de la commande.
  - `examples` (`[]string`, required): exemples affichés à l'utilisateur.
  - `accepts_help_flag` (`bool`, required): indique si `--help` / `-h` est pris en charge.
- **Validation rules**:
  - Chaque commande primaire du MVP doit avoir une entrée unique.
  - Les sorties d'aide doivent être dérivées de ces entrées pour éviter la dérive entre code et documentation.

## AsciiGuide

- **Purpose**: Représente la persona ASCII commune affichée sur chaque sortie visible du CLI.
- **Fields**:
  - `art_lines` (`[]string`, required): lignes ASCII stables du guide.
  - `message` (`string`, required): texte fonctionnel présenté avec le guide.
  - `channel` (`string`, required): `stdout` ou `stderr`.
- **Validation rules**:
  - Le bloc doit rester lisible dans un terminal étroit.
  - Le guide ne doit pas masquer ni remplacer les informations fonctionnelles essentielles.
  - Le même art doit être réutilisé sur toutes les commandes visibles pour rester cohérent.