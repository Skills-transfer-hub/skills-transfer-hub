# Data Model: Multiple Organizations — CLI Org Context (spec 009)

## Overview

Spec 009 introduces **no database table and no migration**. The many-to-many model already exists
(`memberships`, `users.active_organization_id`). The only new persistence is CLI-side, inside the
existing `~/.config/sth/settings.json`. The backend gains a derived value (selected org) on its
in-memory `AuthContext`, not a stored one.

---

## New Types

### ActiveOrgSelection (CLI — persisted in `UserSettings`)

Added to `internal/fs/user_settings.go`. Absence of the block means "use the key's home org",
keeping older settings files valid.

```go
type ActiveOrgSelection struct {
    OrgID      string    `json:"org_id"`
    OrgName    string    `json:"org_name"`
    Role       string    `json:"role"`
    Plan       string    `json:"plan"`
    SelectedAt time.Time `json:"selected_at"`
}
```

`UserSettings` gains one optional field:

```go
type UserSettings struct {
    Telemetry    TelemetrySettings   `json:"telemetry"`
    ProActive    bool                `json:"pro_active,omitempty"`
    ProCheckedAt time.Time           `json:"pro_checked_at,omitempty"`
    Entitlements *EntitlementsCache  `json:"entitlements,omitempty"`
    ActiveOrg    *ActiveOrgSelection `json:"active_org,omitempty"`
}
```

Writes go through the existing `settingsWriteMu`-guarded read-modify-write path so a
`SetActiveOrg` does not race a `SaveEntitlements`.

### TargetableOrg (wire — one row of `sth org list`)

The shape returned by `GET /v1/orgs` and rendered by `sth org list` (text and `--json`).

```go
type TargetableOrg struct {
    ID     string `json:"id"`
    Name   string `json:"name"`
    Role   string `json:"role"`
    Plan   string `json:"plan"`
    Active bool   `json:"active"`
}
```

---

## Modified Types

### AuthContext (backend — `sth-back/shared/auth.ts`)

No field is added; the **source** of `org` and `plan` changes. Today both derive from
`license.organization_id`. After 009, when a valid active-org selector is present, `org` and the
effective `plan` derive from the **selected** org instead.

| Field | Before 009 | After 009 |
|-------|------------|-----------|
| `org` | license's home org (`licenses.organization_id`) | selected org when selector is valid for `assignee_id` membership; else home org |
| `plan` | effective plan of home org | effective plan of the resolved org (selected or home) |

Selector resolution (read-only):

1. Extract the active-org selector (header) from the request. Absent ⇒ home org (unchanged path).
2. Look up `memberships` where `user_id = license.assignee_id AND organization_id = <selector>`.
   - No row (or `assignee_id` is NULL) ⇒ reject with an authorization error that does not reveal
     whether the org exists.
3. Load the selected org + its subscription and compute the effective plan exactly as the existing
   resolver does for the home org.

### Capability map (backend + dashboard mirror)

`multi_org` is added to three tiers. No migration — capabilities are derived from `plan_tier`.

```ts
// sth-back/shared/capabilities.ts  AND  sth-dashboard/src/lib/billing/plans.ts (KEEP IN SYNC)
starter: []
pro:     ['private_repos', 'pack_init', 'multi_target', 'multi_org']
team:    ['private_repos', 'pack_init', 'multi_target', 'fleet_sync', 'multi_org']
alpha:   ['private_repos', 'pack_init', 'multi_target', 'fleet_sync', 'alpha_preview', 'multi_org']
```

---

## Persistence & lifecycle

```
sth login            → store key (keychain); record home org; hint if >1 membership
sth org use <org>    → validate membership (server) → write UserSettings.active_org
any org-scoped cmd   → read active_org → send selector → backend resolves selected org
stale active_org     → server rejects selector → CLI warns once → clears block → home org
sth logout           → clear active_org alongside the key
```

No DB writes occur for any of the above except the existing `licenses.last_used_at` touch.
