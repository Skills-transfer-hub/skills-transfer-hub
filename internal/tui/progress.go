package tui

import (
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbletea"
)

type progressMsg struct {
	step      int
	total     int
	label     string
	finished  bool
	succeeded bool
}

type progressModel struct {
	spinner   spinner.Model
	step      int
	total     int
	label     string
	finished  bool
	succeeded bool
}

func newProgressModel() progressModel {
	return progressModel{spinner: spinner.New(spinner.WithSpinner(spinner.Dot))}
}

func (model progressModel) Init() tea.Cmd {
	return model.spinner.Tick
}

func (model progressModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		model.spinner, cmd = model.spinner.Update(msg)
		return model, cmd
	case progressMsg:
		model.step = msg.step
		model.total = msg.total
		model.label = msg.label
		model.finished = msg.finished
		model.succeeded = msg.succeeded
		if model.finished {
			return model, tea.Quit
		}
		return model, nil
	default:
		return model, nil
	}
}

func (model progressModel) View() string {
	marker := DimStyle.Render("•")
	if model.finished && model.succeeded {
		marker = SuccessStyle.Render("✔")
	} else if !model.finished {
		marker = AccentStyle.Render(model.spinner.View())
	}
	status := model.label
	if model.total > 0 {
		status = fmt.Sprintf("%s   %s", model.label, DimStyle.Render(fmt.Sprintf("(step %d of %d)", model.step, model.total)))
	}
	lines := []string{
		"    " + marker,
		"  ╭─────╮",
		"  │ O O │",
		"  ╰─────╯",
		"  " + status,
	}
	return strings.Join(lines, "\n")
}

type Progress struct {
	stdout io.Writer
	stderr io.Writer
	tty    bool

	mutex   sync.Mutex
	program *tea.Program
	total   int
	current int
	label   string
	done    chan struct{}
	stopped sync.Once
}

func NewProgress(stdout, stderr io.Writer, tty bool) *Progress {
	return &Progress{stdout: stdout, stderr: stderr, tty: tty}
}

func (progress *Progress) Start(total int) {
	if progress == nil {
		return
	}
	progress.mutex.Lock()
	progress.total = total
	progress.mutex.Unlock()
	if !progress.tty {
		return
	}
	model := newProgressModel()
	program := tea.NewProgram(model, tea.WithOutput(progress.stdout), tea.WithInput(nil), tea.WithoutSignalHandler())
	progress.program = program
	progress.done = make(chan struct{})
	go func() {
		defer close(progress.done)
		if _, err := program.Run(); err != nil {
			fmt.Fprintln(progress.stderr, "progress display unavailable:", err)
		}
	}()
}

func (progress *Progress) Step(label string) {
	if progress == nil {
		return
	}
	progress.mutex.Lock()
	progress.current++
	step := progress.current
	total := progress.total
	progress.label = label
	progress.mutex.Unlock()
	if !progress.tty {
		if total > 0 {
			fmt.Fprintf(progress.stderr, "→ [%d/%d] %s\n", step, total, label)
		} else {
			fmt.Fprintf(progress.stderr, "→ %s\n", label)
		}
		return
	}
	if progress.program != nil {
		progress.program.Send(progressMsg{step: step, total: total, label: label})
	}
}

func (progress *Progress) Note(label string) {
	if progress == nil {
		return
	}
	progress.mutex.Lock()
	progress.label = label
	step := progress.current
	total := progress.total
	progress.mutex.Unlock()
	if !progress.tty {
		fmt.Fprintf(progress.stderr, "→ %s\n", label)
		return
	}
	if progress.program != nil {
		progress.program.Send(progressMsg{step: step, total: total, label: label})
	}
}

func (progress *Progress) Print(message string) bool {
	if progress == nil || !progress.tty {
		return false
	}
	progress.mutex.Lock()
	program := progress.program
	progress.mutex.Unlock()
	if program == nil {
		return false
	}
	printCommand := tea.Printf("%s", strings.TrimSuffix(message, "\n"))
	program.Send(printCommand())
	return true
}

func (progress *Progress) Interactive() bool {
	return progress != nil && progress.tty
}

func (progress *Progress) Stop() {
	progress.finish(false)
}

func (progress *Progress) Complete() {
	progress.finish(true)
}

func (progress *Progress) finish(succeeded bool) {
	if progress == nil || !progress.tty {
		return
	}
	progress.stopped.Do(func() {
		if progress.program != nil {
			progress.mutex.Lock()
			message := progressMsg{step: progress.current, total: progress.total, label: progress.label, finished: true, succeeded: succeeded}
			progress.mutex.Unlock()
			progress.program.Send(message)
		}
		if progress.done != nil {
			<-progress.done
		}
		fmt.Fprint(progress.stdout, ansiShowCursor)
	})
}
