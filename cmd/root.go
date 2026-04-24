package cmd

import (
	"fmt"
	"io"
	lorehttp "loreskills/internal/http"
	"os"
	"strings"
)

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
		usage:   "loreskills init",
		summary: "Configure pas a pas un depot distant heberge supporte pour le projet courant",
		details: []string{
			"Providers heberges supportes : github.com, gitlab.com, dev.azure.com et bitbucket.org.",
			"Le wizard affiche les URLs officielles pour creer le token/API token, la permission minimale attendue et les variables d'environnement reconnues.",
			"Le repository_id accepte un identifiant canonique ou une URL hebergee supportee. Les hosts self-hosted sont rejetes.",
			"La guidance provider peut etre relue pendant le wizard et aucun secret n'est persiste dans .loreskills/project.json.",
		},
		examples: []string{
			"loreskills init",
		},
	},
	{
		name:    "list",
		usage:   "loreskills list",
		summary: "Lit SKILLS.md ou détecte les bundles category/folder/SKILL.md du dépôt courant",
		details: []string{
			"SKILLS.md garde toujours la priorité quand il existe et se parse correctement.",
			"Si SKILLS.md est absent, LoreSkills scanne les bundles compatibles et affiche leurs références canoniques category/folder.",
		},
		examples: []string{
			"loreskills list",
		},
	},
	{
		name:    "install",
		usage:   "loreskills install <resource-ref> [--dry-run] [--force]",
		summary: "Installe une ressource du catalogue distant dans le projet courant",
		examples: []string{
			"loreskills install react/form-wizard",
			"loreskills install skills/analyse-risque",
			"loreskills install skills/analyse-risque --dry-run",
		},
	},
	{
		name:    "update",
		usage:   "loreskills update [self] [--dry-run]",
		summary: "Met à jour les ressources suivies ou le binaire LoreSkills lui-même",
		examples: []string{
			"loreskills update",
			"loreskills update --dry-run",
			"loreskills update self",
		},
	},
	{
		name:    "status",
		usage:   "loreskills status [--json]",
		summary: "Affiche l'état de chaque ressource installée (à jour, obsolète, modifiée localement)",
		examples: []string{
			"loreskills status",
			"loreskills status --json",
		},
	},
	{
		name:    "remove",
		usage:   "loreskills remove <resource-name> [--yes]",
		summary: "Supprime une ressource installée et ses fichiers gérés",
		examples: []string{
			"loreskills remove skills/analyse-risque",
			"loreskills remove skills/analyse-risque --yes",
		},
	},
	{
		name:    "help",
		usage:   "loreskills help [commande]",
		summary: "Affiche l'aide générale ou l'aide ciblée d'une commande visible",
		examples: []string{
			"loreskills help",
			"loreskills help list",
		},
	},
	{
		name:    "version",
		usage:   "loreskills version",
		summary: "Affiche la signature de version minimale du CLI",
		examples: []string{
			"loreskills version",
		},
	},
}

type app struct {
	stdin       io.Reader
	stdout      io.Writer
	stderr      io.Writer
	client      *lorehttp.Client
	interactive func() bool
}

func newApp(stdin io.Reader, stdout, stderr io.Writer, client *lorehttp.Client) *app {
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
		client = lorehttp.NewClient(nil)
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

	return false
}

func (a *app) nextStepGuidanceLines() []string {
	return startupGuidanceLines(a.loadGuidedStartupState())
}

// Run routes CLI arguments to the appropriate LoreSkills command.
func Run(args []string) error {
	return newApp(os.Stdin, os.Stdout, os.Stderr, nil).run(args)
}

func (a *app) run(args []string) error {
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
			err = fmt.Errorf("usage : loreskills init")
			break
		}
		err = a.runInit()
	case "list":
		if len(args) == 2 && isHelpFlag(args[1]) {
			a.writeStdout(commandHelpText("list"))
			return nil
		}
		if len(args) != 1 {
			err = fmt.Errorf("usage : loreskills list")
			break
		}
		err = a.runList()
	case "install":
		if len(args) >= 2 && isHelpFlag(args[1]) {
			a.writeStdout(commandHelpText("install"))
			return nil
		}
		err = a.runInstall(args[1:])
	case "update":
		if len(args) >= 2 && isHelpFlag(args[1]) {
			a.writeStdout(commandHelpText("update"))
			return nil
		}
		if len(args) >= 2 && args[1] == "self" {
			if len(args) != 2 {
				err = fmt.Errorf("usage : loreskills update [self]")
				break
			}
			err = a.runSelfUpdate()
		} else {
			err = a.runUpdate(args[1:])
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
			err = fmt.Errorf("usage : loreskills help [commande]")
		}
	case "version":
		if len(args) == 2 && isHelpFlag(args[1]) {
			a.writeStdout(commandHelpText("version"))
			return nil
		}
		if len(args) != 1 {
			err = fmt.Errorf("usage : loreskills version")
			break
		}
		err = a.runVersion()
	case "stockholm":
		if len(args) != 1 {
			err = fmt.Errorf("usage : loreskills stockholm")
			break
		}
		err = a.runStockholm()
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

	return nil
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
	builder.WriteString("LoreSkills\n")
	builder.WriteString("Sans argument, `loreskills` ouvre le menu guidé en TTY et affiche la prochaine étape utile hors TTY.\n\n")
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
