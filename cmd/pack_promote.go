package cmd

import (
	"errors"
	"fmt"
	"strings"

	"sth/internal/fs"
	"sth/internal/tui"
)

type packPromoteOptions struct {
	libraryID string
	output    string
	dryRun    bool
	yes       bool
}

func (a *app) runPackPromote(args []string) error {
	options, err := parsePackPromoteOptions(args)
	if err != nil {
		return err
	}
	plan, err := fs.PlanLocalLibraryPromotion(".", options.libraryID, options.output)
	if err != nil {
		return err
	}
	a.writePromotionPlan(plan)
	if options.dryRun {
		a.writeStdout("No files written (--dry-run). Git remains the source of truth.")
		return nil
	}
	if !options.yes {
		session := tui.NewSession(a.stdin, a.stdout)
		confirmed, promptErr := tui.Confirm(session, tui.ConfirmConfig{Question: "Create this Git bundle without committing or pushing?", DefaultYes: false})
		if promptErr != nil {
			if errors.Is(promptErr, tui.ErrAborted) {
				return errors.New("promotion canceled by user")
			}
			return promptErr
		}
		if !confirmed {
			return errors.New("promotion canceled by user")
		}
	}
	if err := fs.ApplyLocalLibraryPromotion(plan); err != nil {
		return err
	}
	a.writeStdout(
		fmt.Sprintf("Bundle created in %s.", plan.Output),
		"STH did not commit or push: Git remains the source of truth.",
		fmt.Sprintf("Next steps: git add -- %q, commit, then open a pull request.", plan.Output),
	)
	return nil
}

func parsePackPromoteOptions(args []string) (packPromoteOptions, error) {
	options := packPromoteOptions{}
	for index := 0; index < len(args); index++ {
		argument := args[index]
		switch {
		case argument == "--dry-run":
			options.dryRun = true
		case argument == "--yes":
			options.yes = true
		case argument == "--output":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return packPromoteOptions{}, errors.New("--output requires a skills/<slug> path")
			}
			index++
			if options.output != "" {
				return packPromoteOptions{}, errors.New("--output can only be provided once")
			}
			options.output = args[index]
			if strings.TrimSpace(options.output) == "" {
				return packPromoteOptions{}, errors.New("--output requires a skills/<slug> path")
			}
		case strings.HasPrefix(argument, "--output="):
			if options.output != "" {
				return packPromoteOptions{}, errors.New("--output can only be provided once")
			}
			options.output = strings.TrimPrefix(argument, "--output=")
			if strings.TrimSpace(options.output) == "" {
				return packPromoteOptions{}, errors.New("--output requires a skills/<slug> path")
			}
		case strings.HasPrefix(argument, "-"):
			return packPromoteOptions{}, fmt.Errorf("unknown promote option: %q", argument)
		case options.libraryID == "":
			options.libraryID = argument
		default:
			return packPromoteOptions{}, errors.New("usage: sth pack promote <library-id> [--output skills/<slug>] [--dry-run] [--yes]")
		}
	}
	if strings.TrimSpace(options.libraryID) == "" {
		return packPromoteOptions{}, errors.New("usage: sth pack promote <library-id> [--output skills/<slug>] [--dry-run] [--yes]")
	}
	return options, nil
}

func (a *app) writePromotionPlan(plan fs.LibraryPromotionPlan) {
	a.writeStdout(
		fmt.Sprintf("Local promotion %s -> %s", plan.LibraryID, plan.Output),
		"Files to create:",
	)
	for _, file := range plan.Files {
		a.writeStdout(fmt.Sprintf("  %s (%d bytes, sha256 %s)", file.Path, file.Size, file.SHA256))
	}
}
