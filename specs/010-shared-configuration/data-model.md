# Data Model: Shared Configuration — Team CLI Config (spec 010)

## Overview

Spec 010 introduces **one new table** (`org_shared_configs`, migration 0030) and **one new CLI value
type** (the merge/provenance result). It reuses the existing `ProjectConfigV2` schema as the shape of
the stored blob; no change to `internal/fs/schema.go` is required.

---

## New Table — `org_shared_configs`

**File**: `sth-dashboard/supabase/migrations/0030_org_shared_config.sql` (all schema lives in the
dashboard repo; next free number is 0030 after `0029_rls_hardening.sql`).

```sql
CREATE TABLE IF NOT EXISTS public.org_shared_configs (
  organization_id  UUID PRIMARY KEY REFERENCES public.organizations(id) ON DELETE CASCADE,
  config           JSONB   NOT NULL,
  config_version   INTEGER NOT NULL DEFAULT 1 CHECK (config_version >= 1),
  updated_by       TEXT    REFERENCES public.users(id) ON DELETE SET NULL,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

- `organization_id` is the primary key ⇒ exactly **one shared config per org**.
- `config` holds the shareable `ProjectConfigV2` subset (below), internally schema-versioned via its
  own `schema_version` field — distinct from `config_version`, which is the monotonic publish counter.
- `config_version` drives optimistic concurrency on `PUT`.

### RLS (mirrors the existing org-scoped patterns in `0001_init.sql`)

```sql
ALTER TABLE public.org_shared_configs ENABLE ROW LEVEL SECURITY;

CREATE POLICY "org_shared_configs_select_member" ON public.org_shared_configs
  FOR SELECT USING (
    organization_id IN (
      SELECT organization_id FROM public.memberships
      WHERE user_id = (SELECT requesting_user_id())
    )
  );

CREATE POLICY "org_shared_configs_write_admin" ON public.org_shared_configs
  FOR ALL USING (
    organization_id IN (
      SELECT organization_id FROM public.memberships
      WHERE user_id = (SELECT requesting_user_id()) AND role IN ('owner','admin')
    )
  ) WITH CHECK (
    organization_id IN (
      SELECT organization_id FROM public.memberships
      WHERE user_id = (SELECT requesting_user_id()) AND role IN ('owner','admin')
    )
  );
```

**Service-role bypass**: the CLI Edge Function runs with the service-role client and therefore
bypasses RLS; it enforces member-read / owner-admin-write **in the handler** (FR-009). RLS is the
second defence layer for any direct dashboard access via the user's Clerk JWT.

---

## Stored blob — shareable `ProjectConfigV2` subset

The `config` JSONB stores only non-secret, shareable fields. `ProviderEntry` already excludes secrets
(tokens are resolved at runtime per spec 007), so the existing struct is safe to serialise as-is.

```jsonc
{
  "schema_version": 2,
  "providers": [
    { "id": "...", "provider": "github.com", "repository_id": "...",
      "catalog_ref": "...", "catalog_path": "...", "target_dir": "...", "private": false }
  ],
  "resources": [
    { "resource_name": "...", "provider_id": "...", "pinned_version": "..." }
  ],
  "targets": [ /* optional */ ]
}
```

**Excluded by construction**: any API token / credential, and per-machine-only values that must not be
shared. The push path extracts this subset via `ShareableSubset()` (below); it never reads the
keychain or env secrets.

---

## New CLI type — merge & provenance

**File**: `internal/fs/shared_config.go`

```go
type FieldProvenance string

const (
    ProvenanceOrg   FieldProvenance = "org"
    ProvenanceLocal FieldProvenance = "local"
)

type MergeAction string

const (
    ActionApply     MergeAction = "apply"     // org field added locally
    ActionOverwrite MergeAction = "overwrite" // org field replaces a local managed value
    ActionKeep      MergeAction = "keep"      // purely-local resource preserved
)

type ConfigMergeEntry struct {
    Field      string          `json:"field"`
    Provenance FieldProvenance `json:"provenance"`
    Action     MergeAction     `json:"action"`
}

type ConfigMergeResult struct {
    Merged  ProjectConfigV2    `json:"-"`
    Entries []ConfigMergeEntry `json:"entries"`
}
```

### Merge algorithm (org-priority on managed fields)

`MergeOrgConfig(local, org ProjectConfigV2) ConfigMergeResult`:

1. **Providers**: keyed by `id`. A provider present in the org config is taken from the org
   (`overwrite` if it also existed locally, else `apply`). A provider present only locally is kept
   (`keep`).
2. **Pinned versions / resources**: keyed by `resource_name`. The org's `pinned_version` wins for any
   resource the org config declares (`overwrite`/`apply`); resources declared only locally are kept
   (`keep`).
3. **Targets**: org-provided when present, else local.
4. The result `Merged` is what `pull` writes (unless `--dry-run`); `Entries` is what `show` and
   `pull --dry-run` print.

`ShareableSubset(local ProjectConfigV2) ProjectConfigV2` returns the push payload: providers,
resources-with-pins, and targets — with no secret-bearing field populated.

---

## Lifecycle

```
sth config push  (owner/admin, fleet_sync)
    → ShareableSubset(local) → PUT /v1/org-config {config, base_version}
    → server: role+capability check → version match → upsert, config_version++
sth config pull  (any member)
    → GET /v1/org-config → MergeOrgConfig(local, org)
    → --dry-run ? print Entries : WriteProjectConfigV2(Merged)
sth config show
    → GET /v1/org-config → print Entries (provenance per field)
empty org config → pull/show: message + exit 0 ; push: create config_version = 1
```
