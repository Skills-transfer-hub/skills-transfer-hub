# Contract: CLI `org` Commands & Active-Org Selection

This contract specifies the `sth org` command surface, the wire selector that re-scopes
authenticated requests, and the backend resolution rules. Assertions here are verified by
re-reading the cited source — no execution is required to validate the spec.

---

## CLI commands

### `sth org current`

Prints the active org context. Never capability-gated.

```
$ sth org current
Active organization: Acme Corp (org_3f2a…)   [default — home org]
Role: admin   Plan: team
```

- Reads `UserSettings.active_org`; when absent, reports the key's home org and marks it `[default]`.
- Offline: serves cached values within the entitlements stale window, marked `[cached]`.
- `--json` emits `{ "id", "name", "role", "plan", "active": true, "source": "selected|home" }`.

### `sth org list`

Lists targetable orgs. Gated by `multi_org` (`enforceCapability(ctx, "multi_org")`).

```
$ sth org list
* Acme Corp     org_3f2a…   admin    team
  Side Project  org_91bc…   owner    pro
```

- `*` marks the active org. `--json` emits an array of `TargetableOrg`.
- Source: `GET /v1/orgs` (see below).

### `sth org use <id|name>`

Switches the active org. Gated by `multi_org`. Requires network (membership is validated server-side).

```
$ sth org use "Side Project"
Switched active organization to Side Project (org_91bc…).
```

- On a non-member target ⇒ exit ≠ 0, previous selection unchanged.
- On an ambiguous name (two orgs share it) ⇒ ask for the id.
- On success ⇒ write `UserSettings.active_org`.

**Wiring**: add `org` to `commandCatalog` and the `run()` switch in `cmd/root.go`; implement
`runOrg(args ...string)` on `*app` dispatching `current|list|use`. Strict Go style applies.

---

## Wire selector

Authenticated org-scoped requests carry the active org as an HTTP header:

```
X-STH-Org: <organization_id>
```

- Sent by the CLI only when `UserSettings.active_org` is set.
- Absent header ⇒ backend uses the license home org (today's behavior, FR-011).
- The header is advisory: the backend honors it ONLY after validating membership (below).

---

## Backend: `resolveAuth` selector handling (`sth-back/shared/auth.ts`)

After the existing license → home-org resolution, extend the resolver:

```ts
const selector = req.headers.get("x-sth-org");
if (selector && license.assignee_id) {
  const { data: membership } = await sb
    .from("memberships")
    .select("role")
    .eq("user_id", license.assignee_id)
    .eq("organization_id", selector)
    .maybeSingle();
  if (!membership) throw new ApiError("forbidden_org");   // do not reveal existence
  // resolve org + subscription + effective plan for `selector` instead of the home org
}
```

- `assignee_id` NULL (CI keys) ⇒ selector ignored/rejected; home org only (FR-012).
- Invalid selector ⇒ `forbidden_org` (generic; no org-existence disclosure, FR-010).
- `ctx.role` MAY be surfaced for callers that need it (e.g. `sth org current`).

---

## Backend: `GET /v1/orgs`

New route in `functions/cli/index.ts` → `routes/orgs_list.ts`. Authenticated (any key with an
assignee). Returns the orgs the caller's identity is a member of, with the caller's role and each
org's effective plan; marks the active one if the request carried a selector.

```
GET /v1/orgs
200 → { "orgs": [ { "id", "name", "role", "plan", "active" }, … ] }
```

- Implementation: `resolveAuth` for the identity, then `memberships` joined to `organizations`
  filtered by `user_id = assignee_id`. Service-role client (`getServiceClient`), `jsonOk`.
- Keys with NULL `assignee_id` ⇒ return only the home org.

---

## Alternative (documented, not the primary path): one key per org

If the selector model is rejected, the fallback is **one `lsk_live_` key per org**: the user runs
`sth login` once per org, keys are stored in the keychain under `service = "sth-cloud"`,
`user = <org_id>` (instead of the single `"default"`), and `sth org use` selects which key is
active. This requires zero backend change but a keyring-of-keys in the CLI and a clunkier login
UX. The selector model is preferred because it keeps a single key and a single login.
