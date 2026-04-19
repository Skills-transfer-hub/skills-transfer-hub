package cmd

import (
	"fmt"
	"loreskills/internal/fs"
)

func runInit() error {
	path, err := fs.WriteDefaultConfig()
	if err != nil {
		return fmt.Errorf("init : %w", err)
	}

	fmt.Printf("✅ Fichier de configuration créé : %s\n", path)
	fmt.Println("👉 Pensez à modifier 'repo_raw_url' avec l'URL de votre dépôt de skills.")
	return nil
}
