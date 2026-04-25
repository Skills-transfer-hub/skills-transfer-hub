package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"sth/internal/fs"
)

const (
	ansiHideCursor = "\x1b[?25l"
	ansiShowCursor = "\x1b[?25h"
	ansiCursorHome = "\x1b[H"
	ansiClearAll   = "\x1b[2J"
	ansiClearToEnd = "\x1b[J"
)

// runBuddyWatch takes over the terminal and redraws the buddy whenever the
// watched directories change. Exits on Ctrl+C or when the user types "q\n".
func (a *app) runBuddyWatch() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("création du watcher fsnotify échouée : %w", err)
	}
	defer watcher.Close()

	for _, dir := range a.buddyWatchDirs() {
		if err := watcher.Add(dir); err != nil {
			fmt.Fprintf(a.stderr, "avertissement: surveillance de %s ignorée : %v\n", dir, err)
		}
	}

	fmt.Fprint(a.stdout, ansiHideCursor+ansiClearAll+ansiCursorHome)
	defer fmt.Fprint(a.stdout, ansiShowCursor+"\n")

	quit := make(chan struct{}, 1)
	requestQuit := func() {
		select {
		case quit <- struct{}{}:
		default:
		}
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	defer signal.Stop(sigCh)
	go func() {
		<-sigCh
		requestQuit()
	}()

	stdinDone := make(chan struct{})
	go func() {
		defer close(stdinDone)
		reader := bufio.NewReader(a.stdin)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			if strings.TrimSpace(strings.ToLower(line)) == "q" {
				requestQuit()
				return
			}
		}
	}()

	render := func() {
		state := a.computeBuddyState()
		fmt.Fprint(a.stdout, ansiCursorHome+renderBuddy(state)+"\n"+ansiClearToEnd)
	}
	render()

	debounce := time.NewTimer(time.Hour)
	debounce.Stop()
	pending := false

	for {
		select {
		case <-quit:
			return nil
		case _, ok := <-watcher.Events:
			if !ok {
				return errors.New("watcher fsnotify fermé")
			}
			if !pending {
				debounce.Reset(150 * time.Millisecond)
				pending = true
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return errors.New("watcher fsnotify fermé")
			}
			fmt.Fprintf(a.stderr, "avertissement: erreur fsnotify : %v\n", err)
		case <-debounce.C:
			pending = false
			render()
		}
	}
}

// buddyWatchDirs returns the directories the watch loop should subscribe to.
// Missing directories are silently skipped.
func (a *app) buddyWatchDirs() []string {
	wd, err := os.Getwd()
	if err != nil {
		return nil
	}
	candidates := []string{filepath.Join(wd, ".sth")}
	if profile, err := fs.ReadProjectProfileIfPresent(); err == nil && profile != nil && profile.TargetDir != "" {
		td := profile.TargetDir
		if !filepath.IsAbs(td) {
			td = filepath.Join(wd, td)
		}
		candidates = append(candidates, td)
	}

	dirs := make([]string, 0, len(candidates))
	seen := make(map[string]struct{})
	for _, d := range candidates {
		clean := filepath.Clean(d)
		if _, ok := seen[clean]; ok {
			continue
		}
		if info, err := os.Stat(clean); err != nil || !info.IsDir() {
			continue
		}
		seen[clean] = struct{}{}
		dirs = append(dirs, clean)
	}
	return dirs
}
