package main

import (
	"os"

	"sth/cmd"
	"sth/internal/timing"
	"sth/internal/tui"
)

func main() {
	timing.Mark("proc-start")
	tui.EnsureQuietTerminal()
	timing.Mark("main-dispatch")
	if err := cmd.Run(os.Args[1:]); err != nil {
		os.Exit(cmd.ExitCode(err))
	}
}
