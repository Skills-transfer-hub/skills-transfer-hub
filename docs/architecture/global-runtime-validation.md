# Global skill runtime validation

Executed on 25 October 2026 on macOS 27.0.1 (26A434), Darwin 27.0.0, arm64. These checks are separate from the automated STH installation/lifecycle tests. No model turn, paid inference, downloaded script, actual user installation or real user configuration was used.

| Surface | Version observed | Result | Limit |
| --- | --- | --- | --- |
| Codex CLI / private app-server | `codex-cli 0.164.0-alpha.11.1` | Global fixture discovered from two empty projects: `scope=user`, `enabled=true`, no discovery errors | Discovery only; invocation and model loading were not tested |
| Claude Code | `2.1.293 (Claude Code)` | Version/help and SDK initialization succeeded; custom user/project fixtures were not enumerated by the isolated `--bare` initialization command list | Normal-mode discovery and invocation remain unverified; `--bare` is not a normal Claude session |
| Cursor CLI | Not found on PATH (`cursor-agent`, `agent`) | Not executed | IDE not tested |
| Copilot CLI / VS Code | CLI not found on PATH (`copilot`, `code`) | Not executed | IDE not tested |
| Gemini CLI | Not found on PATH (`gemini`) | Not executed | No runtime certification |
| Antigravity CLI / App / IDE | CLI not found on PATH (`antigravity`, `antigravity-cli`) | Not executed | App/IDE not tested |
| Linux / Windows | Not executed on these systems | Not runtime-verified | Portable path validation and compilation/tests are separate evidence |

## Isolation

Each probe used an automatically deleted temporary directory with a dedicated home and empty project directories. The subprocess environment was constructed from an explicit allowlist: a minimal system PATH, temporary HOME/USERPROFILE and XDG configuration/cache roots, and a temporary CODEX_HOME or CLAUDE_CONFIG_DIR. No credential environment variables were inherited. All proxy variables pointed to the closed loopback endpoint `http://127.0.0.1:9`; the Claude API base URL also pointed there. No authentication state was copied. Processes were terminated after their metadata responses.

The fixture was authored directly in the temporary home to test the consumer's discovery path. It was not a remote Ponytail download or an end-to-end STH install test. It contained only a small `SKILL.md` with `name: sth-global-probe`, a description and inert text. No scripts, hooks, plugins or MCP servers were supplied.

## Codex evidence

Executable: `/Applications/ChatGPT.app/Contents/Resources/codex-cli/CodexCLI.app/Contents/MacOS/codex`.

The local protocol schema was read from `codex app-server generate-json-schema --out <temporary-directory>`. A new private `codex app-server --listen stdio://` process received only:

1. `initialize` with client name `sth-isolated-global-validation` and version `1`.
2. `initialized` notification.
3. `skills/list` with `cwds` for two empty projects and `forceReload: true`.
4. A second `skills/list` after adding a project skill with the same name.

Both initially empty projects returned the same fixture at `<temporary-home>/.agents/skills/sth-global-probe/SKILL.md`, with `scope: "user"`, `enabled: true`, `pluginId: null`, and an empty errors array.

After creating `<project-two>/.agents/skills/sth-global-probe/SKILL.md`, the second query returned **two entries**: the project fixture with `scope: "repo"` and the global fixture with `scope: "user"`. This establishes visible coexistence in that runtime version, not an invocation priority or deduplication guarantee. No `thread/start` or `turn/start` request was sent.

## Claude evidence and limitation

Executable: `/Users/hugo/.local/bin/claude`.

The metadata-only process used `--bare --print --input-format stream-json --output-format stream-json --verbose --strict-mcp-config --mcp-config '{"mcpServers":{}}'`. Its only input was a `control_request` with subtype `initialize`. The response was `control_response`, subtype `success`, with 48 available commands. Neither the fixture under the temporary `~/.claude/skills` nor a second fixture under the temporary project's `.claude/skills` appeared in that list. No user message was sent.

The installed help states that `--bare` skips hooks, plugin synchronization, background prefetch, keychain reads and automatic context discovery. The experiment therefore does not establish normal-session skill discovery, and the CLI's files-installed result must remain `discovery-unverified` for Claude. A first setup attempt with `--max-budget-usd 0` was refused by the runtime because it requires a strictly positive number; it exited before initialization. The final probe omitted that flag and remained offline without credentials or a user turn.

No results here certify cross-agent duplicate handling, native invocation, plugin-equivalent behavior, permanent activation, or untested operating systems. The CLI should continue to report the installation's runtime discovery as unverified unless a specific runtime verification is attached to that installation.
