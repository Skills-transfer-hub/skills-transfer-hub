package cmd

import (
	"fmt"
	"runtime"
	"strings"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
	target    = runtime.GOOS + "/" + runtime.GOARCH
)

func formatVersionOutput() string {
	lines := []string{
		fmt.Sprintf("version: %s", valueOrDefault(version, "dev")),
		fmt.Sprintf("commit: %s", valueOrDefault(commit, "unknown")),
		fmt.Sprintf("build_date: %s", valueOrDefault(buildDate, "unknown")),
		fmt.Sprintf("target: %s", valueOrDefault(target, runtime.GOOS+"/"+runtime.GOARCH)),
	}

	return strings.Join(lines, "\n")
}

func (a *app) runVersion() error {
	a.writeGuidedStdout(formatVersionOutput())
	return nil
}

func valueOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}

	return value
}