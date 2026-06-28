package cmd

import (
	"strings"
)

type packageManager struct {
	name        string
	upgradeHint string
}

func detectPackageManager(execPath string) packageManager {
	if strings.TrimSpace(execPath) == "" {
		return packageManager{}
	}
	normalizedPath := strings.ToLower(strings.ReplaceAll(execPath, `\`, "/"))

	switch {
	case strings.Contains(normalizedPath, "/cellar/sth/"),
		strings.Contains(normalizedPath, "/homebrew/cellar/"),
		strings.Contains(normalizedPath, "/linuxbrew/cellar/"):
		return packageManager{
			name:        "Homebrew",
			upgradeHint: "brew upgrade skills-transfer-hub/sth/sth",
		}
	case strings.Contains(normalizedPath, "/scoop/apps/sth/"):
		return packageManager{
			name:        "Scoop",
			upgradeHint: "scoop update sth",
		}
	case strings.Contains(normalizedPath, "/chocolatey/bin/sth.exe"),
		strings.Contains(normalizedPath, "/chocolatey/lib/sth/"):
		return packageManager{
			name:        "Chocolatey",
			upgradeHint: "choco upgrade sth",
		}
	case strings.Contains(normalizedPath, "/winget/packages/sth.sth"):
		return packageManager{
			name:        "winget",
			upgradeHint: "winget upgrade STH.STH",
		}
	default:
		return packageManager{}
	}
}
