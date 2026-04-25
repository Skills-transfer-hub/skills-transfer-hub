# Data Model: State & Config Foundation

**Phase 1 output for `specs/005-state-config-foundation`**

---

## Entities

### ProviderEntry

Represents one configured source repository of skills.

| Field | Type | Required | Constraints | Notes |
|-------|------|----------|-------------|-------|
| `id` | string | yes | `[a-zA-Z0-9_-]+`, unique within project | Short name; "default" assigned during v1 migration |
| `provider` | string | yes | `github\|gitlab\|azure-devops\|bitbucket` | Normalised to lowercase |
| `repository_id` | string | yes | Provider-specific format | e.g., `owner/repo` for GitHub |
| `catalog_ref` | string | yes | Non-empty (default: `"main"`) | Replaces old `ref` field |
| `catalog_path` | string | yes | Non-empty (default: `"SKILLS.md"`) | Path to catalog file in repo |
| `target_dir` | string | yes | Non-empty (default: `".claude/skills"`) | Project-relative target directory |

---

### ResourceEntry (in project.json)

Declares a desired resource and optional version pin. Optional — resources can be installed
without a corresponding entry here.

| Field | Type | Required | Constraints | Notes |
|-------|------|----------|-------------|-------|
| `resource_name` | string | yes | Non-empty | Matches `catalog_id` format (`type::folder::name`) |
| `provider_id` | string | yes | Must reference a `ProviderEntry.id` in the same config | |
| `pinned_version` | string | no | Commit SHA or tag; empty = unpinned | Absence or `""` = treat as unpinned |

---

### ManagedFile

One file written to disk during a resource installation.

| Field | Type | Required | Constraints | Notes |
|-------|------|----------|-------------|-------|
| `path` | string | yes | Relative to project root, forward-slash separated | |
| `sha256` | string | yes | 64-char lowercase hex | Hash of bytes as written (after skillHeader) |

---

### InstalledResourceRecordV2 (in lore-state.json)

Tracks the installed state of one resource, replacing `InstalledResourceRecord`.

| Field | Type | Required | Constraints | Notes |
|-------|------|----------|-------------|-------|
| `resource_name` | string | yes | Non-empty | Replaces `catalog_id` |
| `provider_id` | string | yes | Non-empty | References `ProviderEntry.id` |
| `resolved_version` | string | no | Commit SHA or tag, or `""` | Empty for migrated records |
| `previous_version` | string | no | Commit SHA or tag, or `""` | Empty on first install |
| `installed_at` | string | no | ISO 8601 UTC, or `""` | Empty for migrated records |
| `managed_files` | []ManagedFile | yes | May be empty array | Never absent after v2 |

---

### ProjectConfigV2

Top-level v2 structure for `.loreskills/project.json`.

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `schema_version` | int | yes | Value: `2` |
| `providers` | []ProviderEntry | yes | Min 1 after init |
| `resources` | []ResourceEntry | yes | May be empty; populated on install or manual pin |

---

### InstalledResourceStateV2

Top-level v2 structure for `<target_dir>/lore-state.json`.

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `schema_version` | int | yes | Value: `2` |
| `entries` | []InstalledResourceRecordV2 | yes | May be empty |

---

## Schema Versioning

| File | Field | V1 value | V2 value | Detection |
|------|-------|----------|----------|-----------|
| `project.json` | `schema_version` | absent (or 1) | 2 | Absent or != 2 → migrate |
| `lore-state.json` | `schema_version` | absent (or 1) | 2 | Absent or != 2 → migrate |

---

## Migration: project.json v1 → v2

**Input (v1)**:
```json
{
  "provider": "github",
  "repository_id": "org/skills-catalog",
  "ref": "main",
  "catalog_path": "SKILLS.md",
  "target_dir": ".claude/skills"
}
```

**Output (v2)**:
```json
{
  "schema_version": 2,
  "providers": [
    {
      "id": "default",
      "provider": "github",
      "repository_id": "org/skills-catalog",
      "catalog_ref": "main",
      "catalog_path": "SKILLS.md",
      "target_dir": ".claude/skills"
    }
  ],
  "resources": []
}
```

**Migration rules**:
1. `ref` → `catalog_ref` (renamed, same value)
2. All flat provider fields → single `ProviderEntry` with `id: "default"`
3. `migrated_from_legacy` field is dropped (no longer needed)
4. `resources` array starts empty (existing installations live in lore-state.json)

---

## Migration: lore-state.json v1 → v2

**Input (v1)**:
```json
{
  "entries": [
    {
      "catalog_id": "skill::skills::coding",
      "resource_type": "skill",
      "name": "coding",
      "folder_name": "skills",
      "installed_path": ".claude/skills/skills/coding.md",
      "source_path": "skills/coding.md",
      "status": "installed"
    }
  ]
}
```

**Output (v2)**:
```json
{
  "schema_version": 2,
  "entries": [
    {
      "resource_name": "skill::skills::coding",
      "provider_id": "default",
      "resolved_version": "",
      "previous_version": "",
      "installed_at": "",
      "managed_files": []
    }
  ]
}
```

**Migration rules**:
1. `catalog_id` → `resource_name` (renamed, same value)
2. `resource_type`, `name`, `folder_name`, `status` fields dropped (derivable from `resource_name`)
3. `installed_path`, `source_path` dropped (replaced by `managed_files` array)
4. `provider_id` = `"default"` (the only provider after project.json migration)
5. `resolved_version`, `previous_version`, `installed_at` = `""` (unknown; populated on next install)
6. `managed_files` = `[]` (populated on next install/update)

---

## State Transitions

### InstalledResourceRecordV2 lifecycle

```
(none) ──install──> {resolved_version: "<sha>", managed_files: [...], previous_version: ""}
                          │
                     update (new sha available)
                          │
                          ▼
              {resolved_version: "<new_sha>",
               previous_version: "<old_sha>",
               managed_files: [...updated...]}
                          │
                     remove (spec 006)
                          │
                          ▼
                       (deleted from state)
```

---

## Validation Rules

| Entity | Field | Rule |
|--------|-------|------|
| ProviderEntry | `id` | `regexp: ^[a-zA-Z0-9_-]+$`; unique within providers array |
| ProviderEntry | `provider` | One of: `github`, `gitlab`, `azure-devops`, `bitbucket` |
| ResourceEntry | `provider_id` | Must match an existing `ProviderEntry.id` in same config |
| ManagedFile | `sha256` | Exactly 64 hex chars (`[0-9a-f]{64}`) |
| InstalledResourceRecordV2 | `installed_at` | ISO 8601 UTC or empty string |
| ProjectConfigV2 | `providers` | Length >= 1 after init (may be 0 in empty projects before init) |
