package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"sth/internal/artifact"
	"sth/internal/fs"
)

func (a *app) runList() error {
	return a.runListWithArgs(nil)
}

func (a *app) runListWithArgs(args []string) (resultErr error) {
	defer func() {
		if !slices.Contains(args, "--installed") && !hasArtifactListOptions(args) {
			a.recordOperationResult("list", "", resultErr)
		}
		if resultErr != nil && slices.Contains(args, "--json") {
			emitJSONError(a.stdout, resultErr, 1)
		}
	}()
	if hasArtifactListOptions(args) {
		return a.runArtifactList(args)
	}
	flags, _ := parseFlags(args, "--json", "--installed")
	jsonOut := flags["--json"]

	if cfgPath, err := fs.ProjectProfilePath(); err == nil {
		if abs, absErr := filepath.Abs(cfgPath); absErr == nil {
			a.verbose.Path("project.json", abs)
		}
	}

	if flags["--installed"] {
		return a.runInstalledList(jsonOut)
	}

	cfg, err := fs.ReadProjectConfigV2ReadOnly()
	if err != nil {
		return err
	}

	catalog, err := a.aggregateCatalog(context.Background(), cfg)
	if err != nil {
		return err
	}
	a.reportProviderFailures(catalog.failures)
	a.describeCatalogSkills(context.Background(), cfg, catalog.entries)
	if jsonOut {
		payload, err := json.Marshal(catalog.entries)
		if err != nil {
			return err
		}
		a.writeStdout(string(payload))
		return nil
	}

	if len(catalog.entries) == 0 {
		a.writeStdout("No resources available in the remote catalog.")
		return nil
	}

	showProvider := len(fs.UsableProviders(cfg)) > 1
	lines := []string{fmt.Sprintf("%d resource(s) available:", len(catalog.entries))}
	for _, entry := range catalog.entries {
		line := fmt.Sprintf("- [%s] %s", entry.ResourceType, catalogDisplayRef(entry))
		if showProvider {
			line = fmt.Sprintf("%s [%s]", line, fs.CatalogEntryProviderID(entry))
		}
		if entry.ResourceType != "bundle" {
			line = fmt.Sprintf("%s (folder: %s)", line, entry.FolderName)
		}
		if entry.Description != "" {
			line += fmt.Sprintf(" — %s", entry.Description)
		}
		lines = append(lines, line)
	}

	a.writeStdout(lines...)
	return nil
}

type artifactListEntry struct {
	ID            string                       `json:"id"`
	Kind          artifact.Kind                `json:"kind"`
	Dialect       artifact.Dialect             `json:"dialect"`
	Path          string                       `json:"path"`
	Scope         artifact.Scope               `json:"scope"`
	Status        fs.ResourceStatus            `json:"status,omitempty"`
	Compatibility []artifact.CompatibilityPlan `json:"compatibility"`
}

func hasArtifactListOptions(args []string) bool {
	for _, argument := range args {
		for _, prefix := range []string{"--type", "--scope", "--target", "--status"} {
			if argument == prefix || strings.HasPrefix(argument, prefix+"=") {
				return true
			}
		}
	}
	return false
}

