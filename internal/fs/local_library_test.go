package fs

import (
	"os"
	"path/filepath"
	"runtime"
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
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
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

func TestLocalLibrarySaveUsesSharedStateLock(t *testing.T) {
	root := t.TempDir()
	directory, err := localCodeStateDirectory(root, true)
	if err != nil {
		t.Fatal(err)
	}
	releaseLock, err := AcquireStateLock(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseLock()

	if _, err := SaveLocalLibraryItem(root, LocalLibrarySaveInput{
		Kind: "prompt", Title: "Concurrent", Body: "must wait",
	}); err == nil {
		t.Fatal("expected the shared state lock to reject a concurrent save")
	}
}

func TestLocalLibraryRejectsInvalidItemsWithoutRewritingThem(t *testing.T) {
	root := t.TempDir()
	directory, err := localCodeStateDirectory(root, true)
	if err != nil {
		t.Fatal(err)
	}
	originalData := []byte(`{"version":1,"items":[{"id":"valid","kind":"prompt","title":"Valid","body":"keep me","tags":[],"createdAt":"","updatedAt":"","source":"local"},{"id":"future","kind":"prompt","title":"Future","body":"do not delete me","tags":[],"createdAt":"","updatedAt":"","source":"local","future":"preserve"}]}`)
	path := filepath.Join(directory, "library.json")
	if err := os.WriteFile(path, originalData, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := ReadLocalLibrary(root); err == nil {
		t.Fatal("expected an invalid item to reject the library")
	}
	if _, err := SaveLocalLibraryItem(root, LocalLibrarySaveInput{
		Kind: "prompt", Title: "New", Body: "must not overwrite",
	}); err == nil {
		t.Fatal("expected save to preserve an invalid library")
	}
	currentData, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(currentData) != string(originalData) {
		t.Fatalf("library changed after rejected save: %q", currentData)
	}
}
