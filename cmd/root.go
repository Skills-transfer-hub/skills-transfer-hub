package cmd

import (
	"fmt"
	"io"
	lorehttp "loreskills/internal/http"
	"os"
)

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
		a.printHelp(a.stdout)
		return nil
	}

	var (
		err      error
		showHelp bool
	)

	switch args[0] {
	case "init":
		err = a.runInit()
	case "install":
		if len(args) != 2 {
			err = fmt.Errorf("usage : loreskills install <nom-du-skill>")
			break
		}
		err = a.runInstall(args[1])
	case "update":
		if len(args) != 1 {
			err = fmt.Errorf("usage : loreskills update")
			break
		}
		err = a.runUpdate()
	case "help", "--help", "-h":
		a.printHelp(a.stdout)
		return nil
	default:
		err = fmt.Errorf("commande inconnue : %q", args[0])
		showHelp = true
	}

	if err != nil {
		fmt.Fprintf(a.stderr, "Erreur : %v\n", err)
		if showHelp {
			fmt.Fprintln(a.stderr)
			a.printHelp(a.stderr)
		}
		return err
	}

	return nil
}

func (a *app) printHelp(w io.Writer) {
	fmt.Fprint(w, `LoreSkills — Gestionnaire de contexte IA

Commandes disponibles :
  init                Crée le fichier de configuration ~/.loreskillsrc
  install <skill>     Télécharge et installe un skill dans le projet courant
  update              Met à jour tous les skills installés dans le projet courant

Utilisation :
  loreskills init
  loreskills install mon-skill
  loreskills update
`)
}
