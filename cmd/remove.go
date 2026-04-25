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

	state, err := fs.ReadInstalledStateV2(profile.TargetDir)
	if err != nil {
		return err
	}

	var record *fs.InstalledResourceRecordV2
	for _, rec := range state.Entries {
		rec := rec
		if rec.ResourceName == resourceName {
			record = &rec
			break
		}
	}
	if record == nil {
		return fmt.Errorf("ressource %q introuvable", resourceName)
	}

	status, modFiles := fs.ComputeResourceStatus(*record, "")
	if status == fs.ResourceStatusModifiedLocally && !skipConfirm {
		if !a.isInteractiveSession() {
			return fmt.Errorf("ressource %q modifiée localement ; utilisez --yes pour confirmer la suppression en mode non-interactif", resourceName)
		}
		a.writeStdout("Warning: the following files have been locally modified:")
		for _, f := range modFiles {
			a.writeStdout(fmt.Sprintf("  %s", f))
		}
		fmt.Fprint(a.stdout, "Remove anyway? [y/N] ")
		sc := bufio.NewScanner(a.stdin)
		if sc.Scan() {
			answer := strings.TrimSpace(sc.Text())
			if answer != "y" && answer != "Y" {
				return fmt.Errorf("suppression annulée par l'utilisateur")
			}
		}
	}

	deleted, failed := fs.DeleteManagedFiles(*record)
	_ = deleted

	var hardFailures []string
	for _, fp := range failed {
		if strings.HasPrefix(fp, "(missing) ") {
			a.writeStdout(fmt.Sprintf("warning: file not found, skipping: %s", strings.TrimPrefix(fp, "(missing) ")))
		} else {
			a.writeStderr(fmt.Sprintf("error: failed to delete: %s", fp))
			hardFailures = append(hardFailures, fp)
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
