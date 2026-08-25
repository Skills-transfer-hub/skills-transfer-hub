package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"

	sthfs "sth/internal/fs"
	sthhttp "sth/internal/http"
	sthlog "sth/internal/log"
)

func contextWithTimeout(ms int) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), time.Duration(ms)*time.Millisecond)
}

type commandSpec struct {
	name     string
	usage    string
	summary  string
	details  []string
	examples []string
	flags    []string
	hidden   bool
}

var commandCatalog = []commandSpec{
	{
		name:    "init",
		usage:   "sth init [--no-cloud-prompt]",
		summary: "Configure a supported hosted remote repository for the current project step by step",
		details: []string{
			"Supported hosted providers: github.com, gitlab.com, dev.azure.com, and bitbucket.org.",
			"The wizard shows the official token or API-token URLs, the minimum required permission, and the supported environment variables.",
			"repository_id accepts a canonical identifier or a supported hosted URL. Self-hosted domains are rejected.",
			"The wizard asks which AI tools and agents to target (Claude Code, GitHub Copilot, OpenAI Codex, Cursor, Google Antigravity), preselects detected tools, and derives the installation directory automatically.",
			"Skills are deduplicated: .claude/skills is the shared hub read by Claude, Cursor, and Copilot; .agents/skills is used by Codex and Gemini/Antigravity. init also updates .gitignore for STH state files.",
			"Provider guidance remains available throughout the wizard, and no secret is stored in .sth/project.json.",
			"On first init, the wizard offers telemetry modes off, anonymous, and connected. Connected mode then opens an `sth login` prompt.",
			"--no-cloud-prompt skips the login prompt even when connected mode is selected (CI and wrappers).",
		},
		examples: []string{
			"sth init",
			"sth init --no-cloud-prompt",
		},
		flags: []string{"--no-cloud-prompt"},
	},
	{
		name:    "list",
		usage:   "sth list [--json]",
		summary: "List the configured remote provider repository catalog (requires `sth init`)",
		details: []string{
			"Text mode reads SKILLS.md from the remote provider repository (GitHub, GitLab, and others). If it is missing, STH scans the remote tree and infers category/folder bundles. Requires `sth init` and network access.",
			"To generate a local SKILLS.md from the current repository, use `catalog init` instead.",
			"With --json, returns the state of locally installed resources (resource_name, provider_id, status, resolved_version, pinned_version). This differs from text mode.",
		},
		examples: []string{
			"sth list",
			"sth list --json",
		},
		flags: []string{"--json"},
	},
	{
		name:    "install",
		usage:   "sth install <resource-ref> [--dry-run] [--force]",
		summary: "Install a remote catalog resource or compatible bundle in the current project",
		details: []string{
			"Accepts a standard catalog name or a canonical category/folder bundle reference.",
			"For a bundle discovered by scanning, STH synchronizes every file in the source directory and tracks its manifest in sth-state.json.",
		},
		examples: []string{
			"sth install react/form-wizard",
			"sth install skills/analyse-risque",
			"sth install skills/analyse-risque --dry-run",
		},
		flags: []string{"--dry-run", "--force", "--json"},
	},
	{
		name:    "update",
		usage:   "sth update [self] [--dry-run] [--fail-on-changes] [--json]",
		summary: "Update tracked resources or the STH binary itself",
		details: []string{
			"--fail-on-changes exits with status 1 without updating when a managed resource has local changes (its SHA-256 no longer matches).",
		},
		examples: []string{
			"sth update",
			"sth update --dry-run",
			"sth update --fail-on-changes",
			"sth update self",
		},
		flags: []string{"--dry-run", "--fail-on-changes", "--json"},
	},
	{
		name:    "status",
		usage:   "sth status [--json]",
		summary: "Show the status of each installed resource (current, outdated, or locally modified)",
		examples: []string{
			"sth status",
			"sth status --json",
		},
		flags: []string{"--json"},
	},
	{
		name:    "remove",
		usage:   "sth remove <resource-name> [--yes]",
		summary: "Remove an installed resource and its managed files",
		examples: []string{
			"sth remove skills/analyse-risque",
			"sth remove skills/analyse-risque --yes",
		},
		flags: []string{"--yes"},
	},
	{
		name:    "buddy",
		usage:   "sth buddy [--watch]",
		summary: "Show the STH project state as a visual companion",
		details: []string{
			"Without flags, shows the current state and exits immediately.",
			"--watch starts dashboard mode, watches .sth/ and the state directory, and refreshes after every change.",
			"Exit with Ctrl+C.",
		},
		examples: []string{
			"sth buddy",
			"sth buddy --watch",
		},
	},
	{
		name:    "help",
		usage:   "sth help [command]",
		summary: "Show general help or detailed help for a visible command",
		examples: []string{
			"sth help",
			"sth help list",
		},
	},
	{
		name:    "version",
		usage:   "sth version [--json]",
		summary: "Show the CLI version signature",
		examples: []string{
			"sth version",
			"sth version --json",
		},
		flags: []string{"--json"},
	},
	{
		name:    "doctor",
		usage:   "sth doctor",
		summary: "Diagnose the current project's STH setup (token, config, state, and provider access)",
		details: []string{
			"Shows pass or fail for every check, with a remediation step after each failure.",
			"If project.json does not exist, provider and state checks are marked [SKIP].",
		},
		examples: []string{
			"sth doctor",
		},
	},
	{
		name:    "telemetry",
		usage:   "sth telemetry <status|off|anon|connect|output>",
		summary: "Manage telemetry mode (off, anonymous, or connected) and read usage records",
		details: []string{
			"status shows the active mode, decision date, and STH Cloud link when connected.",
			"off disables all telemetry submissions.",
			"anon sends the timestamp, CLI version, and command name to /v1/usage/anon without a key. No dashboard is available.",
			"connect switches to connected mode and opens an `sth login` prompt to link the key.",
			"on is a deprecated alias for `anon` and prints a warning to stderr.",
			"output [--limit=N] reads recent records for the current key from the cloud (connected mode required).",
		},
		examples: []string{
			"sth telemetry status",
			"sth telemetry anon",
			"sth telemetry connect",
			"sth telemetry output --limit=20",
		},
		flags: []string{"--limit"},
	},
	{
		name:    "completion",
		usage:   "sth completion <bash|zsh|powershell>",
		summary: "Generate a shell completion script for STH",
		details: []string{
			"Source the output in your shell to enable completion for Bash, Zsh, or PowerShell.",
		},
		examples: []string{
			"sth completion bash",
			"sth completion zsh",
			"sth completion powershell",
		},
	},
	{
		name:    "pack",
		usage:   "sth pack <init|promote>",
		summary: "Create or extend an STH-compatible Git pack locally",
		details: []string{
			"Available locally and offline in the open-source Starter plan.",
			"Offers two layouts: flat (anthropics/skills) and multi-stack (skills, agents, commands, and instructions, as in Lore.Skills).",
			"The root manifest.json provides a strong MultiStackScanner hint for consumers.",
			"promote intentionally turns an item from .sth/local/code/library.json into a skills/<slug> bundle after preview, without committing or pushing.",
		},
		examples: []string{
			"sth pack init",
			"sth pack promote <library-id> --dry-run",
			"sth pack promote <library-id> --output skills/review-pr --yes",
		},
		flags: []string{"--output", "--dry-run", "--yes"},
	},
	{
		name:    "catalog",
		usage:   "sth catalog init [--force]",
		summary: "Generate SKILLS.md from the current repository's category/skill/SKILL.md structure",
		details: []string{
			"Includes only directories that contain a SKILL.md; descriptions come from front matter.",
			"The generated SKILLS.md is a single table sorted by category and name, ready for `sth list`.",
			"If SKILLS.md already exists, STH asks before overwriting it; --force skips confirmation. Manual edits will be lost.",
		},
		examples: []string{
			"sth catalog init",
			"sth catalog init --force",
		},
		flags: []string{"--force"},
	},
	{
		name:    "new",
		usage:   "sth new <category>/<name>",
		summary: "Scaffold a compliant skill in <category>/<name>/SKILL.md",
		details: []string{
			"Creates the full directory path and a prefilled SKILL.md with name and description front matter plus a ## Usage section.",
			"Refuses to overwrite an existing directory. Each path segment may contain letters, digits, hyphens, and underscores.",
		},
		examples: []string{
			"sth new prompts/code-review",
		},
	},
	{
		name:    "alpha",
		usage:   "sth alpha request",
		summary: "Request access to paid features during the Alpha (account required)",
		details: []string{
			"`sth alpha request` sends the STH team a request for paid-plan access.",
			"You must be signed in first with `sth login`.",
			"Requests are approved or declined in the dashboard. After approval, permissions refresh on the next CLI request.",
		},
		examples: []string{
			"sth alpha request",
		},
	},
	{
		name:    "org",
		usage:   "sth org <current|list|use>",
		summary: "Manage the CLI's active organization (multi-organization)",
		details: []string{
			"current shows the active organization (ID, name, role, and plan). Cached data remains available offline.",
			"list shows available organizations and marks the active one (requires the multi_org capability).",
			"use <id|name> switches the active organization after server-side membership validation.",
		},
		examples: []string{
			"sth org current",
			"sth org list",
			"sth org use \"Acme Corp\"",
		},
		flags:  []string{"--json"},
		hidden: true,
	},
	{
		name:    "mcp",
		usage:   "sth mcp [install]",
		summary: "Connect AI agents to local or shared STH assets through MCP",
		details: []string{
			"sth mcp starts the MCP stdio server (library_search, library_get, library_save, asset_search, asset_get). Without an account, it uses only local project data.",
			"STH_MCP_LOCAL_ONLY=1 forces local mode even when a Cloud connection exists.",
			"sth mcp install writes ./.mcp.json for Claude Code and shows the configuration to paste into Codex (~/.codex/config.toml) and Gemini (~/.gemini/settings.json).",
			"After `sth login`, agents can also use the active organization's library under control-plane role and quota rules.",
			"For Claude on the web or mobile, use the remote connector instead: Settings → Connectors → Add custom connector → https://<domain>/api/mcp.",
		},
		examples: []string{
			"sth mcp install",
			"sth mcp",
		},
	},
	{
		name:    "config",
		usage:   "sth config <show|pull|push> [--dry-run]",
		summary: "Synchronize shared organization configuration (Team)",
		details: []string{
			"show displays the effective configuration and each field's origin (organization or local).",
			"pull applies organization-managed fields over local configuration while preserving local resources. --dry-run previews the result.",
			"push publishes the local subset as the organization baseline (requires fleet_sync and an owner or admin role).",
		},
		examples: []string{
			"sth config show",
			"sth config pull --dry-run",
			"sth config push",
		},
		flags:  []string{"--dry-run"},
		hidden: true,
	},
}

