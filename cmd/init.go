package cmd

import (
	"fmt"
	"loreskills/internal/fs"
)

func (a *app) runInit() error {
	path, err := fs.WriteDefaultConfig()
	if err != nil {
		return err
	}

	fmt.Fprintf(a.stdout, "Fichier de configuration créé : %s\n", path)
	fmt.Fprintln(a.stdout, "Modifiez `repo_raw_url` avec l'URL brute de votre dépôt de skills avant d'utiliser install ou update.")
	return nil
}
