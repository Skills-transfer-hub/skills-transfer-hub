package cmd

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

var installLoaderFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

const (
	installLoaderInterval = 120 * time.Millisecond
	installStepCount      = 4
)

type installProgress struct {
	stdout io.Writer
	stderr io.Writer
	tty    bool

	mutex    sync.Mutex
	total    int
	current  int
	label    string
	frameIdx int

	frameHeight int
	ticker      *time.Ticker
	done        chan struct{}
	finished    sync.WaitGroup
	stopOnce    sync.Once
}

func newInstallProgress(stdout, stderr io.Writer, tty bool) *installProgress {
	return &installProgress{stdout: stdout, stderr: stderr, tty: tty}
}

func (p *installProgress) start(total int) {
	if p == nil {
		return
	}
	p.mutex.Lock()
	p.total = total
	frame := p.renderFrame()
	p.mutex.Unlock()
	if !p.tty {
		return
	}
	fmt.Fprint(p.stdout, ansiHideCursor)
	p.frameHeight = strings.Count(frame, "\n")
	fmt.Fprint(p.stdout, frame)
	p.ticker = time.NewTicker(installLoaderInterval)
	p.done = make(chan struct{})
	p.finished.Add(1)
	go p.animate()
}

func (p *installProgress) step(label string) {
	if p == nil {
		return
	}
	p.mutex.Lock()
	p.current++
	p.label = label
	current := p.current
	total := p.total
	p.mutex.Unlock()
	if !p.tty {
		fmt.Fprintf(p.stderr, "→ [%d/%d] %s\n", current, total, label)
	}
}

func (p *installProgress) note(label string) {
	if p == nil {
		return
	}
	p.mutex.Lock()
	p.label = label
	p.mutex.Unlock()
	if !p.tty {
		fmt.Fprintf(p.stderr, "→ %s\n", label)
	}
}

func (p *installProgress) stop() {
	if p == nil || !p.tty {
		return
	}
	p.stopOnce.Do(func() {
		close(p.done)
		p.ticker.Stop()
		p.finished.Wait()
		fmt.Fprintf(p.stdout, "\x1b[%dA\r%s", p.frameHeight, ansiClearToEnd)
		fmt.Fprint(p.stdout, ansiShowCursor)
	})
}

func (p *installProgress) animate() {
	defer p.finished.Done()
	for {
		select {
		case <-p.done:
			return
		case <-p.ticker.C:
			p.mutex.Lock()
			p.frameIdx = (p.frameIdx + 1) % len(installLoaderFrames)
			frame := p.renderFrame()
			p.mutex.Unlock()
			fmt.Fprintf(p.stdout, "\x1b[%dA\r", p.frameHeight)
			fmt.Fprint(p.stdout, frame)
		}
	}
}

func (p *installProgress) renderFrame() string {
	loader := installLoaderFrames[p.frameIdx]
	status := p.label
	if p.total > 0 {
		status = fmt.Sprintf("%s   (étape %d sur %d)", p.label, p.current, p.total)
	}
	lines := []string{
		"    " + loader,
		"  ╭─────╮",
		"  │ O O │",
		"  ╰─────╯",
		"  " + status,
	}
	var builder strings.Builder
	for _, line := range lines {
		builder.WriteString(line)
		builder.WriteString(ansiClearToEnd)
		builder.WriteString("\n")
	}
	return builder.String()
}
