# Implementation Plan: Interactive UX Overhaul — TUI Navigation & Consistency

**Branch**: `011-interactive-ux` | **Date**: 2026-08-22 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `specs/011-interactive-ux/spec.md`

## Summary

Rebuild every interactive surface of the `sth` CLI on `charmbracelet/bubbletea` + `lipgloss` + `bubbles`, behind a new `internal/tui` package of buddy-styled reusable components (Narrator, Select, Confirm, Input, Secret, MultiSelect, Progress). The guided menu gains universal return-to-menu with arrow-key navigation; the init wizard becomes a screen-stack flow with early alpha gates and an explicit Public/Private select replacing the inverted y/N trap; watch moves to the alternate screen; progress feedback becomes universal (install + update + self-update); a consistency sweep makes all copy English, unifies confirmations, and replaces the unknown-command help dump with closest-match suggestions. Pure decision logic stays framework-free. Non-TTY scripting surfaces (`--json/--yes/--force/--dry-run`, JSON envelopes, bridge/MCP contracts) are untouched.

## Technical Context

**Language/Version**: Go 1.25 (CLI only — no backend/dashboard changes)
**Primary Dependencies**: new `github.com/charmbracelet/bubbletea`, `github.com/charmbracelet/lipgloss`, `github.com/charmbracelet/bubbles`; existing `golang.org/x/term` retained for non-TTY secret fallback until fully superseded.
**Storage**: no schema change — same `.sth/project.json`, `.gitignore`, `.sth/last-operation.json` contracts.
**Testing**: stdlib `testing` headless harnesses per component (injected answer sequences, no pty) plus a bounded set of teatest key-driven integration tests; existing sequential `t.Setenv` discipline preserved.
**Target Platform**: Windows, macOS, Linux — bubbletea/lipgloss are pure Go, `CGO_ENABLED=0` unchanged.
**Project Type**: CLI-only refactor inside `cmd/` + new `internal/tui/`.
**Performance Goals**: bare `sth` first paint ≤ current banner timing (fast buddy state path kept).
**Constraints**: strict repo Go style (no single-letter vars, zero comments incl. godoc); no color palette yet; bridge/MCP untouched.
**Scale/Scope**: ~15 files rewritten/new in `internal/tui` + `cmd/interactive layer`.

## Constitution Check

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Zéro Dépendance Externe | ⚠️ AMENDEMENT ASSUMÉ | bubbletea/lipgloss/bubbles added by explicit product decision (spec Context); precedent set by fsnotify/keyring/MCP SDK. Hand-rolled ANSI cannot deliver cross-platform arrow navigation. |
| II. Agnostique — Aucune Valeur Codée en Dur | ✅ PASS | No new hardcoded URLs/paths; provider vocabulary and docs URLs reused from existing constants. |
| III. Cross-Platform — CGO Désactivé | ✅ PASS | bubbletea/lipgloss pure Go, first-class Windows support. |
| IV. Découpage Modulaire Strict | ✅ PASS | `cmd/` screens consume `internal/tui` components; decision logic (startup state, buddy states, dispatch tables) stays framework-free in `cmd/`. No inversion: internal/tui never imports cmd/. |
| Tests (TDD strict) | ✅ PASS | Every component ships a headless harness test before its consumer is ported. |

## Project Structure

### Documentation (this feature)

```text
specs/011-interactive-ux/
├── spec.md
└── plan.md
```

### Source Code (per repo)

```text
# skills-transfer-hub (CLI)
internal/tui/
├── theme.go            # lipgloss styles, box metrics ported from bannerOuterWidth vocabulary
├── narrator.go         # mascot box + moods (hello/thinking/worried/sad/happy/error)
├── select.go           # arrow list + typed aliases + recommended default
├── confirm.go          # single confirmation primitive o/oui/y/yes/n/non
├── input.go            # text input with defaults
├── secret.go           # password-masked input (bubbles/textinput EchoPassword)
├── multiselect.go      # checkbox list, grammar 1,3|slug-list|all preserved
├── progress.go         # braille spinner + mini buddy face + step N/M
├── program.go          # shared run helpers: TTY detect, headless answer-seq runner, alt-screen lifecycle
└── *_test.go           # headless harness tests per component

cmd/
├── menu.go             # GuidedMenuModel on bubbletea; universal return-to-menu
├── init_wizard.go      # InitFlowModel screen stack; early gates; access select
├── init_secret.go      # delegates to tui.Secret (TTY) / x/term fallback removed once ported
├── init_telemetry_mode.go  # tui.Select based
├── init_cloud_login.go     # tui.Confirm + tui.Secret
├── banner.go           # static/animated frames rendered via tui theme (timing kept)
├── buddy_watch.go      # bubbletea alt-screen program; fsnotify events -> model messages
├── install_progress.go # thin adapter over tui.Progress
├── update.go           # instantiates Progress (fix nil no-op steps)
├── login.go            # argv-key warning + masked prompt via tui
├── recoverable.go      # English strings + Confirm/Input components
├── remove.go catalog.go pack_init.go pack_promote.go  # unified Confirm, FR copy
├── root.go             # unknown-command suggestions (Levenshtein over commandCatalog), dispatch intact
└── doctor.go completion.go  # FR summary strings
```

### Per-phase breakdown

1. **Foundation**: `go get` charm libs; `internal/tui` theme + program runner + each component with headless harness tests.
2. **Menu**: port `menu.go` onto Select/Narrator; universal return-to-menu; keep `loadGuidedStartupState` untouched.
3. **Wizard**: screen-stack rewrite; sth-cloud rejection at provider step; private-cap loop-back at access step; access as two-option select.
4. **Watch & progress**: alt-screen watch model fed by fsnotify messages; Progress wired into update/self-update.
5. **Consistency sweep**: FR strings, single Confirm everywhere, suggestion engine in root dispatch, argv-key warning in login.
6. **Test port**: rework stdin-injection tests to headless harnesses; teatest for menu + wizard critical paths; full build/test green.

## Complexity Tracking

| Question | Answer |
|----------|--------|
| Does the spec introduce new external deps? | Yes — charmbracelet trio, documented as constitution amendment. |
| Any backward-incompatible CLI contract? | No — flags, JSON shapes, exit codes, bridge/MCP payloads unchanged. |
| Biggest risk | Big-bang diff size; mitigated by phase-ordered commits, framework-free core logic, and per-component headless tests enabling mechanical port verification. |
