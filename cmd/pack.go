package cmd

import (
	"errors"
	"fmt"
)

// runPack dispatches local, open-source Git pack operations.
//
// Local pack creation is part of the open-source Starter workstation. A
// future cloud publish operation must enforce its own hosted capability at
// the server boundary; it must not make this local scaffold paywalled.
func (a *app) runPack(args []string) error {
	if len(args) == 0 {
		return errors.New("usage : sth pack <init|promote>")
	}
	if isHelpFlag(args[0]) {
		a.writeStdout(commandHelpText("pack"))
		return nil
	}
	switch args[0] {
	case "init":
		if len(args) >= 2 && isHelpFlag(args[1]) {
			a.writeStdout(commandHelpText("pack"))
			return nil
		}
		return a.runPackInit(args[1:])
	case "promote":
		if len(args) >= 2 && isHelpFlag(args[1]) {
			a.writeStdout(commandHelpText("pack"))
			return nil
		}
		return a.runPackPromote(args[1:])
	}
	return fmt.Errorf("commande pack inconnue : %q", args[0])
}
