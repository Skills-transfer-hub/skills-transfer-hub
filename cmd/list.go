package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sth/internal/fs"
)

// runList implements `sth list`. With --json, the semantics differ from text
// mode: text mode shows the remote catalog; JSON mode shows the installed
// resources state (FR-020 schema). The schema documents this divergence.
func (a *app) runList() error {
	return a.runListWithArgs(nil)
}

func (a *app) runListWithArgs(args []string) error {
	flags, _ := parseFlags(args, "--json")
	jsonOut := flags["--json"]

	if cfgPath, err := fs.ProjectProfilePath(); err == nil {
		if abs, absErr := filepath.Abs(cfgPath); absErr == nil {
			a.verbose.Path("project.json", abs)
		}
	}

	if jsonOut {
		return a.runListJSON()
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

// listJSONEntry matches FR-020 acceptance scenario 1.
type listJSONEntry struct {
	ResourceName    string `json:"resource_name"`
	ProviderID      string `json:"provider_id"`
	Status          string `json:"status"`
	ResolvedVersion string `json:"resolved_version"`
	PinnedVersion   any    `json:"pinned_version"` // null when unpinned
}

func (a *app) runListJSON() error {
	cfg, err := fs.ReadProjectConfigV2()
	if err != nil {
		emitJSONError(a.stdout, err, 1)
		return err
	}
	if len(cfg.Providers) == 0 {
		a.writeStdout("[]")
		return nil
	}

	stateDirs, dirsErr := projectStateDirs(cfg)
	if dirsErr != nil {
		emitJSONError(a.stdout, dirsErr, 1)
		return dirsErr
	}

	state, err := fs.ReadInstalledStateAcrossDirs(stateDirs)
	if err != nil {
		emitJSONError(a.stdout, err, 1)
		return err
	}

	projectRoot, _ := filepath.Abs(".")
	results := fs.ComputeAllStatuses(state, cfg, nil, projectRoot)

	out := make([]listJSONEntry, 0, len(results))
	for _, r := range results {
		entry := listJSONEntry{
			ResourceName:    r.ResourceName,
			ProviderID:      r.ProviderID,
			Status:          string(r.Status),
			ResolvedVersion: r.InstalledVersion,
			PinnedVersion:   nil,
		}
		if r.Pinned {
			entry.PinnedVersion = r.PinnedVersion
		}
		out = append(out, entry)
	}

	data, err := json.Marshal(out)
	if err != nil {
		emitJSONError(a.stdout, err, 1)
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
