package fs

import (
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

func lockGlobalFile(file *os.File) error {
	deadline := time.Now().Add(10 * time.Second)
	for {
		err := windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, new(windows.Overlapped))
		if err == nil {
			return nil
		}
		if err != windows.ERROR_LOCK_VIOLATION || time.Now().After(deadline) {
			return fmt.Errorf("acquire global lock: %w", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func unlockGlobalFile(file *os.File) {
	windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, new(windows.Overlapped))
}

func syncGlobalDirectory(path string) error {
	return nil
}
