# Feature Specification: Interactive UX Overhaul — TUI Navigation & Consistency

**Feature Branch**: `011-interactive-ux`
**Created**: 2026-08-22
**Status**: Draft
**Input**: Product decision — "Je trouve le menu de navigation CLI de STH horrible et pas ergonomique on doit revoir tout cela." Full interactive surface (guided menu, init wizard, watch dashboard, progress, confirmations) is rebuilt on a real TUI framework while preserving the buddy mascot identity and the non-TTY scripting surfaces.

**Depends On**: 005 (state/config foundation), 006 (resource lifecycle), 009 (active org)
**Blocks**: none

## Context

Today every interactive screen in `sth` is hand-rolled stdlib: a giant `switch` dispatch in `cmd/root.go`, numbered typed menus in `cmd/menu.go`, a ~12-prompt sequential wizard in `cmd/init_wizard.go`, raw ANSI cursor tricks in `cmd/buddy_watch.go`, and one braille spinner in `cmd/install_progress.go`. There is no arrow-key navigation anywhere, no fuzzy matching, and no colors. The result works but is inconsistent and error-prone:

- The guided menu silently **exits the program** for `list/install/update/init` but **returns to the menu** for `help/show-config/provider-guide`. Nothing tells the user which behavior to expect.
- `promptAccessType` (`cmd/init_wizard.go`) answers `y` = **private**, inverted vs every other y/N prompt in the CLI.
- Three different confirmation implementations exist; two of them (`remove.go`, `catalog.go`) reject legacy aliases like "oui".
- UI copy is predominantly French, while the product language is now English; every visible interactive surface must be migrated consistently.
- `update` and `update self` show **no progress at all**: the shared spinner widget is never instantiated outside `install`, so its step calls are nil no-ops.
- Alpha gates in the wizard (sth-cloud rejection, 1-private-repo cap) fire *after* several answers were consumed, killing the flow instead of looping back.
- An unknown command dumps the entire help text to stderr.
- `buddy --watch` redraws with clear-home instead of an alternate screen buffer (scrollback pollution) and needs `q` + Enter to quit.

This feature rebuilds the whole interactive layer on `charmbracelet/bubbletea` + `lipgloss` + `bubbles`, behind a new `internal/tui` package of buddy-styled components. Pure logic (state machines, action dispatch) stays framework-free so it remains testable without a pty. Non-TTY degradation (`--json/--yes/--force/--dry-run`, static banner, stderr progress lines) is preserved exactly.

This spec **amends** the historical "Zéro Dépendance Externe" principle by explicit product decision: a maintained TUI framework replaces ~1 500 lines of hand-rolled ANSI/prompt code that cannot deliver arrow-key navigation safely cross-platform.

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Universal return-to-menu (Priority: P1)

A developer launches bare `sth` and picks actions from the guided menu. Whatever they run — list, install, update, init — they land back on the menu afterwards; only the explicit "Quit" entry exits.

**Why this priority**: The current asymmetry (silent exit vs return) is the single most disorienting navigation flaw; fixing it changes every session's feel.

**Independent Test**: Launch bare `sth` in a TTY, run each menu entry in sequence, verify the menu re-renders with refreshed state after each action, and verify only "Quit" terminates the process.

**Acceptance Scenarios**:

1. **Given** a configured project, **When** the user picks `list` then `update` from the guided menu, **Then** after each command completes the menu is displayed again with state recomputed.
2. **Given** any menu session, **When** the user selects `Quit` or presses `q`/`ctrl+c`, **Then** the program exits cleanly.
3. **Given** a stable binary update was downloaded previously, **When** the menu renders, **Then** the update offer block appears as before and selecting it applies the update then returns to the menu.
4. **Given** a non-TTY stdout, **When** bare `sth` runs, **Then** the existing plain guidance lines are printed (unchanged behavior).

---

### User Story 2 — Arrow-key selection everywhere (Priority: P1)

All choice screens — guided menu, wizard provider/access/targets steps, first-run telemetry mode, confirmations — are navigable with ↑/↓ and Enter, while the existing typed grammar (action slugs, `o/oui/y/yes/n/non`, `1,3`/`claude,copilot`/`all`) keeps working for muscle memory and scripts.

**Why this priority**: Typed-number-only menus are the core ergonomic complaint; arrow keys are table stakes for a modern CLI.

**Independent Test**: Drive each select/multi-select/confirm component through both input modes (keys via teatest, tokens via headless harness) and assert identical resulting values.

**Acceptance Scenarios**:

1. **Given** the guided menu, **When** the user presses ↓ twice then Enter, **Then** the third visible action executes.
2. **Given** the wizard targets multi-select pre-checked from filesystem detection, **When** the user toggles entries with space/x and confirms with Enter, **Then** exactly those targets are saved; typing `1,3` or `all` produces the same selection.
3. **Given** any yes/no confirmation, **When** the user types `oui`/`o`/`y`/`yes` or presses → on "Yes", **Then** the answer is yes regardless of which legacy prompt site triggered it.
4. **Given** the repository access step, **When** the user selects between the two explicitly labeled options "Public" / "Private", **Then** no y/N inversion trap exists (the old `y`=private semantics disappear).

