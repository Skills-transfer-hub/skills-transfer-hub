package writer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Each non-Claude writer prepends the minimal HTML-comment marker to Markdown
// files when AddManagedHeader is true, and is idempotent on re-write.
func TestMinimalMarker_AppliedAndIdempotent(t *testing.T) {
	cases := []struct {
		name    string
		writer  TargetWriter
		fileExt string
	}{
		{"copilot-md", CopilotWriter{}, ".instructions.md"},
		{"codex-md", CodexWriter{}, ".md"},
		{"cursor-mdc", CursorWriter{}, ".mdc"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "leaf"+tc.fileExt)

			first, err := tc.writer.Write(WriteRequest{
				InstalledPath:    target,
				Content:          []byte("# Hello\nbody\n"),
				AddManagedHeader: true,
			})
			if err != nil {
				t.Fatalf("first write: %v", err)
			}
			data, _ := os.ReadFile(target)
			if !strings.HasPrefix(string(data), "<!-- managed by sth -->") {
				t.Fatalf("marker missing, got prefix %q", string(data[:30]))
			}

			second, err := tc.writer.Write(WriteRequest{
				InstalledPath:    target,
				Content:          data,
				AddManagedHeader: true,
			})
			if err != nil {
				t.Fatalf("second write: %v", err)
			}
			if first.SHA256 != second.SHA256 {
				t.Fatalf("marker should be idempotent across re-writes, got %s vs %s", first.SHA256, second.SHA256)
			}
		})
	}
}

func TestMinimalMarker_InjectedAfterFrontmatter(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "x.instructions.md")
	src := []byte("---\napplyTo: '**/*.tsx'\n---\nbody\n")

	if _, err := (CopilotWriter{}).Write(WriteRequest{
		InstalledPath:    target,
		Content:          src,
		AddManagedHeader: true,
	}); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(target)
	str := string(data)
	if !strings.HasPrefix(str, "---\napplyTo: '**/*.tsx'\n---\n") {
		t.Fatalf("frontmatter not preserved on line 1, got: %q", str)
	}
	if !strings.Contains(str, "<!-- managed by sth -->") {
		t.Fatal("marker missing after frontmatter")
	}
}

func TestMinimalMarker_SkippedForNonMarkdown(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "data.json")
	if _, err := (CodexWriter{}).Write(WriteRequest{
		InstalledPath:    target,
		Content:          []byte(`{"k":1}`),
		AddManagedHeader: true,
	}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(target)
	if string(data) != `{"k":1}` {
		t.Fatalf("non-markdown should not get marker, got %q", string(data))
	}
}

func TestCursorWriter_HonorsMdcAsMarkdown(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "rule.mdc")
	if _, err := (CursorWriter{}).Write(WriteRequest{
		InstalledPath:    target,
		Content:          []byte("rule body\n"),
		AddManagedHeader: true,
	}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(target)
	if !strings.HasPrefix(string(data), "<!-- managed by sth -->") {
		t.Fatalf(".mdc should be treated as markdown for marker purposes, got %q", string(data[:30]))
	}
}
