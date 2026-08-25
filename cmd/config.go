package cmd

import (
	"context"
	"errors"
	"fmt"
	"time"

	"sth/internal/cloud"
	"sth/internal/fs"
)

const configCommandTimeout = 15 * time.Second

func (a *app) runConfig(args []string) error {
	if !experimentalEnabled() {
		return errExperimentalDisabled("sth config")
	}
	if len(args) == 0 {
		return errors.New("usage: sth config <show|pull|push>")
	}
	if isHelpFlag(args[0]) {
		a.writeStdout(commandHelpText("config"))
		return nil
	}
	switch args[0] {
	case "show":
		return a.runConfigShow()
	case "pull":
		flags, _ := parseFlags(args[1:], "--dry-run")
		return a.runConfigPull(flags["--dry-run"])
	case "push":
		flags, _ := parseFlags(args[1:], "--dry-run")
		return a.runConfigPush(flags["--dry-run"])
	default:
		return fmt.Errorf("unknown config command: %q", args[0])
	}
}

func activeOrgID() (string, error) {
	active, err := fs.ReadActiveOrg()
	if err != nil {
		return "", err
	}
	if active == nil {
		return "", nil
	}
	return active.OrgID, nil
}

func (a *app) fetchOrgConfig(ctx context.Context) (cloud.OrgConfig, error) {
	orgID, err := activeOrgID()
	if err != nil {
		return cloud.OrgConfig{}, err
	}
	result, err := cloud.GetOrgConfig(ctx, nil, orgID)
	if errors.Is(err, cloud.ErrForbiddenOrg) {
		a.healStaleActiveOrg()
		return cloud.GetOrgConfig(ctx, nil, "")
	}
	return result, err
}

func (a *app) healStaleActiveOrg() {
	active, _ := fs.ReadActiveOrg()
	if active == nil {
		return
	}
	if err := fs.ClearActiveOrg(); err != nil {
		a.writeStderr(fmt.Sprintf("Warning: stale organization selection was not cleared: %v", err))
		return
	}
	a.writeStderr(fmt.Sprintf("Warning: the active organization (%s) is no longer accessible — reverting to the default organization.", active.OrgName))
}

func (a *app) runConfigShow() error {
	local, err := fs.ReadProjectConfigV2()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), configCommandTimeout)
	defer cancel()
	remote, err := a.fetchOrgConfig(ctx)
	if err != nil {
		return a.translateOrgError(err)
	}
	orgConfig := fs.ProjectConfigV2{SchemaVersion: 2}
	if remote.Config != nil {
		orgConfig = *remote.Config
	}
	result := fs.MergeOrgConfig(local, orgConfig)

	a.writeStdout("providers:")
	for _, entry := range result.Entries {
		if entry.Kind == fs.ConfigEntryProvider {
			a.writeStdout(fmt.Sprintf("  %-24s [%s]", entry.Field, entry.Provenance))
		}
	}
	a.writeStdout("pins:")
	for _, entry := range result.Entries {
		if entry.Kind != fs.ConfigEntryResource {
			continue
		}
		detail := entry.Detail
		if detail == "" || detail == "(none)" {
			detail = "(unpinned)"
		}
		a.writeStdout(fmt.Sprintf("  %-24s [%s] %s", entry.Field, entry.Provenance, detail))
	}
	if remote.Config == nil {
		a.writeStdout("", "No shared configuration for this organization (showing local values only).")
	}
	return nil
}

func (a *app) runConfigPull(dryRun bool) error {
	local, err := fs.ReadProjectConfigV2()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), configCommandTimeout)
	defer cancel()
	remote, err := a.fetchOrgConfig(ctx)
	if err != nil {
		return a.translateOrgError(err)
	}
	if remote.Config == nil {
		a.writeStdout("No shared configuration for this organization.")
		return nil
	}
	result := fs.MergeOrgConfig(local, *remote.Config)
	changes := pullChanges(result.Entries)

	if dryRun {
		a.writeStdout(fmt.Sprintf("Organization configuration (version %d) — preview:", remote.ConfigVersion))
		if len(changes) == 0 {
			a.writeStdout("  (no changes)")
		}
		for _, entry := range changes {
			a.writeStdout(formatPullChange(entry))
		}
		a.writeStdout("No files written (--dry-run).")
		return nil
	}

	if _, err := fs.WriteProjectConfigV2(result.Merged); err != nil {
		return err
	}
	a.writeStdout(fmt.Sprintf("Organization configuration applied (version %d).", remote.ConfigVersion))
	for _, entry := range changes {
		a.writeStdout(formatPullChange(entry))
	}
	return nil
}

func (a *app) runConfigPush(dryRun bool) error {
	local, err := fs.ReadProjectConfigV2()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), configCommandTimeout)
	defer cancel()
	if err := a.enforceCapability(ctx, "fleet_sync"); err != nil {
		return err
	}
	subset := fs.ShareableSubset(local)

	if dryRun {
		a.writeStdout(
			"Would publish the shared configuration (local subset, without secrets):",
			fmt.Sprintf("  %d provider(s), %d pinned resource(s).", len(subset.Providers), countPins(subset.Resources)),
			"No files written (--dry-run).",
		)
		return nil
	}

	orgID, err := activeOrgID()
	if err != nil {
		return err
	}
	current, err := cloud.GetOrgConfig(ctx, nil, orgID)
	if errors.Is(err, cloud.ErrForbiddenOrg) {
		a.healStaleActiveOrg()
		orgID = ""
		current, err = cloud.GetOrgConfig(ctx, nil, orgID)
	}
	if err != nil {
		return a.translateOrgError(err)
	}

	version, err := cloud.PutOrgConfig(ctx, nil, orgID, subset, current.ConfigVersion)
	if err != nil {
		return a.translatePushError(err)
	}
	a.writeStdout(fmt.Sprintf("Shared configuration published (version %d).", version))
	return nil
}

func pullChanges(entries []fs.ConfigMergeEntry) []fs.ConfigMergeEntry {
	changes := make([]fs.ConfigMergeEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Action == fs.MergeActionUnchanged {
			continue
		}
		changes = append(changes, entry)
	}
	return changes
}

func formatPullChange(entry fs.ConfigMergeEntry) string {
	label := fmt.Sprintf("  [%s] %s %s", entry.Action, entry.Kind, entry.Field)
	if entry.Detail != "" {
		label += fmt.Sprintf("  (%s)", entry.Detail)
	}
	return label
}

func countPins(resources []fs.ResourceEntry) int {
	count := 0
	for _, resource := range resources {
		if resource.PinnedVersion != "" {
			count++
		}
	}
	return count
}

func (a *app) translatePushError(err error) error {
	switch {
	case errors.Is(err, cloud.ErrVersionConflict):
		return errors.New("the server-side configuration changed — run `sth config pull` and then `sth config push` again")
	case errors.Is(err, cloud.ErrForbiddenRole):
		return errors.New("publication rejected: the owner or admin role is required for this organization")
	case errors.Is(err, cloud.ErrNotEntitled):
		return errors.New("publication rejected: shared configuration requires the Team plan (fleet_sync capability)")
	default:
		return a.translateOrgError(err)
	}
}