type app struct {
	stdin       io.Reader
	stdout      io.Writer
	stderr      io.Writer
	client      *sthhttp.Client
	interactive func() bool
	verbose     sthlog.Verbose
	verboseMode bool
	progress    *installProgress
	fanoutJSON  *fanoutJSONAccumulator

	updateRefreshOnce sync.Once
}

func newApp(stdin io.Reader, stdout, stderr io.Writer, client *sthhttp.Client) *app {
	if stdin == nil {
		stdin = os.Stdin
	}
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if client == nil {
		client = sthhttp.NewClient(nil)
	}

	v := sthlog.NewVerbose(stderr, false)
	client.SetVerbose(v)
	return &app{
		stdin:   stdin,
		stdout:  stdout,
		stderr:  stderr,
		client:  client,
		verbose: v,
	}
}

func (a *app) isInteractiveSession() bool {
	return a.isInteractiveOutput(a.stdout)
}

func (a *app) isInteractiveErrorSession() bool {
	return a.isInteractiveOutput(a.stderr)
}

func (a *app) isInteractiveOutput(output io.Writer) bool {
	if a.interactive != nil {
		return a.interactive()
	}

	inputFile, inputIsFile := a.stdin.(*os.File)
	outputFile, outputIsFile := output.(*os.File)
	if !inputIsFile || !outputIsFile {
		return false
	}
	return term.IsTerminal(int(inputFile.Fd())) && term.IsTerminal(int(outputFile.Fd()))
}

