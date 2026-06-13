package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"sth/internal/cloud"
	"sth/internal/fs"
)

const orgCommandTimeout = 10 * time.Second

func (a *app) runOrg(args []string) error {
	if len(args) == 0 {
		return errors.New("usage : sth org <current|list|use>")
	}
	if isHelpFlag(args[0]) {
		a.writeStdout(commandHelpText("org"))
		return nil
	}
	switch args[0] {
	case "current":
		flags, _ := parseFlags(args[1:], "--json")
		return a.runOrgCurrent(flags["--json"])
	case "list":
		flags, _ := parseFlags(args[1:], "--json")
		return a.runOrgList(flags["--json"])
	case "use":
		_, positional := parseFlags(args[1:], "--json")
		return a.runOrgUse(positional)
	default:
		return fmt.Errorf("commande org inconnue : %q", args[0])
	}
}

type orgCurrentRecord struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Role   string `json:"role"`
	Plan   string `json:"plan"`
	Active bool   `json:"active"`
	Source string `json:"source"`
}

func (a *app) runOrgCurrent(asJSON bool) error {
	active, err := fs.ReadActiveOrg()
	if err != nil {
		return err
	}
	if active != nil {
		a.writeOrgCurrent(orgCurrentRecord{
			ID:     active.OrgID,
			Name:   active.OrgName,
			Role:   active.Role,
			Plan:   active.Plan,
			Active: true,
			Source: "selected",
		}, asJSON)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), orgCommandTimeout)
	defer cancel()
	orgs, err := cloud.ListOrgs(ctx, nil)
	if err != nil {
		if asJSON {
			return a.translateOrgError(err)
		}
		a.writeStdout(
			"Organisation active : organisation par défaut de la clé.",
			"Détails indisponibles hors-ligne — réessaie en ligne ou lance `sth org list`.",
		)
		return nil
	}
	home, found := homeOrg(orgs)
	if !found {
		a.writeStdout("Aucune organisation résolue pour cette clé.")
		return nil
	}
	a.writeOrgCurrent(orgCurrentRecord{
		ID:     home.ID,
		Name:   home.Name,
		Role:   home.Role,
		Plan:   home.Plan,
		Active: true,
		Source: "home",
	}, asJSON)
	return nil
}

func (a *app) writeOrgCurrent(record orgCurrentRecord, asJSON bool) {
	if asJSON {
		encoded, err := json.MarshalIndent(record, "", "  ")
		if err != nil {
			a.writeStderr(fmt.Sprintf("Erreur : %v", err))
			return
		}
		a.writeStdout(string(encoded))
		return
	}
	label := "par défaut — home"
	if record.Source == "selected" {
		label = "sélectionnée"
	}
	a.writeStdout(
		fmt.Sprintf("Organisation active : %s (%s)   [%s]", record.Name, record.ID, label),
		fmt.Sprintf("Rôle : %s   Plan : %s", record.Role, record.Plan),
	)
}

func (a *app) runOrgList(asJSON bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), orgCommandTimeout)
	defer cancel()
	if err := a.enforceCapability(ctx, "multi_org"); err != nil {
		return err
	}
	orgs, err := cloud.ListOrgs(ctx, nil)
	if err != nil {
		return a.translateOrgError(err)
	}

	active, err := fs.ReadActiveOrg()
	if err != nil {
		return err
	}
	a.reconcileStaleActiveOrg(active, orgs)

	activeID := ""
	if active != nil {
		activeID = active.OrgID
	}

	if asJSON {
		encoded, err := json.MarshalIndent(annotateActive(orgs, activeID), "", "  ")
		if err != nil {
			return err
		}
		a.writeStdout(string(encoded))
		return nil
	}

	if len(orgs) == 0 {
		a.writeStdout("Aucune organisation.")
		return nil
	}
	lines := make([]string, 0, len(orgs))
	for _, org := range orgs {
		marker := " "
		if isActiveOrg(org, activeID) {
			marker = "*"
		}
		lines = append(lines, fmt.Sprintf("%s %-24s %-16s %-8s %s", marker, org.Name, org.ID, org.Role, org.Plan))
	}
	a.writeStdout(lines...)
	return nil
}

func (a *app) runOrgUse(positional []string) error {
	if len(positional) == 0 {
		return errors.New("usage : sth org use <id|nom>")
	}
	target := strings.TrimSpace(strings.Join(positional, " "))
	ctx, cancel := context.WithTimeout(context.Background(), orgCommandTimeout)
	defer cancel()
	if err := a.enforceCapability(ctx, "multi_org"); err != nil {
		return err
	}
	orgs, err := cloud.ListOrgs(ctx, nil)
	if err != nil {
		return a.translateOrgError(err)
	}
	match, err := selectOrg(orgs, target)
	if err != nil {
		return err
	}
	if err := fs.SaveActiveOrg(fs.ActiveOrgSelection{
		OrgID:      match.ID,
		OrgName:    match.Name,
		Role:       match.Role,
		Plan:       match.Plan,
		SelectedAt: time.Now().UTC(),
	}); err != nil {
		return err
	}
	a.writeStdout(fmt.Sprintf("Organisation active : %s (%s).", match.Name, match.ID))
	return nil
}

func selectOrg(orgs []cloud.Org, target string) (cloud.Org, error) {
	for _, org := range orgs {
		if org.ID == target {
			return org, nil
		}
	}
	matches := make([]cloud.Org, 0)
	for _, org := range orgs {
		if strings.EqualFold(org.Name, target) {
			matches = append(matches, org)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return cloud.Org{}, fmt.Errorf("tu n'es membre d'aucune organisation nommée %q", target)
	default:
		ids := make([]string, 0, len(matches))
		for _, org := range matches {
			ids = append(ids, org.ID)
		}
		return cloud.Org{}, fmt.Errorf("nom ambigu %q — précise un id parmi : %s", target, strings.Join(ids, ", "))
	}
}

func homeOrg(orgs []cloud.Org) (cloud.Org, bool) {
	for _, org := range orgs {
		if org.Active {
			return org, true
		}
	}
	if len(orgs) == 1 {
		return orgs[0], true
	}
	return cloud.Org{}, false
}

func isActiveOrg(org cloud.Org, activeID string) bool {
	if activeID != "" {
		return org.ID == activeID
	}
	return org.Active
}

func annotateActive(orgs []cloud.Org, activeID string) []cloud.Org {
	annotated := make([]cloud.Org, len(orgs))
	copy(annotated, orgs)
	if activeID == "" {
		return annotated
	}
	for index := range annotated {
		annotated[index].Active = annotated[index].ID == activeID
	}
	return annotated
}

func (a *app) reconcileStaleActiveOrg(active *fs.ActiveOrgSelection, orgs []cloud.Org) {
	if active == nil {
		return
	}
	for _, org := range orgs {
		if org.ID == active.OrgID {
			return
		}
	}
	if err := fs.ClearActiveOrg(); err != nil {
		a.writeStderr(fmt.Sprintf("Avertissement : sélection d'organisation périmée non effacée : %v", err))
		return
	}
	a.writeStderr(fmt.Sprintf("Avertissement : l'organisation active (%s) n'est plus accessible — retour à l'organisation par défaut.", active.OrgName))
}

func (a *app) translateOrgError(err error) error {
	switch {
	case errors.Is(err, cloud.ErrUnauthorized):
		return errors.New("clé STH Cloud refusée — relance `sth login`.")
	case errors.Is(err, cloud.ErrNoKey):
		return errors.New("aucune clé STH Cloud configurée — exécute `sth login`.")
	default:
		return err
	}
}
