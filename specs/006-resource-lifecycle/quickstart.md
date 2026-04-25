# Quickstart: Resource Lifecycle (spec 006)

Developer guide for implementing spec 006. Read this before touching any code.

---

## Prerequisites

- Spec 005 fully implemented: `sth-state-v2.json`, `InstalledResourceRecordV2`,
  `ManagedFile` with SHA256, `ProjectConfigV2` with `pinned_version` in `ResourceEntry`.
- `go test ./...` passes on the current `006-cli-product-roadmap` branch.
- `go vet ./...` passes with zero warnings.

---

## Implementation Order

Implement in this exact order to minimise merge conflicts and ensure each increment
is independently testable:

```
Step 1  internal/fs/ignore.go + ignore_test.go          (no dependencies on other steps)
Step 2  internal/http/client.go — GetLatestCommitSHA()  (needed by status)
Step 3  internal/fs/status.go + status_test.go          (needs Step 2 result injected)
Step 4  internal/fs/fs.go — BuildOwnerIndex(),          (needed by install conflict check)
                             ComputeFileSHA256()
Step 5  cmd/status.go + status_test.go                  (needs Steps 2–4)
Step 6  cmd/remove.go + remove_test.go                  (needs Step 4)
Step 7  cmd/install.go — --dry-run, --force, ownership  (needs Steps 1, 4)
Step 8  cmd/update.go  — --dry-run, pinning             (needs Steps 1, 2)
Step 9  cmd/root.go    — wire all new cases and flags
Step 10 Fix resolved_version: store commit SHA          (touches install + update)
```

---

## Step 1 — `internal/fs/ignore.go`

Create `ignore.go` with two exported functions:

```go
package fs

import (
    "bufio"
    "os"
    "path"
    "path/filepath"
    "strings"
)

const ignoreFileName = ".sthignore"

// ParseIgnoreFile reads .sthignore from projectRoot.
// Returns nil patterns (not an error) if the file does not exist.
func ParseIgnoreFile(projectRoot string) ([]string, error) {
    p := filepath.Join(projectRoot, ignoreFileName)
    f, err := os.Open(p)
    if os.IsNotExist(err) {
        return nil, nil
    }
    if err != nil {
        return nil, err
    }
    defer f.Close()

    var patterns []string
    sc := bufio.NewScanner(f)
    for sc.Scan() {
        line := strings.TrimSpace(sc.Text())
        if line == "" || strings.HasPrefix(line, "#") {
            continue
        }
        patterns = append(patterns, filepath.ToSlash(line))
    }
    return patterns, sc.Err()
}

// MatchIgnore reports whether bundleRelPath matches any pattern.
// bundleRelPath must use forward slashes.
func MatchIgnore(patterns []string, bundleRelPath string) bool {
    normalized := filepath.ToSlash(bundleRelPath)
    for _, pattern := range patterns {
        if matchGlob(pattern, normalized) {
            return true
        }
    }
    return false
}

// matchGlob implements ** support on top of path.Match.
func matchGlob(pattern, name string) bool {
    if !strings.Contains(pattern, "**") {
        ok, _ := path.Match(pattern, name)
        return ok
    }
    // Split on ** and try all combinations of path segments
    parts := strings.SplitN(pattern, "**", 2)
    prefix, suffix := parts[0], parts[1]
    suffix = strings.TrimPrefix(suffix, "/")
    prefix = strings.TrimSuffix(prefix, "/")
    // Match prefix against the start of the path
    if prefix != "" {
        ok, _ := path.Match(prefix+"/*", strings.SplitN(name, "/", 2)[0]+"/x")
        if !ok {
            // Simple prefix check
            if !strings.HasPrefix(name+"/", prefix+"/") {
                return false
            }
        }
    }
    // Suffix must match the end of some sub-path
    segments := strings.Split(name, "/")
    for i := range segments {
        candidate := strings.Join(segments[i:], "/")
        if suffix == "" {
            return true
        }
        ok, _ := path.Match(suffix, candidate)
        if ok {
            return true
        }
    }
    return false
}
```

> **Test coverage required** (ignore_test.go):
> - `*.test.md` matches `guide.test.md`, does not match `guide.md`
> - `**/*.test.md` matches `sub/dir/guide.test.md`
> - `#comment` lines are skipped
> - blank lines are skipped
> - non-existent `.sthignore` returns nil, nil

---

## Step 2 — `internal/http/client.go`: `GetLatestCommitSHA`

Add to `Client`:

```go
// GetLatestCommitSHA returns the HEAD commit SHA for the given provider's catalog_ref.
// Returns an error (with non-nil err) if the provider is not found in config or the
// network call fails.
func (c *Client) GetLatestCommitSHA(ctx context.Context, config fs.ProjectConfigV2, providerID string) (string, error)
```

Implementation notes:
- Find the `ProviderEntry` with `ID == providerID` from `config.Providers`.
- Call `GET https://api.github.com/repos/{owner}/{repo}/commits/{ref}` where
  `{ref}` is `provider.CatalogRef` and `{owner}/{repo}` is parsed from
  `provider.RepositoryID`.
- Parse the JSON response and return `.sha` (top-level string field).
- Honour the existing credential-resolution logic already in `client.go`.

---

## Step 3 — `internal/fs/status.go`

```go
// ComputeAllStatuses computes lifecycle status for every entry in state.
// latestSHAs maps providerID → current HEAD SHA (from GetLatestCommitSHA calls).
// projectRoot is used to resolve managed file paths for SHA256 comparison.
func ComputeAllStatuses(
    state      InstalledResourceStateV2,
    config     ProjectConfigV2,
    latestSHAs map[string]string,
    projectRoot string,
) []StatusResult
```

