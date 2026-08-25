package tui

import (
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbletea"
)

func TestInputHeadlessDefaultAndRequired(t *testing.T) {
	session := NewSession(strings.NewReader("\n"), &strings.Builder{})
	value, err := Input(session, InputConfig{Label: "Référence Git", Default: "main", Required: true})
	if err != nil {
		t.Fatalf("défaut : %v", err)
	}
	if value != "main" {
		t.Fatalf("valeur %q, attendu main", value)
	}

	emptySession := NewSession(strings.NewReader("\n"), &strings.Builder{})
	_, err = Input(emptySession, InputConfig{Label: "Alias", Required: true})
	if err != ErrAborted {
		t.Fatalf("champ requis vide puis fin de flux : abandon attendu, obtenu : %v", err)
	}
}

func TestInputHeadlessEofAborts(t *testing.T) {
	session := NewSession(strings.NewReader(""), &strings.Builder{})
	_, err := Input(session, InputConfig{Label: "Alias", Required: false})
	if err != ErrAborted {
		t.Fatalf("EOF doit aborter, obtenu : %v", err)
	}
}

func TestInputModelSubmitTrims(t *testing.T) {
	model := newInputModel(InputConfig{Label: "Dépôt", Default: "", Required: true})
	model.field.SetValue("  owner/repo  ")
	submitted := pressKeys(model, tea.KeyMsg{Type: tea.KeyEnter}).(inputModel)
	if !submitted.submitted {
		t.Fatal("soumission attendue")
	}
	if strings.TrimSpace(submitted.field.Value()) != "owner/repo" {
		t.Fatalf("valeur non nettoyée : %q", submitted.field.Value())
	}
}

func TestSecretHeadlessPreservesBytes(t *testing.T) {
	session := NewSession(strings.NewReader("  lsk_live_abc \n"), &strings.Builder{})
	value, err := Secret(session, SecretConfig{Label: "Token"})
	if err != nil {
		t.Fatalf("secret : %v", err)
	}
	if value != "  lsk_live_abc " {
		t.Fatalf("secret altéré : %q", value)
	}
	eofSession := NewSession(strings.NewReader(""), &strings.Builder{})
	value, err = Secret(eofSession, SecretConfig{Label: "Token"})
	if err != nil || value != "" {
		t.Fatalf("EOF secret doit donner chaîne vide sans erreur : %q %v", value, err)
	}
}

func TestSecretMasksTerminalInputWhenOutputIsRedirected(t *testing.T) {
	inputFile, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("open terminal fixture: %v", err)
	}
	defer inputFile.Close()

	var output strings.Builder
	session := NewSession(inputFile, &output)
	session.stdinTTY = true
	readSecretCalled := false
	session.readSecret = func(fileDescriptor int) ([]byte, error) {
		readSecretCalled = true
		if fileDescriptor != int(inputFile.Fd()) {
			t.Fatalf("read secret descriptor = %d, want %d", fileDescriptor, inputFile.Fd())
		}
		return []byte("lsk_live_masked"), nil
	}

	value, err := Secret(session, SecretConfig{Label: "Token"})
	if err != nil {
		t.Fatalf("Secret: %v", err)
	}
	if session.Interactive {
		t.Fatal("redirected output must disable interactive rendering")
	}
	if !readSecretCalled {
		t.Fatal("terminal secret reader was not called")
	}
	if value != "lsk_live_masked" {
		t.Fatalf("secret = %q", value)
	}
	if strings.Contains(output.String(), value) {
		t.Fatalf("secret echoed to redirected output: %q", output.String())
	}
}

func TestEmbeddedFieldsReopenAfterValidationError(t *testing.T) {
	inputField := NewInputField(InputConfig{Label: "Alias"})
	inputField.SetValue("bad")
	inputField.Forward(tea.KeyMsg{Type: tea.KeyEnter})
	_, inputDone, _ := inputField.Result()
	if !inputDone {
		t.Fatal("input was not submitted")
	}
	inputField.SetError("invalid alias")
	_, inputDone, _ = inputField.Result()
	if inputDone {
		t.Fatal("input remained submitted after validation error")
	}
	inputField.Forward(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	inputValue, inputDone, _ := inputField.Result()
	if inputDone || inputValue != "badx" {
		t.Fatalf("input retry = %q, done=%v", inputValue, inputDone)
	}

	secretField := NewSecretField(SecretConfig{Label: "Token"})
	secretField.Forward(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("bad")})
	secretField.Forward(tea.KeyMsg{Type: tea.KeyEnter})
	_, secretDone, _ := secretField.Result()
	if !secretDone {
		t.Fatal("secret was not submitted")
	}
	secretField.SetError("invalid key")
	_, secretDone, _ = secretField.Result()
	if secretDone {
		t.Fatal("secret remained submitted after validation error")
	}
	secretField.Forward(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	secretValue, secretDone, _ := secretField.Result()
	if secretDone || secretValue != "badx" {
		t.Fatalf("secret retry = %q, done=%v", secretValue, secretDone)
	}
}

func TestSelectHeadlessLoopReprompts(t *testing.T) {
	session := NewSession(strings.NewReader("inconnu\n3\n"), &strings.Builder{})
	stdout := session.Stdout.(*strings.Builder)
	index, err := Select(session, SelectConfig{
		Title:   "Menu",
		Options: selectTestOptions(),
	})
	if err != nil {
		t.Fatalf("select headless : %v", err)
	}
	if index != 2 {
		t.Fatalf("index %d, attendu 2", index)
	}
	if !strings.Contains(stdout.String(), "invalid choice") {
		t.Fatalf("re-prompt après invalide attendu :\n%s", stdout.String())
	}
}
