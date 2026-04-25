package cmd

import (
	"bufio"
	"fmt"
	"io"
	"sth/internal/fs"
	"strconv"
	"strings"
)

type projectState string

const (
	projectStateUnconfigured    projectState = "unconfigured"
	projectStateConfiguredBad   projectState = "configured-invalid"
	projectStateConfiguredReady projectState = "configured-ready"
)

type guidedStartupState struct {
	InteractionMode  string
	ProjectState     projectState
	PrimaryAction    string
	AvailableActions []string
	ProviderHint     string
	UpdateOffer      *fs.StableUpdateOffer
	Detail           string
}

func (a *app) loadGuidedStartupState() guidedStartupState {
	mode := "non-interactive"
	if a.isInteractiveSession() {
		mode = "interactive"
	}

	withUpdateOffer := func(state guidedStartupState) guidedStartupState {
		if mode == "interactive" {
			state.UpdateOffer = a.startupStableUpdateOffer()
			if state.ProjectState == projectStateConfiguredReady && state.UpdateOffer != nil && state.UpdateOffer.DownloadReady {
				state.AvailableActions = []string{"list", "install", "update", "update-self", "show-config", "provider-guide", "init", "help", "quit"}
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
			PrimaryAction:    "init",
			AvailableActions: []string{"init", "help", "quit"},
			Detail:           "Configuration projet absente.",
		})
	}

	return withUpdateOffer(guidedStartupState{
		InteractionMode:  mode,
		ProjectState:     projectStateConfiguredReady,
		PrimaryAction:    "list",
		AvailableActions: []string{"list", "install", "update", "show-config", "provider-guide", "init", "help", "quit"},
		ProviderHint:     profile.Provider,
		Detail:           fmt.Sprintf("Provider configuré : %s", profile.Provider),
	})
}

func renderGuidedMenuLines(state guidedStartupState) []string {
	lines := []string{
		"Menu guidé STH",
		fmt.Sprintf("Etat projet : %s", state.ProjectState),
		fmt.Sprintf("Action recommandée : %s", state.PrimaryAction),
	}
	if state.Detail != "" {
		lines = append(lines, state.Detail)
	}
	if state.UpdateOffer != nil && strings.TrimSpace(state.UpdateOffer.Message) != "" {
		lines = append(lines, state.UpdateOffer.Message)
	}
	lines = append(lines, "Actions :")
	for index, action := range state.AvailableActions {
		label := action
		if action == state.PrimaryAction {
			label += " (recommandé)"
		}
		lines = append(lines, fmt.Sprintf("%d. %s", index+1, label))
	}

	return lines
}

func (a *app) runGuidedMenu() error {
	a.playLaunchBanner(a.computeBuddyStateFast())

	reader := bufio.NewReader(a.stdin)
	for {
		state := a.loadGuidedStartupState()
		a.writeStdout(renderGuidedMenuLines(state)...)

		action, err := promptGuidedMenuAction(reader, a, state)
		if err != nil {
			return err
		}

		continueLoop, err := a.executeGuidedMenuAction(reader, action)
		if err != nil {
			return err
		}
		if !continueLoop {
			return nil
		}
	}
}

func promptGuidedMenuAction(reader *bufio.Reader, a *app, state guidedStartupState) (string, error) {
	defaultIndex := 1
	for index, action := range state.AvailableActions {
		if action == state.PrimaryAction {
			defaultIndex = index + 1
			break
		}
	}

	for {
		if _, err := fmt.Fprintf(a.stdout, "Choix [%d]: ", defaultIndex); err != nil {
			return "", err
		}
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return "", fmt.Errorf("lecture de l'entrée utilisateur échouée : %w", err)
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			return state.PrimaryAction, nil
		}
		for _, action := range state.AvailableActions {
			if trimmed == action {
				return action, nil
			}
		}
		if i, convErr := strconv.Atoi(trimmed); convErr == nil {
			index := i - 1
			if index >= 0 && index < len(state.AvailableActions) {
				return state.AvailableActions[index], nil
			}
		}
		a.writeStdout("Choix invalide. Sélectionnez un numéro ou une action visible.")
	}
}

func (a *app) executeGuidedMenuAction(reader *bufio.Reader, action string) (bool, error) {
	switch action {
	case "list":
		return false, a.runList()
	case "install":
		resourceRef, err := promptValue(reader, a.stdout, "Référence de ressource ou bundle", "")
		if err != nil {
			return false, err
		}
		return false, a.runInstall([]string{resourceRef})
	case "update":
		return false, a.runUpdate([]string{})
	case "update-self":
		return false, a.runSelfUpdate()
	case "init":
		return false, a.runInit()
	case "show-config":
		profile, err := fs.ReadProjectProfile()
		if err != nil {
			return false, err
		}
		a.writeStdout(renderProjectProfileLines(*profile)...)
		return true, nil
	case "provider-guide":
		profile, err := fs.ReadProjectProfile()
		if err != nil {
			return false, err
		}
		a.writeStdout(renderProviderGuideLines(profile.Provider)...)
		return true, nil
	case "help":
		a.writeStdout(helpText())
		return true, nil
	case "quit":
		return false, nil
	default:
		return false, fmt.Errorf("action de menu inconnue : %s", action)
	}
}

func renderProjectProfileLines(profile fs.ProjectProfile) []string {
	return []string{
		"Configuration projet courante",
		fmt.Sprintf("Provider: %s", profile.Provider),
		fmt.Sprintf("Dépôt: %s", profile.RepositoryID),
		fmt.Sprintf("Catalogue: %s @ %s", profile.CatalogPath, profile.Ref),
		fmt.Sprintf("Dossier cible: %s", profile.TargetDir),
	}
}

func renderProviderGuideLines(provider string) []string {
	guide, ok := findProviderGuide(provider)
	if !ok {
		return []string{fmt.Sprintf("Provider non pris en charge : %s", provider)}
	}

	lines := []string{
		fmt.Sprintf("Guide provider : %s", guide.DisplayName),
		fmt.Sprintf("Host supporté : %s", guide.HostedDomain),
		fmt.Sprintf("Documentation officielle : %s", guide.DocumentationURL),
		fmt.Sprintf("Créer un token/API token : %s", guide.ActionURL),
		fmt.Sprintf("Permission minimale : %s", guide.MinimumPermission),
		fmt.Sprintf("Variables supportées : %s", strings.Join(guide.EnvVars, ", ")),
	}
	for _, note := range guide.Notes {
		lines = append(lines, note)
	}

	return lines
}

func startupGuidanceLines(state guidedStartupState) []string {
	lines := []string{
		fmt.Sprintf("Etat projet : %s", state.ProjectState),
		fmt.Sprintf("Action recommandée : sth %s", state.PrimaryAction),
	}
	if state.ProviderHint != "" {
		lines = append(lines, fmt.Sprintf("Provider configuré : %s", state.ProviderHint))
	}
	if state.Detail != "" {
		lines = append(lines, state.Detail)
	}
	if state.UpdateOffer != nil && strings.TrimSpace(state.UpdateOffer.Message) != "" {
		lines = append(lines, state.UpdateOffer.Message)
	}

	return lines
}
