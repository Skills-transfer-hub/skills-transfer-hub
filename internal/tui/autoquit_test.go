package tui

import (
	"io"
	"os"
	"testing"
	"time"

	"github.com/charmbracelet/bubbletea"
)

type programRunResult struct {
	model tea.Model
	err   error
}

func TestAutoQuitModelSurvivesUntilInnerFinishes(t *testing.T) {
	wrapper := wrapAutoQuit(newSelectModel(SelectConfig{
		Title:   "Action",
		Options: []Option{{Label: "First"}, {Label: "Second"}},
	}))

	nextModel, command := wrapper.Update(tea.KeyMsg{Type: tea.KeyDown})
	if command != nil {
		t.Fatal("navigation should not quit the program")
	}
	nextWrapper, ok := nextModel.(autoQuitModel)
	if !ok {
		t.Fatalf("model type = %T, want autoQuitModel", nextModel)
	}

	finalModel, command := nextWrapper.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command == nil {
		t.Fatal("selection should quit the program")
	}
	if _, ok := command().(tea.QuitMsg); !ok {
		t.Fatalf("command message type = %T, want tea.QuitMsg", command())
	}
	result, ok := unwrapAutoQuit(finalModel).(selectModel)
	if !ok {
		t.Fatalf("unwrapped model type = %T, want selectModel", unwrapAutoQuit(finalModel))
	}
	if !result.chosen || result.chosenIndex != 1 {
		t.Fatalf("selection = chosen:%v index:%d, want chosen:true index:1", result.chosen, result.chosenIndex)
	}
}

func TestAutoQuitCoversInteractiveComponentModels(t *testing.T) {
	testCases := []struct {
		name  string
		model tea.Model
	}{
		{
			name: "select",
			model: newSelectModel(SelectConfig{
				Title:   "Action",
				Options: []Option{{Label: "Continue"}},
			}),
		},
		{
			name:  "input",
			model: newInputModel(InputConfig{Label: "Value"}),
		},
		{
			name:  "secret",
			model: newSecretModel(SecretConfig{Label: "Token"}),
		},
		{
			name: "multi-select",
			model: newMultiSelectModel(MultiSelectConfig{
				Title:   "Targets",
				Options: []Option{{Label: "Codex"}},
			}),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			wrapper := wrapAutoQuit(testCase.model)
			finalModel, command := wrapper.Update(tea.KeyMsg{Type: tea.KeyEnter})
			if command == nil {
				t.Fatal("component should quit after submission")
			}
			if _, ok := command().(tea.QuitMsg); !ok {
				t.Fatalf("command message type = %T, want tea.QuitMsg", command())
			}
			if _, wrapped := finalModel.(autoQuitModel); !wrapped {
				t.Fatalf("final model type = %T, want autoQuitModel", finalModel)
			}
		})
	}
}

func TestSessionRunProgramAutoQuitsFinishedComponent(t *testing.T) {
	readInput, writeInput, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer readInput.Close()
	defer writeInput.Close()

	session := &Session{
		Stdout:      io.Discard,
		Interactive: true,
		stdinFile:   readInput,
		ownedTTY:    readInput,
	}
	resultChannel := make(chan programRunResult, 1)
	go func() {
		finalModel, runErr := session.RunProgram(newSelectModel(SelectConfig{
			Title:   "Action",
			Options: []Option{{Label: "Continue"}},
		}))
		resultChannel <- programRunResult{model: finalModel, err: runErr}
	}()

	if _, err := writeInput.Write([]byte("\r")); err != nil {
		t.Fatalf("write input: %v", err)
	}

	select {
	case runResult := <-resultChannel:
		if runResult.err != nil {
			t.Fatalf("RunProgram: %v", runResult.err)
		}
		result, ok := runResult.model.(selectModel)
		if !ok {
			t.Fatalf("final model type = %T, want selectModel", runResult.model)
		}
		if !result.chosen || result.chosenIndex != 0 {
			t.Fatalf("selection = chosen:%v index:%d, want chosen:true index:0", result.chosen, result.chosenIndex)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunProgram did not quit after selection")
	}
}
