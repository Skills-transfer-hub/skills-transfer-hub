package cmd

import "os"

// Kept for backward compatibility with the Alpha provider opt-in. It no
// longer gates normal CLI execution: local commands must remain usable with an
// outdated release, without an account and without network connectivity.
const alphaGateSkipEnvVar = "STH_ALPHA_GATE"

// osExecutable is package-level so tests can stub it without touching the
// global os.Executable function.
var osExecutable = os.Executable

// upgradeCommand returns the explicit update command shown by opt-in update
// surfaces. Update discovery is never a prerequisite for local CLI use.
func upgradeCommand() string {
	if execPath, err := osExecutable(); err == nil {
		if pm := detectPackageManager(execPath); pm.name != "" {
			return pm.upgradeHint
		}
	}
	return "sth update self"
}
