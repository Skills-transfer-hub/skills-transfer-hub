package fs

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDetectInstalling_ReportsLockPresence(t *testing.T) {
	root := t.TempDir()
	if got := DetectInstalling(root); got {
		t.Fatal("DetectInstalling = true with no lock file, want false")
	}

	lock := filepath.Join(root, lockFileName)
	if err := os.WriteFile(lock, []byte("123"), 0o644); err != nil {
		t.Fatalf("write lock: %v", err)
	}
	if got := DetectInstalling(root); !got {
		t.Fatal("DetectInstalling = false with lock file present, want true")
	}
}

func TestDetectInstalling_EmptyTargetReturnsFalse(t *testing.T) {
	if DetectInstalling("") {
		t.Fatal("DetectInstalling(\"\") = true, want false")
	}
}

func TestDetectRecentDone_TrueWithinWindow(t *testing.T) {
	now := time.Now()
	op := &LastOperation{
		Status:    LastOperationStatusOK,
		Timestamp: now.Add(-1 * time.Second).UTC().Format(time.RFC3339),
	}
	if !DetectRecentDone(op, now) {
		t.Fatal("DetectRecentDone = false within window, want true")
	}
}

func TestDetectRecentDone_FalseAfterWindow(t *testing.T) {
	now := time.Now()
	op := &LastOperation{
		Status:    LastOperationStatusOK,
		Timestamp: now.Add(-(BuddyDoneWindow + time.Second)).UTC().Format(time.RFC3339),
	}
	if DetectRecentDone(op, now) {
		t.Fatal("DetectRecentDone = true after window, want false")
	}
}

func TestDetectRecentDone_FalseOnError(t *testing.T) {
	now := time.Now()
	op := &LastOperation{
		Status:    LastOperationStatusError,
		Timestamp: now.UTC().Format(time.RFC3339),
	}
	if DetectRecentDone(op, now) {
		t.Fatal("DetectRecentDone = true for error, want false")
	}
}

func TestDetectRecentDone_FalseOnNil(t *testing.T) {
	if DetectRecentDone(nil, time.Now()) {
		t.Fatal("DetectRecentDone(nil) = true, want false")
	}
}
