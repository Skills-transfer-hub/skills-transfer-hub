package tui

import (
	"os"
	"testing"
)

func TestNewSessionRequiresTerminalInputAndOutput(t *testing.T) {
	inputFile, err := os.CreateTemp(t.TempDir(), "input")
	if err != nil {
		t.Fatalf("create input file: %v", err)
	}
	defer inputFile.Close()

	outputFile, err := os.CreateTemp(t.TempDir(), "output")
	if err != nil {
		t.Fatalf("create output file: %v", err)
	}
	defer outputFile.Close()

	inputRedirected := NewSession(inputFile, os.Stdout)
	if inputRedirected.Interactive {
		t.Fatal("session with redirected stdin must not be interactive")
	}

	outputRedirected := NewSession(os.Stdin, outputFile)
	if outputRedirected.Interactive {
		t.Fatal("session with redirected stdout must not be interactive")
	}
}
