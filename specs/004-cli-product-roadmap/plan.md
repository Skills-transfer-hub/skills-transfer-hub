# Implementation Plan: CLI Product Roadmap — Decomposition

**Branch**: `006-cli-product-roadmap` | **Date**: 2026-04-23 | **Spec**: [spec.md](spec.md)

## Summary

The initial roadmap spec (004) was reviewed and found to contain foundational ambiguities
that made it unimplementable as a single feature: credential storage model conflicts,
an insufficient state schema, overloaded `ref` terminology, implicit bundle semantics,
and missing multi-provider configuration UX.

**Decision: split into 4 sequentially-dependent specs**, each independently plannable
and implementable. The rubber-duck review identified that all lifecycle, CI/auth, and
ecosystem features depend on a new state/config foundation that does not yet exist.

## Decomposition

| Spec | Title | Priority | Depends On |
|------|-------|----------|------------|
| [005-state-config-foundation](../005-state-config-foundation/spec.md) | State & Config Foundation | P0 | — |
| [006-resource-lifecycle](../006-resource-lifecycle/spec.md) | Resource Lifecycle | P1 | 005 |
| [007-ci-auth-observability](../007-ci-auth-observability/spec.md) | CI, Auth & Observability | P1 | 005 |
| [008-ecosystem-tooling](../008-ecosystem-tooling/spec.md) | Ecosystem Tooling | P2 | 005, 006 |

## Technical Context

**Language/Version**: Go (latest stable, currently 1.24)  
**Primary Dependencies**: Go standard library only (constitution constraint)  
**Storage**: JSON files — `.sth/project.json` (project config) + `sth-state.json` (install state)  
**Testing**: `go test ./...` — stdlib `testing` package only  
**Target Platform**: Windows, macOS, Linux (cross-platform, CGO_ENABLED=0)  
**Project Type**: CLI tool  
**Constraints**: zero external modules, static binary, backward-compatible with existing project.json  

## Constitution Check

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Zéro Dépendance Externe | ✅ | All 4 sub-specs use stdlib only |
| II. Agnostique — Aucune Valeur Codée en Dur | ✅ | No hardcoded URLs or paths |
| III. Cross-Platform — CGO Désactivé | ✅ | No OS-specific APIs |
| IV. Découpage Modulaire Strict | ✅ | cmd/ ← internal/http/ + internal/fs/ |

## Project Structure

```text
specs/
├── 004-cli-product-roadmap/     ← this decomposition document
├── 005-state-config-foundation/ ← foundational state + config redesign
├── 006-resource-lifecycle/      ← status, remove, dry-run, pinning
├── 007-ci-auth-observability/   ← auth, OIDC, doctor, verbose, --json, completion
└── 008-ecosystem-tooling/       ← catalog init, skill new, changelog

internal/
├── fs/          ← state schema extensions, migration, integrity
└── http/        ← credential resolution, OIDC exchange

cmd/
├── status.go    ← sth status
├── remove.go    ← sth remove
├── doctor.go    ← sth doctor
├── completion.go← sth completion
├── catalog.go   ← sth catalog
└── new.go       ← sth new
```

## Implementation Order

1. **005 first** — must ship and be adopted before any other spec is started.
   All other specs assume the new state schema and provider terminology are in place.
2. **006 and 007 in parallel** — both build on 005 independently; no cross-dependency.
3. **008 last** — depends on bundle semantics from 005 + managed-file inventory from 006.
