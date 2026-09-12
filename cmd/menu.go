package cmd

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"sth/internal/fs"
	"sth/internal/timing"
	"sth/internal/tui"
)

var errMenuQuit = errors.New("menu: exit requested")

type projectState string

const (
	projectStateUnconfigured         projectState = "unconfigured"
	projectStateConfiguredBad        projectState = "configured-invalid"
	projectStateConfiguredReady      projectState = "configured-ready"
	projectStateConnectionUnverified projectState = "connection-unverified"
)

func projectStateLabel(state projectState) string {
	switch state {
	case projectStateUnconfigured:
		return "not configured"
	case projectStateConfiguredBad:
		return "invalid configuration"
	case projectStateConnectionUnverified:
		return "connexion non vérifiée"
	case projectStateConfiguredReady:
		return "ready"
	default:
		return string(state)
	}
}

type guidedStartupState struct {
	InteractionMode   string
	ProjectState      projectState
	PrimaryAction     string
	AvailableActions  []string
	ProviderHint      string
	UpdateOffer       *fs.StableUpdateOffer
	RepositoryUpdates *repositoryUpdateReport
	Detail            string
}

func (a *app) loadGuidedStartupState() guidedStartupState {
	mode := "non-interactive"
	if a.isInteractiveSession() {
		mode = "interactive"
	}

	withUpdateOffer := func(state guidedStartupState) guidedStartupState {
		if mode == "interactive" {
			state.UpdateOffer = a.cachedStableUpdateOffer()
			a.refreshUpdateOfferInBackground()
			if state.ProjectState == projectStateConfiguredReady && state.UpdateOffer != nil && state.UpdateOffer.DownloadReady {
				state.AvailableActions = []string{"install", "status", "remove", "update", "doctor", "list", "sources", "check-updates", "update-self", "advanced", "quit"}
			}
		}
		return state
	}

	profile, err := fs.ReadProjectProfileIfPresent()
	if err != nil {
		return withUpdateOffer(guidedStartupState{
			InteractionMode:  mode,
			ProjectState:     projectStateConfiguredBad,
			PrimaryAction:    "init",
			AvailableActions: []string{"init", "help", "quit"},
			Detail:           err.Error(),
		})
	}
	if profile == nil {
		return withUpdateOffer(guidedStartupState{
			InteractionMode:  mode,
			ProjectState:     projectStateUnconfigured,
			PrimaryAction:    "install",
			AvailableActions: []string{"install", "sources", "advanced", "quit"},
			Detail:           "No project configuration found.",
		})
	}

	state := projectStateConfiguredReady
	if lastOperation, err := fs.ReadLastOperation(); err == nil && lastOperation != nil && lastOperation.Status == fs.LastOperationStatusError {
		state = projectStateConnectionUnverified
	}
	return withUpdateOffer(guidedStartupState{
		InteractionMode:  mode,
		ProjectState:     state,
		PrimaryAction:    "install",
		AvailableActions: []string{"install", "status", "remove", "update", "doctor", "list", "sources", "check-updates", "advanced", "quit"},
		ProviderHint:     profile.Provider,
		Detail:           fmt.Sprintf("Configured provider: %s", profile.Provider),
	})
}

func renderGuidedMenuLines(state guidedStartupState) []string {
	if state.RepositoryUpdates != nil {
		if state.RepositoryUpdates.Error != "" {
			state.ProjectState = projectStateConnectionUnverified
		}
		for _, resource := range state.RepositoryUpdates.Resources {
			if resource.Status == "unverified" {
				state.ProjectState = projectStateConnectionUnverified
				break
			}
		}
	}
	lines := []string{
		tui.TitleStyle.Render("STH guided menu"),
		projectStateBadge(state.ProjectState),
	}
	if state.Detail != "" {
		lines = append(lines, tui.DimStyle.Render("  "+state.Detail))
	}
	lines = append(lines, updateOfferLines(state.UpdateOffer)...)
	if state.RepositoryUpdates != nil {
		lines = append(lines, repositoryUpdateMenuLines(*state.RepositoryUpdates)...)
	}

	return lines
}

func updateOfferLines(offer *fs.StableUpdateOffer) []string {
	if offer == nil || !offer.Actionable() || strings.TrimSpace(offer.Message) == "" {
		return nil
	}
	lines := []string{offer.Message}
	for _, highlight := range offer.Highlights {
		lines = append(lines, "  • "+highlight)
	}
	if strings.TrimSpace(offer.ReleaseNotesURL) != "" {
		lines = append(lines, "  Details: "+strings.TrimSpace(offer.ReleaseNotesURL))
	}
	return lines
}

