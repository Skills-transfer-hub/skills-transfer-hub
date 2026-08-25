package cmd

import (
	"fmt"
	"io"
	"strings"

	"sth/internal/tui"
)

const installStepCount = 4

type installProgress struct {
	inner         *tui.Progress
	stdout        io.Writer
	stderr        io.Writer
	tty           bool
	total         int
	current       int
	label         string
	started       bool
	suspended     bool
	pendingErrors []string
}

func newInstallProgress(stdout, stderr io.Writer, tty bool) *installProgress {
	return &installProgress{inner: tui.NewProgress(stdout, stderr, tty), stdout: stdout, stderr: stderr, tty: tty}
}

func (progress *installProgress) start(total int) {
	if progress == nil || progress.inner == nil {
		return
	}
	progress.total = total
	progress.started = true
	progress.inner.Start(total)
}

func (progress *installProgress) step(label string) {
	if progress == nil || progress.inner == nil {
		return
	}
	progress.current++
	progress.label = label
	progress.inner.Step(label)
}

func (progress *installProgress) note(label string) {
	if progress == nil || progress.inner == nil {
		return
	}
	progress.label = label
	progress.inner.Note(label)
}

func (progress *installProgress) print(message string) bool {
	if progress == nil || progress.inner == nil {
		return false
	}
	return progress.inner.Print(message)
}

func (progress *installProgress) printError(message string) bool {
	if progress == nil || progress.inner == nil || !progress.started || progress.suspended || !progress.inner.Interactive() {
		return false
	}
	progress.pendingErrors = append(progress.pendingErrors, message)
	progress.inner.Note(strings.TrimSuffix(message, "\n"))
	return true
}

func (progress *installProgress) suspend() {
	if progress == nil || progress.inner == nil || !progress.started || !progress.inner.Interactive() || progress.suspended {
		return
	}
	progress.inner.Stop()
	progress.flushErrors()
	progress.suspended = true
}

func (progress *installProgress) resume() {
	if progress == nil || !progress.suspended {
		return
	}
	progress.inner = tui.NewProgress(progress.stdout, progress.stderr, progress.tty)
	progress.inner.Start(progress.total)
	for i := 0; i < progress.current; i++ {
		progress.inner.Step(progress.label)
	}
	progress.suspended = false
}

func (progress *installProgress) stop() {
	if progress == nil || progress.inner == nil {
		return
	}
	if !progress.started {
		progress.flushErrors()
		return
	}
	if !progress.suspended {
		progress.inner.Stop()
	}
	progress.flushErrors()
}

func (progress *installProgress) complete() {
	if progress == nil || progress.inner == nil {
		return
	}
	if !progress.started {
		progress.flushErrors()
		return
	}
	if !progress.suspended {
		progress.inner.Complete()
	}
	progress.flushErrors()
}

func (progress *installProgress) flushErrors() {
	if progress == nil || len(progress.pendingErrors) == 0 {
		return
	}
	for _, message := range progress.pendingErrors {
		fmt.Fprint(progress.stderr, message)
	}
	progress.pendingErrors = nil
}
