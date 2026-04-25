# Developer Quickstart: State & Config Foundation

**Spec**: [spec.md](spec.md) | **Plan**: [plan.md](plan.md) | **Branch**: `006-cli-product-roadmap`

## Prerequisites

- Go 1.22+ installed
- `CGO_ENABLED=0` for builds (already configured in CI)
- Repository cloned and on branch `006-cli-product-roadmap`

## Build & Test

```bash
# Build
CGO_ENABLED=0 go build -o sth .

# Run all tests
go test ./...

# Run only fs package tests (most relevant for this spec)
go test ./internal/fs/...

# Run with verbose output
go test -v ./internal/fs/...
```

## Key Files to Modify

| File | Purpose |
|------|---------|
| `internal/fs/schema.go` | **New** — v2 type definitions |
| `internal/fs/migrate.go` | **New** — migration logic |
| `internal/fs/fs.go` | **Extend** — add `ReadProjectConfigV2()`, SHA256 helper |
| `cmd/init.go` | **Extend** — multi-provider append flow |
| `cmd/install.go` | **Extend** — populate `managed_files` on install |
| `cmd/update.go` | **Extend** — update `managed_files` + `previous_version` |

## v2 JSON Schemas

- **project.json v2**: `specs/005-state-config-foundation/contracts/project-config-v2.json`
- **sth-state.json v2**: `specs/005-state-config-foundation/contracts/sth-state-v2.json`

## Understanding the Migration

The migration is triggered automatically on first read after an upgrade. No user action is
required. The flow is:

```
ReadProjectConfigV2()
  └─> read .sth/project.json
        ├─ schema_version == 2 → return as-is
        └─ schema_version absent or != 2 → MigrateProjectConfig()
              ├─> create .sth/project.json.bak
              ├─> rewrite .sth/project.json with v2 schema
              └─> return v2 config
```

Same pattern for `ReadInstalledStateV2()` and `sth-state.json`.

## Adding a Second Provider (manual test)

```bash
# After initial init (provider "default" created)
sth init
# > "Configuration projet existante détectée"
# > "Ajouter un nouveau provider ?" [Y/n]: y
# > "ID du provider (ex: public)": public
# ... follow prompts ...
# > project.json now has 2 entries in the providers array
```

## Common Test Patterns

```go
// Test migration from v1 to v2
func TestMigrateProjectConfig_V1ToV2(t *testing.T) {
    dir := t.TempDir()
    v1 := `{"provider":"github","repository_id":"org/repo","ref":"main","catalog_path":"SKILLS.md","target_dir":".claude/skills"}`
    os.WriteFile(filepath.Join(dir, "project.json"), []byte(v1), 0o600)

    cfg, err := MigrateProjectConfig(filepath.Join(dir, "project.json"))
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if cfg.SchemaVersion != 2 {
        t.Errorf("want schema_version 2, got %d", cfg.SchemaVersion)
    }
    if len(cfg.Providers) != 1 || cfg.Providers[0].ID != "default" {
        t.Errorf("want single provider with id 'default', got %+v", cfg.Providers)
    }
    if cfg.Providers[0].CatalogRef != "main" {
        t.Errorf("want catalog_ref 'main', got %s", cfg.Providers[0].CatalogRef)
    }

    // Verify backup created
    if _, err := os.Stat(filepath.Join(dir, "project.json.bak")); err != nil {
        t.Error("backup file not created")
    }
}

// Test SHA256 is written for managed files
func TestInstallWritesManagedFiles(t *testing.T) {
    // ... install a resource and verify sth-state.json has managed_files with sha256
}
```

## Dependency Chain Reminder

This spec (005) MUST be fully complete and merged before work begins on:

- **006** (resource lifecycle): uses `managed_files` for `remove` and `status`
- **007** (CI/auth/observability): uses v2 state for `--fail-on-changes`
- **008** (ecosystem tooling): uses `previous_version` for changelog display

---

## Implementation Status (as of plan date)

| Requirement | Status | Notes |
|-------------|--------|-------|
| Schema types (`schema.go`) | ✅ Done | All v2 types defined |
| `MigrateProjectConfig` | ⚠️ Partial | Missing: `io.Writer` param, stderr progress, atomic write |
| `MigrateInstalledState` | ⚠️ Partial | Missing: `io.Writer` param, stderr progress, atomic write; also targets wrong file |
| `WriteInstalledStateV2` | ⚠️ Partial | Missing: atomic write-then-rename, lock |
| `WriteProjectConfigV2` | ⚠️ Partial | Missing: atomic write-then-rename |
| Multi-provider `sth init` | ✅ Done | |
| `PinnedVersion` lookup | ✅ Done | |
| Ownership conflict detection | ✅ Done | |
| `atomicWriteFile` helper | ❌ Missing | Create `internal/fs/atomic.go` |
| `AcquireStateLock` | ❌ Missing | Create `internal/fs/lock.go` |
| `resolved_version` population | ❌ Missing | `InstallCatalogEntryV2` needs `resolvedVersion` param |
| Auto-migration on command entry | ❌ Missing | `runMigrations(stderr)` call in each `cmd/` handler |
| File naming: `sth-state.json` | ❌ Missing | `WriteInstalledStateV2` writes to wrong file (`sth-state-v2.json`) |

## New API Signatures to Implement

```go
// internal/fs/atomic.go
func atomicWriteFile(path string, data []byte, perm os.FileMode) error

// internal/fs/lock.go
func AcquireStateLock(lockPath string) (io.Closer, error)

// internal/fs/migrate.go — updated signatures
func MigrateProjectConfig(path string, progress io.Writer) (ProjectConfigV2, error)
func MigrateInstalledState(path string, progress io.Writer) (InstalledResourceStateV2, error)

// internal/fs/fs.go — updated signature
func InstallCatalogEntryV2(targetDir string, entry CatalogEntry, content []byte, resolvedVersion string) (InstalledResourceRecordV2, error)
```
