package cmd

import (
	"context"
	"fmt"
	"slices"

	"sth/internal/fs"
	sthhttp "sth/internal/http"
)

func selectGlobalInstallations(state fs.GlobalState, selectors []string) ([]fs.GlobalInstallation, error) {
	if len(selectors) == 0 {
		return slices.Clone(state.Installations), nil
	}
	selected := []fs.GlobalInstallation{}
	for _, installation := range state.Installations {
		if installation.ID == selectors[0] {
			return []fs.GlobalInstallation{installation}, nil
		}
		if installation.SkillName == selectors[0] {
			selected = append(selected, installation)
		}
	}
	if len(selected) != 1 {
		return nil, globalSelectionError("global skill %q matches %d installations; use a canonical ID from sth list --global", selectors[0], len(selected))
	}
	return selected, nil
}

func (a *app) globalStatus(installContext fs.GlobalContext, installations []fs.GlobalInstallation, result *globalResult) error {
	for _, installation := range installations {
		resource := makeGlobalResource(installation, "installed")
		files, err := fs.InspectGlobalInstallation(installContext, installation)
		if err != nil {
			return err
		}
		resource.Files = files
		resource.FileState = "files-installed"
		for _, file := range files {
			if file.Status != "ok" {
				resource.FileState = file.Status
			}
		}
		resource.RemoteState = "pinned"
		if !installation.Pinned {
			resolution, err := a.client.ResolveGlobalSource(context.Background(), globalHTTPSource(installation.Source), installation.RequestedRef)
			switch {
			case err != nil:
				resource.RemoteState = "unknown"
				result.Warnings = append(result.Warnings, fmt.Sprintf("%s: remote update status unknown: %v", installation.ID, err))
			case resolution.Revision == installation.ResolvedRevision:
				resource.RemoteState = "current"
			default:
				resource.RemoteState = "outdated"
			}
		}
		result.Resources = append(result.Resources, resource)
	}
	return nil
}

func (a *app) globalMutate(installContext fs.GlobalContext, state fs.GlobalState, selected []fs.GlobalInstallation, operation string, options globalOptions, result *globalResult) error {
	for _, installation := range selected {
		resource, nextState, warnings, err := a.globalMutateOne(installContext, state, installation, operation, options)
		result.Warnings = append(result.Warnings, warnings...)
		if err != nil {
			resource = makeGlobalResource(installation, "preserved")
			resource.Error = err.Error()
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", installation.ID, err))
		} else {
			state = nextState
		}
		result.Resources = append(result.Resources, resource)
	}
	if len(result.Errors) > 0 {
		return fmt.Errorf("global %s failed for %d of %d installations", operation, len(result.Errors), len(selected))
	}
	return nil
}

func (a *app) globalMutateOne(installContext fs.GlobalContext, state fs.GlobalState, installation fs.GlobalInstallation, operation string, options globalOptions) (globalResourceResult, fs.GlobalState, []string, error) {
	after := fs.GlobalState{SchemaVersion: 1, Installations: slices.Clone(state.Installations)}
	warnings := []string{}
	writes := []fs.GlobalWrite{}
	outcome := "removed"
	if operation == "remove" {
		after.Installations = slices.DeleteFunc(after.Installations, func(candidate fs.GlobalInstallation) bool { return candidate.ID == installation.ID })
	} else {
		reference := installation.RequestedRef
		if installation.Pinned {
			reference = installation.ResolvedRevision
		}
		resolution, err := a.client.ResolveGlobalSource(context.Background(), globalHTTPSource(installation.Source), reference)
		if err != nil {
			return globalResourceResult{}, state, warnings, err
		}
		bundle, err := a.client.DownloadGlobalSkill(context.Background(), resolution, sthhttp.GlobalCandidate{Name: installation.SkillName, Path: installation.Source.Subdir})
		if err != nil {
			return globalResourceResult{}, state, warnings, err
		}
		replacement, plannedWrites, planWarnings, err := planGlobalBundle(installContext, bundle, installation.RequestedConsumers, installation.RequestedRef, installation.Pinned, installation.AllAgents, &installation)
		if err != nil {
			return globalResourceResult{}, state, warnings, err
		}
		warnings = append(warnings, bundle.Warnings...)
		warnings = append(warnings, planWarnings...)
		installation = replacement
		writes = plannedWrites
		after = replaceGlobalInstallation(after, replacement)
		outcome = "updated"
	}
	mutation, err := fs.ApplyGlobalChange(installContext, state, after, writes, fs.GlobalMutationOptions{DryRun: options.DryRun, Force: options.Force})
	if err != nil {
		return globalResourceResult{}, state, warnings, err
	}
	warnings = append(warnings, mutation.Warnings...)
	resource := makeGlobalResource(installation, outcome)
	resource.BackupPath = mutation.BackupPath
	resource.FileState = "files-installed"
	if operation == "remove" {
		resource.FileState = "removed"
	}
	if !mutation.Changed {
		resource.Outcome = "unchanged"
	}
	if options.DryRun {
		resource.Outcome = "planned"
		resource.FileState = "planned"
		after = state
	}
	return resource, after, warnings, nil
}

func globalHTTPSource(source fs.GlobalSource) sthhttp.GlobalSource {
	return sthhttp.GlobalSource{Provider: source.Provider, Host: source.Host, Repository: source.Repository}
}
