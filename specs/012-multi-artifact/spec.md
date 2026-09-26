# Feature Specification: STH multi-artifact V1

**Created**: 2026-08-25
**Status**: Implemented contract; dashboard PR automation awaits repository-owner approval
**Scope**: project-local artifacts only

## Artifact kinds

V1 supports `skill`, `agent`, `instruction`, `rule`, `command`, and `workflow`. Hooks, MCP servers, plugins, settings, permissions, memories, personal configuration, backend, billing, and Supabase changes are outside this version.

## Targets and native forms

| Target | Native project forms |
|---|---|
| Claude | `.claude/skills/*/SKILL.md`, `.claude/agents/**/*.md`, `CLAUDE.md`, `.claude/rules/**/*.md`, `.claude/commands/**/*.md`, `.claude/workflows/**/*.js` |
| Codex | `.agents/skills/*/SKILL.md`, `AGENTS.md`, `.codex/agents/*.toml`, `.codex/rules/*.rules` |
| Gemini | `.agents/skills`, `.gemini/skills`, `GEMINI.md`, `.gemini/agents/*.md`, `.gemini/commands/**/*.toml` |
| Antigravity | `.agents/skills`, `.agents/agents/<name>/agent.md`, `.agents/agents.md`, `.agents/rules/**/*.md`, `.agents/workflows/**/*.md` |
| Copilot | `.agents/skills`, `.github/skills`, `.github/copilot-instructions.md`, `.github/instructions/*.instructions.md`, `.github/agents/*.agent.md`, `.github/prompts/*.prompt.md` |
| Cursor | `.agents/skills`, `.cursor/skills`, `AGENTS.md`, `.cursor/rules/*.mdc`, `.cursor/agents/*.md` |

Agent Skills consumers coalesce portable skills under `.agents/skills`. Claude keeps its native `.claude/skills` hub. Codex, Cursor, and Copilot instructions contribute to one managed region in `AGENTS.md`; Gemini instructions contribute imports to a managed region in `GEMINI.md`.

## Compatibility contract

Every target and artifact-kind cell is emitted by `sth capabilities --json` with exactly one of:

- `exact`: no semantic loss and no confirmation.
- `lossy`: mapped fields and dropped fields are reported; interactive approval or `--allow exact,lossy` is required.
- `native-only`: accepted only when the discovered source belongs to the destination dialect.
- `unsupported`: rejected before download or filesystem mutation.

Codex `.rules`, Claude JavaScript workflows, and Antigravity Markdown workflows are not converted. Gemini commands using `!{...}` or `@{...}` are not converted. Cursor commands are unsupported until a documented, runtime-certified contract exists. Discovered content is parsed as data and is never executed.

## State and local protection

`.sth/sth-state-v3.json` records the stable `provider_id + resource_id` identity, source dialect/path/commit/hash, compatibility plans, transformer versions, outputs, consumers, hashes, and managed regions. Read-only commands aggregate v2 state in memory when v3 is absent. The first successful mutation creates v3 atomically while retaining v1 and v2 files.

Mutations run under `.sth/.sth-state.lock`. Missing or changed outputs fail the complete mutation. `--force` transfers an existing STH ownership conflict only and never bypasses local drift.

Status precedence is `modified-locally`, `outdated`, `partially-converted`, `not-installable`, `up-to-date`.

## CLI contract

- `list --type/--scope/--target/--status`
- `inspect <selector|path> [--target]`
- `validate [selector|path] [--target] [--strict]`
- `install <ref> [--kind] [--target] [--allow exact,lossy]`
- `status [--json]`
- `capabilities --json`

Machine-readable data is written to stdout. Prompts, progress, and human diagnostics are written to stderr where the command surface requires separation. JSON arrays and manifest fields use stable deterministic ordering.

## Runtime certification

A generated compatibility manifest is an implementation claim, not runtime proof. Runtime reports use only `OK`, `KO`, and `NON_EXECUTE`. A cell is `OK` only after the released binary writes the fixture and the named client loads and executes its sentinel on the stated minimum version. Static parsing, rendering, build success, and discovery are not execution proof.
