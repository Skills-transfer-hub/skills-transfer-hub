package cmd

import (
	"bufio"
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

// offerLegacyProjectDirMigration checks for .loreskills and offers to migrate to .sth.
// It is called at the start of every command that may need project config.
// Returns nil (no action) when migration is not needed or user declines.
func (a *app) offerLegacyProjectDirMigration() error {
	if !a.isInteractiveSession() {
		return nil
	}
	hasLegacy, err := fs.HasLegacyProjectDir()
	if err != nil || !hasLegacy {
		return nil
	}
	reader := bufio.NewReader(a.stdin)
	a.writeStdout("Dossier .loreskills detecte. STH utilise maintenant .sth/project.json.")
	migrate, promptErr := promptYesNo(reader, a.stdout, "Migrer la configuration vers .sth ?", true)
	if promptErr != nil || !migrate {
		return nil
	}
	legacyProfile, _ := fs.ReadProjectProfileIfPresentFrom(".loreskills")
	targetDir := ""
	if legacyProfile != nil {
		targetDir = legacyProfile.TargetDir
	}
	return fs.MigrateLegacyProjectDir(targetDir, a.stderr)
}
