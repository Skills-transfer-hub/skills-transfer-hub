# Global GitHub source: TDD evidence

Executed on 25 October 2026. Each row records tests written and run before its production implementation, then the same tests rerun after the minimal implementation.

| Increment | RED command and observed failure | GREEN |
| --- | --- | --- |
| Direct source parsing, normalized identity, unsafe URL rejection | `go test ./internal/http -run TestGlobalSourceParsing -count=1`: `undefined: ParseGlobalSource` | Same command passed |
| Default branch and explicit reference resolution to a full SHA, authentication errors | `go test ./internal/http -run TestGlobalResolve -count=1`: `ResolveGlobalSource undefined` | `go test ./internal/http -run 'TestGlobalSourceParsing|TestGlobalResolve' -count=1` passed |
| Canonical selection, explicit path, catalogue reuse, ambiguity, frontmatter and hooks, complete/safe trees | `go test ./internal/http -run 'TestGlobalDiscovery|TestGlobalSelection' -count=1`: missing discovery/selection APIs | Same tests and previous increments passed |
| Byte-preserving bundles, executable modes, root licence, shared SHA tree, links/plugins/count/size rejection | `go test ./internal/http -run TestGlobalBundle -count=1`: `DownloadGlobalSkill undefined` | `go test ./internal/http -run TestGlobal -count=1` passed |
| Dot-prefixed repositories, reference traversal, mutated cached revision | `go test ./internal/http -run 'TestGlobalSourceAcceptsDot|TestGlobalBundleRejectsMutated' -count=1`: rejected valid `.github`; attempted download from mutable `main` | Same tests passed; no mutable-reference download |
| Source ancestors, Unicode-equivalent paths, Windows device variants | `go test ./internal/http -run 'TestGlobalBundleRejectsNonDirectory|TestGlobalBundleRejectsUnicode' -count=1`: accepted symlink ancestor; attempted unsafe downloads | Same tests passed before downloads |
| Unknown metadata extensions remain unchanged and visible | `go test ./internal/http -run TestGlobalBundleReportsUnknown -count=1`: missing warning for `future-agent-setting` | Same test passed, including original CRLF bytes |
| Explicit SHA cannot resolve to another SHA | `go test ./internal/http -run TestGlobalResolveRejectsDifferent -count=1`: `explicit commit changed during resolution` | Same test passed |

Final source validation: `go test ./internal/http/... ./internal/scanner/... -count=1` passed. `go test -race ./internal/http -run TestGlobal -count=1` passed. `go vet ./internal/http/...` passed.

One initial attempt of the final RED was blocked by another workstream's incomplete filesystem transaction increment (`pendingGlobalTransactions` and journal helpers not yet defined). This compilation failure was not counted as the expected RED. After that increment compiled, the actual assertion failed for the intended SHA mismatch before the fix.

Implementation reuses the existing credential resolver, bounded HTTP transport, provider URL builder, catalogue parser, multi-stack scanner and frontmatter parser. New code is confined to `internal/http/global*.go`; local provider behavior is unchanged.

Limits are 16 MiB per HTTP response (existing transport), 64 MiB cumulative bytes per selected skill including provenance, and 4096 files including provenance. Bounds are checked both against tree metadata before downloads and against actual downloaded byte totals. Trees marked truncated fail explicitly. Source metadata and files are only read; downloaded scripts are never run.

GitHub repository identities are normalized to lowercase because GitHub owner/repository names are case-insensitive. A resolved SHA is checked before every tree operation, and the in-memory tree snapshot is bound to its source and SHA. Explicit SHA resolution must return exactly that SHA.

Active-manifest rejection is deliberately conservative: files named `plugin.json`, `gemini-extension.json` or `hooks.json` anywhere inside a selected bundle are refused, including examples with those names. Root manifests outside the selected skill are neither downloaded nor installed. Repository-root `SKILL.md` is unsupported. Root licence/notice files are returned separately as provenance.

Additional integration regressions handed to the coordinator in RED: `go test ./cmd -run TestGlobalReinstall -count=1` failed as expected for independently installed skills at different SHAs, a single installed skill removed from the default branch, and wildcard extension of existing consumers alongside a new skill. The tests are in `cmd/global_reinstall_test.go`; the coordinator owns the command implementation and its GREEN verification.
