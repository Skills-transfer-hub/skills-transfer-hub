package fs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMatchIgnore(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		path     string
		want     bool
	}{
		{"star matches extension", []string{"*.test.md"}, "guide.test.md", true},
		{"star does not match non-extension", []string{"*.test.md"}, "guide.md", false},
		{"double-star matches nested", []string{"**/*.test.md"}, "sub/dir/guide.test.md", true},
		{"double-star does not match non-extension", []string{"**/*.test.md"}, "sub/dir/guide.md", false},
		{"exact match", []string{"config.yaml"}, "config.yaml", true},
		{"no patterns", []string{}, "anything.md", false},
		{"multiple patterns first matches", []string{"*.log", "*.test.md"}, "app.log", true},
		{"multiple patterns second matches", []string{"*.log", "*.test.md"}, "guide.test.md", true},
		{"multiple patterns none match", []string{"*.log", "*.test.md"}, "guide.md", false},
		{"dir prefix pattern", []string{"vendor/**"}, "vendor/pkg/file.go", true},
		{"dir prefix no match", []string{"vendor/**"}, "src/pkg/file.go", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := MatchIgnore(tc.patterns, tc.path)
			if got != tc.want {
				t.Errorf("MatchIgnore(%v, %q) = %v, want %v", tc.patterns, tc.path, got, tc.want)
			}
		})
	}
}

func TestParseIgnoreFile_absent(t *testing.T) {
	dir := t.TempDir()
	patterns, err := ParseIgnoreFile(dir)
	if err != nil {
		t.Fatalf("ParseIgnoreFile on absent file: %v", err)
	}
	if patterns != nil {
		t.Errorf("expected nil patterns, got %v", patterns)
	}
}

func TestParseIgnoreFile_commentsAndBlanks(t *testing.T) {
	dir := t.TempDir()
	content := "# this is a comment\n\n*.log\n\n# another comment\n*.tmp\n"
	if err := os.WriteFile(filepath.Join(dir, ".sthignore"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	patterns, err := ParseIgnoreFile(dir)
	if err != nil {
		t.Fatalf("ParseIgnoreFile: %v", err)
	}
	if len(patterns) != 2 {
		t.Fatalf("expected 2 patterns, got %d: %v", len(patterns), patterns)
	}
	if patterns[0] != "*.log" {
		t.Errorf("patterns[0] = %q, want %q", patterns[0], "*.log")
	}
	if patterns[1] != "*.tmp" {
		t.Errorf("patterns[1] = %q, want %q", patterns[1], "*.tmp")
	}
}

func TestParseIgnoreFile_matchesWithPatterns(t *testing.T) {
	dir := t.TempDir()
	content := "*.test.md\n"
	if err := os.WriteFile(filepath.Join(dir, ".sthignore"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	patterns, err := ParseIgnoreFile(dir)
	if err != nil {
		t.Fatalf("ParseIgnoreFile: %v", err)
	}

	if !MatchIgnore(patterns, "guide.test.md") {
		t.Error("expected guide.test.md to match *.test.md pattern")
	}
	if MatchIgnore(patterns, "guide.md") {
		t.Error("expected guide.md NOT to match *.test.md pattern")
	}
}
