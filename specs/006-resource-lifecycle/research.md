# Research: Resource Lifecycle (spec 006)

All NEEDS CLARIFICATION items resolved below. Each entry follows the standard format:
Decision / Rationale / Alternatives Considered.

---

## R-001 · resolved_version semantic: commit SHA vs content SHA256

**Decision**: `resolved_version` in `lore-state-v2.json` stores the **Git commit SHA**
of the provider's `catalog_ref` branch at the time the resource was installed or last
updated — not the SHA256 of the downloaded content.

**Rationale**: The spec states "outdated is determined by comparing `resolved_version`
against the latest available commit on the provider's `catalog_ref` branch." Storing the
commit SHA makes this comparison a single cheap equality check. The content integrity role
is already covered by `managed_files[].sha256` per record.

**Impact on existing code**: `update.go` currently writes
`v2rec.ResolvedVersion = fs.ComputeSHA256(content)`. This must be changed to fetch and
store the commit SHA returned by the GitHub API. The content SHA256 stays in
`managed_files[].sha256`.

**Alternatives considered**:
- **Content SHA256 as version**: Would require re-downloading every resource to detect
  outdated status — O(N) downloads per `status` run. Rejected: violates SC-001
  (< 3 s for 20 resources) and wastes bandwidth.
- **ETag / Last-Modified headers**: Not stable across CDN nodes for raw GitHub content.
  Rejected: unreliable.

---

## R-002 · Detecting "outdated" status without downloading all content

**Decision**: For `loreskills status`, issue **one GitHub API call per provider** to
retrieve the current HEAD commit SHA of `catalog_ref`. Compare it to each installed
resource's `resolved_version` within that provider. A mismatch → `outdated`.

**Rationale**: Resources from the same provider share the same `catalog_ref` branch, so
one API round-trip is sufficient per provider regardless of how many resources are
installed. This keeps status well under 3 s for 20 resources across 2 providers
(SC-001).

**HTTP method to add**: `GetLatestCommitSHA(ctx context.Context, providerID string,
profile ProjectConfigV2) (string, error)` in `internal/http/client.go`. Uses
`GET /repos/{owner}/{repo}/commits/{ref}` (GitHub REST) and returns `.sha`.

**Alternatives considered**:
- **Download and hash each resource**: Accurate but N × download. Rejected for
  performance.
- **Compare file tree hash**: GitHub Trees API returns SHA per blob — would let us check
  file-level changes but adds complexity and extra API calls. Rejected: overkill for P1.

---

## R-003 · Detecting "modified-locally" status

**Decision**: For each `ManagedFile` entry in a resource record, recompute the SHA256 of
the file at `Path` on disk (using `crypto/sha256` from stdlib) and compare to the stored
`sha256`. Any mismatch → resource is `modified-locally`; collect the differing paths.

**Rationale**: SHA256 baselines are already stored by spec 005 in `managed_files[].sha256`.
No additional state is required.

**Edge case — unknown baseline**: If a resource record exists but `managed_files` is
empty or all entries have an empty `sha256`, the status is `unknown-baseline`. The tool
suggests `loreskills install --force <name>` to re-establish integrity tracking (FR-004).

**Alternatives considered**:
- **mtime-based staleness**: Unreliable; mtime can be reset by VCS operations or file
  copies. Rejected.

---

## R-004 · gitignore-style pattern matching — stdlib-only implementation

**Decision**: Implement a custom, minimal gitignore-style matcher in
`internal/fs/ignore.go` using only `path.Match` (from `path` package) plus manual
segment-splitting for `**` glob support.

**Algorithm**:
1. Read `.loreskillsignore` line-by-line with `bufio.Scanner`.
2. Skip blank lines and lines starting with `#`.
3. For each pattern:
   - Normalise to forward-slash form (`filepath.ToSlash`).
   - If the pattern contains `**`, split the input path and the pattern into segments
     and evaluate with a recursive segment matcher.
   - Otherwise delegate to `path.Match(pattern, normalizedPath)`.
4. A file is excluded if any pattern matches its **bundle-relative path** (not the
   final install path per spec assumption).

**Limitations accepted**: Leading `!` negation patterns (gitignore re-include) are out
of scope; the spec does not require them. Double-star `**` handling covers the most
common use cases (`**/*.test.md`).

**Alternatives considered**:
- **`filepath.Match` only**: Does not support `**`. Rejected: breaks `**/*.md` patterns
  from spec examples.
- **Third-party gitignore library (e.g., `go-gitignore`)**: Violates constitution
  Principle I. Rejected.

---

## R-005 · Dry-run threading through install/update pipelines

