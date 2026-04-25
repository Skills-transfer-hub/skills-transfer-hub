package fs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicWriteFile_NewFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "out.json")

	data := []byte(`{"hello":"world"}`)
	if err := atomicWriteFile(target, data, 0o644); err != nil {
		t.Fatalf("atomicWriteFile returned error: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("content mismatch: got %q, want %q", got, data)
	}
}

func TestAtomicWriteFile_OverwriteExisting(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "state.json")

	original := []byte(`{"v":1}`)
	if err := os.WriteFile(target, original, 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	updated := []byte(`{"v":2}`)
	if err := atomicWriteFile(target, updated, 0o644); err != nil {
		t.Fatalf("atomicWriteFile returned error: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if string(got) != string(updated) {
		t.Errorf("content mismatch: got %q, want %q", got, updated)
	}
}

func TestAtomicWriteFile_NoTempFileLeftOnSuccess(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "clean.json")

	if err := atomicWriteFile(target, []byte(`{}`), 0o644); err != nil {
		t.Fatalf("atomicWriteFile returned error: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir returned error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly 1 file in dir, got %d", len(entries))
	}
	for _, e := range entries {
		if e.Name() != "clean.json" {
			t.Errorf("unexpected file left in dir: %s", e.Name())
		}
	}
}

func TestAtomicWriteFile_FailsOnNonExistentDir(t *testing.T) {
	target := filepath.Join(t.TempDir(), "nonexistent", "out.json")
	err := atomicWriteFile(target, []byte(`{}`), 0o644)
	if err == nil {
		t.Error("expected non-nil error for non-existent parent directory, got nil")
	}
}
