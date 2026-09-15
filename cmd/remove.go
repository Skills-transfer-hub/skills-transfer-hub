package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"sth/internal/fs"
	"sth/internal/tui"
)

func (a *app) runRemove(args []string) error {
	options, optionErr := parseCommandOptions(args, []string{"--yes", "--json"}, []string{"--provider"})
	if optionErr != nil {
		return optionErr
	}
	flags, positional := options.flags, options.positional
	if flags["--json"] {
		originalOutput := a.stdout
		var output bytes.Buffer
		a.stdout = &output
		defer func() { a.stdout = originalOutput }()
		filtered := make([]string, 0, len(args))
		for _, argument := range args {
			if argument != "--json" {
				filtered = append(filtered, argument)
			}
		}
		err := a.runRemove(filtered)
		a.stdout = originalOutput
		if err != nil {
			emitJSONError(a.stdout, err, 1)
			return err
		}
		payload, _ := json.Marshal(map[string]any{"resource_name": resourceFromArgs(filtered), "outcome": "removed", "message": strings.TrimSpace(output.String())})
		a.writeStdout(string(payload))
		return nil
	}
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

	config, err := fs.ReadProjectConfigV2ReadOnly()
	if err != nil {
		return err
	}
	selections, err := installedSelections(config)
	if err != nil {
		return err
	}
	providerFlag := ""
	if providers := optionValues(options, "--provider"); len(providers) == 1 {
		providerFlag = providers[0]
	} else if len(providers) > 1 {
		return fmt.Errorf("--provider accepts one source ID")
	}
	selected, err := resolveInstalledSelection(selections, resourceName, providerFlag)
	if err != nil {
		return err
	}
	resourceName = selected.resourceName
	records := selected.records
	providerRelease, err := a.prepareProviderStateAccess(selected.stateDir, profile.TargetDir)
	if err != nil {
		return err
	}
	defer providerRelease()
	state := fs.InstalledResourceStateV2{SchemaVersion: 2, Entries: records}
	if driftErr := refuseModifiedResources(state, config, resourceName); driftErr != nil {
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
	if handled, removeErr := a.removeRecordsWithManagedRegions(selected.stateDir, resourceName, records); handled {
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
	transaction, transactionErr := newFanoutFileTransaction(projectRoot, fanoutTransactionPaths(selected.stateDir, managedFiles, nil))
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

	if err := fs.RemoveResourceFromState(selected.stateDir, resourceName, records[0].ProviderID); err != nil {
		return transaction.rollback(err)
	}

	a.writeStdout(fmt.Sprintf("Resource '%s' removed.", resourceName))
	return nil
}
