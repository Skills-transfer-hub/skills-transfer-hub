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
	if len(args) == 0 {
		return errors.New("usage : sth config <show|pull|push>")
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
		return fmt.Errorf("commande config inconnue : %q", args[0])
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
		a.writeStderr(fmt.Sprintf("Avertissement : sélection d'organisation périmée non effacée : %v", err))
		return
	}
	a.writeStderr(fmt.Sprintf("Avertissement : l'organisation active (%s) n'est plus accessible — retour à l'organisation par défaut.", active.OrgName))
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

	a.writeStdout("providers :")
	for _, entry := range result.Entries {
		if entry.Kind == fs.ConfigEntryProvider {
			a.writeStdout(fmt.Sprintf("  %-24s [%s]", entry.Field, entry.Provenance))
		}
	}
	a.writeStdout("pins :")
	for _, entry := range result.Entries {
		if entry.Kind != fs.ConfigEntryResource {
			continue
		}
		detail := entry.Detail
		if detail == "" || detail == "(none)" {
			detail = "(non épinglé)"
		}
		a.writeStdout(fmt.Sprintf("  %-24s [%s] %s", entry.Field, entry.Provenance, detail))
	}
	if remote.Config == nil {
		a.writeStdout("", "Aucune configuration partagée pour cette organisation (affichage local uniquement).")
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
		a.writeStdout("Aucune configuration partagée pour cette organisation.")
		return nil
	}
	result := fs.MergeOrgConfig(local, *remote.Config)
	changes := pullChanges(result.Entries)

	if dryRun {
		a.writeStdout(fmt.Sprintf("Configuration org (version %d) — aperçu :", remote.ConfigVersion))
		if len(changes) == 0 {
			a.writeStdout("  (aucun changement)")
		}
		for _, entry := range changes {
			a.writeStdout(formatPullChange(entry))
		}
		a.writeStdout("Aucune écriture (--dry-run).")
		return nil
	}

	if _, err := fs.WriteProjectConfigV2(result.Merged); err != nil {
		return err
	}
	a.writeStdout(fmt.Sprintf("Configuration org appliquée (version %d).", remote.ConfigVersion))
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
			"Publierait la configuration partagée (sous-ensemble local, sans secret) :",
			fmt.Sprintf("  %d provider(s), %d ressource(s) épinglée(s).", len(subset.Providers), countPins(subset.Resources)),
			"Aucune écriture (--dry-run).",
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
	a.writeStdout(fmt.Sprintf("Configuration partagée publiée (version %d).", version))
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
		return errors.New("la configuration a changé côté serveur — relance `sth config pull` puis `sth config push`.")
	case errors.Is(err, cloud.ErrForbiddenRole):
		return errors.New("publication refusée : rôle owner/admin requis sur cette organisation.")
	case errors.Is(err, cloud.ErrNotEntitled):
		return errors.New("publication refusée : la configuration partagée requiert l'offre Team (capability fleet_sync).")
	default:
		return a.translateOrgError(err)
	}
}
