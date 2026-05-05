package cmd

import (
	"sth/internal/fs"
	"os"
)

// prepareStateAccess migrates project config, reads the project profile,
// acquires the state lock, and promotes any v1 installed state to v2.
// The caller must invoke the returned release function when done.
func (a *app) prepareStateAccess() (*fs.ProjectProfile, func(), error) {
	cfgPath, err := fs.ProjectProfilePath()
	if err != nil {
		return nil, nil, err
	}
	if _, err := os.Stat(cfgPath); err == nil {
		if _, err := fs.MigrateProjectConfig(cfgPath, a.stderr); err != nil {
			return nil, nil, err
		}
	}

	profile, err := fs.ReadProjectProfile()
	if err != nil {
		return nil, nil, err
	}

	release, err := fs.AcquireStateLock(profile.TargetDir)
	if err != nil {
		return nil, nil, err
	}

	if err := fs.PromoteInstalledState(profile.TargetDir, a.stderr); err != nil {
		release()
		return nil, nil, err
	}

	return profile, release, nil
}
