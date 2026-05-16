package cmd

import (
	"context"
	"errors"
	"fmt"
)

// runPack dispatches `sth pack <subcommand>`. Today the only subcommand is
// `init` (scaffold a new skills repository); `publish` / `validate` are
// reserved for later.
//
// Pack is gated Pro — every subcommand re-enters enforceProGate so a future
// addition can't accidentally leak into Free without an explicit decision.
func (a *app) runPack(args []string) error {
	if len(args) == 0 {
		return errors.New("usage : sth pack <init>")
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
		if err := a.enforceProGate(context.Background()); err != nil {
			return err
		}
		return a.runPackInit(args[1:])
	}
	return fmt.Errorf("commande pack inconnue : %q", args[0])
}
