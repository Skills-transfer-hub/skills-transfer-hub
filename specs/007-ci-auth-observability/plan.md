# Implementation Plan: CI, Auth & Observability

**Branch**: `007-ci-auth-observability` | **Date**: 2026-05-09 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `specs/007-ci-auth-observability/spec.md`

## Summary

Spec 007 closes the four "production tool" gaps that block daily-driver use and CI:
(1) credential model with explicit precedence (`STH_TOKEN` → OIDC → prompt → fail);
(2) auth recovery on `401`/`403` in TTY without restarting the command;
(3) `--verbose`/`--debug` for zero-friction diagnostics;
(4) `sth doctor` checklist with remediation per failed item;
(5) `--fail-on-changes` for `sth update` as a CI gate;
(6) `--json` for `list`/`status`/`version` so STH composes into automation pipelines;
(7) `sth completion` for bash/zsh/powershell.

All logic is pure Go stdlib, no new dependencies.

## Technical Context

**Language/Version**: Go 1.22
**Primary Dependencies**: stdlib only — `os`, `net/http`, `encoding/json`, `bufio`, `strings`, `errors`, `path/filepath` (existing `fsnotify`, `go-keyring`, `x/term` are unchanged — no new deps for spec 007)
**Storage**: same as spec 005/006 — `.sth/project.json` v2, `.sth/sth-state-v2.json`
**Testing**: `go test ./...`, stdlib `testing` package only, `httptest` for mock servers
**Target Platform**: Windows, macOS, Linux — `CGO_ENABLED=0`, pure Go
**Constraints**: zero new external deps; OIDC via `net/http` only; TTY detection via `os.ModeCharDevice`; tests sériels (no `t.Parallel`) because retry path uses `os.Setenv("STH_TOKEN", ...)`

## Constitution Check

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Zéro Dépendance Externe | ✅ PASS | No new modules. OIDC via `net/http`. Prompt input via `bufio.NewReader` (no `x/term` for the new flow). |
| II. Agnostique — Aucune Valeur Codée en Dur | ✅ PASS | OIDC URL/token from `ACTIONS_ID_TOKEN_REQUEST_URL`/`_TOKEN` env vars. Audience `sth-cli` is a documented constant, not a service URL. Doctor reachability honors `STH_*_BASE_URL` overrides. |
| III. Cross-Platform — CGO Désactivé | ✅ PASS | Pure Go; TTY detection uses `os.ModeCharDevice` portably. |
| IV. Découpage Modulaire Strict | ✅ PASS | Auth recovery wrapper lives in `cmd/recoverable.go` (TTY/prompt awareness stays out of the network layer). New `internal/log/` is a leaf utility with no policy. |

## Architecture

### New packages and files

```text
internal/log/
  log.go               # Verbose interface { HTTP, Path, Decision } with no-op default
  log_test.go

internal/http/
  auth.go              # AuthError + IsAuthError + resolveCredentialHeadersWithPrompter (full chain)
  oidc.go              # exchangeGitHubActionsOIDC(ctx, audience) — JWT exchange via stdlib
  auth_test.go
  oidc_test.go

cmd/
  doctor.go            # 4 independent checks: project config / state / credential / provider HEAD
  completion.go        # bash/zsh/powershell renderers from commandCatalog (filter Hidden=true)
  recoverable.go       # withAuthRecovery(ctx, fn) — 401/403 + TTY → prompt → os.Setenv → retry once
  jsonerror.go         # emitJSONError(w, err, code) — shared by list/status/version
  doctor_test.go
  completion_test.go
  recoverable_test.go
  jsonoutput_test.go
  fail_on_changes_test.go
  verbose_test.go
```

### Modified files

| File | Change |
|------|--------|
| `cmd/root.go` | `app.verbose` field, global `--verbose`/`--debug` stripping via `applyGlobalFlags`, `commandSpec.flags`/`hidden`, doctor + completion routes, hidden-filter in `helpText`/`usageExamples`, `commandCatalog` updated for new commands |
| `cmd/list.go` | `--json` returns installed-state schema (FR-020), text mode unchanged (catalog), verbose Path emit |
| `cmd/status.go` | `runStatusInner` separation so `emitJSONError` fires when `--json` is set, verbose Path emit |
| `cmd/update.go` | `--fail-on-changes` (FR-017/018/019) using `fs.ComputeAllStatuses` |
| `cmd/version.go` | `--json` returns `{"version", "commit"}` |
| `cmd/install.go` | `withAuthRecovery` wraps `ListCatalog` |
| `cmd/migrations.go` | `verbose.Path` emit at `prepareStateAccess` |
| `cmd/init.go` | Completion hint at end of wizard (FR-026) |
| `internal/http/client.go` | `verbose` field + `SetVerbose` + `emitHTTP`; switch L372 returns `*AuthError` for 401/403; `resolveCredentialHeaders` delegates to the new chain |
| `internal/http/cloud.go` | `c.emitHTTP` after the 2 cloud `Do` sites |
| `README.md` | Documents new commands, flags, credential model, OIDC precondition |

### Credential resolution chain

```
profile.APIToken → sth-cloud-keychain → STH_TOKEN → OIDC (sth-cli aud) → provider env → git-credential → prompter (TTY only) → AuthError
```

OIDC is inserted between `STH_TOKEN` and provider-specific env vars so CI runners with `id-token: write` can authenticate without configuring a PAT, **provided** the target service trusts the `sth-cli` audience claim. `api.github.com` does not — for Git providers in CI, set `STH_TOKEN`.

## Phases

| # | Phase | Stories |
|---|-------|---------|
| 0 | Branch dédiée | — |
| 1 | Setup : parseFlags + commandSpec.flags/hidden + route stubs | — |
| 2 | Foundational : `internal/log` + `internal/http/auth.go` + `AuthError` typed | — |
| 3 | US2 — Credential model + OIDC | P1 |
| 4 | US1 — Auth recovery (`withAuthRecovery` + wiring) | P1 |
| 5 | US3 — `--verbose` / `--debug` | P1 |
| 6 | US4 — `sth doctor` | P1 |
| 7 | US5 — `--fail-on-changes` | P2 |
| 8 | US6 — `--json` for `version` + `list` (status existant) + `emitJSONError` | P2 |
| 9 | US7 — `sth completion` | P2 |
| 10 | Polish — README + smoke OIDC | — |

## Verification

- `go vet ./...` clean
- `go test ./...` clean
- `go test ./... -run Verbose|Doctor|AuthRecovery|Completion|JSONOutput|FailOnChanges|OIDC` clean
- README updated
- No new dep in `go.mod` (verify `go mod tidy` no-op)
