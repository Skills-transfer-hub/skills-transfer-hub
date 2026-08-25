package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fsnotify/fsnotify"
	"sth/internal/fs"
)

type watcherEventMsg struct{}

type debounceTickMsg struct{}

type networkResultMsg struct {
	state fs.BuddyState
}

type watcherFailureMsg struct {
	err   error
	fatal bool
}

type watchModel struct {
	app             *app
	signals         *watchSignals
	state           fs.BuddyState
	debouncePending bool
	watchErr        error
	watchWarning    error
}

type watchSignals struct {
	watcherEvents   chan struct{}
	networkResults  chan fs.BuddyState
	networkRequests chan struct{}
	watcherFailures chan watcherFailureMsg
	done            <-chan struct{}
}

func newWatchSignals(done <-chan struct{}) *watchSignals {
	return &watchSignals{
		watcherEvents:   make(chan struct{}, 1),
		networkResults:  make(chan fs.BuddyState, 1),
		networkRequests: make(chan struct{}, 1),
		watcherFailures: make(chan watcherFailureMsg, 1),
		done:            done,
	}
}

func newWatchModel(application *app, signals *watchSignals) watchModel {
	return watchModel{app: application, signals: signals, state: application.computeBuddyStateFast()}
}

func (model watchModel) Init() tea.Cmd {
	return tea.Batch(model.signals.waitWatcherEvent(), model.signals.waitNetworkResult(), model.signals.waitWatcherFailure())
}

func (signals *watchSignals) waitWatcherEvent() tea.Cmd {
	return func() tea.Msg {
		select {
		case <-signals.watcherEvents:
			return watcherEventMsg{}
		case <-signals.done:
			return nil
		}
	}
}

func (signals *watchSignals) waitNetworkResult() tea.Cmd {
	return func() tea.Msg {
		select {
		case state := <-signals.networkResults:
			return networkResultMsg{state: state}
		case <-signals.done:
			return nil
		}
	}
}

func (signals *watchSignals) waitWatcherFailure() tea.Cmd {
	return func() tea.Msg {
		select {
		case failure := <-signals.watcherFailures:
			return failure
		case <-signals.done:
			return nil
		}
	}
}

func (signals *watchSignals) requestNetworkRefresh() {
	select {
	case signals.networkRequests <- struct{}{}:
	default:
	}
}

func (signals *watchSignals) sendWatcherEvent() {
	select {
	case signals.watcherEvents <- struct{}{}:
	default:
	}
}

func (signals *watchSignals) sendWatcherFailure(err error) {
	signals.sendWatcherMessage(watcherFailureMsg{err: err, fatal: true})
}

func (signals *watchSignals) sendWatcherWarning(err error) {
	signals.sendWatcherMessage(watcherFailureMsg{err: err})
}

func (signals *watchSignals) sendWatcherMessage(message watcherFailureMsg) {
	select {
	case signals.watcherFailures <- message:
	case <-signals.done:
	default:
	}
}

func (model watchModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return model, tea.Quit
		case tea.KeyRunes:
			if len(msg.Runes) > 0 && msg.Runes[0] == 'q' {
				return model, tea.Quit
			}
		}
	case watcherEventMsg:
		commands := []tea.Cmd{model.signals.waitWatcherEvent()}
		if !model.debouncePending {
			model.debouncePending = true
			commands = append(commands, tea.Tick(150*time.Millisecond, func(time.Time) tea.Msg {
				return debounceTickMsg{}
			}))
		}
		return model, tea.Batch(commands...)
	case debounceTickMsg:
		model.debouncePending = false
		model.state = model.app.computeBuddyStateFast()
		model.signals.requestNetworkRefresh()
		return model, nil
	case networkResultMsg:
		model.state = msg.state
		return model, model.signals.waitNetworkResult()
	case watcherFailureMsg:
		if msg.fatal {
			model.watchErr = msg.err
			return model, tea.Quit
		}
		model.watchWarning = msg.err
		return model, model.signals.waitWatcherFailure()
	}
	return model, nil
}

func (model watchModel) View() string {
	view := renderBuddy(model.state) + "\n"
	if model.watchWarning != nil {
		view += fmt.Sprintf("warning: %v\n", model.watchWarning)
	}
	return view + "q quit"
}

func (a *app) runBuddyWatch() error {
	if !a.isInteractiveSession() {
		return fmt.Errorf("buddy watch requires an interactive terminal")
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("unable to create fsnotify watcher: %w", err)
	}
	defer watcher.Close()

	addRecursive := func(root string) {
		_ = filepath.WalkDir(root, func(path string, dirEntry os.DirEntry, walkErr error) error {
			if walkErr != nil || !dirEntry.IsDir() {
				return nil
			}
			if addErr := watcher.Add(path); addErr != nil {
				fmt.Fprintf(a.stderr, "warning: unable to watch %s: %v\n", path, addErr)
			}
			return nil
		})
	}

	for _, dir := range a.buddyWatchDirs() {
		addRecursive(dir)
	}

	done := make(chan struct{})
	defer close(done)
	signals := newWatchSignals(done)

	go func() {
		for {
			select {
			case <-done:
				return
			case event, ok := <-watcher.Events:
				if !ok {
					signals.sendWatcherFailure(fmt.Errorf("filesystem watcher stopped unexpectedly"))
					return
				}
				if event.Op&fsnotify.Create != 0 {
					if info, statErr := os.Stat(event.Name); statErr == nil && info.IsDir() {
						addRecursive(event.Name)
					}
				}
				signals.sendWatcherEvent()
			case watchErr, ok := <-watcher.Errors:
				if !ok {
					signals.sendWatcherFailure(fmt.Errorf("filesystem watcher error channel closed unexpectedly"))
					return
				}
				signals.sendWatcherWarning(fmt.Errorf("filesystem watcher warning: %w", watchErr))
			}
		}
	}()

	go func() {
		for {
			select {
			case <-done:
				return
			case <-signals.networkRequests:
				state := a.computeBuddyState()
				select {
				case signals.networkResults <- state:
				case <-done:
					return
				}
			}
		}
	}()

	signals.requestNetworkRefresh()
	model := newWatchModel(a, signals)
	options := []tea.ProgramOption{tea.WithOutput(a.stdout), tea.WithAltScreen()}
	if file, ok := a.stdin.(*os.File); ok {
		options = append(options, tea.WithInput(file))
	} else {
		options = append(options, tea.WithInput(nil))
	}
	program := tea.NewProgram(model, options...)
	finalModel, runErr := program.Run()
	if runErr != nil {
		return runErr
	}
	result := finalModel.(watchModel)
	return result.watchErr
}

func (a *app) buddyWatchDirs() []string {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return nil
	}
	candidates := []string{filepath.Join(workingDirectory, ".sth")}
	if profile, err := fs.ReadProjectProfileIfPresent(); err == nil && profile != nil && profile.TargetDir != "" {
		targetDir := profile.TargetDir
		if !filepath.IsAbs(targetDir) {
			targetDir = filepath.Join(workingDirectory, targetDir)
		}
		candidates = append(candidates, targetDir)
	}

	dirs := make([]string, 0, len(candidates))
	seen := make(map[string]struct{})
	for _, candidate := range candidates {
		clean := filepath.Clean(candidate)
		if _, alreadySeen := seen[clean]; alreadySeen {
			continue
		}
		if info, statErr := os.Stat(clean); statErr != nil || !info.IsDir() {
			continue
		}
		seen[clean] = struct{}{}
		dirs = append(dirs, clean)
	}
	return dirs
}
