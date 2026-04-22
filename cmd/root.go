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
	examples []string
}

var commandCatalog = []commandSpec{
	{
		name:    "init",
		usage:   "loreskills init",
		summary: "Configure pas a pas le depot distant public ou prive du projet courant",
		examples: []string{
			"loreskills init",
		},
	},
	{
		name:    "list",
		usage:   "loreskills list",
		summary: "Lit SKILLS.md et liste les ressources installables du dépôt courant",
		examples: []string{
			"loreskills list",
		},
	},
	{
		name:    "install",
		usage:   "loreskills install <resource-ref>",
		summary: "Installe une ressource du catalogue distant dans le projet courant",
		examples: []string{
			"loreskills install skills/analyse-risque",
		},
	},
	{
		name:    "update",
		usage:   "loreskills update [self]",
		summary: "Met à jour les ressources suivies ou le binaire LoreSkills lui-même",
		examples: []string{
			"loreskills update",
			"loreskills update self",
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
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	client *lorehttp.Client
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

// Run routes CLI arguments to the appropriate LoreSkills command.
func Run(args []string) error {
	return newApp(os.Stdin, os.Stdout, os.Stderr, nil).run(args)
}

func (a *app) run(args []string) error {
	if len(args) == 0 {
		a.writeStdout(formatVersionSignature())
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
		if len(args) == 2 && isHelpFlag(args[1]) {
			a.writeStdout(commandHelpText("install"))
			return nil
		}
		if len(args) != 2 {
			err = fmt.Errorf("usage : loreskills install <resource-ref>")
			break
		}
		err = a.runInstall(args[1])
	case "update":
		if len(args) == 2 && isHelpFlag(args[1]) {
			a.writeStdout(commandHelpText("update"))
			return nil
		}
		switch len(args) {
		case 1:
			err = a.runUpdate()
		case 2:
			if args[1] != "self" {
				err = fmt.Errorf("usage : loreskills update [self]")
				break
			}
			err = a.runSelfUpdate()
		default:
			err = fmt.Errorf("usage : loreskills update [self]")
		}
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
	builder.WriteString("LoreSkills\n\n")
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
