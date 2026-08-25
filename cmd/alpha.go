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
		return fmt.Errorf("unknown alpha subcommand: %q (try `sth alpha request`)", args[0])
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
		return fmt.Errorf("could not request access: %w (sign in first with `sth login`)", err)
	}

	switch result.Status {
	case "already_active":
		a.writeStdout(fmt.Sprintf("You already have access to paid features (%s plan).", valueOrDefault(result.Plan, "pro")))
	case "pending":
		a.writeStdout(
			"Request submitted. An STH administrator will review it in the dashboard.",
			"After approval, run your command again — permissions refresh on the next request.",
		)
	default:
		a.writeStdout("Request recorded.")
	}
	return nil
}
