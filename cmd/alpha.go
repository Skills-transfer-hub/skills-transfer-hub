package cmd

import (
	"fmt"

	"sth/internal/fs"
)

func (a *app) runAlpha(args []string) error {
	if len(args) == 0 || isHelpFlag(args[0]) {
		a.writeStdout(commandHelpText("alpha"))
		return nil
	}
	switch args[0] {
	case "request":
		return a.runAlphaRequest(args[1:])
	default:
		return fmt.Errorf("sous-commande inconnue pour alpha : %q (essaie `sth alpha request`)", args[0])
	}
}

func (a *app) runAlphaRequest(args []string) error {
	if len(args) > 0 && isHelpFlag(args[0]) {
		a.writeStdout(commandHelpText("alpha"))
		return nil
	}

	ctx, cancel := contextWithTimeout(8000)
	defer cancel()

	result, err := a.client.RequestAlphaAccess(ctx, fs.ProjectProfile{Provider: loginProviderName})
	if err != nil {
		return fmt.Errorf("demande d'accès impossible : %w (connecte-toi d'abord avec `sth login`)", err)
	}

	switch result.Status {
	case "already_active":
		a.writeStdout(fmt.Sprintf("Tu as déjà accès aux options payantes (plan %s).", valueOrDefault(result.Plan, "pro")))
	case "pending":
		a.writeStdout(
			"Demande envoyée. Un admin STH va l'examiner depuis le dashboard.",
			"Une fois approuvée, relance ta commande — les droits se rafraîchissent au prochain appel.",
		)
	default:
		a.writeStdout("Demande enregistrée.")
	}
	return nil
}
