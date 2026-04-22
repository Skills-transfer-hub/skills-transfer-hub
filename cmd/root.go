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
		summary: "Cree le fichier de configuration ~/.loreskillsrc",
		examples: []string{
			"loreskills init",
		},
	},
	{
		name:    "install",
		usage:   "loreskills install <skill>",
		summary: "Telecharge et installe un skill dans le projet courant",
		examples: []string{
			"loreskills install mon-skill",
		},
	},
	{
		name:    "update",
		usage:   "loreskills update",
		summary: "Met a jour tous les skills installes dans le projet courant",
		examples: []string{
			"loreskills update",
		},
	},
	{
		name:    "help",
		usage:   "loreskills help [commande]",
		summary: "Affiche l'aide generale ou l'aide ciblee d'une commande",
		examples: []string{
			"loreskills help",
			"loreskills help install",
		},
	},
	{
		name:    "version",
		usage:   "loreskills version",
		summary: "Affiche la version installee du CLI",
		examples: []string{
			"loreskills version",
		},
	},
}

const asciiGuide = ` /\_/\\
( o.o )
 > ^ <`

type app struct {
	stdout io.Writer
	stderr io.Writer
	client *lorehttp.Client
}

func newApp(stdout, stderr io.Writer, client *lorehttp.Client) *app {
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
		stdout: stdout,
		stderr: stderr,
		client: client,
	}
}

// Run routes CLI arguments to the appropriate LoreSkills command.
func Run(args []string) error {
	return newApp(os.Stdout, os.Stderr, nil).run(args)
}

func (a *app) run(args []string) error {
	if len(args) == 0 {
		a.writeGuidedStdout(helpText())
		return nil
	}

	var (
		err      error
		showHelp bool
	)

	switch args[0] {
	case "init":
		if len(args) == 2 && isHelpFlag(args[1]) {
			a.writeGuidedStdout(commandHelpText("init"))
			return nil
		}
		if len(args) != 1 {
			err = fmt.Errorf("usage : loreskills init")
			break
		}
		err = a.runInit()
	case "install":
		if len(args) == 2 && isHelpFlag(args[1]) {
			a.writeGuidedStdout(commandHelpText("install"))
			return nil
		}
		if len(args) != 2 {
			err = fmt.Errorf("usage : loreskills install <nom-du-skill>")
			break
		}
		err = a.runInstall(args[1])
	case "update":
		if len(args) == 2 && isHelpFlag(args[1]) {
			a.writeGuidedStdout(commandHelpText("update"))
			return nil
		}
		if len(args) != 1 {
			err = fmt.Errorf("usage : loreskills update")
			break
		}
		err = a.runUpdate()
	case "help":
		switch len(args) {
		case 1:
			a.writeGuidedStdout(helpText())
			return nil
		case 2:
			if _, ok := findCommandSpec(args[1]); !ok {
				err = fmt.Errorf("commande inconnue pour help : %q", args[1])
				showHelp = true
				break
			}
			a.writeGuidedStdout(commandHelpText(args[1]))
			return nil
		default:
			err = fmt.Errorf("usage : loreskills help [commande]")
		}
	case "version":
		if len(args) == 2 && isHelpFlag(args[1]) {
			a.writeGuidedStdout(commandHelpText("version"))
			return nil
		}
		if len(args) != 1 {
			err = fmt.Errorf("usage : loreskills version")
			break
		}
		err = a.runVersion()
	case "--help", "-h":
		a.writeGuidedStdout(helpText())
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
		a.writeGuidedStderr(lines...)
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

func renderGuideMessage(lines ...string) string {
	parts := []string{asciiGuide}
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			parts = append(parts, "")
			continue
		}
		parts = append(parts, line)
	}

	return strings.Join(parts, "\n") + "\n"
}

func (a *app) writeGuidedStdout(lines ...string) {
	fmt.Fprint(a.stdout, renderGuideMessage(lines...))
}

func (a *app) writeGuidedStderr(lines ...string) {
	fmt.Fprint(a.stderr, renderGuideMessage(lines...))
}

func helpText() string {
	var builder strings.Builder
	builder.WriteString("LoreSkills — Gestionnaire de contexte IA\n\n")
	builder.WriteString("Commandes disponibles :\n")
	for _, spec := range commandCatalog {
		builder.WriteString(fmt.Sprintf("  %-20s %s\n", commandDisplayName(spec), spec.summary))
	}
	builder.WriteString("\nUtilisation :\n")
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
	if len(spec.examples) == 0 {
		return strings.TrimRight(builder.String(), "\n")
	}

	builder.WriteString("\nExemples :\n")
	for _, example := range spec.examples {
		builder.WriteString(fmt.Sprintf("  %s\n", example))
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
