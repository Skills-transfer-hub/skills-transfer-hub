package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	sthfs "sth/internal/fs"
	sthhttp "sth/internal/http"
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
}

var commandCatalog = []commandSpec{
	{
		name:    "init",
		usage:   "sth init",
		summary: "Configure pas a pas un depot distant heberge supporte pour le projet courant",
		details: []string{
			"Providers heberges supportes : github.com, gitlab.com, dev.azure.com et bitbucket.org.",
			"Le wizard affiche les URLs officielles pour creer le token/API token, la permission minimale attendue et les variables d'environnement reconnues.",
			"Le repository_id accepte un identifiant canonique ou une URL hebergee supportee. Les hosts self-hosted sont rejetes.",
			"La guidance provider peut etre relue pendant le wizard et aucun secret n'est persiste dans .sth/project.json.",
		},
		examples: []string{
			"sth init",
		},
	},
	{
		name:    "list",
		usage:   "sth list",
		summary: "Lit SKILLS.md ou détecte les bundles category/folder/SKILL.md du dépôt courant",
		details: []string{
			"SKILLS.md garde toujours la priorité quand il existe et se parse correctement.",
			"Si SKILLS.md est absent, STH scanne les bundles compatibles et affiche leurs références canoniques category/folder.",
		},
		examples: []string{
			"sth list",
		},
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
	},
	{
		name:    "update",
		usage:   "sth update [self] [--dry-run]",
		summary: "Met à jour les ressources suivies ou le binaire STH lui-même",
		examples: []string{
			"sth update",
			"sth update --dry-run",
			"sth update self",
		},
	},
	{
		name:    "status",
		usage:   "sth status [--json]",
		summary: "Affiche l'état de chaque ressource installée (à jour, obsolète, modifiée localement)",
		examples: []string{
			"sth status",
			"sth status --json",
		},
	},
	{
		name:    "remove",
		usage:   "sth remove <resource-name> [--yes]",
		summary: "Supprime une ressource installée et ses fichiers gérés",
		examples: []string{
			"sth remove skills/analyse-risque",
			"sth remove skills/analyse-risque --yes",
		},
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
		usage:   "sth version",
		summary: "Affiche la signature de version minimale du CLI",
		examples: []string{
			"sth version",
		},
	},
}

type app struct {
	stdin       io.Reader
	stdout      io.Writer
	stderr      io.Writer
	client      *sthhttp.Client
	interactive func() bool
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

	return &app{
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
		client: client,
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
		if len(args) != 1 {
			err = fmt.Errorf("usage : sth init")
			break
		}
		err = a.runInit()
	case "list":
		if len(args) == 2 && isHelpFlag(args[1]) {
			a.writeStdout(commandHelpText("list"))
			return nil
		}
		if len(args) != 1 {
			err = fmt.Errorf("usage : sth list")
			break
		}
		err = a.runList()
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
		if len(args) == 2 && isHelpFlag(args[1]) {
			a.writeStdout(commandHelpText("version"))
			return nil
		}
		if len(args) != 1 {
			err = fmt.Errorf("usage : sth version")
			break
		}
		err = a.runVersion()
	case "login":
		if len(args) >= 2 && isHelpFlag(args[1]) {
			a.writeStdout(loginHelpText())
			return nil
		}
		err = a.runLogin(args[1:])
	case "logout":
		err = a.runLogout(args[1:])
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

// reportCloudUsageBestEffort sends a single usage record to the STH Cloud API
// for any successful command, when the active project profile uses the cloud
// provider. Failures are swallowed — telemetry must never block a CLI command.
//
// Runs synchronously with a tight timeout: a goroutine here would be killed
// when main returns (this is the last call before exit), so we'd lose nearly
// every record. 500ms is short enough to stay invisible on a healthy network
// and the request is fully cancelled if it overshoots.
func (a *app) reportCloudUsageBestEffort(command string) {
	if command == "login" || command == "logout" || command == "version" || command == "help" {
		return
	}
	cfg, err := sthfs.ReadProjectConfigV2()
	if err != nil || len(cfg.Providers) == 0 {
		return
	}
	var profile sthfs.ProjectProfile
	found := false
	for _, p := range cfg.Providers {
		if strings.EqualFold(p.Provider, "sth-cloud") {
			profile = sthfs.ProjectProfile{
				Provider:    p.Provider,
				TargetDir:   p.TargetDir,
				CatalogPath: p.CatalogPath,
			}
			found = true
			break
		}
	}
	if !found {
		return
	}
	ctx, cancel := contextWithTimeout(500)
	defer cancel()
	_ = a.client.ReportCloudUsage(ctx, profile, valueOrDefault(version, "dev"))
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
		if len(spec.examples) == 0 {
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
