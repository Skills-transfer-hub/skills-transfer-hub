# Contract: Org Config Endpoints & `sth config` Commands

Specifies the two backend routes and the CLI command surface for shared configuration. The target org
is always the **active org** resolved by `resolveAuth` (spec 009 selector → home org fallback).
Assertions are verified by re-reading the cited source; no execution is required.

---

## `GET /v1/org-config`

Returns the active org's shared config. Readable by **any member** of the org.

```
GET /v1/org-config
Authorization: Bearer lsk_live_…
X-STH-Org: <organization_id>            # optional (spec 009); else home org

200 → { "config": { …ProjectConfigV2 subset… }, "config_version": 3 }
200 → { "config": null, "config_version": 0 }     # no config published yet
401 → unknown/expired key
```

- Handler: `resolveAuth(req)` → read `org_shared_configs` by `ctx.org.id` via `getServiceClient()`.
- Membership is implied by `resolveAuth` (the key resolves to an org the identity belongs to); no extra
  role check for reads.
- Empty result is **not** an error (FR-005): returns `config: null`.

---

## `PUT /v1/org-config`

Upserts the active org's shared config. Requires **`fleet_sync`** (Team) AND **owner/admin** role.

```
PUT /v1/org-config
Authorization: Bearer lsk_live_…
X-STH-Org: <organization_id>            # optional; else home org
If-Match: 3                              # base version the client last saw (or body.base_version)
{ "config": { …ProjectConfigV2 subset… } }

200 → { "config_version": 4 }            # success, version incremented
403 → not_entitled        # caller lacks fleet_sync
403 → forbidden_role      # caller is a member, not owner/admin (or key has no assignee)
409 → version_conflict    # base version != current → "config changed, re-pull"
```

Enforcement order in `routes/org_config.ts`:

1. `resolveAuth(req)` → `ctx` (org, plan, `license.assignee_id`).
2. Capability: `ctx.plan` must include `fleet_sync` (reuse the capability resolver; otherwise `not_entitled`).
3. Role: look up `memberships` where `user_id = ctx.license.assignee_id AND organization_id = ctx.org.id`;
   require `owner|admin`. A NULL `assignee_id` ⇒ `forbidden_role` (FR-009). `resolveAuth` does not expose
   role, so this lookup is mandatory here.
4. Optimistic concurrency: compare `If-Match`/`base_version` to the stored `config_version`; mismatch ⇒
   `version_conflict` (FR-010).
5. Upsert `org_shared_configs` (service-role), set `updated_by = assignee_id`, `config_version = current + 1`.

This mirrors the `routes/access_request.ts` precedent (authenticated org-scoped write via
`getServiceClient`, structured errors via `jsonError`).

---

## CLI commands

### `sth config show`

Prints the effective config with provenance. Not capability-gated.

```
$ sth config show
providers:
  github-acme        [org]
  my-local-fork      [local]
pins:
  reviewer@org       [org]   v1.4.0
  scratch-skill      [local] (unpinned)
```

### `sth config pull [--dry-run]`

Applies the org's managed fields over local, preserving purely-local resources. Not capability-gated
(non-Team orgs simply return no config).

```
$ sth config pull --dry-run
Would apply org configuration (version 3):
  [overwrite] provider github-acme  (catalog_ref main → release/1.x)
  [apply]     pin reviewer@org → v1.4.0
  [keep]      provider my-local-fork (local only)
No changes written (--dry-run).
```

- Source: `GET /v1/org-config` → `fs.MergeOrgConfig(local, org)`.
- Without `--dry-run`: `fs.WriteProjectConfigV2(result.Merged)`.
- Empty org config ⇒ "no shared configuration for this organization", exit 0.

### `sth config push [--dry-run]`

Publishes the local shareable subset as the org baseline. Gated by `fleet_sync`
(`a.enforceCapability(ctx, "fleet_sync")`); server additionally enforces owner/admin + version.

```
$ sth config push
Published shared configuration for Acme Corp (version 4).
```

- Payload: `fs.ShareableSubset(local)` — never includes secrets.
- Sends the last-seen `config_version` for optimistic concurrency; on `409` prints
  "config changed, re-pull" and exits non-zero.

**Wiring**: add `config` to `commandCatalog` and the `run()` switch in `cmd/root.go`; implement
`runConfig(args ...string)` on `*app` dispatching `show|pull|push`, reusing
`fs.ReadProjectConfigV2/WriteProjectConfigV2` and the `internal/http` cloud client. Strict Go style
applies; ship `*_test.go` for the merge precedence, the dry-run no-write guarantee, and the
empty-config degrade path.
