package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbletea"
)

type InputConfig struct {
	Label    string
	Default  string
	Required bool
}

type inputModel struct {
	label     string
	required  bool
	message   string
	submitted bool
	aborted   bool
	field     textinput.Model
}

func (model inputModel) isFinished() bool {
	return model.submitted || model.aborted
}

func newInputModel(config InputConfig) inputModel {
	field := textinput.New()
	field.Placeholder = config.Default
	field.SetValue(config.Default)
	field.Focus()
	field.CharLimit = 512
	field.Width = 64
	return inputModel{label: config.Label, required: config.Required, field: field}
}

func (model inputModel) Init() tea.Cmd {
	return textinput.Blink
}

func (model inputModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	keyPress, isKey := message.(tea.KeyMsg)
	if !isKey {
		var cmd tea.Cmd
		model.field, cmd = model.field.Update(message)
		return model, cmd
	}
	switch keyPress.Type {
	case tea.KeyEnter:
		value := strings.TrimSpace(model.field.Value())
		if value == "" && model.required {
			model.message = fmt.Sprintf("%s is required", strings.ToLower(model.label))
			return model, nil
		}
		model.submitted = true
		return model, nil
	case tea.KeyEsc:
		model.aborted = true
		return model, nil
	case tea.KeyCtrlC:
		model.aborted = true
		return model, nil
	default:
		var cmd tea.Cmd
		model.field, cmd = model.field.Update(message)
		return model, cmd
	}
}

func (model inputModel) View() string {
	var builder strings.Builder
	builder.WriteString(TitleStyle.Render(model.label) + "\n")
	builder.WriteString(model.field.View() + "\n")
	builder.WriteString(DimStyle.Render("Enter submit · Esc cancel") + "\n")
	if model.message != "" {
		builder.WriteString(ErrorStyle.Render(model.message) + "\n")
	}
	return builder.String()
}

func Input(session *Session, config InputConfig) (string, error) {
	model := newInputModel(config)
	if session.Interactive {
		finalModel, err := session.RunProgram(model)
		if err != nil {
			return "", err
		}
		result := finalModel.(inputModel)
		if result.aborted {
			return "", ErrAborted
		}
		return strings.TrimSpace(result.field.Value()), nil
	}
	for {
		prompt := config.Label
		if strings.TrimSpace(config.Default) != "" {
			prompt = fmt.Sprintf("%s [%s]", config.Label, config.Default)
		}
		fmt.Fprintf(session.Stdout, "%s : ", prompt)
		line, readErr := session.readLine()
		if readErr != nil {
			return "", readErr
		}
		if line == "" {
			line = strings.TrimSpace(config.Default)
		}
		if line == "" && config.Required {
			fmt.Fprintf(session.Stdout, "%s is required\n", strings.ToLower(config.Label))
			continue
		}
		return line, nil
	}
}
