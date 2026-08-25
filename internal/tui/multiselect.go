package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbletea"
)

const multiHintLine = "Space/x toggles current · a toggles all · Enter submits · 1,3 / names / all accepted"

type MultiSelectConfig struct {
	Title    string
	Options  []Option
	Selected []int
}

type multiSelectModel struct {
	title           string
	options         []Option
	cursor          int
	checked         []bool
	buffer          string
	message         string
	done            bool
	aborted         bool
	pendingShortcut rune
	shortcutChecked []bool
}

func newMultiSelectModel(config MultiSelectConfig) multiSelectModel {
	checked := make([]bool, len(config.Options))
	for _, index := range config.Selected {
		if index >= 0 && index < len(checked) {
			checked[index] = true
		}
	}
	return multiSelectModel{title: config.Title, options: config.Options, checked: checked}
}

func (model multiSelectModel) isFinished() bool {
	return model.done || model.aborted
}

func selectedFromChecked(checked []bool) []int {
	indices := make([]int, 0, len(checked))
	for index, isChecked := range checked {
		if isChecked {
			indices = append(indices, index)
		}
	}
	return indices
}

func (model multiSelectModel) Init() tea.Cmd {
	return nil
}

func (model multiSelectModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	keyPress, isKey := message.(tea.KeyMsg)
	if !isKey {
		return model, nil
	}
	if keyPress.Type != tea.KeyRunes {
		model.commitPendingShortcut()
	}
	switch keyPress.Type {
	case tea.KeyUp:
		model.cursor = (model.cursor - 1 + len(model.options)) % len(model.options)
		model.message = ""
	case tea.KeyDown:
		model.cursor = (model.cursor + 1) % len(model.options)
		model.message = ""
	case tea.KeySpace:
		model.toggleCurrent()
	case tea.KeyEnter:
		if strings.TrimSpace(model.buffer) == "" {
			model.done = true
			return model, nil
		}
		model.resolveBuffer()
		if model.done {
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
		}
	case tea.KeyRunes:
		if model.pendingShortcut != 0 {
			if len(keyPress.Runes) == 1 && (keyPress.Runes[0] == 'a' || keyPress.Runes[0] == 'x') {
				model.commitPendingShortcut()
				model.startShortcut(keyPress.Runes[0])
				return model, nil
			}
			prefix := model.restorePendingShortcut()
			model.buffer = string(prefix) + string(keyPress.Runes)
			return model, nil
		}
		if model.buffer == "" && len(keyPress.Runes) == 1 && (keyPress.Runes[0] == 'a' || keyPress.Runes[0] == 'x') {
			model.startShortcut(keyPress.Runes[0])
			return model, nil
		}
		model.buffer += string(keyPress.Runes)
	}
	return model, nil
}

func (model *multiSelectModel) startShortcut(shortcut rune) {
	model.pendingShortcut = shortcut
	model.shortcutChecked = append([]bool(nil), model.checked...)
	if shortcut == 'a' {
		model.toggleAll()
		return
	}
	model.toggleCurrent()
}

func (model *multiSelectModel) commitPendingShortcut() {
	model.pendingShortcut = 0
	model.shortcutChecked = nil
}

func (model *multiSelectModel) restorePendingShortcut() rune {
	shortcut := model.pendingShortcut
	copy(model.checked, model.shortcutChecked)
	model.commitPendingShortcut()
	return shortcut
}

func (model *multiSelectModel) toggleCurrent() {
	if len(model.checked) == 0 {
		return
	}
	model.checked[model.cursor] = !model.checked[model.cursor]
}

func (model *multiSelectModel) toggleAll() {
	allChecked := true
	for _, isChecked := range model.checked {
		if !isChecked {
			allChecked = false
			break
		}
	}
	for index := range model.checked {
		model.checked[index] = !allChecked
	}
}

func (model *multiSelectModel) resolveBuffer() {
	nextChecked, resolveErr := parseMultiSelection(model.buffer, model.options)
	model.buffer = ""
	if resolveErr != nil {
		model.message = resolveErr.Error()
		return
	}
	model.checked = nextChecked
	model.done = true
}

func parseMultiSelection(input string, options []Option) ([]bool, error) {
	token := strings.TrimSpace(input)
	checked := make([]bool, len(options))
	if token == "" {
		return checked, nil
	}
	lowerToken := strings.ToLower(token)
	if lowerToken == "all" || lowerToken == "tous" || lowerToken == "toutes" {
		for index := range checked {
			checked[index] = true
		}
		return checked, nil
	}
	fields := strings.FieldsFunc(token, func(separator rune) bool {
		return separator == ',' || separator == ' ' || separator == '\t'
	})
	for _, field := range fields {
		index, resolveErr := resolveSelectToken(field, options, -1)
		if resolveErr != nil {
			labels := make([]string, 0, len(options))
			for position, option := range options {
				labels = append(labels, fmt.Sprintf("%d=%s", position+1, option.Label))
			}
			return nil, fmt.Errorf("%s (choices: %s)", resolveErr.Error(), strings.Join(labels, ", "))
		}
		checked[index] = true
	}
	return checked, nil
}

func (model multiSelectModel) selectedIndices() []int {
	return selectedFromChecked(model.checked)
}

func (model multiSelectModel) View() string {
	var builder strings.Builder
	builder.WriteString(TitleStyle.Render(model.title) + "\n")
	for index, option := range model.options {
		check := DimStyle.Render("[ ]")
		if model.checked[index] {
			check = SuccessStyle.Render("[x]")
		}
		if index == model.cursor {
			builder.WriteString(AccentStyle.Render(cursorGlyph+" ") + check + " " + CursorStyle.Render(option.Label) + "\n")
			continue
		}
		builder.WriteString("  " + check + " " + option.Label + "\n")
	}
	builder.WriteString(DimStyle.Render(multiHintLine) + "\n")
	if model.buffer != "" {
		builder.WriteString(AccentStyle.Render(cursorGlyph+" ") + model.buffer + "\n")
	}
	if model.message != "" {
		builder.WriteString(ErrorStyle.Render(model.message) + "\n")
	}
	return builder.String()
}

func MultiSelect(session *Session, config MultiSelectConfig) ([]int, error) {
	model := newMultiSelectModel(config)
	if session.Interactive {
		finalModel, err := session.RunProgram(model)
		if err != nil {
			return nil, err
		}
		result := finalModel.(multiSelectModel)
		if result.aborted {
			return nil, ErrAborted
		}
		return result.selectedIndices(), nil
	}
	for {
		fmt.Fprint(session.Stdout, model.View())
		line, readErr := session.readLine()
		if readErr != nil {
			return nil, readErr
		}
		if strings.TrimSpace(line) == "" {
			return config.Selected, nil
		}
		checked, resolveErr := parseMultiSelection(line, config.Options)
		if resolveErr != nil {
			fmt.Fprintln(session.Stdout, resolveErr.Error())
			continue
		}
		return selectedFromChecked(checked), nil
	}
}