---

### User Story 3 — Fully English interface (Priority: P1)

Every string printed to the terminal during interactive use is English, including confirmations, removals, doctor output, completion errors, and auth recovery prompts. French typed aliases remain accepted where they preserve compatibility.

**Why this priority**: Language leaks undermine trust in a polished product and were flagged directly by the product owner.

**Independent Test**: Run all interactive flows and grep captured output for the known French leak sites; zero matches expected.

**Acceptance Scenarios**:

1. **Given** `sth remove <ref>` with locally modified files, **When** confirmation is requested, **Then** the warning list and prompt are English ("Remove anyway?").
2. **Given** `sth catalog init` over an existing SKILLS.md, **When** prompted, **Then** the overwrite question is English and still accepts `oui` as a compatibility alias.
3. **Given** `sth doctor`, **When** all checks pass, **Then** the summary line is English.
4. **Given** a 401 mid-command in an interactive session, **When** token recovery triggers, **Then** the announcement and masked re-entry prompt are English.

---

### User Story 4 — Unified progress feedback (Priority: P2)

Every long-running operation — `install`, `update`, `update self` — shows the same spinner-with-steps widget (braille spinner + mini buddy face + "step N of M"), erased cleanly on completion.

**Why this priority**: Silent long operations read as hangs; the widget already exists for install and just needs to be universal.

**Independent Test**: Run `sth update --json=false` against a fixture catalog and observe step labels; run `update self` against a fake release server and observe download progress before swap.

**Acceptance Scenarios**:

1. **Given** outdated tracked resources, **When** `sth update` runs interactively, **Then** per-resource step labels render through the same widget as install.
2. **Given** a package-manager-owned binary refuses self-update, **When** the hint prints, **Then** no spinner artifacts remain on screen.
3. **Given** non-TTY output, **When** update runs, **Then** `→ [n/N] label` lines appear on stderr exactly as install does today.

---

### User Story 5 — Early alpha gates in the wizard (Priority: P2)

In `sth init`, gate failures happen at the earliest question that reveals them: choosing `sth-cloud` is rejected immediately at the provider step; the alpha cap of one private repo loops back to the access step with an explanation instead of aborting the wizard.

**Why this priority**: Losing ten minutes of answers to a foreseeable rejection is the wizard's worst failure mode.

**Independent Test**: Attempt `sth init` selecting sth-cloud: rejection occurs before alias/repo questions. Select private access twice for different repos in one project: second attempt loops back to access selection, wizard never aborts.

**Acceptance Scenarios**:

1. **Given** the provider step, **When** the user selects `sth-cloud`, **Then** the waitlist explanation shows immediately and focus returns to the provider list.
2. **Given** one private repo already configured, **When** the user chooses "Private" for a second provider, **Then** a message explains the alpha cap and the access question is asked again.
3. **Given** valid choices throughout, **When** the recap renders, **Then** confirming saves config exactly as today.

---

### User Story 6 — Helpful unknown-command errors (Priority: P2)

Typing an unknown command prints the error plus at most a handful of closest-match suggestions and a pointer to `sth help`; never the full help dump.

**Why this priority**: A wall of stderr text trains users to ignore errors.

**Independent Test**: Run `sth instal`, `sth lst`, `sth statusx` — each prints one suggestion line maximum three candidates.

**Acceptance Scenarios**:

1. **Given** `sth instal`, **When** dispatch fails, **Then** output includes `Unknown command: instal` and suggests `install`.
2. **Given** input with no close match, **When** dispatch fails, **Then** only the error and `sth help` pointer print.

---

### User Story 7 — Clean full-screen watch (Priority: P3)

`sth buddy --watch` runs in the alternate screen buffer, repaints atomically, restores the terminal perfectly on exit, and quits on `q` without Enter.

**Why this priority**: Polish item; scrollback pollution annoys power users daily.

**Independent Test**: Enter watch mode, type `q`, verify prompt returns immediately with original scrollback intact and cursor visible.

**Acceptance Scenarios**:

1. **Given** watch mode active, **When** the user presses `q`, **Then** exit is immediate (no Enter needed).
2. **Given** watch mode exited via ctrl+c, **When** the shell prompt returns, **Then** cursor is visible and the pre-watch scrollback is untouched.

### Edge Cases

- Piped/garbage stdin into an interactive flow — components must not hang or spin; bounded invalid-input handling (telemetry keeps its max-3-attempts fallback to off).
- Narrow terminals (<78 cols) — narrator boxes shrink padding to zero without breaking border alignment; selects degrade to plain lists.
- ctrl+c mid-wizard — alternate screen left cleanly, cursor restored, partial config NOT written (same contract as today).
- Non-TTY stdin with TTY stdout (and vice versa) — each component independently degrades; scripting flags always win.
- `login <key>` argv form kept working but prints a warning that shell history may store the key, steering toward the masked prompt.

## Requirements *(mandatory)*

### Functional Requirements

**Foundation (`internal/tui`)**

