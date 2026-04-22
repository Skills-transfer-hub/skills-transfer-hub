package cmd

import (
	"context"
	"fmt"
	"loreskills/internal/fs"
)

func (a *app) runList() error {
	profile, err := fs.ReadProjectProfile()
	if err != nil {
		return err
	}

	entries, err := a.client.ListCatalog(context.Background(), *profile)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		a.writeStdout("Aucune ressource disponible dans le catalogue distant.")
		return nil
	}

	lines := []string{fmt.Sprintf("%d ressource(s) disponible(s) :", len(entries))}
	for _, entry := range entries {
		line := fmt.Sprintf("- [%s] %s (folder: %s)", entry.ResourceType, entry.Name, entry.FolderName)
		if entry.Description != "" {
			line += fmt.Sprintf(" — %s", entry.Description)
		}
		lines = append(lines, line)
	}

	a.writeStdout(lines...)
	return nil
}