func (a *app) runArtifactList(args []string) error {
	options, err := parseCommandOptions(args, []string{"--json"}, []string{"--type", "--scope", "--target", "--status"})
	if err != nil {
		return err
	}
	if len(options.positional) != 0 {
		return fmt.Errorf("usage: sth list [--type <kind>] [--scope <scope>] [--target <target>] [--status <status>] [--json]")
	}
	targets := optionValues(options, "--target")
	inspections, err := inspectProjectArtifacts("", targets, false)
	if err != nil {
		return err
	}
	typeFilter := stringSet(optionValues(options, "--type"))
	scopeFilter := stringSet(optionValues(options, "--scope"))
	statusFilter := stringSet(optionValues(options, "--status"))
	projectRoot, _ := filepath.Abs(".")
	targetDirectories := make([]string, 0)
	if config, configErr := fs.ReadProjectConfigV2ReadOnly(); configErr == nil {
		for _, provider := range config.Providers {
			targetDirectories = append(targetDirectories, provider.TargetDir)
		}
	}
	state, stateErr := fs.ReadInstalledStateV3OrLegacy(projectRoot, targetDirectories)
	statuses := map[string]fs.ResourceStatus{}
	if stateErr == nil {
		statusResults := fs.ComputeStatusesV3(state, projectRoot)
		for resourceIndex, result := range statusResults {
			statuses[result.ResourceID] = result.Status
			if resourceIndex >= len(state.Resources) {
				continue
			}
			resource := state.Resources[resourceIndex]
			statuses[resource.Source.Path] = result.Status
			for _, output := range resource.Outputs {
				statuses[output.Path] = result.Status
			}
		}
	}
	entries := make([]artifactListEntry, 0)
	for _, inspection := range inspections {
		definition := inspection.Definition
		status := statuses[definition.ID]
		if status == "" {
			status = statuses[definition.SourcePath]
		}
		if len(typeFilter) > 0 && !typeFilter[string(definition.Kind)] {
			continue
		}
		if len(scopeFilter) > 0 && !scopeFilter[definition.Scope.Level] && !scopeFilter[definition.Scope.Path] {
			continue
		}
		if len(statusFilter) > 0 && !statusFilter[string(status)] {
			continue
		}
		entries = append(entries, artifactListEntry{ID: definition.ID, Kind: definition.Kind, Dialect: definition.SourceDialect, Path: definition.SourcePath, Scope: definition.Scope, Status: status, Compatibility: inspection.Plans})
	}
	if options.flags["--json"] {
		data, marshalErr := json.MarshalIndent(entries, "", "  ")
		if marshalErr != nil {
			return marshalErr
		}
		a.writeStdout(string(data))
		return nil
	}
	if len(entries) == 0 {
		a.writeStdout("No local artifacts matched the filters.")
		return nil
	}
	for _, entry := range entries {
		status := string(entry.Status)
		if status == "" {
			status = "not-installed"
		}
		a.writeStdout(fmt.Sprintf("%-28s %-12s %-20s %s", entry.ID, entry.Kind, status, entry.Path))
	}
	return nil
}

func stringSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}

// listJSONEntry matches FR-020 acceptance scenario 1.
type listJSONEntry struct {
	ResourceName    string `json:"resource_name"`
	ProviderID      string `json:"provider_id"`
	Status          string `json:"status"`
	ResolvedVersion string `json:"resolved_version"`
	PinnedVersion   any    `json:"pinned_version"` // null when unpinned
}

func (a *app) runListJSON() error { return a.runInstalledList(true) }

func (a *app) runInstalledList(jsonOut bool) error {
	cfg, err := fs.ReadProjectConfigV2()
	if err != nil {
		return err
	}
	if len(cfg.Providers) == 0 {
		if jsonOut {
			a.writeStdout("[]")
		} else {
			a.writeStdout("Aucun skill installé. Choisissez Installer un skill.")
		}
		return nil
	}

	stateDirs, dirsErr := projectStateDirs(cfg)
	if dirsErr != nil {
		return dirsErr
	}

	state, err := fs.ReadInstalledStateAcrossDirs(stateDirs)
	if err != nil {
		return err
	}

	projectRoot, _ := filepath.Abs(".")
	results := fs.GroupStatusesByResource(fs.ComputeAllStatuses(state, cfg, nil, projectRoot))

	out := make([]listJSONEntry, 0, len(results))
	for _, result := range results {
		entry := listJSONEntry{
			ResourceName:    result.ResourceName,
			ProviderID:      result.ProviderID,
			Status:          string(result.Status),
			ResolvedVersion: result.InstalledVersion,
			PinnedVersion:   nil,
		}
		if result.Pinned {
			entry.PinnedVersion = result.PinnedVersion
		}
		out = append(out, entry)
	}

	if !jsonOut {
		if len(out) == 0 {
			a.writeStdout("Aucun skill installé. Choisissez Installer un skill.")
		}
		for _, entry := range out {
			pin := "non verrouillée"
			if entry.PinnedVersion != nil {
				pin = fmt.Sprintf("verrouillée sur %v", entry.PinnedVersion)
			}
			a.writeStdout(fmt.Sprintf("%s [%s] — %s — version %s — %s", entry.ResourceName, entry.ProviderID, entry.Status, entry.ResolvedVersion, pin))
		}
		return nil
	}
	data, err := json.Marshal(out)
	if err != nil {
		return err
	}
	a.writeStdout(string(data))
	return nil
}

func catalogDisplayRef(entry fs.CatalogEntry) string {
	if entry.ResourceType == "bundle" {
		return entry.FolderName + "/" + entry.Name
	}

	return entry.Name
}
