package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"sth/internal/fs"
	"sth/internal/tui"
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
		return fmt.Errorf("resource %q not found", resourceName)
	}
	state, stateErr := fs.ReadInstalledStateV2(profile.TargetDir)
	if stateErr != nil {
		return stateErr
	}
	if driftErr := refuseModifiedResources(state, fs.ProjectConfigV2{}, resourceName); driftErr != nil {
		return driftErr
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
				return fmt.Errorf("resource %q has local modifications; use --yes to confirm removal in non-interactive mode", resourceName)
			}
			a.writeStdout("Locally modified files:")
			for _, file := range modified {
				a.writeStdout(fmt.Sprintf("  %s", file))
			}
			session := tui.NewSession(a.stdin, a.stdout)
			confirmed, confirmErr := tui.Confirm(session, tui.ConfirmConfig{Question: "Remove anyway?", DefaultYes: false})
			if confirmErr != nil || !confirmed {
				return fmt.Errorf("removal canceled by user")
			}
		}
	}
	if handled, removeErr := a.removeRecordsWithManagedRegions(profile.TargetDir, resourceName, records); handled {
		return removeErr
	}
	projectRoot, pathErr := filepath.Abs(".")
	if pathErr != nil {
		return pathErr
	}
	managedFiles := make([]fs.ManagedFile, 0)
	for _, record := range records {
		managedFiles = append(managedFiles, record.ManagedFiles...)
	}
	transaction, transactionErr := newFanoutFileTransaction(projectRoot, fanoutTransactionPaths(profile.TargetDir, managedFiles, nil))
	if transactionErr != nil {
		return transactionErr
	}

	var hardFailures []string
	for _, record := range records {
		_, failed := fs.DeleteManagedFiles(record)
		for _, failedPath := range failed {
			if strings.HasPrefix(failedPath, "(missing) ") {
				a.writeStdout(fmt.Sprintf("warning: missing file skipped: %s", strings.TrimPrefix(failedPath, "(missing) ")))
			} else {
				a.writeStderr(fmt.Sprintf("error: failed to delete: %s", failedPath))
				hardFailures = append(hardFailures, failedPath)
			}
		}
	}

	if len(hardFailures) > 0 {
		return transaction.rollback(fmt.Errorf("%d file(s) could not be removed — state left unchanged", len(hardFailures)))
	}

	if err := fs.RemoveResourceFromState(profile.TargetDir, resourceName); err != nil {
		return transaction.rollback(err)
	}

	a.writeStdout(fmt.Sprintf("Resource '%s' removed.", resourceName))
	return nil
}
