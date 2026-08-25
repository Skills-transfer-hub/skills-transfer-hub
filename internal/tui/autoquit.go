package tui

import (
	"github.com/charmbracelet/bubbletea"
)

type finisher interface {
	isFinished() bool
}

type autoQuitModel struct {
	inner tea.Model
}

func wrapAutoQuit(inner tea.Model) autoQuitModel {
	return autoQuitModel{inner: inner}
}

func (model autoQuitModel) Init() tea.Cmd {
	return model.inner.Init()
}

func (model autoQuitModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	next, command := model.inner.Update(message)
	nextWrapper := autoQuitModel{inner: next}
	if finished, ok := next.(finisher); ok && finished.isFinished() {
		return nextWrapper, tea.Quit
	}
	return nextWrapper, command
}

func (model autoQuitModel) View() string {
	return model.inner.View()
}

func unwrapAutoQuit(final tea.Model) tea.Model {
	if wrapper, ok := final.(autoQuitModel); ok {
		return wrapper.inner
	}
	return final
}
