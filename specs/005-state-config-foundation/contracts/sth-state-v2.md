# Contract: `sth-state-v2.json` v2 Schema

**File path**: `<target_dir>/sth-state-v2.json`  
**Schema version**: 2  
**Owner**: `internal/fs/` — written atomically by `WriteInstalledStateV2` (via `atomicWriteFile`),
read by `ReadInstalledStateV2`  
**Concurrency guard**: All reads and writes MUST be bracketed by `AcquireStateLock` / `release()`

---

## Schema

```json
{
  "schema_version": 2,
  "entries": [
    {
      "resource_name": "skill::skills::coding",
      "provider_id": "default",
      "resolved_version": "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
      "previous_version": "",
      "installed_at": "2026-04-25T14:30:00Z",
      "managed_files": [
        {
          "path": ".claude/skills/skills/coding.md",
          "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
        }
      ]
    }
  ]
}
```

---

## Field Definitions

### Root Object

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `schema_version` | integer | yes | Always `2` for v2 files |
| `entries` | array | yes | Installed resource records; serialises as `[]` never `null` |

### `entries[]` — `InstalledResourceRecordV2`

| Field | Type | Required | Constraints | Description |
|-------|------|----------|-------------|-------------|
| `resource_name` | string | yes | Non-empty | Catalog ID (`type::folder::name`) |
| `provider_id` | string | yes | Non-empty | References a `ProviderEntry.id` in `project.json` |
| `resolved_version` | string | no | Commit SHA or tag; `""` for migrated records | The actual version installed on disk |
| `previous_version` | string | no | Commit SHA or tag; `""` on first install | The version before the last update |
| `installed_at` | string | no | RFC3339 UTC; `""` for migrated records | Timestamp of last install or update |
| `managed_files` | array | yes | Non-null; may be empty array | Files written to disk by this resource's installation |

### `entries[].managed_files[]` — `ManagedFile`

| Field | Type | Required | Constraints | Description |
|-------|------|----------|-------------|-------------|
| `path` | string | yes | Relative; forward-slash separated; no `../` prefix | Path of the managed file relative to project root |
| `sha256` | string | yes | 64-character lowercase hex string | SHA-256 of the file bytes as written to disk (post `skillHeader` for `.md` files) |

---

## Invariants

1. `schema_version` MUST be `2`.
2. `entries` MUST NOT be `null`; serialises as `[]` when empty.
3. The compound key `(resource_name, provider_id)` MUST be unique within `entries`.
4. `managed_files` MUST NOT be `null`; may be `[]` (e.g., migrated records, empty bundles).
5. `managed_files[].path` MUST use forward slashes and MUST NOT be absolute.
6. All writes MUST use atomic write-then-rename (`atomicWriteFile`) to prevent partial-write corruption.
7. All reads and writes MUST be guarded by the advisory lock at `<stateDir>/.sth-state.lock`.

---

## Atomicity & Locking Guarantees

### Atomic Write Protocol (FR-014)

```
1. Marshal new state to JSON bytes
2. Write to <stateDir>/.sth-tmp-* (same directory as sth-state-v2.json)
3. os.Rename(<stateDir>/.sth-tmp-*, <stateDir>/sth-state-v2.json)
```

A process killed at any step leaves the prior valid `sth-state-v2.json` intact.

### Advisory Lock Protocol (FR-015)

```go
release, err := fs.AcquireStateLock(targetDir)
if err != nil {
    return err  // fail-fast; do not wait
}
defer release()
// ... read / write state ...
```

`release()` removes the lock file. Stale lock files (from crashed processes) require
manual deletion; the error message instructs the user.

---

## v1 → v2 Migration Mapping

| v1 field | v2 location | Notes |
|----------|-------------|-------|
| `entries[].catalog_id` | `entries[].resource_name` | **Renamed** |
| `entries[].resource_type` | *(dropped)* | Derivable from `resource_name` |
| `entries[].name` | *(dropped)* | Derivable from `resource_name` |
| `entries[].folder_name` | *(dropped)* | Derivable from `resource_name` |
| `entries[].installed_path` | *(replaced)* | Replaced by `managed_files[].path` |
| `entries[].source_path` | *(dropped)* | No longer needed in state |
| `entries[].status` | *(dropped)* | Always "installed" in v2; removal deletes the record |
| *(absent)* | `entries[].provider_id` | Set to `"default"` |
| *(absent)* | `entries[].resolved_version` | Set to `""` (populated on next install) |
| *(absent)* | `entries[].previous_version` | Set to `""` |
| *(absent)* | `entries[].installed_at` | Set to `""` |
| *(absent)* | `entries[].managed_files` | Set to `[]` |

Migration stderr output format (FR-016):
```
[sth] migrating sth-state.json v1 → v2 … done
```

---

## Consumers

- `sth install` — reads state (check duplicates/ownership); writes new entry
- `sth update` — reads state (find installed resources); writes updated entry
- `sth status` — reads state (show per-resource status)
- `sth remove` — reads state (find managed files); writes state (removes entry)
- `sth list` — does NOT read state (reads remote catalog only)
- Migration (`MigrateInstalledState`) — reads v1; writes v2 atomically with backup