func (a *app) nextStepGuidanceLines() []string {
	return startupGuidanceLines(a.loadGuidedStartupState())
}

// Run routes CLI arguments to the appropriate STH command.
func Run(args []string) error {
	return newApp(os.Stdin, os.Stdout, os.Stderr, nil).run(args)
}

func (a *app) run(args []string) error {
	args = a.applyGlobalFlags(args)

	if len(args) == 0 {
		if a.isInteractiveSession() {
			return a.runGuidedMenu()
		}
		a.writeStdout(a.nextStepGuidanceLines()...)
		return nil
	}

	var (
		err             error
		showHelp        bool
		suggestionInput = args[0]
	)

	switch args[0] {
	case "init":
		if len(args) == 2 && isHelpFlag(args[1]) {
			a.writeStdout(commandHelpText("init"))
			return nil
		}
		err = a.runInit(args[1:]...)
	case "list":
		if len(args) >= 2 && isHelpFlag(args[1]) {
			a.writeStdout(commandHelpText("list"))
			return nil
		}
		err = a.runListWithArgs(args[1:])
	case "install":
		if len(args) >= 2 && isHelpFlag(args[1]) {
			a.writeStdout(commandHelpText("install"))
			return nil
		}
		installFlags, _ := parseFlags(args[1:], "--dry-run")
		err = a.runInstall(args[1:])
		if !installFlags["--dry-run"] {
			a.recordOperationResult("install", resourceFromArgs(args[1:]), err)
		}
	case "update":
		if len(args) >= 2 && isHelpFlag(args[1]) {
			a.writeStdout(commandHelpText("update"))
			return nil
		}
		if len(args) >= 2 && args[1] == "self" {
			if len(args) != 2 {
				err = fmt.Errorf("usage: sth update [self]")
				break
			}
			err = a.runSelfUpdate()
		} else {
			updateFlags, _ := parseFlags(args[1:], "--dry-run")
			err = a.runUpdate(args[1:])
			if !updateFlags["--dry-run"] {
				a.recordOperationResult("update", "", err)
			}
		}
	case "status":
		if len(args) >= 2 && isHelpFlag(args[1]) {
			a.writeStdout(commandHelpText("status"))
			return nil
		}
		err = a.runStatus(args[1:])
	case "bridge":
		err = a.runBridge(args[1:])
	case "remove":
		if len(args) >= 2 && isHelpFlag(args[1]) {
			a.writeStdout(commandHelpText("remove"))
			return nil
		}
		err = a.runRemove(args[1:])
		a.recordOperationResult("remove", resourceFromArgs(args[1:]), err)
	case "help":
		switch len(args) {
		case 1:
			a.writeStdout(helpText())
			return nil
		case 2:
			if args[1] == "login" {
				a.writeStdout(loginHelpText())
				return nil
			}
			if _, ok := findCommandSpec(args[1]); !ok {
				err = fmt.Errorf("unknown command for help: %q", args[1])
				showHelp = true
				suggestionInput = args[1]
				break
			}
			a.writeStdout(commandHelpText(args[1]))
			return nil
		default:
			err = fmt.Errorf("usage: sth help [command]")
		}
	case "version":
		if len(args) >= 2 && isHelpFlag(args[1]) {
			a.writeStdout(commandHelpText("version"))
			return nil
		}
		err = a.runVersion(args[1:])
	case "login":
		if len(args) >= 2 && isHelpFlag(args[1]) {
			a.writeStdout(loginHelpText())
			return nil
		}
		err = a.runLogin(args[1:])
	case "logout":
		err = a.runLogout(args[1:])
	case "telemetry":
		if len(args) >= 2 && isHelpFlag(args[1]) {
			a.writeStdout(telemetryHelpText())
			return nil
		}
		err = a.runTelemetry(args[1:])
	case "stockholm":
		if len(args) != 1 {
			err = fmt.Errorf("usage: sth stockholm")
			break
		}
		err = a.runStockholm()
	case "buddy":
		if len(args) >= 2 && isHelpFlag(args[1]) {
			a.writeStdout(commandHelpText("buddy"))
			return nil
		}
		err = a.runBuddy(args[1:])
	case "doctor":
		if len(args) >= 2 && isHelpFlag(args[1]) {
			a.writeStdout(commandHelpText("doctor"))
			return nil
		}
		err = a.runDoctor(args[1:])
	case "completion":
		if len(args) >= 2 && isHelpFlag(args[1]) {
			a.writeStdout(commandHelpText("completion"))
			return nil
		}
		err = a.runCompletion(args[1:])
	case "pack":
		err = a.runPack(args[1:])
	case "catalog":
		err = a.runCatalog(args[1:])
	case "new":
		err = a.runNew(args[1:])
	case "alpha":
		if len(args) >= 2 && isHelpFlag(args[1]) {
			a.writeStdout(commandHelpText("alpha"))
			return nil
		}
		err = a.runAlpha(args[1:])
	case "org":
		err = a.runOrg(args[1:])
	case "mcp":
		if len(args) >= 2 && isHelpFlag(args[1]) {
			a.writeStdout(commandHelpText("mcp"))
			return nil
		}
		err = a.runMCP(args[1:])
	case "config":
		err = a.runConfig(args[1:])
	case "--help", "-h":
		a.writeStdout(helpText())
		return nil
	default:
		err = fmt.Errorf("unknown command: %q", args[0])
		showHelp = true
	}

	if err != nil {
		lines := []string{fmt.Sprintf("Error: %v", err)}
		if showHelp {
			lines = append(lines, commandSuggestions(suggestionInput)...)
			lines = append(lines, "Run `sth help` to list available commands.")
		}
		a.writeStderr(lines...)
		return err
	}

	a.reportCloudUsageBestEffort(args[0])
	return nil
}

