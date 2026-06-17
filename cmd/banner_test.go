package cmd

import (
	"bytes"
	"strings"
	"testing"

	"sth/internal/fs"
)

func TestRenderBanner_AllStatesIncludeFaceAndEmoji(t *testing.T) {
	cases := []struct {
		name  string
		state fs.BuddyState
		emoji string
		face  string
	}{
		{"AllGood", fs.BuddyAllGood, "💧", ">/<"},
		{"UpdateAvailable", fs.BuddyUpdateAvailable, "⚠️", "o.O"},
		{"NoConfig", fs.BuddyNoConfig, "😶", "._."},
		{"Error", fs.BuddyError, "❌", "x x"},
		{"Installing", fs.BuddyInstalling, "⏳", ">_<"},
		{"Done", fs.BuddyDone, "✅", "^.^"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := renderBanner(tc.state)
			if !strings.Contains(out, tc.emoji) {
				t.Errorf("banner missing emoji %q in:\n%s", tc.emoji, out)
			}
			if !strings.Contains(out, tc.face) {
				t.Errorf("banner missing face %q in:\n%s", tc.face, out)
			}
		})
	}
}

func TestRenderBanner_HasExpectedTextElements(t *testing.T) {
	out := renderBanner(fs.BuddyAllGood)
	for _, want := range []string{
		"STH ",
		"Skills Transfer Hub.",
		"Astuce",
		"sth help",
		".claude/skills",
		"╭", "╮", "╰", "╯", // outer + inner box drawing chars
	} {
		if !strings.Contains(out, want) {
			t.Errorf("banner missing %q in:\n%s", want, out)
		}
	}
}

func TestRenderBanner_BoxIsRectangular(t *testing.T) {
	out := renderBanner(fs.BuddyAllGood)
	lines := strings.Split(out, "\n")
	if len(lines) < 5 {
		t.Fatalf("expected multi-line banner, got %d lines", len(lines))
	}
	// First and last line are the top/bottom of the outer box; both must
	// have the same visual width.
	top := displayCells(lines[0])
	bottom := displayCells(lines[len(lines)-1])
	if top != bottom {
		t.Errorf("top width %d != bottom width %d", top, bottom)
	}
	if top != bannerOuterWidth {
		t.Errorf("top width = %d, want %d", top, bannerOuterWidth)
	}

	// Every interior line must close at the right border with `│`.
	for i := 1; i < len(lines)-1; i++ {
		line := lines[i]
		if !strings.HasPrefix(line, "│") {
			t.Errorf("line %d does not start with │: %q", i, line)
		}
		if !strings.HasSuffix(line, "│") {
			t.Errorf("line %d does not end with │: %q", i, line)
		}
		if got := displayCells(line); got != bannerOuterWidth {
			t.Errorf("line %d width = %d, want %d: %q", i, got, bannerOuterWidth, line)
		}
	}
}

func TestPlayLaunchBanner_NonTTY_WritesFinalFrameOnly(t *testing.T) {
	// bytes.Buffer is not an *os.File, so isWriterTTY returns false → no animation.
	var buf bytes.Buffer
	a := &app{stdout: &buf, stderr: &buf}
	a.playLaunchBanner(fs.BuddyAllGood)

	out := buf.String()
	// Must not contain ANSI cursor-up sequences (animation off).
	if strings.Contains(out, "\x1b[") {
		t.Errorf("banner emitted ANSI escapes in non-TTY mode:\n%s", out)
	}
	// Must contain the final frame for "all good".
	if !strings.Contains(out, ">/<") || !strings.Contains(out, "💧") {
		t.Errorf("banner missing final state markers in non-TTY:\n%s", out)
	}
}

func TestDisplayCells_AsciiAndEmoji(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"abc", 3},
		{">/<", 3},
		{"💧", 2},
		{"⚠️", 2},  // ⚠ + variation selector (counted as 2 + 0)
		{"😶", 2},
		{"╭─╮", 3}, // box drawing = 1 cell each
		{"abc💧def", 8},
		{"⏳", 2}, // sablier (U+23F3)
		{"✅", 2}, // U+2705
		{"❌", 2}, // U+274C
		{"^.^", 3},
	}
	for _, c := range cases {
		if got := displayCells(c.in); got != c.want {
			t.Errorf("displayCells(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestPadFace_FixedWidthAlignment(t *testing.T) {
	for _, face := range []string{">/<", "o.O", "._.", "x x", ">_<", "^.^"} {
		got := padFace(face, 3)
		if displayCells(got) != 5 {
			t.Errorf("padFace(%q, 3) cells = %d, want 5: %q", face, displayCells(got), got)
		}
	}
}

func TestBannerRewindRows_ReturnsCursorToFirstLine(t *testing.T) {
	frame := renderBannerFrame(bannerFrameFor(fs.BuddyError))
	lines := strings.Split(frame, "\n")
	want := len(lines) - 1
	if got := bannerRewindRows(frame); got != want {
		t.Errorf("bannerRewindRows = %d, want %d for a %d-line frame; an off-by-one leaves border residue in the menu", got, want, len(lines))
	}
}
