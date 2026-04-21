# Research: Core CLI et MVP

## Decision 1: Router manuellement les commandes CLI avec `os.Args`

- **Decision**: Utiliser un routage manuel des commandes dans `cmd/` plutôt qu'un framework CLI externe.
- **Rationale**: La constitution interdit les dépendances externes et le scope MVP ne requiert que trois commandes simples (`init`, `install`, `update`) avec un message d'aide minimal. Un dispatch manuel garde le binaire petit, le comportement explicite et les tests simples.
- **Alternatives considered**: `flag` pour chaque sous-commande a été jugé possible mais peu utile au MVP; Cobra/Viper sont rejetés car contraires à la constitution.

## Decision 2: Stocker configuration et état en JSON local

- **Decision**: Utiliser `.loreskillsrc` et `lore-state.json` comme fichiers JSON pilotés par `encoding/json`.
- **Rationale**: Le format JSON est imposé par la spec pour la configuration initiale, n'introduit aucune dépendance, et permet des tests unitaires directs sur le contenu sérialisé.
- **Alternatives considered**: TOML ou YAML auraient exigé une dépendance externe ou une logique de parsing supplémentaire non justifiée pour le MVP.

## Decision 3: Encapsuler les téléchargements dans une couche `internal/http`

- **Decision**: Centraliser les requêtes HTTP dans `internal/http` avec validation explicite des statuts, notamment `404`.
- **Rationale**: Cela satisfait la séparation constitutionnelle des responsabilités, simplifie la réutilisation entre `install` et `update`, et permet de tester les erreurs réseau via `httptest.Server`.
- **Alternatives considered**: Lancer `curl`/`wget` ou mélanger les appels HTTP dans `cmd/` a été rejeté pour des raisons de portabilité, de testabilité et de respect de la constitution.

## Decision 4: Tester chaque couche avec la bibliothèque standard `testing`

- **Decision**: Écrire des tests unitaires table-driven dans `cmd/`, `internal/http/` et `internal/fs/`, avec `httptest` pour le réseau et `t.TempDir()` pour le disque.
- **Rationale**: La constitution impose une couverture unitaire stricte sans framework externe. Cette approche couvre les flux heureux et les erreurs sans couplage à l'environnement utilisateur réel.
- **Alternatives considered**: Tests manuels seuls ou frameworks d'assertion externes rejetés car insuffisants ou non conformes.

## Decision 5: Utiliser des chemins portables basés sur `os.UserHomeDir` et `filepath.Join`

- **Decision**: Résoudre le répertoire utilisateur avec `os.UserHomeDir` et construire tous les chemins avec `filepath.Join`.
- **Rationale**: Cela garantit le fonctionnement sur Windows, macOS et Linux sans branches spécifiques, conformément à la constitution.
- **Alternatives considered**: Concaténation manuelle de chaînes ou variables d'environnement spécifiques à un OS rejetées pour cause de fragilité.
