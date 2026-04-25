package cmd

import (
	"context"
	"fmt"
	"loreskills/internal/fs"
	"path/filepath"
)

func (a *app) runInstall(args []string) error {
	flags, positional := parseFlags(args, "--dry-run", "--json", "--force")
	dryRun := flags["--dry-run"]
	jsonOut := flags["--json"]
	force := flags["--force"]

	if len(positional) != 1 {
		return fmt.Errorf("usage: loreskills install <resource-ref> [--dry-run] [--force]")
	}
	resourceRef := positional[0]

	profile, err := fs.ReadProjectProfile()
	if err != nil {
		return err
	}

	providerID := "default"
	if cfg, cfgErr := fs.ReadProjectConfigV2(); cfgErr == nil && len(cfg.Providers) > 0 {
		providerID = cfg.Providers[0].ID
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

	workingDir, _ := filepath.Abs(".")
	ignorePats, _ := fs.ReadLoreSkillsIgnore(workingDir)

	// Build incoming managed files for dry-run or conflict check.
	incomingRecord := fs.RefreshManagedFileHashes(fs.InstalledResourceRecordV2{
		ResourceName: entry.CatalogID,
		ManagedFiles: []fs.ManagedFile{{Path: fs.ResolveManagedFilePath(profile.TargetDir, entry)}},
	}, content)

	if dryRun {
		state, _ := fs.ReadInstalledStateV2(profile.TargetDir)
		var existing *fs.InstalledResourceRecordV2
		for _, rec := range state.Entries {
			rec := rec
			if rec.ResourceName == entry.CatalogID {
				existing = &rec
				break
			}
		}
		changes := fs.ComputeDryRunChanges(existing, incomingRecord.ManagedFiles, ignorePats)
		if jsonOut {
			data, err := fs.FormatDryRunJSON(changes)
			if err != nil {
				return err
			}
			a.writeStdout(string(data))
		} else {
			a.writeStdout(fs.FormatDryRunText(changes))
		}
		return nil
	}

	// Ownership conflict check.
	if !force {
		state, _ := fs.ReadInstalledStateV2(profile.TargetDir)
		for _, mf := range incomingRecord.ManagedFiles {
			if conflict := fs.CheckOwnershipConflict(&state, entry.CatalogID, mf.Path); conflict != nil {
				return conflict
			}
		}
	}

	record, err := fs.InstallCatalogEntry(profile.TargetDir, entry, content)
	if err != nil {
		return err
	}
	if _, err := fs.InstallCatalogEntryV2(profile.TargetDir, entry, content, providerID); err != nil {
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

