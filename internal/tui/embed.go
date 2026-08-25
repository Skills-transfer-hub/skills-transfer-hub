package tui

import (
	"strings"

	"github.com/charmbracelet/bubbletea"
)

type SelectModel struct {
	inner selectModel
}

func NewSelectModel(config SelectConfig) *SelectModel {
	return &SelectModel{inner: newSelectModel(config)}
}

func (model *SelectModel) Forward(message tea.Msg) tea.Cmd {
	next, cmd := model.inner.Update(message)
	if updated, ok := next.(selectModel); ok {
		model.inner = updated
	}
	return cmd
}

func (model *SelectModel) View() string {
	return model.inner.View()
}

func (model *SelectModel) Result() (int, bool, bool) {
	return model.inner.chosenIndex, model.inner.chosen, model.inner.aborted
}

func (model *SelectModel) Reset(config SelectConfig) {
	model.inner = newSelectModel(config)
}

type InputField struct {
	inner inputModel
}

func NewInputField(config InputConfig) *InputField {
	return &InputField{inner: newInputModel(config)}
}

func (field *InputField) Forward(message tea.Msg) tea.Cmd {
	next, cmd := field.inner.Update(message)
	if updated, ok := next.(inputModel); ok {
		field.inner = updated
	}
	return cmd
}

func (field *InputField) View() string {
	return field.inner.View()
}

func (field *InputField) Result() (string, bool, bool) {
	return strings.TrimSpace(field.inner.field.Value()), field.inner.submitted, field.inner.aborted
}

type SecretField struct {
	inner secretModel
}

func NewSecretField(config SecretConfig) *SecretField {
	return &SecretField{inner: newSecretModel(config)}
}

func (field *SecretField) Forward(message tea.Msg) tea.Cmd {
	next, cmd := field.inner.Update(message)
	if updated, ok := next.(secretModel); ok {
		field.inner = updated
	}
	return cmd
}

func (field *SecretField) View() string {
	return field.inner.View()
}

func (field *SecretField) Result() (string, bool, bool) {
	return field.inner.field.Value(), field.inner.submitted, field.inner.aborted
}

type MultiField struct {
	inner multiSelectModel
}

func NewMultiField(config MultiSelectConfig) *MultiField {
	return &MultiField{inner: newMultiSelectModel(config)}
}

func (field *MultiField) Forward(message tea.Msg) tea.Cmd {
	next, cmd := field.inner.Update(message)
	if updated, ok := next.(multiSelectModel); ok {
		field.inner = updated
	}
	return cmd
}

func (field *MultiField) View() string {
	return field.inner.View()
}

func (field *MultiField) Result() ([]int, bool, bool) {
	if field.inner.aborted {
		return nil, false, true
	}
	return field.inner.selectedIndices(), field.inner.done, false
}

func (field *InputField) SetValue(value string) {
	field.inner.field.SetValue(value)
}

func (field *InputField) SetError(message string) {
	field.inner.message = message
	field.inner.submitted = false
}

func (field *SecretField) SetError(message string) {
	field.inner.message = message
	field.inner.submitted = false
}
