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

	record, err := a.installCatalogEntry(context.Background(), *profile, entry)
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

func (a *app) installCatalogEntry(ctx context.Context, profile fs.ProjectProfile, entry fs.CatalogEntry) (fs.InstalledResourceRecord, error) {
	if entry.ResourceType != "bundle" {
		content, err := a.client.DownloadCatalogEntry(ctx, profile, entry)
		if err != nil {
			return fs.InstalledResourceRecord{}, err
		}

		return fs.InstallCatalogEntry(profile.TargetDir, entry, content)
	}

	bundleContents, err := a.downloadBundleContents(ctx, profile, entry)
	if err != nil {
		return fs.InstalledResourceRecord{}, err
	}

	return fs.SyncManagedBundle(profile.TargetDir, entry, bundleContents)
}

func (a *app) downloadBundleContents(ctx context.Context, profile fs.ProjectProfile, entry fs.CatalogEntry) (map[string][]byte, error) {
	bundlePaths, err := a.client.ListBundleFiles(ctx, profile, entry)
	if err != nil {
		return nil, err
	}

	bundleContents := make(map[string][]byte, len(bundlePaths))
	for _, sourcePath := range bundlePaths {
		fileEntry := entry
		fileEntry.SourcePath = sourcePath

		content, err := a.client.DownloadCatalogEntry(ctx, profile, fileEntry)
		if err != nil {
			return nil, err
		}
		bundleContents[sourcePath] = content
	}

	return bundleContents, nil
}
