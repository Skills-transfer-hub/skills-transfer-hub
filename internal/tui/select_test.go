package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbletea"
)

func selectTestOptions() []Option {
	return []Option{
		{Label: "Lister", Aliases: []string{"list", "l"}},
		{Label: "Installer", Aliases: []string{"install"}},
		{Label: "Quitter", Aliases: []string{"quit", "q"}},
	}
}

func pressKeys(current tea.Model, keys ...tea.KeyMsg) tea.Model {
	for _, key := range keys {
		next, _ := current.Update(key)
		current = next
	}
	return current
}

func runeKey(value string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(value)}
}

func TestResolveSelectToken(t *testing.T) {
	options := selectTestOptions()
	cases := []struct {
		token       string
		recommended int
		wantIndex   int
		wantError   bool
	}{
		{token: "", recommended: 1, wantIndex: 1},
		{token: "2", wantIndex: 1},
		{token: "3", wantIndex: 2},
		{token: "list", wantIndex: 0},
		{token: "INSTALL", wantIndex: 1},
		{token: "q", wantIndex: 2},
		{token: "Quitter", wantIndex: 2},
		{token: "9", wantError: true},
		{token: "inconnu", wantError: true},
	}
	for _, testCase := range cases {
		index, err := resolveSelectToken(testCase.token, options, testCase.recommended)
		if testCase.wantError {
			if err == nil {
				t.Fatalf("token %q : erreur attendue", testCase.token)
			}
			continue
		}
		if err != nil {
			t.Fatalf("token %q : %v", testCase.token, err)
		}
		if index != testCase.wantIndex {
			t.Fatalf("token %q : index %d, attendu %d", testCase.token, index, testCase.wantIndex)
		}
	}
}

func TestSelectModelArrowNavigation(t *testing.T) {
	model := newSelectModel(SelectConfig{
		Title:       "Menu",
		Options:     selectTestOptions(),
		Recommended: 0,
	})
	model = pressKeys(model,
		tea.KeyMsg{Type: tea.KeyDown},
		tea.KeyMsg{Type: tea.KeyDown},
		tea.KeyMsg{Type: tea.KeyEnter},
	).(selectModel)
	if !model.chosen || model.chosenIndex != 2 {
		t.Fatalf("navigation fléchée : choisi=%v index=%d", model.chosen, model.chosenIndex)
	}
}

func TestSelectModelTypedAlias(t *testing.T) {
	model := newSelectModel(SelectConfig{
		Title:   "Menu",
		Options: selectTestOptions(),
	})
	typed := "install"
	keys := make([]tea.KeyMsg, 0, len(typed)+1)
	for _, letter := range typed {
		keys = append(keys, runeKey(string(letter)))
	}
	keys = append(keys, tea.KeyMsg{Type: tea.KeyEnter})
	model = pressKeys(model, keys...).(selectModel)
	if !model.chosen || model.chosenIndex != 1 {
		t.Fatalf("alias tapé : choisi=%v index=%d message=%q", model.chosen, model.chosenIndex, model.message)
	}
}

func TestSelectModelInvalidThenEscapeClearsBuffer(t *testing.T) {
	model := newSelectModel(SelectConfig{
		Title:   "Menu",
		Options: selectTestOptions(),
	})
	model = pressKeys(model, runeKey("z"), runeKey("z"), tea.KeyMsg{Type: tea.KeyEnter}).(selectModel)
	if model.chosen || model.message == "" {
		t.Fatalf("saisie invalide : message attendu, obtenu choisi=%v message=%q", model.chosen, model.message)
	}
	if model.buffer != "zz" {
		t.Fatalf("le tampon doit rester éditable après erreur : %q", model.buffer)
	}
	model = pressKeys(model, tea.KeyMsg{Type: tea.KeyEsc}).(selectModel)
	if model.buffer != "" || model.aborted {
		t.Fatalf("échape doit vider le tampon sans abandonner : buffer=%q aborted=%v", model.buffer, model.aborted)
	}
	model = pressKeys(model, tea.KeyMsg{Type: tea.KeyEsc}).(selectModel)
	if !model.aborted {
		t.Fatal("second échape doit abandonner")
	}
}

func TestSelectViewShowsRecommendedSuffix(t *testing.T) {
	model := newSelectModel(SelectConfig{
		Title:       "Menu",
		Options:     selectTestOptions(),
		Recommended: 1,
	})
	view := model.View()
	if !strings.Contains(view, "Installer (recommended)") {
		t.Fatalf("vue sans suffixe recommandé :\n%s", view)
	}
	if strings.Contains(view, "Lister (recommended)") {
		t.Fatalf("suffixe appliqué à la mauvaise option :\n%s", view)
	}
}

func TestSelectHeadlessFallsBackAfterBoundedInvalidAttempts(t *testing.T) {
	session := NewSession(strings.NewReader("bad\nstill-bad\nnope\nvalid\n"), &strings.Builder{})
	index, err := Select(session, SelectConfig{
		Title:       "Bounded",
		Options:     selectTestOptions(),
		Recommended: 1,
		MaxAttempts: 3,
	})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if index != 1 {
		t.Fatalf("index = %d, want recommended index 1", index)
	}
}

func TestSelectModelFallsBackAfterBoundedInvalidAttempts(t *testing.T) {
	model := newSelectModel(SelectConfig{
		Title:       "Bounded",
		Options:     selectTestOptions(),
		Recommended: 1,
		MaxAttempts: 3,
	})
	model = pressKeys(model, runeKey("bad"), tea.KeyMsg{Type: tea.KeyEnter}).(selectModel)
	if model.chosen || model.attempts != 1 {
		t.Fatalf("first invalid attempt = chosen %v, attempts %d", model.chosen, model.attempts)
	}
	model = pressKeys(model, tea.KeyMsg{Type: tea.KeyEnter}).(selectModel)
	if model.chosen || model.attempts != 2 {
		t.Fatalf("second invalid attempt = chosen %v, attempts %d", model.chosen, model.attempts)
	}
	model = pressKeys(model, tea.KeyMsg{Type: tea.KeyEnter}).(selectModel)
	if !model.chosen || model.chosenIndex != 1 {
		t.Fatalf("fallback = chosen %v, index %d", model.chosen, model.chosenIndex)
	}
}
