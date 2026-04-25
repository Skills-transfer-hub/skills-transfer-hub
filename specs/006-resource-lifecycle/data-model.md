# Data Model: Resource Lifecycle (spec 006)

## Overview

Spec 006 introduces three new value types and extends the install/update pipelines with
additional control parameters. All types are pure Go structs in `internal/fs/schema.go`
or `internal/fs/status.go`. No new storage files are introduced — the existing
`sth-state-v2.json` and `.sth/project.json` (v2) are extended in place.

---

## New Types

### ResourceStatus (enum string)

Represents the computed health of a single installed resource after comparing disk state
against the stored SHA256 baselines and the upstream commit SHA.

```go
// ResourceStatus is the computed health label of an installed resource.
type ResourceStatus string

const (
    StatusUpToDate       ResourceStatus = "up-to-date"
    StatusOutdated       ResourceStatus = "outdated"
    StatusModifiedLocally ResourceStatus = "modified-locally"
    StatusUnknownBaseline ResourceStatus = "unknown-baseline"
)
```

**Derivation rules** (evaluated in order — first matching rule wins):

| Condition | Status |
|-----------|--------|
| `managed_files` is empty OR all `sha256` fields are `""` | `unknown-baseline` |
| Any `managed_files[i].sha256` ≠ SHA256(file on disk) | `modified-locally` |
| `resolved_version` ≠ latest provider commit SHA | `outdated` |
| All checks pass | `up-to-date` |

---

### StatusResult

The full output record for one resource, returned by `ComputeAllStatuses()` and used by
`cmd/status.go` for both text and JSON rendering.

```go
// StatusResult holds the computed lifecycle status for a single installed resource.
type StatusResult struct {
    ResourceName     string         `json:"resource_name"`
    ProviderID       string         `json:"provider_id"`
    InstalledVersion string         `json:"installed_version"`   // resolved_version in state
    LatestVersion    string         `json:"latest_version"`      // HEAD commit SHA from provider
    Status           ResourceStatus `json:"status"`
    ModifiedFiles    []string       `json:"modified_files,omitempty"` // only when modified-locally
    Pinned           bool           `json:"pinned"`
    PinnedVersion    string         `json:"pinned_version,omitempty"`
}
```

---

### DryRunChange

Represents one previewed file change during a dry-run of `install` or `update`.

```go
// DryRunChange describes a single file change that would occur during install/update.
type DryRunChange struct {
    Action    string `json:"action"`               // "add" | "modify" | "delete"
    Path      string `json:"path"`
    OldSHA256 string `json:"old_sha256,omitempty"` // present when action == "modify"
    NewSHA256 string `json:"new_sha256,omitempty"` // present when action == "add" or "modify"
}
```

**Text output legend**: `[+]` = add, `[~]` = modify, `[-]` = delete.

---

### OwnershipConflict

Describes a file-path collision between two resources detected at install time.

```go
// OwnershipConflict is returned when a file to be installed is already owned by another resource.
type OwnershipConflict struct {
    FilePath         string
    ExistingOwner    string // resource_name of the current owner
    RequestingOwner  string // resource_name being installed
}

func (c OwnershipConflict) Error() string {
    return fmt.Sprintf(
        "conflict: file %q is already owned by resource %q (installing %q); use --force to override",
        c.FilePath, c.ExistingOwner, c.RequestingOwner,
    )
}
```

---

## Modified Types

### InstalledResourceRecordV2 — `resolved_version` semantic change

**File**: `internal/fs/schema.go` (no struct change, semantic change only — documented
here for implementors)

| Field | Before spec 006 | After spec 006 |
|-------|-----------------|----------------|
| `resolved_version` | SHA256 of downloaded bundle content (transitional) | Git commit SHA of provider's `catalog_ref` at install/update time |
| `managed_files[].sha256` | SHA256 of each file on disk | SHA256 of each file on disk (unchanged) |

The struct itself does not change. Only the value written to `resolved_version` changes:
from `fs.ComputeSHA256(content)` → the commit SHA returned by the GitHub API.

---

### ResourceEntry — `pinned_version` (already present, behaviour activated)

**File**: `internal/fs/schema.go` (no change needed)

```go
// ResourceEntry tracks a managed resource in the providers array.
// pinned_version is set by the user in project.json to lock a resource.
type ResourceEntry struct {
    ResourceName  string `json:"resource_name"`
    ProviderID    string `json:"provider_id"`
    PinnedVersion string `json:"pinned_version,omitempty"` // empty = unpinned
}
```

When `PinnedVersion != ""`:
- `sth update` skips the resource and prints `[pinned at <version>]`.
- `sth status` shows a notice if a newer version is available.
- If the provider cannot resolve `PinnedVersion`, the command exits non-zero.

---

## New Functions (internal/fs/)

### ignore.go

```go
// ParseIgnoreFile reads .sthignore from the project root and returns active patterns.
// Lines starting with '#' and blank lines are ignored.
func ParseIgnoreFile(projectRoot string) ([]string, error)

// MatchIgnore reports whether bundleRelativePath matches any of the given gitignore patterns.
// Patterns support *, **, and ? glob syntax. bundleRelativePath must use forward slashes.
func MatchIgnore(patterns []string, bundleRelativePath string) bool
```

### status.go

```go
// ComputeAllStatuses returns one StatusResult per entry in sth-state-v2.json.
// latestSHAs maps providerID to the current HEAD commit SHA fetched from the provider.
func ComputeAllStatuses(
    state     InstalledResourceStateV2,
    config    ProjectConfigV2,
    latestSHAs map[string]string,
    projectRoot string,
) []StatusResult
```

### fs.go (additions)

```go
// BuildOwnerIndex returns a map of file path → resource_name from the v2 state.
// Used to detect ownership conflicts before installing a new resource.
func BuildOwnerIndex(state InstalledResourceStateV2) map[string]string

// ComputeFileSHA256 returns the hex-encoded SHA256 of the file at the given path,
// or an error if the file cannot be read.
func ComputeFileSHA256(path string) (string, error)
```

---

## State Transitions

```
Resource lifecycle states:
                    ┌─────────────┐
         install    │             │   update (same version)
       ──────────►  │  up-to-date │ ◄─────────────────────
                    │             │
                    └──────┬──────┘
                           │ new upstream commit
                           ▼
                    ┌─────────────┐
                    │   outdated  │
                    └──────┬──────┘
                           │ sth update
                           ▼
                    ┌─────────────┐
                    │  up-to-date │
                    └─────────────┘

                    ┌──────────────────┐
    disk edit  ───► │ modified-locally │
                    └──────────────────┘
    (only sth install --force re-establishes integrity)

                    ┌──────────────────┐
    legacy install  │ unknown-baseline │
    (no SHA256) ──► │                  │
                    └──────────────────┘
    (sth install --force re-establishes baseline)
```
