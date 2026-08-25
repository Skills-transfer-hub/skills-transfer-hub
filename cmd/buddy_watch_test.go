package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"sth/internal/fs"
)

func TestWatchSignalsAreIsolated(t *testing.T) {
	firstDone := make(chan struct{})
	secondDone := make(chan struct{})
	firstSignals := newWatchSignals(firstDone)
	secondSignals := newWatchSignals(secondDone)

	firstSignals.sendWatcherEvent()

	select {
	case <-firstSignals.watcherEvents:
	default:
		t.Fatal("first watcher did not receive its event")
	}
	select {
	case <-secondSignals.watcherEvents:
		t.Fatal("second watcher received an event from the first watcher")
	default:
	}

	close(firstDone)
	close(secondDone)
}

func TestWatchSignalsCoalesceNetworkRequests(t *testing.T) {
	done := make(chan struct{})
	signals := newWatchSignals(done)

	signals.requestNetworkRefresh()
	signals.requestNetworkRefresh()

	if requestCount := len(signals.networkRequests); requestCount != 1 {
		t.Fatalf("network request count = %d, want 1", requestCount)
	}
	close(done)
}

func TestWatchModelAppliesNetworkResultAndRearmsWait(t *testing.T) {
	done := make(chan struct{})
	signals := newWatchSignals(done)
	model := watchModel{app: &app{}, signals: signals, state: fs.BuddyAllGood}

	updatedModel, command := model.Update(networkResultMsg{state: fs.BuddyUpdateAvailable})
	updatedWatchModel := updatedModel.(watchModel)
	if updatedWatchModel.state != fs.BuddyUpdateAvailable {
		t.Fatalf("state = %v, want %v", updatedWatchModel.state, fs.BuddyUpdateAvailable)
	}
	if command == nil {
		t.Fatal("network result did not rearm the network wait")
	}

	close(done)
	if message := command(); message != nil {
		t.Fatalf("wait command returned %T after shutdown", message)
	}
}

func TestBuddyWatchRejectsRedirectedOutput(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	application := newApp(strings.NewReader(""), &stdout, &stderr, nil)
	err := application.runBuddyWatch()
	if err == nil || err.Error() != "buddy watch requires an interactive terminal" {
		t.Fatalf("runBuddyWatch error = %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestWatchModelQuitsOnWatcherFailure(t *testing.T) {
	done := make(chan struct{})
	signals := newWatchSignals(done)
	model := watchModel{app: &app{}, signals: signals, state: fs.BuddyAllGood}
	watchErr := fmt.Errorf("watch failed")

	updatedModel, command := model.Update(watcherFailureMsg{err: watchErr, fatal: true})
	updatedWatchModel := updatedModel.(watchModel)
	if !errors.Is(updatedWatchModel.watchErr, watchErr) {
		t.Fatalf("watch error = %v", updatedWatchModel.watchErr)
	}
	if command == nil {
		t.Fatal("watcher failure did not quit")
	}
	close(done)
}

func TestWatchModelKeepsListeningAfterWatcherWarning(t *testing.T) {
	done := make(chan struct{})
	signals := newWatchSignals(done)
	model := watchModel{app: &app{}, signals: signals, state: fs.BuddyAllGood}
	warningErr := fmt.Errorf("watch queue overflow")

	updatedModel, command := model.Update(watcherFailureMsg{err: warningErr})
	updatedWatchModel := updatedModel.(watchModel)
	if updatedWatchModel.watchErr != nil || !errors.Is(updatedWatchModel.watchWarning, warningErr) {
		t.Fatalf("watch warning = %v, fatal error = %v", updatedWatchModel.watchWarning, updatedWatchModel.watchErr)
	}
	if command == nil {
		t.Fatal("watcher warning did not rearm the error listener")
	}
	if !strings.Contains(updatedWatchModel.View(), "watch queue overflow") {
		t.Fatalf("warning is not visible: %q", updatedWatchModel.View())
	}

	signals.sendWatcherFailure(fmt.Errorf("watcher closed"))
	message := command()
	failure, ok := message.(watcherFailureMsg)
	if !ok || !failure.fatal {
		t.Fatalf("rearmed listener returned %#v", message)
	}
	close(done)
}
