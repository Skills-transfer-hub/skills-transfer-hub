# Implementation Plan: Core CLI et MVP

**Branch**: `[001-build-core-cli-mvp]` | **Date**: 2026-04-21 | **Spec**: `/specs/001-build-core-cli-mvp/spec.md`
**Input**: Feature specification from `/specs/001-build-core-cli-mvp/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/plan-template.md` for the execution workflow.

## Summary

Construire le MVP du CLI LoreSkills autour de trois commandes `init`, `install` et `update`, en gardant un binaire Go sans dépendance externe, piloté par configuration utilisateur et portable sur Windows, macOS et Linux. L'implémentation s'appuiera sur un routage CLI manuel, des fichiers JSON locaux pour la configuration et l'état, une couche HTTP dédiée pour les téléchargements de skills, et des tests unitaires standard Go couvrant les flux de commande, les erreurs HTTP et les opérations de fichiers.

## Technical Context

<!--
  ACTION REQUIRED: Replace the content in this section with the technical details
  for the project. The structure here is presented in advisory capacity to guide
  the iteration process.
-->

**Language/Version**: Go 1.22  
**Primary Dependencies**: Go standard library only (`os`, `fmt`, `path/filepath`, `encoding/json`, `net/http`, `io`, `testing`, `net/http/httptest`)  
**Storage**: Local JSON files in the user home directory (`.loreskillsrc`) and current project (`<target_dir>/lore-state.json`)  
**Testing**: `go test ./...` with package-local `*_test.go` files using `testing`, `httptest`, and `t.TempDir()`  
**Target Platform**: Windows, macOS, Linux with `CGO_ENABLED=0`
**Project Type**: Single-binary CLI application  
**Performance Goals**: Help and init commands complete near-instantly on local disk; install/update add only one network round-trip per skill and avoid redundant filesystem work  
**Constraints**: Standard library only, no hardcoded runtime repository values outside the generated placeholder config, constitution-mandated separation `cmd/` → `internal/http/` and `internal/fs/`, explicit error messages, unit tests required for all new behavior  
**Scale/Scope**: MVP for one local user and one current project at a time, with support for a handful to a few dozen managed skills per project

## Constitution Check

_GATE: Must pass before Phase 0 research. Re-check after Phase 1 design._

- **Gate 1 - Standard library only**: PASS. The plan uses only Go stdlib packages for CLI parsing, HTTP, JSON, filesystem access, and tests.
- **Gate 2 - Configuration-driven behavior**: PASS. Runtime repo URL and target directory come from `.loreskillsrc`; the seeded placeholder value is only initial user-editable config.
- **Gate 3 - Cross-platform static build**: PASS. The plan uses `os.UserHomeDir`, `filepath.Join`, JSON files, and `CGO_ENABLED=0` build/test commands with no OS-specific APIs.
- **Gate 4 - Modular layering**: PASS WITH REQUIRED ALIGNMENT. Implementation must preserve CLI routing in `cmd/`, filesystem logic in `internal/fs/`, and rename or replace the current network package so it lives under `internal/http/` per constitution.
- **Gate 5 - TDD and unit coverage**: PASS IF IMPLEMENTED. Every command flow and helper will ship with package-local tests before task completion.

**Post-Design Check**: Expected to remain PASS after creating the CLI contract, data model, quickstart, and explicit test strategy below.

## Project Structure

### Documentation (this feature)

```text
specs/[###-feature]/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
cmd/
├── root.go
├── init.go
├── install.go
└── update.go

internal/
├── http/
│   ├── client.go
│   └── client_test.go
└── fs/
    ├── fs.go
    └── fs_test.go

main.go
go.mod
```

**Structure Decision**: Keep the existing single-binary root layout and align it with the constitution by retaining command routing in `cmd/`, filesystem concerns in `internal/fs/`, and consolidating all network retrieval logic into `internal/http/`. Tests stay adjacent to the production code in the same directories using `*_test.go` files.

## Complexity Tracking

No constitution violations are expected after the planned rename/alignment of the network package to `internal/http/`.
