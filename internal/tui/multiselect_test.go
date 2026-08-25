package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbletea"
)

func TestParseMultiSelection(t *testing.T) {
	options := []Option{
		{Label: "Claude", Aliases: []string{"claude"}},
		{Label: "Copilot", Aliases: []string{"copilot"}},
		{Label: "Codex", Aliases: []string{"codex"}},
	}
	cases := []struct {
		input     string
		wantTrue  []int
		wantError bool
	}{
		{input: "1,3", wantTrue: []int{0, 2}},
		{input: "claude codex", wantTrue: []int{0, 2}},
		{input: "all", wantTrue: []int{0, 1, 2}},
		{input: "TOUS", wantTrue: []int{0, 1, 2}},
		{input: "copilot", wantTrue: []int{1}},
		{input: "2", wantTrue: []int{1}},
		{input: "9", wantError: true},
		{input: "claude,inconnu", wantError: true},
	}
	for _, testCase := range cases {
		checked, err := parseMultiSelection(testCase.input, options)
		if testCase.wantError {
			if err == nil {
				t.Fatalf("entrée %q : erreur attendue", testCase.input)
			}
			continue
		}
		if err != nil {
			t.Fatalf("entrée %q : %v", testCase.input, err)
		}
		for _, index := range testCase.wantTrue {
			if !checked[index] {
				t.Fatalf("entrée %q : index %d devrait être coché", testCase.input, index)
			}
		}
		for index, isChecked := range checked {
			if isChecked && !containsInt(testCase.wantTrue, index) {
				t.Fatalf("entrée %q : index %d ne devrait pas être coché", testCase.input, index)
			}
		}
	}
}

func containsInt(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestMultiSelectModelToggleAndSubmit(t *testing.T) {
	model := newMultiSelectModel(MultiSelectConfig{
		Title:    "Cibles",
		Options:  []Option{{Label: "Claude"}, {Label: "Copilot"}, {Label: "Codex"}},
		Selected: []int{0},
	})
	model = pressKeys(model,
		tea.KeyMsg{Type: tea.KeyDown},
		tea.KeyMsg{Type: tea.KeySpace},
		tea.KeyMsg{Type: tea.KeyEnter},
	).(multiSelectModel)
	if !model.done {
		t.Fatal("validation attendue après Entrée")
	}
	indices := model.selectedIndices()
	if len(indices) != 2 || indices[0] != 0 || indices[1] != 1 {
		t.Fatalf("sélection inattendue : %v", indices)
	}
}

func TestMultiSelectTypedGrammarInModel(t *testing.T) {
	model := newMultiSelectModel(MultiSelectConfig{
		Title:   "Cibles",
		Options: []Option{{Label: "Claude"}, {Label: "Copilot"}, {Label: "Codex"}},
	})
	model = pressKeys(model,
		runeKey("2"), runeKey(","), runeKey("3"),
		tea.KeyMsg{Type: tea.KeyEnter},
	).(multiSelectModel)
	indices := model.selectedIndices()
	if len(indices) != 2 || indices[0] != 1 || indices[1] != 2 {
		t.Fatalf("grammaire typée : sélection %v", indices)
	}
}

func TestMultiSelectModelKeyboardToggles(t *testing.T) {
	model := newMultiSelectModel(MultiSelectConfig{
		Title:   "Targets",
		Options: []Option{{Label: "Claude"}, {Label: "Copilot"}, {Label: "Codex"}},
	})

	model = pressKeys(model, runeKey("x")).(multiSelectModel)
	if !model.checked[0] {
		t.Fatal("x should toggle the current option")
	}

	model = pressKeys(model, runeKey("a")).(multiSelectModel)
	for index, isChecked := range model.checked {
		if !isChecked {
			t.Fatalf("a should select every option, index %d is not selected", index)
		}
	}

	model = pressKeys(model, runeKey("a")).(multiSelectModel)
	for index, isChecked := range model.checked {
		if isChecked {
			t.Fatalf("a should clear every option when all are selected, index %d is still selected", index)
		}
	}
}

func TestMultiSelectModelShortcutsPreserveTypedGrammar(t *testing.T) {
	model := newMultiSelectModel(MultiSelectConfig{
		Title:   "Targets",
		Options: []Option{{Label: "Agent"}, {Label: "Copilot"}, {Label: "Codex"}},
	})

	model = pressKeys(model,
		runeKey("a"), runeKey("l"), runeKey("l"),
		tea.KeyMsg{Type: tea.KeyEnter},
	).(multiSelectModel)
	if !model.done {
		t.Fatal("typed all should submit")
	}
	for index, isChecked := range model.checked {
		if !isChecked {
			t.Fatalf("typed all should select every option, index %d is not selected", index)
		}
	}

	model = newMultiSelectModel(MultiSelectConfig{
		Title:   "Targets",
		Options: []Option{{Label: "Agent"}, {Label: "Copilot"}, {Label: "Codex"}},
	})
	model = pressKeys(model,
		runeKey("a"), runeKey("g"), runeKey("e"), runeKey("n"), runeKey("t"),
		tea.KeyMsg{Type: tea.KeyEnter},
	).(multiSelectModel)
	if !model.done || !model.checked[0] {
		t.Fatal("typed name beginning with a should remain selectable")
	}
}

func TestMultiSelectViewMarkers(t *testing.T) {
	model := newMultiSelectModel(MultiSelectConfig{
		Title:    "Cibles",
		Options:  []Option{{Label: "Claude"}, {Label: "Copilot"}},
		Selected: []int{1},
	})
	view := model.View()
	if !strings.Contains(view, "[x] Copilot") || !strings.Contains(view, "[ ] Claude") {
		t.Fatalf("marqueurs de cases manquants :\n%s", view)
	}
}
