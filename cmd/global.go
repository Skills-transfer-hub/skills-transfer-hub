package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"sth/internal/fs"
	sthhttp "sth/internal/http"
	"sth/internal/writer"
)

type globalResult struct {
	SchemaVersion int                    `json:"schema_version"`
	Scope         string                 `json:"scope"`
	Operation     string                 `json:"operation"`
	Outcome       string                 `json:"outcome"`
	Resources     []globalResourceResult `json:"resources"`
	Warnings      []string               `json:"warnings"`
	Errors        []string               `json:"errors"`
}

type globalResourceResult struct {
	fs.GlobalInstallation
	Outcome         string                     `json:"outcome"`
	ValidationLevel string                     `json:"validation_level"`
	FileState       string                     `json:"file_state"`
	RemoteState     string                     `json:"remote_state,omitempty"`
	Files           []fs.GlobalFileStatus      `json:"files"`
	BackupPath      string                     `json:"backup_path,omitempty"`
	Destinations    []writer.GlobalDestination `json:"destinations"`
	Error           string                     `json:"error,omitempty"`
}

func (a *app) runGlobal(operation string, arguments []string) error {
	result := globalResult{SchemaVersion: 1, Scope: "global", Operation: operation, Outcome: "success", Resources: []globalResourceResult{}, Warnings: []string{}, Errors: []string{}}
	options, err := parseGlobalOptions(operation, arguments)
	if err == nil && options.Help {
		a.writeStdout(commandHelpText(operation))
		return nil
	}
	if err == nil {
		err = a.executeGlobal(operation, options, &result)
	}
	if err != nil {
		var selectionError *sthhttp.GlobalSelectionError
		if errors.As(err, &selectionError) {
			err = globalSelectionError("%s", err)
		}
		result.Outcome = "error"
		if len(result.Errors) == 0 {
			result.Errors = append(result.Errors, err.Error())
		}
	}
	result.Warnings = uniqueGlobalWarnings(result.Warnings)
	if options.JSON || slices.Contains(arguments, "--json") {
		if encodeErr := json.NewEncoder(a.stdout).Encode(result); encodeErr != nil {
			return encodeErr
		}
	} else {
		for _, resource := range result.Resources {
			a.writeStdout(fmt.Sprintf("%s: %s (%s) [%s]", resource.SkillName, resource.Outcome, resource.ResolvedRevision, resource.ValidationLevel))
			a.writeStdout(fmt.Sprintf("  Requested consumers: %v", resource.RequestedConsumers))
			for _, destination := range resource.Destinations {
				a.writeStdout(fmt.Sprintf("  Possible readers of %s: %v", filepath.Join(destination.Root, destination.RelativePath), destination.PossibleReaders))
			}
			if resource.BackupPath != "" {
				a.writeStdout("  Backup: " + resource.BackupPath)
			}
			if resource.RemoteState != "" {
				a.writeStdout("  Files: " + resource.FileState + "; remote: " + resource.RemoteState)
			}
			for _, placement := range resource.Placements {
				a.writeStdout("  " + filepath.Join(placement.ResolvedRoot, placement.RelativePath))
			}
		}
		if err == nil && len(result.Resources) == 0 {
			a.writeStdout("No global skills installed.")
		}
		for _, warning := range result.Warnings {
			a.writeStderr("Warning: " + warning)
		}
		for _, detail := range result.Errors {
			if err == nil || detail != err.Error() {
				a.writeStderr("Error: " + detail)
			}
		}
	}
	if err != nil {
		a.writeStderr("Error: " + err.Error())
	}
	return err
}

func (a *app) executeGlobal(operation string, options globalOptions, result *globalResult) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	installContext, err := fs.NewGlobalContext(home, writer.GlobalRoots(home))
	if err != nil {
		return err
	}
	if operation == "install" {
		if _, err := sthhttp.ParseGlobalSource(options.Positional[0]); err != nil {
			return err
		}
		if len(options.Agents) > 0 || options.AllAgents || options.JSON || options.Yes || !a.isInteractiveErrorSession() {
			if _, _, err := writer.ResolveGlobalConsumers(options.Agents, options.AllAgents); err != nil {
				return globalSelectionError("%s", err)
			}
		}
	}
	if !options.DryRun && (operation == "install" || operation == "update" || operation == "remove") {
		if err := fs.RecoverGlobalTransactions(installContext); err != nil {
			return err
		}
	}
	state, err := fs.ReadGlobalState(installContext)
	if err != nil {
		return err
	}
	if operation == "install" {
		return a.installGlobal(installContext, state, options, result)
	}
	selected, err := selectGlobalInstallations(state, options.Positional)
	if err != nil {
		return err
	}
	if operation == "status" {
		return a.globalStatus(installContext, selected, result)
	}
	if operation == "update" || operation == "remove" {
		return a.globalMutate(installContext, state, selected, operation, options, result)
	}
	for _, installation := range state.Installations {
		result.Resources = append(result.Resources, makeGlobalResource(installation, "installed"))
	}
	return nil
}

func uniqueGlobalWarnings(warnings []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, warning := range warnings {
		if !seen[warning] {
			seen[warning] = true
			result = append(result, warning)
		}
	}
	return result
}
