package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestRenderNarratorLayout(t *testing.T) {
	output := RenderNarrator(MoodHello, 0, "Première ligne", "Deuxième")
	lines := strings.Split(output, "\n")
	if len(lines) < 6 {
		t.Fatalf("rendu trop court :\n%s", output)
	}
	if !strings.Contains(output, "😊") {
		t.Fatalf("emoji absent :\n%s", output)
	}
	if !strings.Contains(output, "╭─────╮   Première ligne") {
		t.Fatalf("première ligne mal alignée :\n%s", output)
	}
	if !strings.Contains(output, "│ ^_^ │   Deuxième") {
		t.Fatalf("alignement narration attendu sur la ligne du visage :\n%s", output)
	}
	if strings.Count(output, "Première ligne") != 1 {
		t.Fatalf("ligne dupliquée :\n%s", output)
	}
}

func TestFrameWidthClampsToTerminal(t *testing.T) {
	if got := frameWidth(200); got != outerWidth {
		t.Fatalf("largeur large : %d, attendu %d", got, outerWidth)
	}
	if got := frameWidth(50); got != 50 {
		t.Fatalf("largeur étroite : %d, attendu 50", got)
	}
	if got := frameWidth(10); got != 10 {
		t.Fatalf("largeur très étroite : %d, attendu 10", got)
	}
}

func TestRenderNarratorAlwaysClosesFaceBox(t *testing.T) {
	output := RenderNarrator(MoodHello, 30, "Only one line")
	if !strings.Contains(output, "╭─────╮") || !strings.Contains(output, "│ ^_^ │") || !strings.Contains(output, "╰─────╯") {
		t.Fatalf("incomplete narrator face box:\n%s", output)
	}
	for _, line := range strings.Split(output, "\n") {
		if lipgloss.Width(line) > 30 {
			t.Fatalf("line width = %d, want <= 30: %q", lipgloss.Width(line), line)
		}
	}
}

func TestPadFaceCentersThreeCells(t *testing.T) {
	if got := padFace("^_^", 3); got != " ^_^ " {
		t.Fatalf("visage plein : %q", got)
	}
	if got := padFace(":( ", 3); lipgloss.Width(got) != 5 {
		t.Fatalf("largeur paddée inattendue : %q (%d)", got, lipgloss.Width(got))
	}
}
