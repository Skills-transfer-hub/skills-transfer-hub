package cmd

import (
	"strings"
	"testing"

	"sth/internal/fs"
)

func TestRenderBuddy_AllStatesIncludeEmojiAndFace(t *testing.T) {
	cases := []struct {
		name    string
		state   fs.BuddyState
		emoji   string
		face    string
		caption string
	}{
		{"AllGood", fs.BuddyAllGood, "💧", ">/<", "up to date"},
		{"UpdateAvailable", fs.BuddyUpdateAvailable, "⚠️", "o.O", "Update available."},
		{"NoConfig", fs.BuddyNoConfig, "😶", "._.", "No project configuration"},
		{"Error", fs.BuddyError, "❌", "x x", "An error occurred."},
		{"Installing", fs.BuddyInstalling, "⏳", ">_<", "Installing"},
		{"Done", fs.BuddyDone, "✅", "^.^", "Operation finished."},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := renderBuddy(tc.state)
			if !strings.Contains(out, tc.emoji) {
				t.Errorf("missing emoji %q in:\n%s", tc.emoji, out)
			}
			if !strings.Contains(out, tc.face) {
				t.Errorf("missing face %q in:\n%s", tc.face, out)
			}
			if !strings.Contains(out, tc.caption) {
				t.Errorf("missing caption fragment %q in:\n%s", tc.caption, out)
			}
			if !strings.Contains(out, "╭") || !strings.Contains(out, "╰") {
				t.Errorf("missing rounded box in:\n%s", out)
			}
		})
	}
}

func TestRenderBuddy_BoxWidthIsConsistent(t *testing.T) {
	want := strings.Count(renderBuddy(fs.BuddyAllGood), "─")
	for _, state := range []fs.BuddyState{
		fs.BuddyUpdateAvailable, fs.BuddyNoConfig, fs.BuddyError,
		fs.BuddyInstalling, fs.BuddyDone,
	} {
		got := strings.Count(renderBuddy(state), "─")
		if got != want {
			t.Errorf("state %v box width %d, want %d", state, got, want)
		}
	}
}
