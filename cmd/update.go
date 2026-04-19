package cmd

import (
	"fmt"
	"loreskills/internal/fetch"
	"loreskills/internal/fs"
	"os"
)

func runUpdate() error {
	cfg, err := fs.ReadConfig()
	if err != nil {
		return fmt.Errorf("update : %w", err)
	}

	state, err := fs.ReadState(cfg.TargetDir)
	if err != nil {
		return fmt.Errorf("update : %w", err)
	}

	if len(state) == 0 {
		fmt.Println("Aucun skill à mettre à jour.")
		return nil
	}

	// On continue même en cas d'erreur partielle : on tente tous les skills
	var lastErr error
	for skillName := range state {
		url := cfg.RepoRawURL + "/" + skillName + ".md"

		content, err := fetch.Download(url)
		if err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  %s : %v\n", skillName, err)
			lastErr = err
			continue
		}

		if err := fs.SaveSkill(cfg.TargetDir, skillName, content); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  %s : %v\n", skillName, err)
			lastErr = err
			continue
		}

		if err := fs.UpdateState(cfg.TargetDir, skillName); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  %s : %v\n", skillName, err)
			lastErr = err
			continue
		}

		fmt.Printf("✅ '%s' mis à jour\n", skillName)
	}

	return lastErr
}
