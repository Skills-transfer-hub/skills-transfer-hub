# CLI Command Contracts: Resource Lifecycle (spec 006)

Formal schema for every new or modified CLI surface introduced by spec 006.
These contracts govern what the implementor must deliver and what tests must verify.

---

## New Command: `sth status`

### Synopsis

```
sth status [--json]
```

### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | false | Emit a JSON array of StatusResult objects instead of human-readable text |

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success (including "no resources installed") |
| 1 | Error reading state or config file |
| 2 | Network error while fetching latest commit SHAs |

### Text Output Format

One line per resource. Columns separated by two spaces:

```
<resource-name>  <status>  [<details>]
```

Examples:

```
my-skill          up-to-date
another-skill     outdated         installed: abc1234  available: def5678
heavy-skill       modified-locally  modified: .claude/skills/heavy-skill/guide.md
legacy-skill      unknown-baseline  re-install to establish integrity tracking
pinned-skill      up-to-date  (pinned at v1.2.0)
```

No-resources case:

```
No resources installed.
```

### JSON Output Schema

```json
[
  {
    "resource_name": "my-skill",
    "provider_id": "main",
    "installed_version": "abc1234",
    "latest_version": "abc1234",
    "status": "up-to-date",
    "pinned": false
  },
  {
    "resource_name": "heavy-skill",
    "provider_id": "main",
    "installed_version": "abc1234",
    "latest_version": "def5678",
    "status": "modified-locally",
    "modified_files": [".claude/skills/heavy-skill/guide.md"],
    "pinned": false
  },
  {
    "resource_name": "pinned-skill",
    "provider_id": "secondary",
    "installed_version": "v1.2.0",
    "latest_version": "v1.3.0",
    "status": "up-to-date",
    "pinned": true,
    "pinned_version": "v1.2.0"
  }
]
```

### Routing in `cmd/root.go`

```go
case "status":
    // --json flag only; no positional args
    err = a.runStatus(parseStatusFlags(args[1:]))
```

---

## New Command: `sth remove`

### Synopsis

```
sth remove <resource-name> [--yes]
```

### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--yes` | bool | false | Skip confirmation prompt when managed files have been locally modified |

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Resource fully removed (files + state entry deleted) |
| 1 | Resource not found in state |
| 2 | One or more file deletions failed (state entry kept) |
| 3 | User rejected confirmation prompt |

### Interaction Flow

```
sth remove my-skill
  → detect modified files
  → if none: proceed directly
  → if any: print warning + list of modified files
      "Warning: the following files have been locally modified:"
      "  .claude/skills/my-skill/guide.md"
      "Remove anyway? [y/N] "
  → read y/n from stdin
  → y: proceed; N or empty: exit 3
  → delete each managed file
  → on per-file error: collect, report, keep state entry, exit 2
  → on success: remove state entry, print "Resource 'my-skill' removed."
```

With `--yes`: skip confirmation entirely.

### Missing-file Behaviour

If a managed file is missing from disk, print:
```
warning: file not found, skipping: .claude/skills/my-skill/old-file.md
```
Continue removal; count as successfully cleaned up.

### Routing in `cmd/root.go`

```go
case "remove":
    // requires exactly one positional arg + optional --yes
    err = a.runRemove(resourceName, parseRemoveFlags(args[2:]))
```

---

## Modified Command: `sth install`

### Synopsis (extended)

```
sth install <resource-ref> [--dry-run] [--force]
```

### New Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Preview changes without writing to disk or state |
| `--force` | bool | false | Override file ownership conflicts; transfer ownership to new resource |

### Dry-Run Text Output

```
Dry run — no changes will be written.

[+] .claude/skills/new-skill/guide.md
[+] .claude/skills/new-skill/examples.md
```

If no changes:
```
No changes would be made.
```

### Dry-Run JSON Output (`--dry-run --json`)

```json
[
  { "action": "add", "path": ".claude/skills/new-skill/guide.md",
    "new_sha256": "e3b0c44298fc1c149afb..." },
  { "action": "add", "path": ".claude/skills/new-skill/examples.md",
    "new_sha256": "2cf24dba5fb0a30e26e8..." }
]
```

### Conflict Error (without `--force`)

```
Error: conflict: file ".claude/skills/shared/common.md" is already owned by resource
"skill-a" (installing "skill-b"); use --force to override
```

Exit code: 1

---

## Modified Command: `sth update`

### Synopsis (extended)

```
sth update [self] [--dry-run]
```

### New Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Preview changes for all updateable resources; no disk or state writes |

### Pinning Output

When a resource is pinned:
```
[pinned at v1.2.0]  my-skill  (newer version available: def5678)
```

When no newer version is available:
```
[pinned at v1.2.0]  my-skill
```

### Pinned Version Not Found Error

```
Error: pinned version "v1.2.0" not found for resource "my-skill" on provider "main"
```

Exit code: 1

### Dry-Run Text Output

```
Dry run — no changes will be written.

my-skill:
  [~] .claude/skills/my-skill/guide.md
  [+] .claude/skills/my-skill/new-file.md
  [-] .claude/skills/my-skill/deprecated.md

No changes for: another-skill
```

---

## Global Flag: `--yes`

Available on all interactive commands (`remove`, and any future confirmation prompts).
Parsed in `cmd/root.go` before dispatching to sub-commands. When set, all "are you sure?"
prompts are bypassed.

```go
// Parsed early in run() before the switch:
yesAll := containsFlag(args, "--yes")
```

---

## Flag Parsing Convention

All flag parsing uses the standard `flag` package from stdlib. Each command creates its
own `flag.FlagSet` keyed to the command name, parsing `args[1:]` (after the verb).
The `--yes` global flag is stripped from args before passing to sub-command flag sets.

```go
// Example pattern for a sub-command flag set:
fset := flag.NewFlagSet("status", flag.ContinueOnError)
jsonOut := fset.Bool("json", false, "JSON output")
if err := fset.Parse(args); err != nil {
    return err
}
```
