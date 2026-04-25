<!--
SYNC IMPACT REPORT
Version change: 1.0.0 → 1.1.0
Added sections: explicit dependency carve-out for fsnotify (Principle I)
Removed sections: none
Templates updated:
  ✅ constitution.md — fsnotify exception added under Principle I
Follow-up TODOs: none
-->

# STH Constitution

## Principes Fondamentaux

### I. Zéro Dépendance Externe (NON-NÉGOCIABLE, sauf exceptions listées)

Le projet DOIT utiliser exclusivement la bibliothèque standard de Go (`os`, `net/http`,
`path/filepath`, `encoding/json`, etc.). Aucun module tiers (Cobra, Viper, resty…) n'est
autorisé. Chaque nouvelle dépendance proposée DOIT être rejetée à la revue sauf si elle
est fournie par la stdlib Go.

**Exceptions ratifiées** (toute nouvelle exception nécessite un amendement explicite) :

- `github.com/fsnotify/fsnotify` — la stdlib n'expose aucune API portable de notification
  filesystem. Requis uniquement par `sth buddy --watch`. Justification : implémenter la
  surveillance manuellement via polling ou ioctl spécifiques à chaque OS dépasse le
  budget complexité raisonnable d'une simple commande d'observation.

**Raison** : garantir un binaire final minuscule, sans chaîne d'approvisionnement externe
et reproductible par `go build` seul.

### II. Agnostique — Aucune Valeur Codée en Dur

Le code source NE DOIT contenir aucune URL, aucun chemin de fichier ou identifiant de
dépôt spécifique. Toutes les valeurs de configuration (remote URL, dossier cible, etc.)
DOIVENT être lues depuis le fichier de configuration utilisateur au moment de l'exécution.

**Raison** : l'outil doit pouvoir pointer vers n'importe quel dépôt sans recompilation.

### III. Cross-Platform — CGO Désactivé

Le code DOIT compiler sans modification sur Windows, macOS et Linux. `CGO_ENABLED=0`
DOIT être positionné pour tous les artefacts de build. Aucune API spécifique à un OS ne
peut être utilisée sans branche de compilation (`//go:build`).

**Raison** : le CLI doit être distribué sous forme de binaire statique unique pour chaque
plateforme cible.

### IV. Découpage Modulaire Strict

La logique DOIT être séparée en trois couches indépendantes :

- **`cmd/`** : routage des sous-commandes CLI, parsing des arguments — aucune logique métier.
- **`internal/http/`** : toutes les communications réseau (téléchargement, listing) —
  aucune écriture disque directe.
- **`internal/fs/`** : toutes les opérations I/O fichier/dossier — aucun appel réseau direct.

Chaque couche peut être testée indépendamment. Les dépendances entre couches descendent
uniquement vers le bas (cmd → http/fs, jamais l'inverse).

**Raison** : facilite les tests unitaires, la lisibilité et l'évolution indépendante de
chaque couche.

## Contraintes Techniques

- **Langage** : Go (dernière version stable).
- **Build** : `go build ./...` DOIT réussir sans warning avec `go vet ./...`.
- **Tests** : `go test ./...` DOIT passer sur les trois OS cibles.
- **Binaire** : produit par `CGO_ENABLED=0 go build` — aucune dépendance dynamique.
- **Configuration** : fichier JSON ou TOML dans un emplacement standard (`~/.config/sth/`
  ou variable d'environnement `STH_CONFIG`).

## Style de Code

- **Idiomatique Go** : nommage, gestion des erreurs et organisation des packages suivent
  les conventions de `Effective Go` et du Go Style Guide officiel.
- **Gestion d'erreurs** : pas de `panic` sauf en cas de bug de programmation avéré ; toujours
  renvoyer des messages d'erreur exploitables par l'utilisateur final (contexte + cause).
- **Commentaires** : concis, en français, expliquant le _pourquoi_ des blocs non évidents ;
  les godoc sur les symboles exportés DOIVENT être en anglais.
- **YAGNI** : pas d'abstraction prématurée ; la complexité DOIT être justifiée.

## Governance

- Cette constitution prime sur tout autre document de pratiques de l'équipe.
- Toute violation détectée en revue de code DOIT bloquer le merge.
- Les amendements DOIVENT être documentés dans ce fichier avec bump de version sémantique :
  - MAJOR : suppression ou redéfinition incompatible d'un principe.
  - MINOR : ajout d'un nouveau principe ou d'une section.
  - PATCH : clarification, reformulation, correction de coquilles.
- Toute PR DOIT vérifier la conformité avec les quatre principes fondamentaux avant approbation.

**Version**: 1.1.0 | **Ratified**: 2026-04-19 | **Last Amended**: 2026-04-25

## Règles de Qualité et Tests (TDD Strict)
1. **Couverture Obligatoire :** TOUTE nouvelle fonctionnalité, commande CLI ou fonction utilitaire doit IMPÉRATIVEMENT être accompagnée de ses Tests Unitaires (TU). C'est non négociable.
2. **Technologie :** Utilise exclusivement le package standard `testing` de Go. N'utilise pas de frameworks d'assertion externes (comme `testify`) pour garder le binaire léger.
3. **Structure des Fichiers :** Les tests doivent être placés dans des fichiers `*_test.go` dans le même dossier que le code testé (ex: si tu crées `installer.go`, tu dois créer `installer_test.go`).
4. **Validation de Fin de Tâche :** Avant de considérer une spécification comme terminée ou de proposer un commit, tu dois générer les tests, et t'assurer (mentalement ou en l'exécutant si tu as accès au terminal) que la commande `go test ./...` passera avec succès.