// reportCloudUsageBestEffort sends a single usage record to the STH Cloud
// API after every successful CLI command. Routing depends on the user's
// telemetry mode:
//
//   - off:        nothing is sent.
//   - anonymous:  POST /v1/usage/anon without any Bearer header. Works
//     even when the project has zero providers configured —
//     the user can contribute to product stats without ever
//     running sth init.
//   - connected:  POST /v1/usage with the keychain Bearer. Requires a
//     sth-cloud provider entry in .sth/project.json. If the
//     entry is missing we degrade silently (the wizard hint
//     at init already pointed the user at `sth login`).
//
// Failures are swallowed — telemetry must never block a CLI command.
// Runs synchronously with a 500ms timeout: a goroutine here would be killed
// when main returns (this is the last call before exit), so we'd lose nearly
// every record.
func (a *app) reportCloudUsageBestEffort(command string) {
	if command == "bridge" || command == "pack" || command == "login" || command == "logout" || command == "version" || command == "help" || command == "telemetry" {
		return
	}
	mode := sthfs.TelemetryModeActive()
	if mode == sthfs.TelemetryOff {
		return
	}

	ctx, cancel := contextWithTimeout(500)
	defer cancel()
	cliVersion := valueOrDefault(version, "dev")

	switch mode {
	case sthfs.TelemetryAnonymous:
		// No provider lookup: anonymous mode is intentionally project-agnostic.
		_ = a.client.ReportCloudUsageAnonymous(ctx, cliVersion, command)
		recordLocalUsage(cliVersion, command)
	case sthfs.TelemetryConnected:
		cfg, err := sthfs.ReadProjectConfigV2()
		if err != nil || len(cfg.Providers) == 0 {
			return
		}
		profile, found := findCloudProviderProfile(cfg)
		if !found {
			return
		}
		_ = a.client.ReportCloudUsage(ctx, profile, cliVersion, command)
		recordLocalUsage(cliVersion, command)
	}
}

