package fs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLocalLibraryRoundTripUsesPrivateCodeState(t *testing.T) {
	root := t.TempDir()
	item, err := SaveLocalLibraryItem(root, LocalLibrarySaveInput{
		Kind: "prompt", Title: "Review", Body: "Review this diff", Tags: []string{"git", "git"},
	})
	if err != nil {
		t.Fatal(err)
	}
	items, err := ReadLocalLibrary(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != item.ID || len(items[0].Tags) != 1 {
		t.Fatalf("items=%+v", items)
	}
	path := filepath.Join(root, ".sth", "local", "code", "library.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("library mode=%o, want 600", info.Mode().Perm())
	}
}

func TestLocalLibraryRejectsSymlinkedStateDirectory(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".sth"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, ".sth", "local")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := SaveLocalLibraryItem(root, LocalLibrarySaveInput{
		Kind: "artifact", Title: "Unsafe", Body: "must not escape",
	}); err == nil {
		t.Fatal("expected symlinked local state to be rejected")
	}
}
