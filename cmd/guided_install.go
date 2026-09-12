package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"sth/internal/fs"
	"sth/internal/tui"
	"sth/internal/writer"
)

func (a *app) runFirstSkill(session *tui.Session) error {
	profile, err := fs.ReadProjectProfileIfPresent()
	if err != nil {
		return err
	}
	if profile == nil {
		repository, err := tui.Input(session, tui.InputConfig{Label: "Dépôt GitHub public de skills", Default: "anthropics/skills", Required: true})
		if err != nil {
			return err
		}
		repository, err = fs.NormalizeRepositoryID("github", repository)
		if err != nil {
			return err
		}
		config := fs.ProjectConfigV2{SchemaVersion: 2, Providers: []fs.ProviderEntry{{ID: "public", Provider: "github", RepositoryID: repository, CatalogRef: "main", CatalogPath: "SKILLS.md", TargetDir: ".sth"}}}
		if _, err := fs.WriteProjectConfigV2(config); err != nil {
			return err
		}
		if projectRoot, err := os.Getwd(); err == nil {
			if _, err := fs.EnsureManagedGitignore(projectRoot); err != nil {
				return err
			}
		}
	}
	config, err := fs.ReadProjectConfigV2ReadOnly()
	if err != nil {
		return err
	}
	catalog, err := a.aggregateCatalog(context.Background(), config)
	a.recordOperationResult("list", "", err)
	if err != nil {
		return fmt.Errorf("catalogue inaccessible : %w; vérifiez la connexion puis réessayez avec `sth list`", err)
	}
	a.reportProviderFailures(catalog.failures)
	skills := make([]fs.CatalogEntry, 0)
	for _, entry := range catalog.entries {
		if artifactKindOf(entry) == "skill" {
			skills = append(skills, entry)
		}
	}
	sort.Slice(skills, func(left, right int) bool { return skills[left].Name < skills[right].Name })
	if len(skills) == 0 {
		return fmt.Errorf("aucun skill disponible; choisissez une autre source dans Réglages avancés")
	}
	choices := make([]tui.Option, 0, len(skills))
	for _, skill := range skills {
		description := strings.TrimSpace(skill.Description)
		if description == "" {
			description = "Description non fournie par l’auteur — consultez le dépôt avant installation"
		}
		choices = append(choices, tui.Option{Label: skill.Name + " [" + fs.CatalogEntryProviderID(skill) + "]", Description: description, Aliases: []string{skill.Name, catalogDisplayRef(skill)}})
	}
	selectedIndex, err := tui.Select(session, tui.SelectConfig{Title: "Choisir un skill", Options: choices})
	if err != nil {
		return err
	}
	skill := skills[selectedIndex]
	specifications := writer.AllTargetSpecs()
	targets := make([]tui.Option, 0, len(specifications))
	for _, specification := range specifications {
		targets = append(targets, tui.Option{Label: specification.DisplayName, Description: "Installer dans ce projet", Aliases: []string{specification.Kind}})
	}
	targetIndex, err := tui.Select(session, tui.SelectConfig{Title: "Choisir son assistant", Options: targets})
	if err != nil {
		return err
	}
	target := specifications[targetIndex]
	targetID := target.Kind
	exists := false
	for index := range config.Targets {
		if config.Targets[index].Kind == target.Kind {
			targetID = config.Targets[index].ID
			config.Targets[index].Enabled = true
			exists = true
			break
		}
	}
	if !exists {
		config.Targets = append(config.Targets, fs.TargetEntry{ID: targetID, Kind: target.Kind, RootDir: target.RootDir, Enabled: true})
	}
	if _, err := fs.WriteProjectConfigV2(config); err != nil {
		return err
	}
	reference := fs.CatalogEntryProviderID(skill) + ":" + catalogDisplayRef(skill)
	arguments := []string{reference, "--target", targetID}
	a.writeStdout("Fichiers concernés :")
	if err := a.runInstall(append(append([]string{}, arguments...), "--dry-run")); err != nil {
		return err
	}
	confirmed, err := tui.Confirm(session, tui.ConfirmConfig{Question: "Installer ce skill dans ce projet ?", DefaultYes: false})
	if err != nil {
		return err
	}
	if !confirmed {
		a.writeStdout("Installation annulée. Aucun fichier de skill installé.")
		return nil
	}
	err = a.runInstall(arguments)
	a.recordOperationResult("install", reference, err)
	if err != nil {
		return err
	}
	a.writeStdout(skillUsageLines(skill.Name, target.Kind, target.DisplayName, reference)...)
	return nil
}

func skillUsageLines(name, targetKind, displayName, reference string) []string {
	invocation := "Utilise le skill " + name + " pour une tâche correspondant à sa description."
	if targetKind == writer.KindCodex {
		invocation = "$" + name + " — décris ici ta tâche"
	}
	if targetKind == writer.KindClaude {
		invocation = "/" + name + " — décris ici ta tâche"
	}
	return []string{"Pour essayer :", "  Ouvrez ce dossier dans " + displayName + " et démarrez une nouvelle conversation.", "  Demandez : " + invocation, "  Vérifiez que l’assistant annonce ou charge le skill avant de suivre sa réponse.", "Pour le retirer : menu Supprimer, ou `sth remove " + reference + " --yes`."}
}

func (a *app) runGuidedRemove(session *tui.Session) error {
	config, err := fs.ReadProjectConfigV2ReadOnly()
	if err != nil {
		return err
	}
	selections, err := installedSelections(config)
	if err != nil {
		return err
	}
	if len(selections) == 0 {
		a.writeStdout("Aucun skill installé. Choisissez Installer un skill.")
		return nil
	}
	options := make([]tui.Option, 0, len(selections))
	for _, selection := range selections {
		options = append(options, tui.Option{Label: selection.resourceName, Description: selection.providerID, Aliases: []string{selection.resourceName}})
	}
	index, err := tui.Select(session, tui.SelectConfig{Title: "Supprimer un skill", Options: options})
	if err != nil {
		return err
	}
	selection := selections[index]
	a.writeStdout("Fichiers concernés :")
	for _, record := range selection.records {
		for _, file := range record.ManagedFiles {
			a.writeStdout("  " + file.Path)
		}
	}
	confirmed, err := tui.Confirm(session, tui.ConfirmConfig{Question: "Supprimer les fichiers gérés de ce skill ?", DefaultYes: false})
	if err != nil || !confirmed {
		return err
	}
	return a.runRemove([]string{selection.resourceName, "--provider", selection.providerID, "--yes"})
}

func (a *app) runAdvancedMenu(session *tui.Session) error {
	actions := []string{"show-config", "provider-guide", "init", "help", "quit"}
	index, err := tui.Select(session, tui.SelectConfig{Title: "Réglages avancés", Options: guidedMenuOptions(guidedStartupState{AvailableActions: actions})})
	if err != nil {
		return err
	}
	if actions[index] == "quit" {
		return nil
	}
	return a.executeGuidedMenuAction(session, actions[index])
}