func recordLocalUsage(cliVersion, command string) {
	_ = sthfs.AppendUsageLogEntry(sthfs.UsageLogRecord{
		Ts:          time.Now().UTC().Format(time.RFC3339),
		CLIVersion:  cliVersion,
		CommandName: command,
	})
}

func findCommandSpec(name string) (commandSpec, bool) {
	for _, spec := range commandCatalog {
		if spec.name == name {
			return spec, true
		}
	}

	return commandSpec{}, false
}

func isHelpFlag(arg string) bool {
	return arg == "--help" || arg == "-h"
}

func renderMessage(lines ...string) string {
	if len(lines) == 0 {
		return ""
	}

	return strings.Join(lines, "\n") + "\n"
}

func (a *app) writeStdout(lines ...string) {
	message := renderMessage(lines...)
	if a.progress.print(message) {
		return
	}
	a.progress.stop()
	fmt.Fprint(a.stdout, message)
}

func (a *app) writeStderr(lines ...string) {
	message := renderMessage(lines...)
	if a.progress.printError(message) {
		return
	}
	a.progress.stop()
	fmt.Fprint(a.stderr, message)
}

func helpText() string {
	var builder strings.Builder
	builder.WriteString("STH\n")
	builder.WriteString("Without arguments, `sth` opens the guided menu in a TTY and shows the next useful step outside a TTY.\n\n")
	builder.WriteString("Available commands:\n")
	builder.WriteString(commandListText())
	builder.WriteString("\nExamples:\n")
	for _, example := range usageExamples() {
		builder.WriteString(fmt.Sprintf("  %s\n", example))
	}

	return strings.TrimRight(builder.String(), "\n")
}

