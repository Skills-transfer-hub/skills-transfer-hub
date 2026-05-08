package cmd

import (
	"strings"
)

// packageManager describes how `sth` was installed when we can detect it from
// the executable path. When the binary lives outside any known package-manager
// layout (e.g. dropped into /usr/local/bin or %LOCALAPPDATA%) name stays empty
// and the caller treats the install as self-managed.
type packageManager struct {
	name        string // "Homebrew", "Scoop", "Chocolatey", "winget", or "" for unknown
	upgradeHint string // shell-ready command, e.g. "brew upgrade grandpied33/sth/sth"
}

// detectPackageManager returns the package manager that appears to own the
// given executable path. Detection is path-based on purpose: it is the only
// signal available at runtime that survives `chmod +x` and renames.
//
// Path conventions:
//   - macOS Intel Homebrew  : /usr/local/Cellar/sth/<v>/bin/sth
//   - macOS Apple Homebrew  : /opt/homebrew/Cellar/sth/<v>/bin/sth
//   - Linuxbrew             : /home/linuxbrew/.linuxbrew/Cellar/sth/<v>/bin/sth
//   - Scoop                 : %USERPROFILE%\scoop\apps\sth\<v>\sth.exe
//   - Chocolatey (shim)     : %ProgramData%\chocolatey\bin\sth.exe
//   - Chocolatey (real)     : %ProgramData%\chocolatey\lib\sth\tools\sth.exe
//   - winget                : %LOCALAPPDATA%\Microsoft\WinGet\Packages\Grandpied33.STH_*\sth.exe
func detectPackageManager(execPath string) packageManager {
	if strings.TrimSpace(execPath) == "" {
		return packageManager{}
	}
	// Normalize path separators ourselves: filepath.ToSlash is a no-op on
	// Linux (where '\' is a valid filename byte, not a separator), so a
	// Windows path tested under a Linux runner would slip through. We always
	// want substring matches against forward-slash patterns regardless of OS.
	p := strings.ToLower(strings.ReplaceAll(execPath, `\`, "/"))

	switch {
	case strings.Contains(p, "/cellar/sth/"),
		strings.Contains(p, "/homebrew/cellar/"),
		strings.Contains(p, "/linuxbrew/cellar/"):
		return packageManager{
			name:        "Homebrew",
			upgradeHint: "brew upgrade grandpied33/sth/sth",
		}
	case strings.Contains(p, "/scoop/apps/sth/"):
		return packageManager{
			name:        "Scoop",
			upgradeHint: "scoop update sth",
		}
	case strings.Contains(p, "/chocolatey/bin/sth.exe"),
		strings.Contains(p, "/chocolatey/lib/sth/"):
		return packageManager{
			name:        "Chocolatey",
			upgradeHint: "choco upgrade sth",
		}
	case strings.Contains(p, "/winget/packages/grandpied33.sth"):
		return packageManager{
			name:        "winget",
			upgradeHint: "winget upgrade Grandpied33.STH",
		}
	default:
		return packageManager{}
	}
}
