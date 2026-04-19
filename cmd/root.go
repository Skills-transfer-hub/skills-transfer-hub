// Package cmd gère le routage des sous-commandes CLI et le parsing des arguments.
// Il ne contient aucune logique métier — tout est délégué aux packages internes.
package cmd

import (
	"fmt"
	"os"
)

// Run est le point d'entrée unique du CLI : il dispatche vers la bonne sous-commande.
func Run(args []string) error {
	if len(args) == 0 {
		printHelp()
		return nil
	}

	switch args[0] {
	case "init":
		return runInit()
	case "install":
		if len(args) < 2 {
			return fmt.Errorf("usage : loreskills install <nom-du-skill>")
		}
		return runInstall(args[1])
	case "update":
		return runUpdate()
	case "help", "--help", "-h":
		printHelp()
		return nil
	default:
		fmt.Fprintf(os.Stderr, "commande inconnue : %q\n\n", args[0])
		printHelp()
		return fmt.Errorf("commande inconnue : %s", args[0])
	}
}

func printHelp() {
	fmt.Print(`LoreSkills — Gestionnaire de contexte IA

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