- **FR-001**: A new `internal/tui` package MUST provide buddy-styled components built on bubbletea/lipgloss/bubbles: `Narrator` (rounded box + mascot mood), `Select`, `Confirm`, `Input`, `Secret` (password-masked), `MultiSelect`, and `Progress` (braille spinner + step counter). All rendering constants (box width, emoji moods) MUST be ported from the existing banner/wizard vocabulary.
- **FR-002**: Every component MUST offer a headless execution path driven by injected answer sequences (stdlib-testable, no pty required); `teatest` MAY cover a small number of key-driven integration tests.
- **FR-003**: Pure decision logic currently framework-free (`loadGuidedStartupState`, buddy state computation, action dispatch tables) MUST remain pure Go with no bubbletea imports.

**Guided menu**

- **FR-004**: Bare `sth` in a TTY MUST render an arrow-navigable menu where every executed action returns to the menu with recomputed state; only `Quit` (or q/ctrl+c) exits.
- **FR-005**: Typed aliases (`list`, `install`, …) and empty-input-takes-recommended MUST keep working alongside arrow keys.

**Wizard init**

- **FR-006**: The init flow (telemetry consent, cloud login sub-prompt, provider loop, repo/ref/path, targets, access, token, recap/edit loop, validation diagnosis) MUST run as TUI screens with narration interludes, preserving today's data contract (config file contents, `.gitignore`, last-operation record).
- **FR-007**: Repository access MUST be an explicit Public/Private select; the legacy `y`=private prompt semantics MUST be removed.
- **FR-008**: Alpha gates MUST fire early: sth-cloud rejected at provider selection; the private-repo cap MUST loop back to the access step rather than aborting.

**Progress & watch**

- **FR-009**: `Progress` MUST be instantiated by install AND update AND self-update; non-TTY fallback lines on stderr MUST match the existing `[n/N]` format.
- **FR-010**: `buddy --watch` MUST use the bubbletea alternate-screen program lifecycle, quit on unbuffered `q` or ctrl+c, and keep fsnotify-driven debounced refresh.

**Consistency**

- **FR-011**: Exactly ONE confirmation implementation (the tui `Confirm`) MUST exist across remove/catalog/pack/init flows, accepting o/oui/y/yes/n/non with visible default.
- **FR-012**: ALL user-facing strings in interactive paths MUST be English (including remove, catalog, doctor summary, completion usage errors, auth recovery). Existing French input aliases MAY remain accepted for compatibility.
- **FR-013**: Unknown commands MUST produce the error plus up to three closest-match suggestions computed over `commandCatalog`, plus a `sth help` pointer; the full help dump MUST be removed.
- **FR-014**: Scripting surfaces MUST be unchanged: `--json/--yes/--force/--dry-run/--no-cloud-prompt` semantics, JSON envelopes, exit codes, bridge/MCP JSON-RPC contracts.

**Code quality**

- **FR-015**: New Go code MUST follow strict repo style (descriptive names, zero comments incl. godoc) and ship `*_test.go` files using stdlib `testing`; tests stay sequential where `t.Setenv` sandboxing applies.

### Key Entities

- **internal/tui components**: `Narrator(mood, body)`, `Select[T](options, recommended, aliases)`, `Confirm(question, defaultYes)`, `Input(label, default)`, `Secret(label)`, `MultiSelect(options, preselected)`, `Progress(steps)` — each with `RunInteractive()` / `RunHeadless(answers)`.
- **GuidedMenuModel**: bubbletea model wrapping the existing startup-state machine; emits executed actions upward, receives refreshed state.
- **InitFlowModel**: ordered screen stack with early-gate guards and recap/edit jump table replacing the recursive re-entry calls.

## Success Criteria *(mandatory)*

- **SC-001**: In a scripted TTY session exercising every guided-menu action, the process terminates exclusively via `Quit`/q/ctrl+c (100% of actions returned to menu).
- **SC-002**: Grep sweep across captured outputs of all interactive flows finds zero occurrences of the known French leak strings.
- **SC-003**: `sth update` and `sth update self` emit visible step progress in TTY mode and `[n/N]` stderr lines otherwise.
- **SC-004**: `sth instal|lst|statusx` each yield ≤3 suggestion lines and no full-help dump.
- **SC-005**: Watch mode leaves scrollback byte-identical and quits on a single `q` keystroke.
- **SC-006**: `go build ./...` and `go test ./internal/fs/... ./cmd/...` pass with the rewritten flows.

## Assumptions

- **Framework amendment**: adopting bubbletea/lipgloss/bubbles deliberately supersedes the "zéro dépendance externe" habit; fsnotify/keyring/MCP deps set the precedent for targeted additions.
- **Big-bang migration** in phase-ordered commits on one branch: foundation → menu → wizard → watch/progress → consistency sweep → test port. The CLI builds green between commits but old prompt code disappears within this feature, not across releases.
- **Buddy identity unchanged**: moods, emoji vocabulary, 78-cell box heritage ported to lipgloss; visual drift limited to alignment robustness and (future, out-of-scope) color.
- **No color introduction yet** beyond what lipgloss gives structurally; palette decisions belong to a later pass.
