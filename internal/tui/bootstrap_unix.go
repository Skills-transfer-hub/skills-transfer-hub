//go:build unix

package tui

import (
	"os"
	"syscall"

	"golang.org/x/term"
	"sth/internal/timing"
)

const passthroughFlag = "STH_TERM_PASSTHROUGH"

func EnsureQuietTerminal() {
	defer timing.Mark("quiet-terminal-done")
	if os.Getenv(passthroughFlag) != "" || os.Getenv("TERM") == "dumb" {
		return
	}
	if !isTerminalFile(os.Stdout) || !term.IsTerminal(int(os.Stdin.Fd())) {
		return
	}
	executablePath, err := os.Executable()
	if err != nil {
		return
	}
	os.Setenv(passthroughFlag, "1")
	os.Setenv("TERM", "dumb")
	_ = syscall.Exec(executablePath, os.Args, os.Environ())
}
