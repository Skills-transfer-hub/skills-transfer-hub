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

	// addRecursive registers root and all its subdirectories with the watcher.
	// fsnotify doesn't recurse on its own, so skills nested under
	// <target_dir>/<category>/<folder>/ would otherwise be missed.
	addRecursive := func(root string) {
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return nil
			}
			if e := watcher.Add(path); e != nil {
				fmt.Fprintf(a.stderr, "avertissement: surveillance de %s ignorée : %v\n", path, e)
			}
			return nil
		})
	}

	for _, dir := range a.buddyWatchDirs() {
		addRecursive(dir)
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

	// networkStateCh delivers the result of a background computeBuddyState call
	// (which involves network I/O) so the UI is never blocked waiting for it.
	done := make(chan struct{})
	defer close(done)
	networkRefreshReq := make(chan struct{}, 1)
	networkStateCh := make(chan fs.BuddyState, 1)
	go func() {
		for {
			select {
			case <-done:
				return
			case <-networkRefreshReq:
				s := a.computeBuddyState()
				select {
				case networkStateCh <- s:
				case <-done:
					return
				}
			}
		}
	}()

	triggerNetworkRefresh := func() {
		select {
		case networkRefreshReq <- struct{}{}:
		default:
		}
	}

	// render draws immediately using the fast (disk-only) state, then kicks off
	// an async network refresh whose result will arrive on networkStateCh.
	render := func() {
		state := a.computeBuddyStateFast()
		fmt.Fprint(a.stdout, ansiCursorHome+renderBuddy(state)+"\n"+ansiClearToEnd)
		triggerNetworkRefresh()
	}
	render()

	debounce := time.NewTimer(time.Hour)
	debounce.Stop()
	pending := false

	for {
		select {
		case <-quit:
			return nil
		case state := <-networkStateCh:
			fmt.Fprint(a.stdout, ansiCursorHome+renderBuddy(state)+"\n"+ansiClearToEnd)
		case event, ok := <-watcher.Events:
			if !ok {
				return errors.New("watcher fsnotify fermé")
			}
			// Dynamically watch newly created subdirectories so installs that
			// create <category>/<folder>/ trees are picked up without restart.
			if event.Op&fsnotify.Create != 0 {
				if info, statErr := os.Stat(event.Name); statErr == nil && info.IsDir() {
					addRecursive(event.Name)
				}
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
