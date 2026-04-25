package fs

import (
	"bytes"
	"strings"
	"testing"
)

func TestAddManagedHeader_NoFrontmatter_PrependsHeader(t *testing.T) {
	got := addManagedHeaderIfMarkdown("alpha.md", []byte("# Alpha\nbody\n"))
	if !bytes.HasPrefix(got, []byte(skillHeader)) {
		t.Fatalf("expected header at top, got:\n%s", got)
	}
	if !bytes.Contains(got, []byte("# Alpha\nbody\n")) {
		t.Fatalf("body not preserved, got:\n%s", got)
	}
}

func TestAddManagedHeader_NonMarkdown_ReturnsUnchanged(t *testing.T) {
	in := []byte("plain text\n")
	got := addManagedHeaderIfMarkdown("notes.txt", in)
	if !bytes.Equal(got, in) {
		t.Fatalf("expected unchanged for .txt; got:\n%s", got)
	}
}

func TestAddManagedHeader_AlreadyPrefixed_NoOp(t *testing.T) {
	already := append([]byte(skillHeader), []byte("# Alpha\n")...)
	got := addManagedHeaderIfMarkdown("alpha.md", already)
	if !bytes.Equal(got, already) {
		t.Fatalf("expected no-op when header already present, got:\n%s", got)
	}
}

func TestAddManagedHeader_PreservesFrontmatterOnLineOne(t *testing.T) {
	in := []byte("---\nname: form-wizard\ndescription: builds React forms\n---\n# Body\n")
	got := addManagedHeaderIfMarkdown("SKILL.md", in)

	if !bytes.HasPrefix(got, []byte("---\n")) {
		t.Fatalf("frontmatter must remain on line 1; got:\n%s", got)
	}
	// Frontmatter intact and unchanged.
	if !bytes.Contains(got, []byte("---\nname: form-wizard\ndescription: builds React forms\n---\n")) {
		t.Fatalf("frontmatter block was altered; got:\n%s", got)
	}
	// Header injected AFTER the closing ---\n
	idx := bytes.Index(got, []byte("---\n# Body\n"))
	idxHeader := bytes.Index(got, []byte(skillHeader))
	closingFM := bytes.Index(got, []byte("description: builds React forms\n---\n"))
	if idxHeader < 0 || closingFM < 0 {
		t.Fatalf("missing markers; got:\n%s", got)
	}
	if idxHeader < closingFM {
		t.Fatalf("header injected before frontmatter close; got:\n%s", got)
	}
	if idx >= 0 {
		// Body should not be glued to the closing fence anymore — header sits between.
		t.Fatalf("body must not directly follow closing ---; got:\n%s", got)
	}
}

func TestAddManagedHeader_FrontmatterWithCRLF(t *testing.T) {
	in := []byte("---\r\nname: foo\r\ndescription: bar\r\n---\r\nbody\r\n")
	got := addManagedHeaderIfMarkdown("SKILL.md", in)

	if !bytes.HasPrefix(got, []byte("---\r\n")) {
		t.Fatalf("CRLF frontmatter must remain on line 1; got:\n%q", got)
	}
	if !bytes.Contains(got, []byte("---\r\nname: foo\r\ndescription: bar\r\n---\r\n")) {
		t.Fatalf("CRLF frontmatter block was altered; got:\n%q", got)
	}
	idxHeader := bytes.Index(got, []byte(skillHeader))
	idxClose := bytes.Index(got, []byte("---\r\nbody"))
	// idxClose is now -1 because header sits between the closing --- and the body.
	if idxClose >= 0 {
		t.Fatalf("body must not directly follow closing CRLF fence; got:\n%q", got)
	}
	if idxHeader < bytes.Index(got, []byte("---\r\nname: foo")) {
		t.Fatalf("header injected before frontmatter close; got:\n%q", got)
	}
}

func TestAddManagedHeader_FrontmatterWithBOM(t *testing.T) {
	in := append([]byte{0xEF, 0xBB, 0xBF}, []byte("---\nname: foo\n---\nbody\n")...)
	got := addManagedHeaderIfMarkdown("SKILL.md", in)

	// BOM must remain at byte 0.
	if !bytes.HasPrefix(got, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatalf("BOM lost; got first bytes %x", got[:4])
	}
	// Frontmatter still on the very first line after BOM.
	if !bytes.HasPrefix(got[3:], []byte("---\n")) {
		t.Fatalf("frontmatter not on line 1 after BOM; got:\n%s", got[3:])
	}
	if !bytes.Contains(got, []byte(skillHeader)) {
		t.Fatalf("header missing; got:\n%s", got)
	}
}

func TestAddManagedHeader_EmptyFrontmatter(t *testing.T) {
	in := []byte("---\n---\nbody\n")
	got := addManagedHeaderIfMarkdown("SKILL.md", in)

	if !bytes.HasPrefix(got, []byte("---\n---\n")) {
		t.Fatalf("empty frontmatter not preserved; got:\n%s", got)
	}
	// Header sits between closing fence and body.
	if !strings.Contains(string(got), "---\n---\n"+skillHeader+"body\n") {
		t.Fatalf("header not inserted between fence and body; got:\n%s", got)
	}
}

func TestAddManagedHeader_DanglingDashesNotFrontmatter(t *testing.T) {
	// `---` followed by content but never closed → fall back to legacy behavior
	// (prepend at top).
	in := []byte("---\nthis looks like frontmatter but never closes\nstill not closed\n")
	got := addManagedHeaderIfMarkdown("alpha.md", in)
	if !bytes.HasPrefix(got, []byte(skillHeader)) {
		t.Fatalf("expected legacy header-on-top when fence is unclosed; got:\n%s", got)
	}
}

func TestAddManagedHeader_AlreadyHasHeaderAfterFrontmatter_NoOp(t *testing.T) {
	in := []byte("---\nname: foo\n---\n" + skillHeader + "body\n")
	got := addManagedHeaderIfMarkdown("SKILL.md", in)
	if !bytes.Equal(got, in) {
		t.Fatalf("expected no-op when header already after frontmatter; got:\n%s", got)
	}
}

func TestAddManagedHeader_DoesNotConfuseLegacyHeaderForFrontmatterDelimiter(t *testing.T) {
	// The legacy header starts with "> ⚠️" which is not "---", so it must not
	// fool the detector into thinking there's a frontmatter.
	in := append([]byte(skillHeader), []byte("---\nthis is just content with --- inside\n")...)
	got := addManagedHeaderIfMarkdown("alpha.md", in)
	// idempotent: already prefixed with header
	if !bytes.Equal(got, in) {
		t.Fatalf("expected idempotence; got:\n%s", got)
	}
}

func TestFrontmatterBodyOffset_NoMatch(t *testing.T) {
	cases := [][]byte{
		[]byte(""),
		[]byte("# heading\n---\nname: foo\n---\n"), // frontmatter not at top
		[]byte("--- not a fence\n"),
		[]byte("---\nname: foo\n"), // unclosed
	}
	for i, c := range cases {
		if got := frontmatterBodyOffset(c); got >= 0 {
			t.Errorf("case %d: expected -1, got %d", i, got)
		}
	}
}

func TestFrontmatterBodyOffset_MatchPositions(t *testing.T) {
	// "---\nname: foo\n---\nbody\n"
	//  0   4         13   17  22
	// Closing "---\n" ends at offset 17+4=21
	in := []byte("---\nname: foo\n---\nbody\n")
	got := frontmatterBodyOffset(in)
	want := bytes.Index(in, []byte("body\n"))
	if got != want {
		t.Fatalf("frontmatterBodyOffset = %d, want %d (start of body)", got, want)
	}
}
