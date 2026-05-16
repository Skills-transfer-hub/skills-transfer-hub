package writer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClaudeWriter_Kind(t *testing.T) {
	if got := (ClaudeWriter{}).Kind(); got != "claude" {
		t.Fatalf("Kind() = %q, want claude", got)
	}
}

func TestClaudeWriter_Write_PrependsBannerForPlainMarkdown(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "nested", "skill.md")

	result, err := NewClaudeWriter().Write(WriteRequest{
		InstalledPath:    target,
		Content:          []byte("# Hello\n\nbody\n"),
		AddManagedHeader: true,
	})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.HasPrefix(string(data), "> ⚠️ ATTENTION : Skill géré par STH") {
		t.Fatalf("missing managed banner, got: %q", string(data[:80]))
	}
	if result.SHA256 == "" {
		t.Fatal("SHA256 should be populated")
	}
	if !strings.Contains(result.Path, "/") {
		t.Fatalf("Path should be slash-normalized, got %q", result.Path)
	}
}

func TestClaudeWriter_Write_InjectsBannerAfterFrontmatter(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "skill.md")
	src := []byte("---\nname: foo\ndescription: bar\n---\nbody line\n")

	if _, err := NewClaudeWriter().Write(WriteRequest{
		InstalledPath:    target,
		Content:          src,
		AddManagedHeader: true,
	}); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	str := string(data)
	if !strings.HasPrefix(str, "---\nname: foo\ndescription: bar\n---\n") {
		t.Fatalf("frontmatter not preserved on line 1, got: %q", str)
	}
	if !strings.Contains(str, "> ⚠️ ATTENTION : Skill géré par STH") {
		t.Fatal("banner missing after frontmatter")
	}
	bannerAfter := strings.Index(str, "> ⚠️ ATTENTION") > strings.Index(str, "---\nname")
	if !bannerAfter {
		t.Fatal("banner should appear AFTER frontmatter close")
	}
}

func TestClaudeWriter_Write_SkipsHeaderForNonMarkdown(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "data.json")

	if _, err := NewClaudeWriter().Write(WriteRequest{
		InstalledPath:    target,
		Content:          []byte(`{"k":"v"}`),
		AddManagedHeader: true,
	}); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	data, _ := os.ReadFile(target)
	if string(data) != `{"k":"v"}` {
		t.Fatalf("non-markdown should not get banner, got %q", string(data))
	}
}

func TestClaudeWriter_Write_NoHeaderWhenDisabled(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "skill.md")

	if _, err := NewClaudeWriter().Write(WriteRequest{
		InstalledPath:    target,
		Content:          []byte("# Hi\n"),
		AddManagedHeader: false,
	}); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(target)
	if string(data) != "# Hi\n" {
		t.Fatalf("disabled header should be no-op, got %q", string(data))
	}
}

func TestClaudeWriter_Write_IdempotentBanner(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "skill.md")

	first, err := NewClaudeWriter().Write(WriteRequest{
		InstalledPath:    target,
		Content:          []byte("# Hi\n"),
		AddManagedHeader: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Re-write with the already-banner-stamped content: header must not double.
	stamped, _ := os.ReadFile(target)
	second, err := NewClaudeWriter().Write(WriteRequest{
		InstalledPath:    target,
		Content:          stamped,
		AddManagedHeader: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.SHA256 != second.SHA256 {
		t.Fatalf("SHA should be stable across re-writes (banner not doubled), got %s vs %s", first.SHA256, second.SHA256)
	}
}

func TestClaudeWriter_Write_RejectsEmptyPath(t *testing.T) {
	if _, err := NewClaudeWriter().Write(WriteRequest{InstalledPath: "  "}); err == nil {
		t.Fatal("Write should reject empty installed_path")
	}
}
