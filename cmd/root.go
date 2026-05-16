package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	sthfs "sth/internal/fs"
	sthhttp "sth/internal/http"
	sthlog "sth/internal/log"
	"strings"
	"time"
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
		summary: "Configure pas a pas un depot distant heberge supporte pour le projet courant",
		details: []string{
			"Providers heberges supportes : github.com, gitlab.com, dev.azure.com et bitbucket.org.",
			"Le wizard affiche les URLs officielles pour creer le token/API token, la permission minimale attendue et les variables d'environnement reconnues.",
			"Le repository_id accepte un identifiant canonique ou une URL hebergee supportee. Les hosts self-hosted sont rejetes.",
			"La guidance provider peut etre relue pendant le wizard et aucun secret n'est persiste dans .sth/project.json.",
			"Au premier init, le wizard propose un menu télémétrie (off / anonymous / connected). Mode connected enchaine sur un sub-prompt `sth login`.",
			"--no-cloud-prompt : skippe le sub-prompt login même quand le mode connecté est choisi (CI, wrappers).",
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
		summary: "Lit SKILLS.md ou détecte les bundles category/folder/SKILL.md du dépôt courant",
		details: []string{
			"SKILLS.md garde toujours la priorité quand il existe et se parse correctement.",
			"Si SKILLS.md est absent, STH scanne les bundles compatibles et affiche leurs références canoniques category/folder.",
			"Avec --json, retourne l'état des ressources installées (resource_name, provider_id, status, resolved_version, pinned_version) — sémantique différente du mode texte.",
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
		summary: "Installe une ressource du catalogue distant ou un bundle compatible dans le projet courant",
		details: []string{
			"Accepte un nom catalogue classique ou une référence bundle canonique category/folder.",
			"Pour un bundle détecté par scan, STH synchronise tous les fichiers du dossier source et suit leur manifeste dans sth-state.json.",
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
		summary: "Met à jour les ressources suivies ou le binaire STH lui-même",
		details: []string{
			"--fail-on-changes : si une ressource gérée a été modifiée localement (SHA256 ne correspond plus), exit 1 sans appliquer la mise à jour.",
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
		summary: "Affiche l'état de chaque ressource installée (à jour, obsolète, modifiée localement)",
		examples: []string{
			"sth status",
			"sth status --json",
		},
		flags: []string{"--json"},
	},
	{
		name:    "remove",
		usage:   "sth remove <resource-name> [--yes]",
		summary: "Supprime une ressource installée et ses fichiers gérés",
		examples: []string{
			"sth remove skills/analyse-risque",
			"sth remove skills/analyse-risque --yes",
		},
		flags: []string{"--yes"},
	},
	{
		name:    "buddy",
		usage:   "sth buddy [--watch]",
		summary: "Affiche l'etat du projet STH sous forme de compagnon visuel",
		details: []string{
			"Sans flag, affiche l'etat courant et quitte immediatement.",
			"--watch lance le mode tableau de bord : surveille .sth/ et le repertoire d'etat, se rafraichit a chaque changement.",
			"Quittez avec Ctrl+C.",
		},
		examples: []string{
			"sth buddy",
			"sth buddy --watch",
		},
	},
	{
		name:    "help",
		usage:   "sth help [commande]",
		summary: "Affiche l'aide générale ou l'aide ciblée d'une commande visible",
		examples: []string{
			"sth help",
			"sth help list",
		},
	},
	{
		name:    "version",
		usage:   "sth version [--json]",
		summary: "Affiche la signature de version minimale du CLI",
		examples: []string{
			"sth version",
			"sth version --json",
		},
		flags: []string{"--json"},
	},
	{
		name:    "doctor",
		usage:   "sth doctor",
		summary: "Diagnostique la configuration STH du projet courant (token, config, état, accès provider)",
		details: []string{
			"Liste pass/fail pour chaque vérification avec une étape de remédiation en cas d'échec.",
			"Si aucun project.json n'existe, les vérifications provider et état sont marquées [SKIP].",
		},
		examples: []string{
			"sth doctor",
		},
	},
	{
		name:    "telemetry",
		usage:   "sth telemetry <status|off|anon|connect|output>",
		summary: "Gère le mode de télémétrie (off / anonymous / connected) et lit les enregistrements",
		details: []string{
			"status : affiche le mode actif, la date de décision et — si connecté — le rattachement STH Cloud.",
			"off : désactive tout envoi.",
			"anon : envoie ts + version CLI + nom de commande sur /v1/usage/anon, sans clé. Pas de dashboard.",
			"connect : passe en mode connecté et lance le sub-prompt `sth login` pour rattacher la clé.",
			"on : alias déprécié de `anon` (warning stderr).",
			"output [--limit=N] : récupère depuis le cloud les derniers enregistrements liés à la clé courante (mode connected requis).",
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
		summary: "Génère un script de complétion shell pour STH",
		details: []string{
			"Source la sortie dans votre shell pour activer la complétion : bash, zsh ou powershell.",
		},
		examples: []string{
			"sth completion bash",
			"sth completion zsh",
			"sth completion powershell",
		},
	},
	{
		name:    "pack",
		usage:   "sth pack init",
		summary: "Scaffolde un repo de skills compatible STH (Pro)",
		details: []string{
			"Disponible sur l'offre Pro. STH_PRO_GATE=0 active la commande en local pour le développement.",
			"Deux layouts proposés : flat (anthropics/skills) et multi-stack (skills + agents + commands + instructions, à la Lore.Skills).",
			"Le manifest.json racine sert d'heuristique forte au MultiStackScanner pour les consommateurs.",
		},
		examples: []string{
			"sth pack init",
		},
	},
}

type app struct {
	stdin       io.Reader
	stdout      io.Writer
	stderr      io.Writer
	client      *sthhttp.Client
	interactive func() bool
	verbose     sthlog.Verbose
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
	if a.interactive != nil {
		return a.interactive()
	}

	f, ok := a.stdin.(*os.File)
	if !ok {
		return false
	}
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return stat.Mode()&os.ModeCharDevice != 0
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

	if err := a.enforceAlphaGate(context.Background(), args); err != nil {
		a.writeStderr(fmt.Sprintf("Erreur : %v", err))
		return err
	}

	if len(args) == 0 {
		if a.isInteractiveSession() {
			return a.runGuidedMenu()
		}
		a.writeStdout(a.nextStepGuidanceLines()...)
		return nil
	}

	var (
		err      error
		showHelp bool
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
		err = a.runInstall(args[1:])
		a.recordOperationResult("install", resourceFromArgs(args[1:]), err)
	case "update":
		if len(args) >= 2 && isHelpFlag(args[1]) {
			a.writeStdout(commandHelpText("update"))
			return nil
		}
		if len(args) >= 2 && args[1] == "self" {
			if len(args) != 2 {
				err = fmt.Errorf("usage : sth update [self]")
				break
			}
			err = a.runSelfUpdate()
		} else {
			err = a.runUpdate(args[1:])
			a.recordOperationResult("update", "", err)
		}
	case "status":
		if len(args) >= 2 && isHelpFlag(args[1]) {
			a.writeStdout(commandHelpText("status"))
			return nil
		}
		err = a.runStatus(args[1:])
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
			if _, ok := findCommandSpec(args[1]); !ok {
				err = fmt.Errorf("commande inconnue pour help : %q", args[1])
				showHelp = true
				break
			}
			a.writeStdout(commandHelpText(args[1]))
			return nil
		default:
			err = fmt.Errorf("usage : sth help [commande]")
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
			err = fmt.Errorf("usage : sth stockholm")
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
	case "--help", "-h":
		a.writeStdout(helpText())
		return nil
	default:
		err = fmt.Errorf("commande inconnue : %q", args[0])
		showHelp = true
	}

	if err != nil {
		lines := []string{fmt.Sprintf("Erreur : %v", err)}
		if showHelp {
			lines = append(lines, "", helpText())
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
//                 even when the project has zero providers configured —
//                 the user can contribute to product stats without ever
//                 running sth init.
//   - connected:  POST /v1/usage with the keychain Bearer. Requires a
//                 sth-cloud provider entry in .sth/project.json. If the
//                 entry is missing we degrade silently (the wizard hint
//                 at init already pointed the user at `sth login`).
//
// Failures are swallowed — telemetry must never block a CLI command.
// Runs synchronously with a 500ms timeout: a goroutine here would be killed
// when main returns (this is the last call before exit), so we'd lose nearly
// every record.
func (a *app) reportCloudUsageBestEffort(command string) {
	if command == "login" || command == "logout" || command == "version" || command == "help" {
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
	}
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
	fmt.Fprint(a.stdout, renderMessage(lines...))
}

func (a *app) writeStderr(lines ...string) {
	fmt.Fprint(a.stderr, renderMessage(lines...))
}

func helpText() string {
	var builder strings.Builder
	builder.WriteString("STH\n")
	builder.WriteString("Sans argument, `sth` ouvre le menu guidé en TTY et affiche la prochaine étape utile hors TTY.\n\n")
	builder.WriteString("Commandes visibles :\n")
	for _, spec := range commandCatalog {
		if spec.hidden {
			continue
		}
		builder.WriteString(fmt.Sprintf("  %-20s %s\n", commandDisplayName(spec), spec.summary))
	}
	builder.WriteString("\nExemples :\n")
	for _, example := range usageExamples() {
		builder.WriteString(fmt.Sprintf("  %s\n", example))
	}

	return strings.TrimRight(builder.String(), "\n")
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
		builder.WriteString("\nExemples :\n")
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
		if a.client != nil {
			a.client.SetVerbose(v)
		}
	}
	return filtered
}
