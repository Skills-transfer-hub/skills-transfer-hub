package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbletea"
)

func TestConfirmHeadlessDefaults(t *testing.T) {
	yesSession := NewSession(strings.NewReader("\n"), &strings.Builder{})
	answer, err := Confirm(yesSession, ConfirmConfig{Question: "Continuer ?", DefaultYes: true})
	if err != nil {
		t.Fatalf("confirm défaut oui : %v", err)
	}
	if !answer {
		t.Fatal("Entrée doit valider le défaut Oui")
	}

	noSession := NewSession(strings.NewReader("non\n"), &strings.Builder{})
	answer, err = Confirm(noSession, ConfirmConfig{Question: "Continuer ?", DefaultYes: true})
	if err != nil {
		t.Fatalf("confirm non : %v", err)
	}
	if answer {
		t.Fatal("« non » doit répondre Non")
	}

	frSession := NewSession(strings.NewReader("o\n"), &strings.Builder{})
	answer, err = Confirm(frSession, ConfirmConfig{Question: "Continuer ?", DefaultYes: false})
	if err != nil {
		t.Fatalf("confirm o : %v", err)
	}
	if !answer {
		t.Fatal("« o » doit répondre Oui")
	}
}

func TestConfirmRejectsInvalidThenAccepts(t *testing.T) {
	session := NewSession(strings.NewReader("peut-être\noui\n"), &strings.Builder{})
	stdout := session.Stdout.(*strings.Builder)
	answer, err := Confirm(session, ConfirmConfig{Question: "Supprimer ?", DefaultYes: false})
	if err != nil {
		t.Fatalf("confirm après invalide : %v", err)
	}
	if !answer {
		t.Fatal("« oui » doit répondre Oui")
	}
	if !strings.Contains(stdout.String(), "invalid choice") {
		t.Fatalf("message d'invalidité attendu, obtenu :\n%s", stdout.String())
	}
}

func TestConfirmModelArrowToNo(t *testing.T) {
	model := newSelectModel(SelectConfig{
		Title:       "Supprimer ?",
		Options:     []Option{{Label: "Oui", Aliases: []string{"o", "oui", "y", "yes"}}, {Label: "Non", Aliases: []string{"n", "non", "no"}}},
		Recommended: 0,
	})
	model = pressKeys(model,
		runeKey("n"),
		tea.KeyMsg{Type: tea.KeyEnter},
	).(selectModel)
	if !model.chosen || model.chosenIndex != 1 {
		t.Fatalf("alias n : index=%d choisi=%v", model.chosenIndex, model.chosen)
	}
}
