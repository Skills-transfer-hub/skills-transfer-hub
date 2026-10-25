# Independent global-installation review

Date: 2026-10-25. Reviewer workstream owns `cmd/global_acceptance_test.go`; production fixes remain with the CLI and filesystem owners.

## Behavioral coverage

- An initialized current project, including `.sth/last-operation.json`, retains identical bytes, permissions, and file modification times throughout install, list, status, update, and remove.
- Invalid selections and syntax produce a versioned global JSON envelope and exit code 2 without persistent files.
- `--path skills --skill '*' --all-agents` selects six canonical skill folders, excludes OpenClaw copies and plugin manifests, and records eight consumers in four physical placements per skill.
- A third-party conflict in a multi-skill invocation publishes no partial installation, even with `--force --yes`.
- Update aggregates independent successes and failures, preserves local edits and registered consumers, repairs missing files, removes tracked files deleted upstream, and preserves foreign additions.
- All lifecycle dry runs preserve files, modification times, state, configuration, and directories, including forced plans over local edits.
- Source installations distinguish existing identities at their recorded revisions from newly introduced skills on the followed branch.
- The public result distinguishes requested consumers from other possible readers of shared roots.
- Corrupt or unknown state and forged removal paths cannot authorize writes.
- Recovery must run before new network work, while edits made after an interruption must block recovery and preserve the journal.

## Executed review findings

1. **Configuration root normalization on macOS.** Initial acceptance execution failed because `/var/folders/...` configuration paths encountered the system `/var` symlink while only the home was canonicalized. This was a portability defect masking the intended behavior assertions. The filesystem owner added a focused RED/GREEN test and canonicalized the independent configuration base. This initial failure is not counted as evidence for unrelated CLI behavior.
2. **Partial updates blocked by another skill's local edits.** Acceptance RED showed an unmodified `review` installation remaining at revision 1 because modified `audit` files were checked during every transaction. The filesystem owner added a dedicated regression and restricted mutation checks to affected installations while retaining global path and ownership validation. Acceptance turned GREEN.
3. **New source identity hidden by an earlier installed revision.** Acceptance RED showed `newcomer` missing after installing another skill from the same repository. The CLI owner separated current-branch discovery from per-identity revision preservation. Acceptance turned GREEN.
4. **Missing possible-reader output.** Acceptance RED observed `destinations: null`. The CLI owner exposed physical destinations with requested consumers and possible readers. Acceptance turned GREEN.
5. **Explicit selection across several recorded revisions.** RED: two installed skills from different revisions could not both receive a new consumer after the older skill disappeared upstream. The CLI owner changed resolution to preserve each existing identity independently. Acceptance turned GREEN.
6. **Recovery delayed until after downloads.** RED: an offline update left interrupted file writes in place because the source request failed before recovery. The CLI owner moved recovery ahead of state loading and network work for real mutations. Acceptance turned GREEN, including preservation of edits made after interruption.
7. **Windows permission semantics.** Static review of Go's local Windows implementation (`os/types_windows.go`, `fileStat.mode`) found that Windows reports writable regular files as mode `0666`, while Git manifests preserve `0644` or `0755`. Exact mode comparisons would incorrectly treat fresh installations as local modifications and block later operations. The filesystem owner introduced a platform-aware comparison that preserves POSIX executable checks and compares Windows writeability. Its regression test passes on the development host, as does Windows cross-compilation. Actual Windows filesystem execution remains unverified.

## Commands and interpretation

- `go test ./cmd -run '^TestGlobalAcceptance' -count=1`: initially failed on incomplete CLI routing, missing destinations, new-identity discovery, and the macOS path issue described above. After the initial fixes, the complete then-existing acceptance set passed.
- `go test ./cmd -run '^TestGlobalAcceptance(AddAgentToSeveral|RecoversInterrupted|RecoveryPreserves|UpdateRepairs)' -count=1`: recorded the two final review regressions before their production fixes. Recovery divergence preservation and missing/upstream-deleted file behavior already passed; these are regression checks, not newly claimed RED/GREEN increments.
- A temporary compile failure while filesystem recovery symbols were being introduced was an integration-in-progress result, not a behavior RED.
- Final acceptance rerun after CLI recovery and multi-revision fixes: `go test ./cmd -run '^TestGlobalAcceptance' -count=1` passed, 7.340s.
- `GOOS=windows GOARCH=amd64 go build ./...` passed.
- `GOOS=linux GOARCH=amd64 go build ./...` passed.
- After the permission fix: `go test ./internal/fs -run '^TestGlobalPermissionComparisonUsesPlatformCapabilities$' -count=1`, the complete acceptance set (5.415s), and Windows cross-compilation all passed.

No unresolved functional finding remained in this review at that checkpoint. The coordinator owns the final whole-repository checks and the separate runtime evidence report.

All fixtures are local HTTP servers and temporary homes/configuration/project directories. No downloaded script is run. The acceptance executions used macOS. The Linux and Windows checks above are cross-compilation, not filesystem execution. These tests do not certify actual agent discovery, invocation, deduplication, or untested operating systems.
