package fs

import (
	"fmt"
	"os"
	"syscall"
	"time"
)

func lockGlobalFile(file *os.File) error {
	deadline := time.Now().Add(10 * time.Second)
	for {
		err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return nil
		}
		if err != syscall.EWOULDBLOCK || time.Now().After(deadline) {
			return fmt.Errorf("acquire global lock: %w", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func unlockGlobalFile(file *os.File) {
	syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
}

func syncGlobalDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
