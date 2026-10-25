package cmd

import (
	"bufio"
	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"time"

	"sth/internal/fs"
	sthhttp "sth/internal/http"
	"sth/internal/writer"
)

func (a *app) installGlobal(installContext fs.GlobalContext, state fs.GlobalState, options globalOptions, result *globalResult) error {
	source, err := sthhttp.ParseGlobalSource(options.Positional[0])
	if err != nil {
		return globalSelectionError("%s", err)
	}
	input := bufio.NewReader(a.stdin)
	options, err = a.chooseGlobalAgents(input, options)
	if err != nil {
		return err
	}
	consumers, warnings, err := writer.ResolveGlobalConsumers(options.Agents, options.AllAgents)
	if err != nil {
		return globalSelectionError("%s", err)
	}
	result.Warnings = append(result.Warnings, warnings...)
	candidates, resolutions, err := a.resolveGlobalInstallCandidates(state, source, options)
	if err != nil {
		return err
	}
	selected, err := a.chooseGlobalSkills(input, candidates, options)
	if err != nil {
		return globalSelectionError("%s", err)
	}
	after := fs.GlobalState{SchemaVersion: 1, Installations: slices.Clone(state.Installations)}
	writes := []fs.GlobalWrite{}
	planned := []fs.GlobalInstallation{}
	for _, candidate := range selected {
		previous := findGlobalInstallation(state, source.CanonicalID(candidate.Path))
		selectedResolution := resolutions[candidate.Path]
		bundle, err := a.client.DownloadGlobalSkill(context.Background(), selectedResolution, candidate)
		if err != nil {
			return err
		}
		requested := slices.Clone(consumers)
		requestedRef := selectedResolution.RequestedRef
		pinned := options.Ref != ""
		allAgents := options.AllAgents
		if previous != nil {
			requested = append(requested, previous.RequestedConsumers...)
			allAgents = allAgents || previous.AllAgents
			if options.Ref == "" {
				requestedRef = previous.RequestedRef
				pinned = previous.Pinned
			}
		}
		sort.Strings(requested)
		requested = slices.Compact(requested)
		installation, bundleWrites, planWarnings, err := planGlobalBundle(installContext, bundle, requested, requestedRef, pinned, allAgents, previous)
		if err != nil {
			return err
		}
		result.Warnings = append(result.Warnings, bundle.Warnings...)
		result.Warnings = append(result.Warnings, planWarnings...)
		after = replaceGlobalInstallation(after, installation)
		planned = append(planned, installation)
		writes = append(writes, bundleWrites...)
	}
	mutation, err := fs.ApplyGlobalChange(installContext, state, after, writes, fs.GlobalMutationOptions{DryRun: options.DryRun, Force: options.Force})
	if err != nil {
		return err
	}
	result.Warnings = append(result.Warnings, mutation.Warnings...)
	result.Warnings = append(result.Warnings, "Only skill files are installed. Plugin features, hooks and permanent activation are not installed; activation depends on the host.")
	for _, installation := range planned {
		resource := makeGlobalResource(installation, "installed")
		resource.BackupPath = mutation.BackupPath
		resource.FileState = "files-installed"
		if !mutation.Changed {
			resource.Outcome = "unchanged"
		}
		if options.DryRun {
			resource.Outcome = "planned"
			resource.FileState = "planned"
		}
		result.Resources = append(result.Resources, resource)
	}
	return nil
}

