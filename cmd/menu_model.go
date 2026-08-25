package cmd

import (
	"strings"

	"github.com/charmbracelet/bubbletea"

	"sth/internal/timing"
	"sth/internal/tui"
)

type guidedMenuModel struct {
	state          guidedStartupState
	list           *tui.SelectModel
	selectedAction string
	actionErr      error
}

func newGuidedMenuModel(application *app) guidedMenuModel {
	state := application.loadGuidedStartupState()
	timing.Mark("menu-state-loaded")
	return guidedMenuModel{
		state: state,
		list:  tui.NewSelectModel(guidedSelectConfig(state)),
	}
}

func guidedSelectConfig(state guidedStartupState) tui.SelectConfig {
	return tui.SelectConfig{
		Title:       "What do you want to do?",
		Options:     guidedMenuOptions(state),
		Recommended: indexOfAction(state.AvailableActions, state.PrimaryAction),
	}
}

func (model guidedMenuModel) Init() tea.Cmd {
	timing.Mark("menu-shown")
	return nil
}

func (model guidedMenuModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if keyPress, ok := message.(tea.KeyMsg); ok {
		if keyPress.Type == tea.KeyCtrlC {
			model.actionErr = tui.ErrAborted
			return model, tea.Quit
		}
		if keyPress.Type == tea.KeyRunes && strings.EqualFold(keyPress.String(), "q") {
			model.selectedAction = "quit"
			return model, tea.Quit
		}
	}
	forwardCommand := model.list.Forward(message)
	index, done, aborted := model.list.Result()
	if aborted {
		model.actionErr = tui.ErrAborted
		return model, tea.Quit
	}
	if !done {
		return model, forwardCommand
	}
	model.selectedAction = model.state.AvailableActions[index]
	return model, tea.Quit
}

func (model guidedMenuModel) View() string {
	var builder strings.Builder
	builder.WriteString(strings.Join(renderGuidedMenuLines(model.state), "\n"))
	builder.WriteString("\n\n")
	builder.WriteString(model.list.View())
	return builder.String()
}
