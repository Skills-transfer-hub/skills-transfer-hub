package cmd

import (
	"os"
	"path/filepath"
	"sth/internal/fs"
)

// prepareStateAccess migrates project config, reads the project profile,
// acquires the state lock, and promotes any v1 installed state to v2.
// The caller must invoke the returned release function when done.
func (a *app) prepareStateAccess() (*fs.ProjectProfile, func(), error) {
	cfgPath, err := fs.ProjectProfilePath()
	if err != nil {
		return nil, nil, err
	}
	if abs, absErr := filepath.Abs(cfgPath); absErr == nil {
		a.verbose.Path("project.json", abs)
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

	if abs, absErr := filepath.Abs(filepath.Join(profile.TargetDir, "sth-state-v2.json")); absErr == nil {
		a.verbose.Path("sth-state-v2.json", abs)
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

func (a *app) prepareStateReadAccess() (*fs.ProjectProfile, error) {
	configPath, err := fs.ProjectProfilePath()
	if err != nil {
		return nil, err
	}
	if absolutePath, pathErr := filepath.Abs(configPath); pathErr == nil {
		a.verbose.Path("project.json", absolutePath)
	}

	profile, err := fs.ReadProjectProfile()
	if err != nil {
		return nil, err
	}
	if absolutePath, pathErr := filepath.Abs(filepath.Join(profile.TargetDir, "sth-state-v2.json")); pathErr == nil {
		a.verbose.Path("sth-state-v2.json", absolutePath)
	}
	return profile, nil
}

func (a *app) prepareProviderStateAccess(stateDir, primaryStateDir string) (func(), error) {
	if filepath.Clean(stateDir) == filepath.Clean(primaryStateDir) {
		return func() {}, nil
	}
	release, err := fs.AcquireStateLock(stateDir)
	if err != nil {
		return nil, err
	}
	if err := fs.PromoteInstalledState(stateDir, a.stderr); err != nil {
		release()
		return nil, err
	}
	return release, nil
}
