package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbletea"
)

type SecretConfig struct {
	Label string
	Hint  string
}

type secretModel struct {
	label     string
	hint      string
	message   string
	submitted bool
	aborted   bool
	field     textinput.Model
}

func (model secretModel) isFinished() bool {
	return model.submitted || model.aborted
}

func newSecretModel(config SecretConfig) secretModel {
	field := textinput.New()
	field.EchoMode = textinput.EchoPassword
	field.EchoCharacter = '•'
	field.Placeholder = "••••••••"
	field.Focus()
	field.CharLimit = 512
	field.Width = 64
	return secretModel{label: config.Label, hint: config.Hint, field: field}
}

func (model secretModel) Init() tea.Cmd {
	return textinput.Blink
}

func (model secretModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	keyPress, isKey := message.(tea.KeyMsg)
	if !isKey {
		var cmd tea.Cmd
		model.field, cmd = model.field.Update(message)
		return model, cmd
	}
	switch keyPress.Type {
	case tea.KeyEnter:
		model.submitted = true
		return model, nil
	case tea.KeyEsc, tea.KeyCtrlC:
		model.aborted = true
		return model, nil
	default:
		var cmd tea.Cmd
		model.field, cmd = model.field.Update(message)
		return model, cmd
	}
}

func (model secretModel) View() string {
	var builder strings.Builder
	if model.hint != "" {
		builder.WriteString(WarningStyle.Render(model.hint) + "\n")
	}
	builder.WriteString(TitleStyle.Render(model.label) + "\n")
	builder.WriteString(model.field.View() + "\n")
	builder.WriteString(DimStyle.Render("Enter submit · Esc abort") + "\n")
	if model.message != "" {
		builder.WriteString(ErrorStyle.Render(model.message) + "\n")
	}
	return builder.String()
}

func Secret(session *Session, config SecretConfig) (string, error) {
	model := newSecretModel(config)
	if session.Interactive {
		finalModel, err := session.RunProgram(model)
		if err != nil {
			return "", err
		}
		result := finalModel.(secretModel)
		if result.aborted {
			return "", ErrAborted
		}
		return result.field.Value(), nil
	}
	fmt.Fprintf(session.Stdout, "%s : ", config.Label)
	if session.stdinTTY {
		secretBytes, err := session.readSecret(int(session.stdinFile.Fd()))
		fmt.Fprintln(session.Stdout)
		if err != nil {
			return "", fmt.Errorf("failed to read input: %w", err)
		}
		return string(secretBytes), nil
	}
	line, readErr := session.reader.ReadString('\n')
	if readErr != nil && readErr != io.EOF {
		return "", fmt.Errorf("failed to read input: %w", readErr)
	}
	return strings.TrimRight(line, "\r\n"), nil
}
