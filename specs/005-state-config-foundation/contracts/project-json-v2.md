# Contract: `project.json` v2 Schema

**File path**: `.loreskills/project.json`  
**Schema version**: 2  
**Owner**: `internal/fs/` — written by `WriteProjectConfigV2`, read by `ReadProjectConfigV2`

---

## Schema

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
  "resources": [
    {
      "resource_name": "skill::skills::coding",
      "provider_id": "default",
      "pinned_version": "abc123def456"
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
| `providers` | array | yes | One or more provider entries; serialises as `[]` never `null` |
| `resources` | array | yes | Resource entries with optional version pin; serialises as `[]` never `null` |

### `providers[]` — `ProviderEntry`

| Field | Type | Required | Constraints | Description |
|-------|------|----------|-------------|-------------|
| `id` | string | yes | `^[a-zA-Z0-9_-]+$`; unique within array | Human-readable short name for the provider |
| `provider` | string | yes | `github\|gitlab\|azure-devops\|bitbucket` | Remote provider type |
| `repository_id` | string | yes | Non-empty; provider-specific format | e.g., `owner/repo` (GitHub/Bitbucket), `group/project` (GitLab), `org/project/repo` (Azure) |
| `catalog_ref` | string | yes | Non-empty; defaults to `"main"` | Branch or tag of the catalog in the remote repo |
| `catalog_path` | string | yes | Must end with `SKILLS.md` | Relative path to the catalog file in the remote repo |
| `target_dir` | string | yes | Relative, non-empty; no `../` traversal | Local directory where this provider's resources are installed |

### `resources[]` — `ResourceEntry`

| Field | Type | Required | Constraints | Description |
|-------|------|----------|-------------|-------------|
| `resource_name` | string | yes | Non-empty | Catalog ID of the resource (`type::folder::name`) |
| `provider_id` | string | yes | Must match an existing `providers[].id` | Links the resource to its source provider |
| `pinned_version` | string | no | Commit SHA or tag; omitted if not pinned | When present, `loreskills update` skips this resource |

---

## Invariants

1. `schema_version` MUST be `2`.
2. `providers` array MUST NOT be `null`; MAY be empty before `loreskills init` runs.
3. Provider `id` values MUST be unique within `providers`.
4. Each `resource.provider_id` MUST reference a `providers[].id` in the same file.
5. The `catalog_path` field uses forward slashes (URL-style paths), not OS-native separators.
6. `target_dir` is relative to the current working directory; MUST NOT start with `/` or `..`.

---

## v1 → v2 Migration Mapping

| v1 field | v2 location | Notes |
|----------|-------------|-------|
| `provider` | `providers[0].provider` | Preserved as-is |
| `repository_id` | `providers[0].repository_id` | Preserved as-is |
| `ref` | `providers[0].catalog_ref` | **Renamed** — `ref` key removed |
| `catalog_path` | `providers[0].catalog_path` | Preserved as-is |
| `target_dir` | `providers[0].target_dir` | Preserved as-is |
| *(absent)* | `providers[0].id` | Set to `"default"` |
| `migrated_from_legacy` | *(dropped)* | No longer stored |
| *(absent)* | `resources` | Initialised to `[]` |

---

## Consumers

- `loreskills init` — writes provider entries
- `loreskills list` — reads `providers[0]` (primary provider for catalog fetch)
- `loreskills install` — reads `providers`, `resources`; writes `resources` on pin
- `loreskills update` — reads `providers`, `resources.pinned_version`
- `loreskills status` — reads `providers`, `resources`
- `loreskills remove` — reads `resources`; removes entry on unpin (spec 006)
