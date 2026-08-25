package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

const (
	outerWidth     = 78
	ansiShowCursor = "\x1b[?25h"
)

type Mood int

const (
	MoodHello Mood = iota
	MoodThinking
	MoodWorried
	MoodSad
	MoodHappy
	MoodError
)

var (
	AccentStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	TitleStyle   = lipgloss.NewStyle().Bold(true)
	DimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	SuccessStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	WarningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	ErrorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	CursorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)
)

func EnableColors() {
	lipgloss.SetColorProfile(termenv.ANSI256)
}

func DisableColors() {
	lipgloss.SetColorProfile(termenv.Ascii)
}

type moodView struct {
	emoji string
	face  string
}

func moodViewFor(mood Mood) moodView {
	switch mood {
	case MoodThinking:
		return moodView{emoji: "🤔", face: "o_o"}
	case MoodWorried:
		return moodView{emoji: "😟", face: ">_<"}
	case MoodSad:
		return moodView{emoji: "🙁", face: ":("}
	case MoodHappy:
		return moodView{emoji: "✅", face: "^.^"}
	case MoodError:
		return moodView{emoji: "⚠️", face: "o_o"}
	default:
		return moodView{emoji: "😊", face: "^_^"}
	}
}

func frameWidth(maxWidth int) int {
	if maxWidth <= 0 || maxWidth >= outerWidth {
		return outerWidth
	}
	return maxWidth
}

func frame(content string, width int) string {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Width(width - 2).
		PaddingLeft(1).
		PaddingRight(0)
	return style.Render(content)
}

func padFace(face string, cells int) string {
	width := lipgloss.Width(face)
	if width >= cells {
		return " " + face + " "
	}
	left := (cells - width) / 2
	right := cells - width - left
	return strings.Repeat(" ", left+1) + face + strings.Repeat(" ", right+1)
}

func RenderNarrator(mood Mood, maxWidth int, lines ...string) string {
	view := moodViewFor(mood)
	face := padFace(view.face, 3)
	faceBoxWidth := lipgloss.Width(face) + 2
	gap := "   "
	narrationIndent := strings.Repeat(" ", 4+faceBoxWidth+len(gap))
	rows := []string{
		"",
		"      " + view.emoji,
	}
	narrationLines := append([]string(nil), lines...)
	for len(narrationLines) < 3 {
		narrationLines = append(narrationLines, "")
	}
	for index, line := range narrationLines {
		switch index {
		case 0:
			rows = append(rows, "    ╭"+strings.Repeat("─", faceBoxWidth-2)+"╮"+gap+line)
		case 1:
			rows = append(rows, "    │"+face+"│"+gap+line)
		case 2:
			row := "    ╰" + strings.Repeat("─", faceBoxWidth-2) + "╯"
			if line != "" {
				row += gap + line
			}
			rows = append(rows, row)
		default:
			rows = append(rows, narrationIndent+line)
		}
	}
	rows = append(rows, "")
	return frame(strings.Join(rows, "\n"), frameWidth(maxWidth))
}