func guidedActionLabel(action string) string {
	switch action {
	case "status":
		return "Mes skills"
	case "remove":
		return "Supprimer"
	case "doctor":
		return "Diagnostic"
	case "advanced":
		return "Réglages avancés"
	case "sources":
		return "Mes dépôts"
	case "check-updates":
		return "Vérifier les mises à jour"
	case "list":
		return "List catalog resources"
	case "install":
		return "Installer un skill"
	case "update":
		return "Mettre à jour"
	case "update-self":
		return "Apply the downloaded sth update"
	case "show-config":
		return "Show project configuration"
	case "provider-guide":
		return "Provider access guide"
	case "init":
		return "Configure this project"
	case "help":
		return "Help"
	case "quit":
		return "Quit"
	default:
		return action
	}
}

func guidedActionDescription(action string) string {
	switch action {
	case "status":
		return "Voir les skills installés dans ce projet"
	case "remove":
		return "Choisir un skill et voir ses fichiers avant suppression"
	case "doctor":
		return "Vérifier la configuration et les accès"
	case "advanced":
		return "Dépôts, tokens, configuration et commandes techniques"
	case "sources":
		return "Ajouter une source publique ou privée et retrouver le guide de clé"
	case "check-updates":
		return "Contrôler les skills de toutes les sources sans modifier les fichiers"
	case "list":
		return "Show resources from the remote catalog"
	case "install":
		return "Choisir un skill, son assistant, puis vérifier les fichiers"
	case "update":
		return "Refresh tracked resources from the repository"
	case "update-self":
		return "Replace the binary with the ready stable version"
	case "show-config":
		return "Current provider, repository, ref and targets"
	case "provider-guide":
		return "Tokens, permissions and docs for the current provider"
	case "init":
		return "Connect your skills repository in a few questions"
	case "help":
		return "Full command list"
	default:
		return ""
	}
}

func guidedMenuOptions(state guidedStartupState) []tui.Option {
	options := make([]tui.Option, 0, len(state.AvailableActions))
	for _, action := range state.AvailableActions {
		options = append(options, tui.Option{
			Label:       guidedActionLabel(action),
			Description: guidedActionDescription(action),
			Aliases:     []string{action},
		})
	}
	return options
}

func projectStateBadge(state projectState) string {
	label := "unknown"
	style := tui.DimStyle
	switch state {
	case projectStateUnconfigured:
		label = "not configured"
		style = tui.WarningStyle
	case projectStateConfiguredBad:
		label = "invalid configuration"
		style = tui.ErrorStyle
	case projectStateConnectionUnverified:
		label = "connexion non vérifiée"
		style = tui.WarningStyle
	case projectStateConfiguredReady:
		label = "ready"
		style = tui.SuccessStyle
	}
	return "Project state: " + style.Render(label)
}

func indexOfAction(actions []string, target string) int {
	for index, action := range actions {
		if action == target {
			return index
		}
	}
	return 0
}

func (a *app) runGuidedMenu() error {
	a.playLaunchBanner(a.computeBuddyStateFast())
	session := tui.NewSession(a.stdin, a.stdout)
	defer func() {
		_ = session.Close()
	}()
	if session.Interactive {
		for {
			finalModel, err := session.RunProgram(newGuidedMenuModel(a))
			if err != nil {
				return err
			}
			result := finalModel.(guidedMenuModel)
			if errors.Is(result.actionErr, tui.ErrAborted) || result.selectedAction == "" || result.selectedAction == "quit" {
				return nil
			}
			executionErr := a.executeGuidedMenuAction(session, result.selectedAction)
			timing.Mark("menu-action-done " + result.selectedAction)
			if executionErr == nil {
				continue
			}
			if errors.Is(executionErr, errMenuQuit) {
				return nil
			}
			if errors.Is(executionErr, tui.ErrAborted) {
				continue
			}
			a.writeStderr(fmt.Sprintf("Error: %v", executionErr))
		}
	}
	timing.Mark("menu-session-created")
	for {
		state := a.loadGuidedStartupState()
		if state.ProjectState == projectStateConfiguredReady || state.ProjectState == projectStateConnectionUnverified {
			report, check := a.prepareRepositoryUpdateCheck(false)
			if check != nil {
				report = check()
			}
			a.repositoryUpdates = &report
			state.RepositoryUpdates = &report
		}
		timing.Mark("menu-state-loaded")
		a.writeStdout(renderGuidedMenuLines(state)...)
		timing.Mark("menu-header-printed")

		action, err := a.promptGuidedMenuAction(session, state)
		if err != nil {
			if errors.Is(err, tui.ErrAborted) {
				return nil
			}
			return err
		}
		execErr := a.executeGuidedMenuAction(session, action)
		if execErr != nil {
			if errors.Is(execErr, errMenuQuit) {
				return nil
			}
			return execErr
		}
	}
}

