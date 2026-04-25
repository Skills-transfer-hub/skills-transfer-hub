package fs

import (
	"fmt"
	"os"
	"path/filepath"
)

const lockFileName = ".lore-state.lock"

// AcquireStateLock creates an advisory lock file in targetDir.
// It calls os.MkdirAll to ensure targetDir exists before creating the lock.
// Returns a release function that removes the lock file.
// Returns an error immediately if the lock cannot be acquired.
func AcquireStateLock(targetDir string) (func(), error) {
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return nil, fmt.Errorf("create target dir: %w", err)
	}
	lockPath := filepath.Join(filepath.Clean(targetDir), lockFileName)
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return nil, fmt.Errorf("loreskills is already running in this directory; if not, remove: %s", lockPath)
		}
		return nil, fmt.Errorf("acquire lock: %w", err)
	}
	fmt.Fprintf(f, "%d\n", os.Getpid())
	f.Close()
	return func() { os.Remove(lockPath) }, nil
}
