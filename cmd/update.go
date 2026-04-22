package cmd

import (
	"context"
	"fmt"
	"loreskills/internal/fs"
	"os"
	"strings"
)

func (a *app) runUpdate() error {
	profile, err := fs.ReadProjectProfile()
	if err != nil {
		return err
	}

	records, err := fs.ListInstalledResources(profile.TargetDir)
	if err != nil {
		return err
	}

	if len(records) == 0 {
		a.writeStdout("Aucune ressource à mettre à jour.")
		return nil
	}

	failures := 0
	for _, record := range records {
		entry := fs.CatalogEntry{
			CatalogID:    record.CatalogID,
			ResourceType: record.ResourceType,
			Name:         record.Name,
			FolderName:   record.FolderName,
			SourcePath:   record.SourcePath,
		}

		content, err := a.client.DownloadCatalogEntry(context.Background(), *profile, entry)
		if err != nil {
			a.writeStderr(fmt.Sprintf("Échec de mise à jour pour '%s' : %v", record.Name, err))
			failures++
			continue
		}

		if err := fs.SaveInstalledResource(record, content); err != nil {
			a.writeStderr(fmt.Sprintf("Échec de mise à jour pour '%s' : %v", record.Name, err))
			failures++
			continue
		}

		if err := fs.UpsertInstalledResource(profile.TargetDir, record); err != nil {
			a.writeStderr(fmt.Sprintf("Échec de mise à jour pour '%s' : %v", record.Name, err))
			failures++
			continue
		}

		a.writeStdout(fmt.Sprintf("Ressource '%s' mise à jour.", record.Name))
	}

	if failures > 0 {
		return fmt.Errorf("mise à jour incomplète : %d ressource(s) en échec", failures)
	}

	return nil
}

func (a *app) runSelfUpdate() error {
	metadata := currentReleaseMetadata()
	release, err := a.client.CheckForStableUpdate(context.Background(), metadata)
	if err != nil {
		return err
	}
	if release == nil {
		a.writeStdout("Aucune mise à jour stable disponible.")
		return nil
	}

	assetData, err := a.client.DownloadURL(context.Background(), release.DownloadURL, nil)
	if err != nil {
		return err
	}
	checksumData, err := a.client.DownloadURL(context.Background(), release.ChecksumURL, nil)
	if err != nil {
		return err
	}

	executablePath, err := currentExecutablePath()
	if err != nil {
		return err
	}
	if err := fs.ReplaceExecutableWithAsset(executablePath, release.AssetName, assetData, checksumData); err != nil {
		return fmt.Errorf("mise à jour stable %s détectée, mais le remplacement automatique a échoué : %w", release.Version, err)
	}

	a.writeStdout(
		fmt.Sprintf("Version stable détectée : %s", release.Version),
		"Mise à jour appliquée.",
	)
	return nil
}

func currentExecutablePath() (string, error) {
	if override := strings.TrimSpace(os.Getenv("LORESKILLS_EXECUTABLE_PATH")); override != "" {
		return override, nil
	}

	path, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("impossible de déterminer le binaire courant : %w", err)
	}

	return path, nil
}
