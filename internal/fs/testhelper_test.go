package fs

import (
	"os"
	"testing"
)

// withCwd swaps the working directory for the duration of a test.
// Used by tests that exercise functions relying on os.Getwd().
func withCwd(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir(%s): %v", dir, err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(prev)
	})
}
