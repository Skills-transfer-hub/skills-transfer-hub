# Global consumer registry: TDD evidence

Date: 2026-10-25. Scope: `internal/writer/global_registry.go`, its behavioral tests, and the missing Cursor entries in the existing project capability registry.

## Design decision

The design document asks both for all six existing families and for runtime-certified surfaces only, while its own evidence table states that no global runtime has been certified. For V1, `Supported` identifies a documented distribution contract exercised by filesystem tests. `ValidationLevel` is `documented`; every selection emits `discovery-unverified`. `--all-agents` expands the eight documented surfaces, without claiming that an installed agent discovered or invoked a skill. Unknown surfaces fail before any destination planning. Runtime certification remains a separate, explicitly unverified layer.

The four writable roots have stable IDs: `agents`, `claude`, `antigravity-ide`, and `antigravity-cli`. Native and legacy read-only roots are used only to warn about coexistence. They are never returned as authorized write roots. Planning uses an explicit absolute home, never changes directories, never creates directories, and does not follow symbolic read-root paths.

## RED / GREEN

1. Selection aliases, eight concrete surfaces, four physical copies, no planning writes, and unrequested readers of the shared hub.
   - RED: `go test ./internal/writer -run '^TestGlobal' -count=1` failed because `ResolveGlobalConsumers`, `GlobalRoots`, and `PlanGlobalDestinations` did not exist.
   - GREEN: same command passed after the registry and planner implementation.
2. Unsafe names and relative home, preserved native coexistence with Gemini alias precedence, possible duplicate entries, and symbolic read roots.
   - RED: same command failed on accepted unsafe names, missing coexistence/duplicate warnings, and undisclosed symbolic roots.
   - GREEN: same command passed after adding portable-name validation using the existing validator and read-only coexistence inspection.
3. Cursor's existing project distribution paths appear in the capability inventory.
   - RED: `go test ./internal/writer -run 'TestArtifactCapabilitiesCoverSix|TestCursorCapabilities' -count=1` failed with 20 instead of 24 capabilities and Cursor absent from consumer IDs.
   - GREEN: `go test ./internal/writer -count=1` passed after registering Cursor's four existing projections. The local writer and its path selection were unchanged.

All filesystem fixtures use `t.TempDir()`. No agent runtime, paid model call, real-home installation, or OS certification was performed for this workstream. The RED failures above were expected missing-behavior failures, not environment failures.

## PR integration note

Before opening the PR, the feature was rebased onto `develop` at `29d9aaf`, which already contains the complete six-consumer × six-artifact local matrix and updated Cursor contracts. That upstream matrix and its tests are preserved byte for byte. The historical 20→24 RED/GREEN increment above remains recorded as development evidence, but those local capability edits are absent from the final PR diff. The global registry remains separate and its tests pass against the current validator.
