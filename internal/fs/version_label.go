package fs

import "strings"

var AlphaPhase = true

func DisplayVersion(version string) string {
	trimmed := strings.TrimSpace(version)
	if trimmed == "" {
		return trimmed
	}
	if AlphaPhase {
		return "Alpha " + trimmed
	}
	return trimmed
}