**Decision**: Add a `DryRun bool` parameter to the internal install/update helpers in
`internal/fs/` and a `--dry-run` CLI flag parsed in `cmd/root.go`. When `DryRun == true`:
- All `os.WriteFile`, `os.MkdirAll`, and `os.Remove` calls are skipped.
- Instead, a `[]DryRunChange` slice is accumulated and returned to the caller.
- The state file is never touched.

**DryRunChange type** (defined in `internal/fs/schema.go`):
```go
type DryRunChange struct {
    Action    string `json:"action"`              // "add" | "modify" | "delete"
    Path      string `json:"path"`
    OldSHA256 string `json:"old_sha256,omitempty"`
    NewSHA256 string `json:"new_sha256,omitempty"`
}
```

**Text output format**: `[+] path/to/file`, `[~] path/to/file`, `[-] path/to/file`.
**JSON output** (`--dry-run --json`): JSON array of `DryRunChange` objects.

**Network behaviour**: Dry-run still performs the network fetch (resolve catalog,
download content) because determining what *would* change requires the upstream content.
If the network is unavailable, an error is returned with a non-zero exit code (FR-013,
spec scenario US3-5).

**Alternatives considered**:
- **Separate dry-run function per command**: High duplication. Rejected.
- **Middleware / decorator pattern**: Over-engineered for a 2-command scope. Rejected.

---

## R-006 · File ownership conflict detection

**Decision**: Before writing any file during `install`, call `BuildOwnerIndex()` to
produce a `map[string]string` (file path → owning resource name) from
`lore-state-v2.json`. If any file to be installed already appears in the index under a
different resource name:
- Without `--force`: halt with `OwnershipConflict` error listing the contested path and
  both resource names (FR-022, FR-023).
- With `--force`: overwrite the file, update the index, transfer ownership in state,
  emit a warning (FR-024).

During `loreskills update` the conflict guard applies only to newly introduced files
(files not previously owned by the resource being updated). Existing owned files are
updated normally (US6-3: skip conflicting resource, report it, continue with others).

**Alternatives considered**:
- **Fail-all-or-nothing on any conflict**: Over-restrictive for update scenarios.
  Rejected.

---

## R-007 · Interactive confirmation for `loreskills remove`

**Decision**: Implement the confirmation prompt by reading from `a.stdin` using
`bufio.NewReader`. The `--yes` flag (a global flag available on all interactive
commands, spec assumption) bypasses the prompt. When stdin is not a terminal (e.g.,
piped in CI), the default behaviour is to require `--yes` (fail safe: no silent
destructive action).

**Terminal detection**: Use `os.Stdin.Stat()` → `(stat.Mode() & os.ModeCharDevice) != 0`
to determine if stdin is interactive. If not interactive and `--yes` is not set, the
command returns an error rather than hanging.

**Alternatives considered**:
- **Always require `--yes` flag**: Too disruptive for interactive use. Rejected.
- **Third-party prompt library**: Violates constitution Principle I. Rejected.

---

## R-008 · Partial deletion failure handling in `loreskills remove`

**Decision**: Attempt to delete all managed files in a loop. Collect individual file
deletion errors. If any deletion fails:
- Report each failed file path in stderr.
- Do **not** remove the state entry (FR-011).
- Return a non-zero exit code.

If a file is missing from disk (externally deleted), log a warning and continue — the
missing file is treated as already deleted (FR-009). The state entry is still cleaned up
in this case.

**Interruption recovery (SIGINT)**: Partial state is intentional. The next
`loreskills remove` run re-reads the (still-present) state entry and attempts deletion
of the remaining files, converging toward a clean state.

---

## R-009 · Pinned version resolution error

**Decision**: If `pinned_version` is set and cannot be resolved by the provider (e.g.,
GitHub returns 404 for that ref), the tool emits:
`"Pinned version <version> not found for <name>"` and exits non-zero (FR-018).
Unresolvable pins are surfaced by `status` and `update`; `install` is unaffected
(install always uses the specified ref).

**Alternatives considered**:
- **Silent skip on 404**: Dangerous — user would think the resource is up-to-date.
  Rejected.

---

## R-010 · `.loreskillsignore` — all-files-excluded edge case

**Decision**: If all files in a bundle are excluded by `.loreskillsignore` patterns,
the install proceeds with zero files written. `managed_files` is empty. A warning is
printed: `"warning: no files installed — all excluded by .loreskillsignore"` (edge case
from spec). The state entry IS written so the resource appears in `status` and `remove`.

**Rationale**: The user made an intentional choice. Recording the (empty) state entry
keeps the lifecycle consistent and allows `remove` to clean up the state cleanly.
