package cmd

import (
	"context"
	"time"

	"sth/internal/fs"
)

// computeBuddyStateFast returns a buddy state derived only from on-disk
// signals (no network). Used by the launch banner to keep startup snappy.
// Skips the "update available" check, which requires a provider API call.
func (a *app) computeBuddyStateFast() fs.BuddyState {
	profile, err := fs.ReadProjectProfileIfPresent()
	if err != nil {
		return fs.BuddyError
	}
	if profile != nil && fs.DetectInstalling(profile.TargetDir) {
		return fs.BuddyInstalling
	}
	if profile == nil {
		return fs.BuddyNoConfig
	}
	lastOp, _ := fs.ReadLastOperation()
	if lastOp != nil && lastOp.Status == fs.LastOperationStatusError {
		return fs.BuddyError
	}
	if fs.DetectRecentDone(lastOp, time.Now()) {
		return fs.BuddyDone
	}
	return fs.BuddyAllGood
}

// computeBuddyState resolves the current buddy display state.
// Priority: Installing > NoConfig > Error > Done > UpdateAvailable > AllGood.
func (a *app) computeBuddyState() fs.BuddyState {
	profile, err := fs.ReadProjectProfileIfPresent()
	if err != nil {
		return fs.BuddyError
	}

	// Lock file lookup needs targetDir from the profile when it exists.
	if profile != nil && fs.DetectInstalling(profile.TargetDir) {
		return fs.BuddyInstalling
	}

	if profile == nil {
		return fs.BuddyNoConfig
	}

	lastOp, _ := fs.ReadLastOperation()
	if lastOp != nil && lastOp.Status == fs.LastOperationStatusError {
		return fs.BuddyError
	}
	if fs.DetectRecentDone(lastOp, time.Now()) {
		return fs.BuddyDone
	}

	cfg, err := fs.ReadProjectConfigV2()
	if err != nil {
		return fs.BuddyError
	}
	state, err := fs.ReadInstalledStateV2(profile.TargetDir)
	if err != nil {
		return fs.BuddyError
	}

	latestByPath, _ := a.latestVersionsByPath(context.Background(), cfg, state)

	statuses := fs.ComputeAllStatuses(state, cfg, latestByPath, "")
	for _, status := range statuses {
		if status.Status == fs.ResourceStatusOutdated || status.Status == fs.ResourceStatusModifiedLocally {
			return fs.BuddyUpdateAvailable
		}
	}

	return fs.BuddyAllGood
}

func (a *app) runBuddy(args []string) error {
	flags, _ := parseFlags(args, "--watch")
	if flags["--watch"] {
		return a.runBuddyWatch()
	}

	state := a.computeBuddyState()
	a.writeStdout(renderBuddy(state))
	return nil
}
