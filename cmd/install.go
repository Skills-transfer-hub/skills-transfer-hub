package cmd

import (
	"context"
	"fmt"
	"loreskills/internal/fs"
)

func (a *app) runInstall(resourceRef string) error {
	profile, err := fs.ReadProjectProfile()
	if err != nil {
		return err
	}

	entries, err := a.client.ListCatalog(context.Background(), *profile)
	if err != nil {
		return err
	}

	entry, err := fs.ResolveCatalogEntry(entries, resourceRef)
	if err != nil {
		return err
	}

	content, err := a.client.DownloadCatalogEntry(context.Background(), *profile, entry)
	if err != nil {
		return err
	}

	record, err := fs.InstallCatalogEntry(profile.TargetDir, entry, content)
	if err != nil {
		return err
	}

	a.writeStdout(
		fmt.Sprintf("Ressource '%s' installée dans %s.", entry.Name, record.InstalledPath),
		fmt.Sprintf("Type: %s", entry.ResourceType),
		fmt.Sprintf("Dossier: %s", entry.FolderName),
		"Prochaine étape : utilisez `loreskills update` pour rafraîchir les ressources suivies.",
	)
	return nil
}
