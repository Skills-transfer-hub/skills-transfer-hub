# Global filesystem implementation: TDD evidence

Date: 2026-10-25. Execution host: macOS. Every filesystem fixture uses an explicit temporary home and a temporary user configuration base. No real agent directory is used.

## Recorded RED → GREEN increments

| Behavior | Command | RED observed before implementation | GREEN |
| --- | --- | --- | --- |
| Explicit user context, absent state without writes, corrupt/unknown schema, restricted roots | `go test ./internal/fs -run '^TestGlobal(State\|Context)' -count=1` | `GlobalContext`, `NewGlobalContext`, `ReadGlobalState` undefined | Passed after introducing the context and schema reader |
| Install, dry-run, idempotence, source ownership, third-party protection, force backups, remove, local status | `go test ./internal/fs -run '^TestGlobal(Install\|Mutation\|Inspect)' -count=1` | `ApplyGlobalChange` and status API undefined | Passed with manifest-based mutations and ownership checks |
| Failure at second destination restores first destination and state | `go test ./internal/fs -run '^TestGlobalMutationRollsBack' -count=1` | Mutation API and failure injection point undefined | Passed; existing bytes and revision restored |
| Independent macOS configuration base containing a trusted `/var` alias | `go test ./internal/fs -run '^TestGlobalContextCanonicalizes' -count=1` | `symlink is not allowed in global path` | Passed; only the explicitly selected configuration base is canonicalized, not the derived state/journal paths |
| Durable interrupted transaction, recovery before the next mutation, post-interruption edits, tampered journal | `go test ./internal/fs -run '^TestGlobal(Recovery\|MutationRecovers\|MutationIgnores)' -count=1` | `RecoverGlobalTransactions` undefined | Passed with an actual subprocess exiting with code 71 after files were replaced and before state publication |
| Independent per-skill update/remove despite another installation's local changes | `go test ./cmd -run '^TestGlobalAcceptanceUpdateAggregatesFailuresAndKeepsConsumers$' -count=1` | Independently observed by the integration reviewer: the unrelated skill stayed at its old SHA | Passed after limiting content checks to changed installations or explicit writes while validating all recorded paths |
| Case collisions in agent root components; writable destinations checked before the first write | `go test ./internal/fs -run '^TestGlobalMutation(RejectsRootCase\|ChecksDestinations)' -count=1` | Case collision accepted; unwritable second root failed only during the transaction | Passed with root-component collision checks and read-only preflight |
| Private state/backups excluded from agent discovery roots | `go test ./internal/fs -run '^TestGlobalContextKeepsPrivate' -count=1` | Private state below `.agents/skills` accepted | Passed |
| Local edit arriving after preflight | `go test ./internal/fs -run '^TestGlobalMutationPreservesEdit' -count=1` | Concurrent local edit overwritten | Passed; the edit and recovery journal remain intact |
| Unicode normalization aliases, reserved superscript Windows device names, invalid UTF-8 paths, trailing journal JSON | `go test ./internal/fs -run '^TestGlobal(MutationRejectsUnicode\|RecoveryRejectsTrailing)' -count=1` | Unsafe paths and trailing JSON accepted | Passed with normalized collision keys and strict decoding |
| Relevant Windows permission comparison | `go test ./internal/fs -run '^TestGlobalPermissionComparison' -count=1` | Platform comparison helper undefined | Passed; Windows compares the read-only bit, Unix compares complete permission bits |
| External state edit immediately before publication | `go test ./internal/fs -run '^TestGlobalMutationPreservesStateEdited' -count=1` | Concurrent state edit overwritten | Passed; publication is blocked and the edit/journal are preserved |
| Invalid UTF-8 in stored state | `go test ./internal/fs -run '^TestGlobalStateRejectsInvalidDocuments' -count=1` | Invalid JSON string bytes silently decoded to replacement characters | Passed with explicit UTF-8 validation |
| Foreign nested directory under an otherwise managed skill | `go test ./internal/fs -run '^TestGlobalMutationDoesNotAdopt' -count=1` | An upstream addition implicitly adopted a pre-existing foreign directory even with `--force` | Passed; directory ownership derives from previous placements and file parents |

One early writable-directory test setup created both parent and child with mode `0555`, failing before exercising the implementation. The fixture was corrected to create the directory first and then make it read-only. That setup failure is not counted as a TDD RED.

Additional verification tests check real cross-process serialization, binary bytes, executable bits, provenance storage, missing files and force-backup contents. These supplemental checks are not presented as new RED/GREEN increments.

## Final checks for this workstream

- `go test ./internal/fs/... -count=1`: passed.
- `go test ./cmd -run '^TestGlobal' -count=1`: passed.
- `go test -race ./internal/fs -run '^TestGlobal' -count=1`: passed.
- `GOOS=linux GOARCH=amd64 go test ./internal/fs -c -o /tmp/sth-global-fs-linux.test`: compiled.
- `GOOS=windows GOARCH=amd64 go test ./internal/fs -c -o /tmp/sth-global-fs-windows.test.exe`: compiled.

Linux and Windows binaries were not executed. Filesystem execution evidence is macOS only. The platform lock implementations cover the three operating systems shipped by `.goreleaser.yaml`. Windows preserves the requested Git executable mode in the manifest but its filesystem status checks use only the read-only bit exposed by Go. File contents are flushed before rename; directory metadata flush is performed on macOS/Linux and is not available through the corresponding Windows directory helper. No power-loss or runtime-agent certification is claimed.

## Implementation decisions

- Existing `UserSettingsPath` semantics and `ComputeSHA256` are reused. No project configuration, migration, working-directory switch, or local last-operation record is involved.
- Global recovery requires stricter ownership and multi-root validation than the project helpers provide. The global journal embeds before/after file snapshots and both states, so recovery paths are re-derived from authorized root IDs instead of trusted absolute paths.
- Atomic file replacement flushes the writable temporary handle before renaming. The existing local `atomicWriteFile` helper lacks this durability step; its behavior was preserved rather than changing the local lifecycle.
- The lock is an OS advisory file lock, automatically released when a process exits. After acquiring it, mutations recover pending journals, reread state, rebuild comparisons and verify each destination again immediately before replacement.
- A dry-run never acquires a persistent lock or performs recovery. A pending recovery is reported as an error requiring a real mutation first.
- Recovery validates all divergences before restoring the first file and checks each file again during restoration. A conflicting edit blocks recovery and keeps its journal and snapshots.
