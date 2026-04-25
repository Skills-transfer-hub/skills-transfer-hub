package fs

import (
	"os"
	"path/filepath"
	"time"
)

// BuddyState is the visual state rendered by `sth buddy`.
type BuddyState int

const (
	BuddyAllGood BuddyState = iota
	BuddyUpdateAvailable
	BuddyNoConfig
	BuddyError
	BuddyInstalling
	BuddyDone
)

// BuddyDoneWindow defines how long the "operation just completed" face is
// displayed after a successful operation before the buddy returns to its
// normal state (typically All Good or Update Available).
const BuddyDoneWindow = 5 * time.Second

// DetectInstalling reports whether an install/update is currently in progress.
// It checks for the advisory state lock file inside targetDir.
func DetectInstalling(targetDir string) bool {
	if targetDir == "" {
		return false
	}
	clean := filepath.Clean(targetDir)
	if !filepath.IsAbs(clean) {
		wd, err := os.Getwd()
		if err != nil {
			return false
		}
		clean = filepath.Join(wd, clean)
	}
	_, err := os.Stat(filepath.Join(clean, lockFileName))
	return err == nil
}

// DetectRecentDone reports whether a successful operation was recorded within
// BuddyDoneWindow. Returns false when no record exists or the record is stale.
func DetectRecentDone(op *LastOperation, now time.Time) bool {
	if op == nil || op.Status != LastOperationStatusOK {
		return false
	}
	ts, err := time.Parse(time.RFC3339, op.Timestamp)
	if err != nil {
		return false
	}
	return now.Sub(ts) < BuddyDoneWindow
}
