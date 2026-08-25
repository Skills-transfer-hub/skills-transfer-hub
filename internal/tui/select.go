package tui

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbletea"
)

var ErrAborted = errors.New("input aborted")

const cursorGlyph = "❯"

type Option struct {
	Label       string
	Description string
	Aliases     []string
}

type SelectConfig struct {
	Title       string
	Options     []Option
	Recommended int
	MaxAttempts int
}

type selectModel struct {
	title       string
	options     []Option
	recommended int
	cursor      int
	buffer      string
	message     string
	attempts    int
	maxAttempts int
	chosenIndex int
	chosen      bool
	aborted     bool
}

func (model selectModel) isFinished() bool {
	return model.chosen || model.aborted
}

func newSelectModel(config SelectConfig) selectModel {
	recommended := config.Recommended
	if recommended < 0 || recommended >= len(config.Options) {
		recommended = 0
	}
	return selectModel{
		title:       config.Title,
		options:     config.Options,
		recommended: recommended,
		cursor:      recommended,
		maxAttempts: config.MaxAttempts,
		chosenIndex: -1,
	}
}

func (model selectModel) Init() tea.Cmd {
	return nil
}

func (model selectModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	keyPress, isKey := message.(tea.KeyMsg)
	if !isKey {
		return model, nil
	}
	switch keyPress.Type {
	case tea.KeyUp, tea.KeyShiftTab:
		model.cursor = (model.cursor - 1 + len(model.options)) % len(model.options)
		model.message = ""
	case tea.KeyDown, tea.KeyTab:
		model.cursor = (model.cursor + 1) % len(model.options)
		model.message = ""
	case tea.KeyEnter:
		model.resolve()
		if model.chosen {
			return model, nil
		}
	case tea.KeyEsc:
		if model.buffer != "" {
			model.buffer = ""
			model.message = ""
		} else {
			model.aborted = true
			return model, nil
		}
	case tea.KeyCtrlC:
		model.aborted = true
		return model, nil
	case tea.KeyBackspace:
		runes := []rune(model.buffer)
		if len(runes) > 0 {
			model.buffer = string(runes[:len(runes)-1])
			model.message = ""
		}
	case tea.KeyRunes:
		model.buffer += string(keyPress.Runes)
		model.message = ""
	case tea.KeySpace:
		model.buffer += " "
		model.message = ""
	}
	return model, nil
}

func (model *selectModel) resolve() {
	token := strings.TrimSpace(model.buffer)
	if token == "" {
		model.buffer = ""
		model.chosen = true
		model.chosenIndex = model.cursor
		return
	}
	index, resolveErr := resolveSelectToken(token, model.options, model.recommended)
	if resolveErr != nil {
		model.attempts++
		if model.maxAttempts > 0 && model.attempts >= model.maxAttempts {
			model.buffer = ""
			model.chosen = true
			model.chosenIndex = model.recommended
			return
		}
		model.message = resolveErr.Error()
		return
	}
	model.buffer = ""
	model.chosen = true
	model.chosenIndex = index
}

func (model selectModel) View() string {
	var builder strings.Builder
	builder.WriteString(TitleStyle.Render(model.title) + "\n")
	for index, option := range model.options {
		label := option.Label
		if index == model.recommended {
			label += DimStyle.Render(" (recommended)")
		}
		if index == model.cursor {
			builder.WriteString(AccentStyle.Render(cursorGlyph+" ") + CursorStyle.Render(label) + "\n")
			if option.Description != "" {
				builder.WriteString(DimStyle.Render("    "+option.Description) + "\n")
			}
			continue
		}
		builder.WriteString("  " + label + "\n")
	}
	builder.WriteString(DimStyle.Render("↑/↓ navigate · Enter confirm · Esc cancel · typing a name also works") + "\n")
	if model.buffer != "" {
		builder.WriteString(AccentStyle.Render(cursorGlyph+" ") + model.buffer + "\n")
	}
	if model.message != "" {
		builder.WriteString(ErrorStyle.Render(model.message) + "\n")
	}
	return builder.String()
}

func resolveSelectToken(token string, options []Option, recommended int) (int, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return recommended, nil
	}
	if index, convErr := strconv.Atoi(token); convErr == nil {
		if index >= 1 && index <= len(options) {
			return index - 1, nil
		}
		return 0, fmt.Errorf("number out of range: %s", token)
	}
	for index, option := range options {
		for _, alias := range option.Aliases {
			if strings.EqualFold(alias, token) {
				return index, nil
			}
		}
	}
	for index, option := range options {
		if strings.EqualFold(option.Label, token) {
			return index, nil
		}
	}
	return 0, fmt.Errorf("invalid choice: %s", token)
}

func Select(session *Session, config SelectConfig) (int, error) {
	model := newSelectModel(config)
	if session.Interactive {
		finalModel, err := session.RunProgram(model)
		if err != nil {
			return 0, err
		}
		result := finalModel.(selectModel)
		if result.aborted {
			return 0, ErrAborted
		}
		return result.chosenIndex, nil
	}
	invalidAttempts := 0
	for {
		fmt.Fprint(session.Stdout, model.View())
		line, readErr := session.readLine()
		if readErr != nil {
			return 0, readErr
		}
		index, resolveErr := resolveSelectToken(line, config.Options, model.recommended)
		if resolveErr != nil {
			fmt.Fprintln(session.Stdout, resolveErr.Error())
			invalidAttempts++
			if config.MaxAttempts > 0 && invalidAttempts >= config.MaxAttempts {
				return model.recommended, nil
			}
			continue
		}
		return index, nil
	}
}
