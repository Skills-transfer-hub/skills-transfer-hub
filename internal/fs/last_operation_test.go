package fs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteLastOperation_NoopWithoutSthDir(t *testing.T) {
	root := t.TempDir()
	withCwd(t, root)

	if err := WriteLastOperation(LastOperation{Op: "install", Status: LastOperationStatusOK}); err != nil {
		t.Fatalf("WriteLastOperation: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, projectConfigDirName, lastOperationFileName)); !os.IsNotExist(err) {
		t.Fatalf("expected no file when .sth missing, got err=%v", err)
	}
}

func TestWriteThenReadLastOperation(t *testing.T) {
	root := t.TempDir()
	withCwd(t, root)

	if err := os.MkdirAll(filepath.Join(root, projectConfigDirName), 0o755); err != nil {
		t.Fatalf("MkdirAll .sth: %v", err)
	}

	want := LastOperation{Op: "install", Status: LastOperationStatusOK, Resource: "alpha"}
	if err := WriteLastOperation(want); err != nil {
		t.Fatalf("WriteLastOperation: %v", err)
	}

	got, err := ReadLastOperation()
	if err != nil {
		t.Fatalf("ReadLastOperation: %v", err)
	}
	if got == nil {
		t.Fatal("ReadLastOperation returned nil, want value")
	}
	if got.Op != want.Op || got.Status != want.Status || got.Resource != want.Resource {
		t.Fatalf("ReadLastOperation = %+v, want fields matching %+v", got, want)
	}
	if got.Timestamp == "" {
		t.Fatal("expected timestamp to be set")
	}
}

func TestReadLastOperation_NilWhenMissing(t *testing.T) {
	root := t.TempDir()
	withCwd(t, root)
	got, err := ReadLastOperation()
	if err != nil {
		t.Fatalf("ReadLastOperation: %v", err)
	}
	if got != nil {
		t.Fatalf("ReadLastOperation = %+v, want nil", got)
	}
}
