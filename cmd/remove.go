package cmd

import (
	"bufio"
	"fmt"
	"sth/internal/fs"
	"strings"
)

func (a *app) runRemove(args []string) error {
	flags, positional := parseFlags(args, "--yes")
	skipConfirm := flags["--yes"]

	if len(positional) != 1 {
		return fmt.Errorf("usage: sth remove <resource-name> [--yes]")
	}
	resourceName := positional[0]

	profile, release, err := a.prepareStateAccess()
	if err != nil {
		return err
	}
	defer release()

	records, err := fs.FindInstalledRecordsV2(profile.TargetDir, resourceName)
	if err != nil {
		return err
	}
	if len(records) == 0 {
		return fmt.Errorf("ressource %q introuvable", resourceName)
	}

	if !skipConfirm {
		modified := make([]string, 0)
		for _, record := range records {
			status, modFiles := fs.ComputeResourceStatus(record, "")
			if status == fs.ResourceStatusModifiedLocally {
				modified = append(modified, modFiles...)
			}
		}
		if len(modified) > 0 {
			if !a.isInteractiveSession() {
				return fmt.Errorf("ressource %q modifiée localement ; utilisez --yes pour confirmer la suppression en mode non-interactif", resourceName)
			}
			a.writeStdout("Warning: the following files have been locally modified:")
			for _, file := range modified {
				a.writeStdout(fmt.Sprintf("  %s", file))
			}
			fmt.Fprint(a.stdout, "Remove anyway? [y/N] ")
			scanner := bufio.NewScanner(a.stdin)
			if scanner.Scan() {
				answer := strings.TrimSpace(scanner.Text())
				if answer != "y" && answer != "Y" {
					return fmt.Errorf("suppression annulée par l'utilisateur")
				}
			}
		}
	}

	var hardFailures []string
	for _, record := range records {
		_, failed := fs.DeleteManagedFiles(record)
		for _, failedPath := range failed {
			if strings.HasPrefix(failedPath, "(missing) ") {
				a.writeStdout(fmt.Sprintf("warning: file not found, skipping: %s", strings.TrimPrefix(failedPath, "(missing) ")))
			} else {
				a.writeStderr(fmt.Sprintf("error: failed to delete: %s", failedPath))
				hardFailures = append(hardFailures, failedPath)
			}
		}
	}

	if len(hardFailures) > 0 {
		return fmt.Errorf("%d fichier(s) n'ont pas pu être supprimés — état non modifié", len(hardFailures))
	}

	if err := fs.RemoveResourceFromState(profile.TargetDir, resourceName); err != nil {
		return err
	}

	a.writeStdout(fmt.Sprintf("Resource '%s' removed.", resourceName))
	return nil
}