func planGlobalBundle(installContext fs.GlobalContext, bundle sthhttp.ResolvedSkillBundle, consumers []string, requestedRef string, pinned, allAgents bool, previous *fs.GlobalInstallation) (fs.GlobalInstallation, []fs.GlobalWrite, []string, error) {
	destinations, warnings, err := writer.PlanGlobalDestinations(installContext.Home, bundle.Name, consumers)
	if err != nil {
		return fs.GlobalInstallation{}, nil, nil, err
	}
	timestamp := time.Now().UTC().Format(time.RFC3339Nano)
	installation := fs.GlobalInstallation{
		ID: bundle.Source.CanonicalID(bundle.Path), SkillName: bundle.Name,
		Source:       fs.GlobalSource{Provider: bundle.Source.Provider, Host: bundle.Source.Host, Repository: bundle.Source.Repository, Subdir: bundle.Path},
		RequestedRef: requestedRef, Pinned: pinned, ResolvedRevision: bundle.Revision,
		RequestedConsumers: consumers, AllAgents: allAgents, InstalledAt: timestamp, UpdatedAt: timestamp,
		Placements: []fs.GlobalPlacement{},
	}
	writes := []fs.GlobalWrite{}
	for _, destination := range destinations {
		placement := fs.GlobalPlacement{RootID: destination.RootID, RelativePath: destination.RelativePath, ResolvedRoot: destination.Root, Consumers: destination.RequestedConsumers, Files: []fs.GlobalFile{}}
		for _, file := range bundle.Files {
			relativePath := filepath.ToSlash(filepath.Join(destination.RelativePath, file.Path))
			placement.Files = append(placement.Files, globalManifestFile(destination.RootID, relativePath, file))
			writes = append(writes, fs.GlobalWrite{RootID: destination.RootID, RelativePath: relativePath, Data: file.Content, Mode: file.Mode})
		}
		installation.Placements = append(installation.Placements, placement)
	}
	provenanceDirectory := fmt.Sprintf("%x", sha256.Sum256([]byte(installation.ID)))
	for _, file := range bundle.Provenance {
		relativePath := provenanceDirectory + "/" + file.Path
		installation.Provenance = append(installation.Provenance, globalManifestFile("provenance", relativePath, file))
		writes = append(writes, fs.GlobalWrite{RootID: "provenance", RelativePath: relativePath, Data: file.Content, Mode: file.Mode})
	}
	if previous != nil {
		installation.InstalledAt = previous.InstalledAt
		installation.UpdatedAt = previous.UpdatedAt
		installation.PreviousRevision = previous.PreviousRevision
		if previous.ResolvedRevision != installation.ResolvedRevision {
			installation.PreviousRevision = previous.ResolvedRevision
		}
		if !reflect.DeepEqual(installation, *previous) {
			installation.UpdatedAt = timestamp
		}
	}
	return installation, writes, warnings, nil
}

func globalManifestFile(rootID, relativePath string, file sthhttp.GlobalBundleFile) fs.GlobalFile {
	return fs.GlobalFile{RootID: rootID, RelativePath: relativePath, SHA256: fmt.Sprintf("%x", sha256.Sum256(file.Content)), Mode: file.Mode}
}

func findGlobalInstallation(state fs.GlobalState, identity string) *fs.GlobalInstallation {
	for _, installation := range state.Installations {
		if installation.ID == identity {
			return &installation
		}
	}
	return nil
}

func replaceGlobalInstallation(state fs.GlobalState, installation fs.GlobalInstallation) fs.GlobalState {
	for index, existing := range state.Installations {
		if existing.ID == installation.ID {
			state.Installations[index] = installation
			return state
		}
	}
	state.Installations = append(state.Installations, installation)
	return state
}

func makeGlobalResource(installation fs.GlobalInstallation, outcome string) globalResourceResult {
	destinations := []writer.GlobalDestination{}
	for _, placement := range installation.Placements {
		readers := []string{}
		for _, consumer := range writer.GlobalConsumers() {
			if slices.Contains(consumer.ReadRoots, placement.RootID) {
				readers = append(readers, consumer.ID)
			}
		}
		sort.Strings(readers)
		destinations = append(destinations, writer.GlobalDestination{RootID: placement.RootID, Root: placement.ResolvedRoot, RelativePath: placement.RelativePath, RequestedConsumers: placement.Consumers, PossibleReaders: readers})
	}
	return globalResourceResult{GlobalInstallation: installation, Outcome: outcome, FileState: "not-checked", ValidationLevel: "discovery-unverified", Files: []fs.GlobalFileStatus{}, Destinations: destinations}
}
