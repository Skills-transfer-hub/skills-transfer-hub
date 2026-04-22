# Implementation Plan: Private Repo Skills

**Branch**: `[004-private-repo-skills]` | **Date**: 2026-04-22 | **Spec**: `/specs/003-private-repo-skills/spec.md`
**Input**: Feature specification from `/specs/003-private-repo-skills/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/plan-template.md` for the execution workflow.

## Summary

Faire évoluer LoreSkills d'un modèle home-scoped vers un modèle project-scoped pour dépôts privés, avec un profil projet sous `.loreskills/`, une découverte distante pilotée par `SKILLS.md`, une commande `list` pour parcourir les skills/agents/ressources disponibles, une mise à jour stable-only du CLI via metadata de release injectée au build, une signature de version minimale partagée entre `loreskills` et `loreskills version`, et une commande cachée `stockholm` pour l'easter egg ASCII. L'implémentation reste stdlib-only, garde le découpage `cmd/` → `internal/http/` / `internal/fs/`, et supprime l'ancien animal ASCII des sorties standard.

## Technical Context

**Language/Version**: Go 1.22  
**Primary Dependencies**: Go standard library only (`os`, `encoding/json`, `bufio`, `strings`, `net/http`, `net/url`, `os/exec`, `io`, `testing`, `net/http/httptest`)  
**Storage**: Profil projet local sous `.loreskills/project.json`, état local sous `<target_dir>/lore-state.json`, catalogue distant `SKILLS.md`, métadonnées de release injectées au build  
**Testing**: `go test ./...`, `go vet ./...`, tests package-local `*_test.go`, `httptest`, assertions sur sorties CLI, tests filesystem avec `t.TempDir()`  
**Target Platform**: CLI statique Windows, macOS et Linux avec dépôts privés accessibles en HTTPS pour GitHub, GitLab, Azure DevOps et Bitbucket  
**Project Type**: Single-binary CLI application  
**Performance Goals**: `loreskills` et `version` restent quasi instantanés en local; `list` effectue une seule lecture distante du catalogue; `update self` choisit la dernière release stable en un seul parcours du flux de releases  
**Constraints**: Standard library only, aucun secret stocké dans le profil projet, aucune valeur de dépôt utilisateur codée en dur, respect strict des couches `cmd/` → `internal/http/` / `internal/fs/`, suppression de l'ancien animal ASCII sur les commandes standard, `stockholm` absent de l'aide visible, tests unitaires obligatoires pour tous les nouveaux flux  
**Scale/Scope**: Un dépôt actif par projet, quelques dizaines de ressources cataloguées par projet, six commandes visibles (`init`, `list`, `install`, `update`, `help`, `version`) et une commande cachée (`stockholm`), un canal officiel de release stable pour le self-update

## Constitution Check

_GATE: Must pass before Phase 0 research. Re-check after Phase 1 design._

- **Gate 1 - Standard library only**: PASS. Le design repose uniquement sur la stdlib Go; l'accès provider-specific passe par `net/http` et éventuellement `os/exec` pour les credential helpers Git existants.
- **Gate 2 - Configuration-driven behavior**: PASS WITH BUILD-TIME ALIGNMENT. Les dépôts utilisateur et leurs chemins restent lus depuis le profil projet; le canal officiel de self-update est injecté au build pour éviter toute valeur de dépôt codée en dur au runtime.
- **Gate 3 - Cross-platform static build**: PASS. Les fichiers restent gérés via `os`/`filepath`, les appels réseau via `net/http`, et le self-update doit utiliser des flux compatibles Windows/macOS/Linux sans dépendance dynamique.
- **Gate 4 - Modular layering**: PASS WITH DISCIPLINE. `cmd/` garde le routage, l'interactivité et le rendu CLI; `internal/http/` centralise lecture de `SKILLS.md`, téléchargement de ressources et lecture des releases; `internal/fs/` gère profil projet, état local et remplacement du binaire.
- **Gate 5 - TDD and unit coverage**: PASS IF IMPLEMENTED. Les nouveaux flux (`init` interactif, `list`, catalog parsing, self-update stable-only, signature de version minimale, commande cachée) exigent des tests package-local avant clôture.

**Post-Design Check**: PASS. Les artefacts Phase 1 conservent le découpage par couches, ne demandent aucune dépendance externe, déplacent la configuration vers un profil projet sans secret, et traitent le canal de self-update comme une metadata de build plutôt qu'une constante runtime du code applicatif.

## Project Structure

### Documentation (this feature)

```text
specs/003-private-repo-skills/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   └── cli-contract.md
└── tasks.md
```

### Source Code (repository root)

```text
.github/
└── copilot-instructions.md

cmd/
├── root.go
├── root_test.go
├── help_test.go
├── init.go
├── init_test.go
├── list.go
├── list_test.go
├── install.go
├── install_test.go
├── update.go
├── update_test.go
├── version.go
├── version_test.go
├── stockholm.go
└── stockholm_test.go

internal/
├── fs/
│   ├── fs.go
│   └── fs_test.go
└── http/
    ├── client.go
    └── client_test.go

main.go
README.md
go.mod
```

**Structure Decision**: Conserver la structure mono-binaire existante. Étendre `cmd/` pour l'onboarding interactif, la découverte distante, la signature de version minimale, le self-update orchestré et la commande cachée; étendre `internal/http/` pour le catalogue `SKILLS.md`, les téléchargements de ressources et la lecture des releases stables; étendre `internal/fs/` pour la persistance du profil projet, la migration depuis `~/.loreskillsrc`, l'état enrichi et les opérations locales de remplacement du binaire. La documentation utilisateur reste centralisée dans `README.md`.

## Complexity Tracking

No constitution violations are expected for this feature.