const helpInlineUsageMax = 36

func commandListText() string {
	inlineColumn := 0
	for _, spec := range commandCatalog {
		if spec.hidden {
			continue
		}
		width := len(commandDisplayName(spec))
		if width <= helpInlineUsageMax && width > inlineColumn {
			inlineColumn = width
		}
	}

	var builder strings.Builder
	for _, spec := range commandCatalog {
		if spec.hidden {
			continue
		}
		name := commandDisplayName(spec)
		if len(name) <= inlineColumn {
			builder.WriteString(fmt.Sprintf("  %-*s  %s\n", inlineColumn, name, spec.summary))
			continue
		}
		builder.WriteString(fmt.Sprintf("  %s\n      %s\n", name, spec.summary))
	}
	return builder.String()
}

func usageExamples() []string {
	examples := make([]string, 0, len(commandCatalog))
	for _, spec := range commandCatalog {
		if spec.hidden || len(spec.examples) == 0 {
			continue
		}
		examples = append(examples, spec.examples[0])
	}

	return examples
}

func commandHelpText(name string) string {
	spec, ok := findCommandSpec(name)
	if !ok {
		return helpText()
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("%s\n\n", spec.usage))
	builder.WriteString(fmt.Sprintf("%s\n", spec.summary))
	if len(spec.details) > 0 {
		builder.WriteString("\n")
		for _, detail := range spec.details {
			builder.WriteString(fmt.Sprintf("- %s\n", detail))
		}
	}
	if len(spec.examples) > 0 {
		builder.WriteString("\nExamples:\n")
		for _, example := range spec.examples {
			builder.WriteString(fmt.Sprintf("  %s\n", example))
		}
	}

	return strings.TrimRight(builder.String(), "\n")
}

func commandDisplayName(spec commandSpec) string {
	parts := strings.Fields(spec.usage)
	if len(parts) < 2 {
		return spec.name
	}

	return strings.Join(parts[1:], " ")
}

// recordOperationResult writes a tiny .sth/last-operation.json entry so
// `sth buddy` can render the Error / Done states. Best-effort: errors are
// swallowed because operation reporting must never block the user-visible
// command outcome.
func (a *app) recordOperationResult(op, resource string, err error) {
	rec := sthfs.LastOperation{Op: op, Resource: resource}
	if err == nil {
		rec.Status = sthfs.LastOperationStatusOK
	} else {
		rec.Status = sthfs.LastOperationStatusError
		rec.Message = err.Error()
	}
	_ = sthfs.WriteLastOperation(rec)
}

// resourceFromArgs returns the first positional (non-flag) arg, or "".
// Used to extract the resource name passed to install/remove for the
// last-operation record.
func resourceFromArgs(args []string) string {
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			return a
		}
	}
	return ""
}

// parseFlags strips bool flags from args and returns the remainder as positional.
// knownFlags examples: "--dry-run", "--yes", "--force", "--json"
func parseFlags(args []string, knownFlags ...string) (flags map[string]bool, positional []string) {
	flags = make(map[string]bool)
	for _, arg := range args {
		matched := false
		for _, f := range knownFlags {
			if arg == f {
				flags[f] = true
				matched = true
				break
			}
		}
		if !matched {
			positional = append(positional, arg)
		}
	}
	return
}

// applyGlobalFlags strips --verbose / --debug from args and wires up the
// verbose sink so it routes to stderr. Returns the args without the global
// flags so per-command parsing isn't disturbed.
func (a *app) applyGlobalFlags(args []string) []string {
	if len(args) == 0 {
		return args
	}
	enabled := false
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--verbose" || arg == "--debug" {
			enabled = true
			continue
		}
		filtered = append(filtered, arg)
	}
	if enabled {
		v := sthlog.NewVerbose(a.stderr, true)
		a.verbose = v
		a.verboseMode = true
		if a.client != nil {
			a.client.SetVerbose(v)
		}
	}
	return filtered
}

func (a *app) interactiveProgress() bool {
	return !a.verboseMode && isWriterTTY(a.stdout)
}
