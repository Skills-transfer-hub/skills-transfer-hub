# Implementation Plan: STH multi-artifact V1

1. Define the versioned artifact model, diagnostics, compatibility planner, target registry, and schemas.
2. Keep the existing writers and fan-out transaction, expanding their paths and capability declarations.
3. Scan the six native project dialects without following symlinks or executing content.
4. Centralize Agent Skills validation and retain raw source bytes and unmapped native fields.
5. Aggregate legacy state for reads and create central v3 only during a successful mutation under the project lock.
6. Coalesce shared skill hubs and instruction regions inside the existing rollback transaction.
7. Require explicit lossy authorization and reject native-only or unsupported cross-dialect plans before writing.
8. Add filtered list, inspect, validate, status, install selection flags, and deterministic capabilities JSON.
9. Publish the capabilities manifest and checksum as release assets. The permission-bearing release dispatch and dashboard PR receiver await explicit repository-owner approval.
10. Gate release claims on runtime evidence reported as `OK`, `KO`, or `NON_EXECUTE`.

## Verification

- `go build ./...`
- `go test ./internal/scanner/... ./internal/writer/... ./internal/fs/... ./cmd/...`
- JSON Schema validation for capabilities and v3 state
- Native scanner and byte-preservation fixtures
- Exact/lossy/native-only/unsupported planner tests
- v2-to-v3 read aggregation, atomic mutation, drift, rollback, and concurrent lock tests
- TTY and non-TTY install authorization tests
- Runtime certification with the built release candidate and installed client versions
