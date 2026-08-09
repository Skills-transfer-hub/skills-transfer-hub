package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"sth/internal/fs"
	"strings"
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
		a.writeStdout("Aucune écriture (--dry-run). Git reste la source de vérité.")
		return nil
	}
	if !options.yes {
		confirmed, promptErr := promptYesNo(bufio.NewReader(a.stdin), a.stdout, "Créer ce bundle Git sans commit ni push", false)
		if promptErr != nil {
			return promptErr
		}
		if !confirmed {
			return errors.New("promotion annulée par l'utilisateur")
		}
	}
	if err := fs.ApplyLocalLibraryPromotion(plan); err != nil {
		return err
	}
	a.writeStdout(
		fmt.Sprintf("Bundle créé dans %s.", plan.Output),
		"STH n'a exécuté ni commit, ni push : Git reste la source de vérité.",
		fmt.Sprintf("Étapes suivantes : git add -- %q, commit, puis ouvre une pull request.", plan.Output),
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
				return packPromoteOptions{}, errors.New("--output exige un chemin skills/<slug>")
			}
			index++
			if options.output != "" {
				return packPromoteOptions{}, errors.New("--output ne peut être fourni qu'une fois")
			}
			options.output = args[index]
			if strings.TrimSpace(options.output) == "" {
				return packPromoteOptions{}, errors.New("--output exige un chemin skills/<slug>")
			}
		case strings.HasPrefix(argument, "--output="):
			if options.output != "" {
				return packPromoteOptions{}, errors.New("--output ne peut être fourni qu'une fois")
			}
			options.output = strings.TrimPrefix(argument, "--output=")
			if strings.TrimSpace(options.output) == "" {
				return packPromoteOptions{}, errors.New("--output exige un chemin skills/<slug>")
			}
		case strings.HasPrefix(argument, "-"):
			return packPromoteOptions{}, fmt.Errorf("option promote inconnue : %q", argument)
		case options.libraryID == "":
			options.libraryID = argument
		default:
			return packPromoteOptions{}, errors.New("usage : sth pack promote <library-id> [--output skills/<slug>] [--dry-run] [--yes]")
		}
	}
	if strings.TrimSpace(options.libraryID) == "" {
		return packPromoteOptions{}, errors.New("usage : sth pack promote <library-id> [--output skills/<slug>] [--dry-run] [--yes]")
	}
	return options, nil
}

func (a *app) writePromotionPlan(plan fs.LibraryPromotionPlan) {
	a.writeStdout(
		fmt.Sprintf("Promotion locale %s -> %s", plan.LibraryID, plan.Output),
		"Fichiers à créer :",
	)
	for _, file := range plan.Files {
		a.writeStdout(fmt.Sprintf("  %s (%d octets, sha256 %s)", file.Path, file.Size, file.SHA256))
	}
}
