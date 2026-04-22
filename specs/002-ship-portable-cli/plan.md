# Implementation Plan: Ship Portable CLI

**Branch**: `[003-ship-portable-cli]` | **Date**: 2026-04-22 | **Spec**: `/specs/002-ship-portable-cli/spec.md`
**Input**: Feature specification from `/specs/002-ship-portable-cli/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/plan-template.md` for the execution workflow.

## Summary

Livrer LoreSkills comme CLI distribuable depuis les GitHub releases avec une matrice MVP de quatre artefacts (`windows/amd64`, `linux/amd64`, `darwin/amd64`, `darwin/arm64`), un fichier SHA256 de vérification, une commande `version`, une aide contextualisée cohérente, et un avatar ASCII présent sur chaque sortie visible. L'implémentation conserve l'architecture stdlib-only existante, étend le package `cmd/` avec un registre de commandes et une couche de présentation partagée, et ajoute un workflow GitHub Actions dédié à la publication cross-platform en `CGO_ENABLED=0`.

## Technical Context

**Language/Version**: Go 1.22  
**Primary Dependencies**: Go standard library only for application code; GitHub Actions workflows for CI/release automation  
**Storage**: Local JSON files (`~/.loreskillsrc`, `<target_dir>/lore-state.json`), repository Markdown/YAML docs, GitHub release artifacts plus SHA256 manifest  
**Testing**: `go test ./...`, `go vet ./...`, package-local `*_test.go`, CLI smoke tests for help/version/avatar, GitHub Actions matrix validation on supported runners  
**Target Platform**: Runtime support on Windows, macOS, and Linux; published MVP artifacts for `windows/amd64`, `linux/amd64`, `darwin/amd64`, and `darwin/arm64` with `CGO_ENABLED=0`
**Project Type**: Single-binary CLI application with repository-hosted release automation  
**Performance Goals**: Help and version commands remain near-instant on local machines; release workflow publishes all four MVP artifacts and checksum data in a single run; avatar rendering must not materially delay local command execution  
**Constraints**: Standard library only, no hardcoded runtime repository values beyond the starter config placeholder, strict `cmd/` → `internal/http/` and `internal/fs/` boundaries, avatar shown on every visible output, build metadata injected at compile time, explicit user-facing errors, unit tests required for all new behavior  
**Scale/Scope**: One CLI binary family, five end-user commands (`init`, `install`, `update`, `help`, `version`), four published platform artifacts, one checksum manifest, and one release/install documentation path per supported OS

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **Gate 1 - Standard library only**: PASS. The planned CLI changes use Go stdlib only; release automation lives in GitHub workflow YAML and does not add Go modules.
- **Gate 2 - Configuration-driven behavior**: PASS. The feature changes distribution, help, and build metadata, but leaves runtime repository/target settings driven by `.loreskillsrc`.
- **Gate 3 - Cross-platform static build**: PASS IF IMPLEMENTED. All release artifacts must be built with `CGO_ENABLED=0`, and packaging/documentation must stay within the chosen MVP platform matrix.
- **Gate 4 - Modular layering**: PASS WITH DISCIPLINE. Command discovery, help rendering, avatar presentation, and version reporting stay inside `cmd/`; filesystem and network responsibilities remain confined to `internal/fs/` and `internal/http/`.
- **Gate 5 - TDD and unit coverage**: PASS IF IMPLEMENTED. New command routing, help output, version output, avatar rendering, and any build metadata formatting helpers must ship with package-local tests; workflow/documentation changes must be covered by validation steps.

**Post-Design Check**: Expected to remain PASS after Phase 1 because the selected design adds no new Go dependency, preserves package boundaries, and keeps release automation outside application runtime paths.

## Project Structure

### Documentation (this feature)

```text
specs/002-ship-portable-cli/
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
├── copilot-instructions.md
└── workflows/
  ├── ci.yml
  └── release.yml

cmd/
├── root.go
├── root_test.go
├── init.go
├── init_test.go
├── install.go
├── install_test.go
├── update.go
├── update_test.go
├── version.go
├── version_test.go
└── help_test.go

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

**Structure Decision**: Keep the current single-binary repository layout. The CLI-facing behavior stays in `cmd/` through a shared command catalog, version output, and avatar-aware presentation helpers; existing filesystem and HTTP layers remain unchanged in responsibility. Release automation is introduced as a separate `.github/workflows/release.yml` workflow, and user-facing installation guidance is updated in `README.md` rather than creating a second documentation system.

## Complexity Tracking

No constitution violations are expected for this feature.
