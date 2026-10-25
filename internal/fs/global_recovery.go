package fs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"unicode/utf8"
)

type globalJournal struct {
	SchemaVersion int             `json:"schema_version"`
	Changes       []globalChange  `json:"changes"`
	StateBefore   *globalSnapshot `json:"state_before"`
	StateAfter    *globalSnapshot `json:"state_after"`
}

func RecoverGlobalTransactions(context GlobalContext) error {
	pending, err := pendingGlobalTransactions(context)
	if err != nil || len(pending) == 0 {
		return err
	}
	release, err := acquireGlobalLock(context)
	if err != nil {
		return err
	}
	defer release()
	return recoverGlobalTransactionsLocked(context)
}

func pendingGlobalTransactions(context GlobalContext) ([]string, error) {
	root := filepath.Join(context.StateRoot, "transactions")
	if err := validateGlobalAbsolutePath(root); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	paths := []string{}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || validateGlobalRelativePath(entry.Name()) != nil {
			return nil, fmt.Errorf("invalid global transaction entry: %s", filepath.Join(root, entry.Name()))
		}
		paths = append(paths, filepath.Join(root, entry.Name(), "journal.json"))
	}
	return paths, nil
}

func recoverGlobalTransactionsLocked(context GlobalContext) error {
	paths, err := pendingGlobalTransactions(context)
	if err != nil {
		return err
	}
	for _, journalPath := range paths {
		if err := recoverGlobalJournal(context, journalPath); err != nil {
			return fmt.Errorf("global recovery blocked; journal preserved at %s: %w", journalPath, err)
		}
	}
	return nil
}

func saveGlobalJournal(context GlobalContext, plan globalChangePlan) (string, error) {
	root := filepath.Join(context.StateRoot, "transactions")
	if err := validateGlobalAbsolutePath(root); err != nil {
		return "", err
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return "", err
	}
	directory, err := os.MkdirTemp(root, "transaction-")
	if err != nil {
		return "", err
	}
	journal := globalJournal{SchemaVersion: 1, Changes: plan.Changes, StateBefore: plan.StateBefore, StateAfter: plan.StateAfter}
	content, err := json.Marshal(journal)
	if err != nil {
		return "", err
	}
	journalPath := filepath.Join(directory, "journal.json")
	if err := writeGlobalSnapshot(journalPath, &globalSnapshot{Data: content, Mode: 0600}); err != nil {
		return "", err
	}
	if err := syncGlobalDirectory(root); err != nil {
		return "", err
	}
	return journalPath, nil
}

func recoverGlobalJournal(context GlobalContext, journalPath string) error {
	if err := validateGlobalAbsolutePath(journalPath); err != nil {
		return err
	}
	content, err := os.ReadFile(journalPath)
	if os.IsNotExist(err) {
		entries, readErr := os.ReadDir(filepath.Dir(journalPath))
		if readErr == nil && len(entries) == 0 {
			return os.Remove(filepath.Dir(journalPath))
		}
	}
	if err != nil {
		return err
	}
	if !utf8.Valid(content) {
		return fmt.Errorf("invalid UTF-8 in global transaction journal")
	}
	var journal globalJournal
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&journal); err != nil {
		return fmt.Errorf("invalid global transaction journal: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("invalid trailing data in global transaction journal")
	}
	if err := validateGlobalJournal(context, journal); err != nil {
		return err
	}
	statePath := filepath.Join(context.StateRoot, "state.json")
	if err := globalSnapshotHasNotDiverged(statePath, journal.StateBefore, journal.StateAfter); err != nil {
		return err
	}
	for _, change := range journal.Changes {
		target, err := GlobalFilePath(context, change.RootID, change.RelativePath)
		if err != nil {
			return err
		}
		if err := globalSnapshotHasNotDiverged(target, change.Before, change.After); err != nil {
			return err
		}
	}
	if err := restoreGlobalChanges(context, journal.Changes); err != nil {
		return err
	}
	if err := writeGlobalSnapshot(statePath, journal.StateBefore); err != nil {
		return err
	}
	return removeGlobalJournal(journalPath)
}

func validateGlobalJournal(context GlobalContext, journal globalJournal) error {
	if journal.SchemaVersion != 1 || journal.StateAfter == nil {
		return fmt.Errorf("invalid global transaction journal version or state")
	}
	before, err := globalJournalState(journal.StateBefore)
	if err != nil {
		return err
	}
	after, err := globalJournalState(journal.StateAfter)
	if err != nil {
		return err
	}
	oldFiles, err := validateGlobalState(context, before)
	if err != nil {
		return err
	}
	newFiles, err := validateGlobalState(context, after)
	if err != nil {
		return err
	}
	seen := make(map[string]bool)
	for _, change := range journal.Changes {
		target, err := GlobalFilePath(context, change.RootID, change.RelativePath)
		if err != nil {
			return err
		}
		key := globalPathKey(target)
		if seen[key] {
			return fmt.Errorf("duplicate global journal path: %s", target)
		}
		seen[key] = true
		oldFile, oldOwned := oldFiles[key]
		newFile, newOwned := newFiles[key]
		if !oldOwned && !newOwned || oldOwned && newOwned && oldFile.Owner != newFile.Owner {
			return fmt.Errorf("global journal path lacks unchanged ownership: %s", target)
		}
		if change.Before != nil && !oldOwned {
			return fmt.Errorf("global journal cannot restore unowned file: %s", target)
		}
		if change.After != nil && (!newOwned || ComputeSHA256(change.After.Data) != newFile.File.SHA256 || change.After.Mode != newFile.File.Mode) {
			return fmt.Errorf("global journal content does not match state: %s", target)
		}
		if change.After == nil && newOwned {
			return fmt.Errorf("global journal deletion conflicts with state: %s", target)
		}
		for _, snapshot := range []*globalSnapshot{change.Before, change.After} {
			if snapshot != nil && snapshot.Mode&^0777 != 0 {
				return fmt.Errorf("invalid mode in global journal: %s", target)
			}
		}
	}
	return nil
}

func globalJournalState(snapshot *globalSnapshot) (GlobalState, error) {
	if snapshot == nil {
		return GlobalState{SchemaVersion: 1, Installations: []GlobalInstallation{}}, nil
	}
	if !utf8.Valid(snapshot.Data) {
		return GlobalState{}, fmt.Errorf("invalid UTF-8 in global state snapshot")
	}
	var state GlobalState
	decoder := json.NewDecoder(bytes.NewReader(snapshot.Data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return GlobalState{}, fmt.Errorf("invalid global state snapshot: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return GlobalState{}, fmt.Errorf("invalid trailing data in global state snapshot")
	}
	if snapshot.Mode&^0777 != 0 {
		return GlobalState{}, fmt.Errorf("invalid global state snapshot mode")
	}
	return state, nil
}

func globalSnapshotHasNotDiverged(target string, before, after *globalSnapshot) error {
	current, err := readGlobalSnapshot(target)
	if err != nil {
		return err
	}
	if !globalSnapshotsEqual(current, before) && !globalSnapshotsEqual(current, after) {
		return fmt.Errorf("path diverged since global transaction interruption: %s", target)
	}
	return nil
}

func removeGlobalJournal(journalPath string) error {
	if err := validateGlobalAbsolutePath(journalPath); err != nil {
		return err
	}
	if err := os.Remove(journalPath); err != nil {
		return err
	}
	if err := os.Remove(filepath.Dir(journalPath)); err != nil {
		return err
	}
	return syncGlobalDirectory(filepath.Dir(filepath.Dir(journalPath)))
}
