package cmd

import (
	"fmt"
	"strings"

	"sth/internal/fs"
)

// buddyView is the immutable visual data for one buddy state.
type buddyView struct {
	emoji   string
	face    string
	caption string
}

func buddyViewFor(state fs.BuddyState) buddyView {
	switch state {
	case fs.BuddyAllGood:
		return buddyView{emoji: "💧", face: ">/<", caption: "Everything is up to date."}
	case fs.BuddyUpdateAvailable:
		return buddyView{emoji: "⚠️", face: "o.O", caption: "Update available."}
	case fs.BuddyNoConfig:
		return buddyView{emoji: "😶", face: "._.", caption: "No project configuration found."}
	case fs.BuddyError:
		return buddyView{emoji: "❌", face: "x x", caption: "An error occurred."}
	case fs.BuddyInstalling:
		return buddyView{emoji: "⏳", face: ">_<", caption: "Installing…"}
	case fs.BuddyDone:
		return buddyView{emoji: "✅", face: "^.^", caption: "Operation finished."}
	default:
		return buddyView{emoji: "💧", face: ">/<", caption: "Everything is up to date."}
	}
}

// renderBuddy returns the multi-line ASCII/Unicode buddy display.
// The face is centered inside a fixed-width rounded box so all states align
// vertically.
func renderBuddy(state fs.BuddyState) string {
	const innerWidth = 5 // big enough for "._. " and "o.O"
	v := buddyViewFor(state)

	face := v.face
	if pad := innerWidth - displayWidth(face); pad > 0 {
		left := pad / 2
		right := pad - left
		face = strings.Repeat(" ", left) + face + strings.Repeat(" ", right)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "    %s\n", v.emoji)
	b.WriteString("\n")
	fmt.Fprintf(&b, "  ╭%s╮\n", strings.Repeat("─", innerWidth))
	fmt.Fprintf(&b, "  │%s│\n", face)
	fmt.Fprintf(&b, "  ╰%s╯\n", strings.Repeat("─", innerWidth))
	b.WriteString(v.caption)
	return b.String()
}

// displayWidth approximates the printable cell width of s. STH's buddy faces
// only use ASCII so a byte count is exact.
func displayWidth(s string) int {
	return len(s)
}