func (a *app) promptGuidedMenuAction(session *tui.Session, state guidedStartupState) (string, error) {
	index, err := tui.Select(session, tui.SelectConfig{
		Title:       "What do you want to do?",
		Options:     guidedMenuOptions(state),
		Recommended: indexOfAction(state.AvailableActions, state.PrimaryAction),
	})
	if err != nil {
		return "", err
	}
	return state.AvailableActions[index], nil
}

func (a *app) executeGuidedMenuAction(session *tui.Session, action string) (resultErr error) {
	defer func() {
		switch action {
		case "doctor", "sources", "init", "advanced":
			a.repositoryUpdates = nil
		}
		switch action {
		case "list", "install", "update", "doctor":
			if !errors.Is(resultErr, tui.ErrAborted) {
				a.recordOperationResult(action, "", resultErr)
			}
		}
	}()
	switch action {
	case "list":
		return a.runList()
	case "install":
		return a.runFirstSkill(session)
	case "status":
		return a.runGuidedSkills()
	case "remove":
		return a.runGuidedRemove(session)
	case "doctor":
		return a.runDoctor(nil)
	case "advanced":
		return a.runAdvancedMenu(session)
	case "sources":
		return a.runGuidedSources(session)
	case "check-updates":
		return a.runCheckUpdates(nil)
	case "update":
		return a.runUpdate([]string{})
	case "update-self":
		updated, err := a.runSelfUpdateResult()
		if err != nil {
			return err
		}
		if !updated {
			return nil
		}
		return a.runUpdatedGuidedMenu()
	case "init":
		return a.runInit()
	case "show-config":
		profile, err := fs.ReadProjectProfile()
		if err != nil {
			return err
		}
		a.writeStdout(renderProjectProfileLines(*profile)...)
		return nil
	case "provider-guide":
		profile, err := fs.ReadProjectProfile()
		if err != nil {
			return err
		}
		a.writeStdout(renderProviderGuideLines(profile.Provider)...)
		return nil
	case "help":
		a.writeStdout(helpText())
		return nil
	case "quit":
		return errMenuQuit
	default:
		return fmt.Errorf("unknown menu action: %s", action)
	}
}

func (a *app) runUpdatedGuidedMenu() error {
	executablePath, err := currentExecutablePath()
	if err != nil {
		return err
	}
	command := exec.Command(executablePath)
	command.Stdin = a.stdin
	command.Stdout = a.stdout
	command.Stderr = a.stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("updated STH could not restart the guided menu: %w", err)
	}
	return errMenuQuit
}

func renderProjectProfileLines(profile fs.ProjectProfile) []string {
	return []string{
		"Current project configuration",
		fmt.Sprintf("Provider: %s", profile.Provider),
		fmt.Sprintf("Repository: %s", profile.RepositoryID),
		fmt.Sprintf("Catalog: %s @ %s", profile.CatalogPath, profile.Ref),
		fmt.Sprintf("Target dir: %s", profile.TargetDir),
	}
}

func renderProviderGuideLines(provider string) []string {
	guide, ok := findProviderGuide(provider)
	if !ok {
		return []string{fmt.Sprintf("Unsupported provider: %s", provider)}
	}

	lines := []string{
		fmt.Sprintf("Provider guide: %s", guide.DisplayName),
		fmt.Sprintf("Supported host: %s", guide.HostedDomain),
		fmt.Sprintf("Official docs: %s", guide.DocumentationURL),
		fmt.Sprintf("Create a token/API token: %s", guide.ActionURL),
		fmt.Sprintf("Minimum permission: %s", guide.MinimumPermission),
		fmt.Sprintf("Supported env vars: %s", strings.Join(guide.EnvVars, ", ")),
	}
	for _, note := range guide.Notes {
		lines = append(lines, note)
	}

	return lines
}

func startupGuidanceLines(state guidedStartupState) []string {
	if state.PrimaryAction == "install" {
		state.PrimaryAction = "init"
	}
	lines := []string{
		fmt.Sprintf("Project state: %s", projectStateLabel(state.ProjectState)),
		fmt.Sprintf("Recommended action: sth %s", state.PrimaryAction),
	}
	if state.ProviderHint != "" {
		lines = append(lines, fmt.Sprintf("Configured provider: %s", state.ProviderHint))
	}
	if state.Detail != "" {
		lines = append(lines, state.Detail)
	}
	lines = append(lines, updateOfferLines(state.UpdateOffer)...)

	return lines
}