Evaluation order per entry (first match wins):
1. `unknown-baseline`: `managed_files` empty or all `sha256 == ""`
2. `modified-locally`: any `ComputeFileSHA256(path) != stored_sha256` (missing file also counts)
3. `outdated`: `entry.ResolvedVersion != latestSHAs[entry.ProviderID]`
4. `up-to-date`: default

For pinned resources: look up `entry.ResourceName` + `entry.ProviderID` in
`config.Resources` to find `PinnedVersion`. Set `StatusResult.Pinned = true` and
`StatusResult.PinnedVersion` accordingly.

---

## Step 4 — `internal/fs/fs.go` additions

```go
// BuildOwnerIndex builds a map of absolute file path → resource_name from state.
func BuildOwnerIndex(state InstalledResourceStateV2) map[string]string {
    idx := make(map[string]string, len(state.Entries)*8)
    for _, entry := range state.Entries {
        for _, mf := range entry.ManagedFiles {
            idx[mf.Path] = entry.ResourceName
        }
    }
    return idx
}

// ComputeFileSHA256 returns the hex SHA256 of the file at path.
func ComputeFileSHA256(filePath string) (string, error) {
    data, err := os.ReadFile(filePath)
    if err != nil {
        return "", err
    }
    sum := sha256.Sum256(data)
    return hex.EncodeToString(sum[:]), nil
}
```

---

## Step 5 — `cmd/status.go`

```go
func (a *app) runStatus(jsonOut bool) error {
    // 1. Read project config (ProjectConfigV2)
    // 2. Read sth-state-v2.json
    // 3. For each unique providerID in state, call client.GetLatestCommitSHA
    //    (one call per provider, cache in map[string]string)
    // 4. Call fs.ComputeAllStatuses(...)
    // 5. If len(results) == 0: print "No resources installed." and return nil
    // 6. If jsonOut: marshal []StatusResult as JSON
    // 7. Else: format text table
}
```

---

## Step 6 — `cmd/remove.go`

```go
func (a *app) runRemove(resourceName string, yesAll bool) error {
    // 1. Read sth-state-v2.json
    // 2. Find entry; if not found: return error (exit 1)
    // 3. For each managed file: ComputeFileSHA256 vs stored sha256 → collect modified
    // 4. If modified and !yesAll:
    //      Print warning + modified file list
    //      Detect interactive stdin; if not interactive and !yesAll: return error
    //      Read y/N from a.stdin
    //      If not "y": return exit-3 error
    // 5. Delete each managed file; collect failures; warn on missing
    // 6. If any deletion failed: report failures, return error (state entry kept)
    // 7. Remove state entry; write updated state; print success
}
```

---

## Step 7 — `cmd/install.go` modifications

Add `--dry-run` and `--force` flag parsing. Thread `dryRun bool` and `force bool`
into the internal helpers. Before writing any file:
1. Check `BuildOwnerIndex` — if conflict and `!force`: return `OwnershipConflict` error.
2. Check `MatchIgnore(patterns, bundleRelPath)` — if matched: skip file.
3. If `dryRun`: append to `[]DryRunChange`, skip write.

If `dryRun`: after collecting all changes, render text or JSON output.
Do NOT write state file.

---

## Step 8 — `cmd/update.go` modifications

Add `--dry-run` flag. Before updating each resource:
1. Look up `ResourceEntry.PinnedVersion` for this resource in `project.json`.
   - If pinned: print `[pinned at <ver>]` + optional newer-version notice; `continue`.
   - If pinned version unresolvable: return error.
2. Apply `.sthignore` patterns to each file before writing.
3. If `dryRun`: collect `DryRunChange` per file; skip writes and state mutation.

Fix `resolved_version` storage: after downloading, get the commit SHA (from
`GetLatestCommitSHA` result already fetched for the provider) and store that instead of
`ComputeSHA256(content)`.

---

## Step 9 — `cmd/root.go` wiring

```go
// In the switch:
case "status":
    flags := parseStatusFlags(args[1:])
    err = a.runStatus(flags.jsonOut)

case "remove":
    if len(posArgs) != 1 {
        err = fmt.Errorf("usage: sth remove <resource-name>")
        break
    }
    err = a.runRemove(posArgs[0], globalFlags.yes)
```

Strip `--yes` globally before dispatching. Update `commandCatalog` in `root.go` with
entries for `status` and `remove`.

---

## Testing Checklist

Every function/command must have a `_test.go` counterpart in the same package.
Use table-driven tests with `t.Run(name, ...)`. No external test libraries.

| Test File | Key Scenarios |
|-----------|---------------|
| `internal/fs/ignore_test.go` | pattern match, no-match, comment lines, missing file, `**` glob |
| `internal/fs/status_test.go` | each status derivation rule, pinned detection, all-empty sha256 |
| `cmd/status_test.go` | text output format, JSON output, no-resources message, offline error |
| `cmd/remove_test.go` | clean remove, modified-file warning+confirm, `--yes` bypass, not-found, partial failure, missing file |
| `cmd/install_test.go` (extend) | dry-run add, dry-run JSON, conflict error, `--force` override, ignore exclusion |
| `cmd/update_test.go` (extend) | pinned skip, pinned newer-version notice, pinned not-found error, dry-run output |

---

## Build & Verify

```bash
CGO_ENABLED=0 go build ./...
go vet ./...
go test ./...
```

All three must pass clean before any task is considered done.
