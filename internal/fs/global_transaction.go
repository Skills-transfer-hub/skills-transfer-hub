package fs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

type globalSnapshot struct {
	Data []byte `json:"data"`
	Mode uint32 `json:"mode"`
}

type globalChange struct {
	RootID       string          `json:"root_id"`
	RelativePath string          `json:"relative_path"`
	Before       *globalSnapshot `json:"before"`
	After        *globalSnapshot `json:"after"`
}

type globalChangePlan struct {
	Changes     []globalChange
	Backups     []globalChange
	StateBefore *globalSnapshot
	StateAfter  *globalSnapshot
}

var globalMutationHook func(stage, target string) error

func ApplyGlobalChange(context GlobalContext, before, after GlobalState, writes []GlobalWrite, options GlobalMutationOptions) (GlobalMutationResult, error) {
	pending, err := pendingGlobalTransactions(context)
	if err != nil {
		return GlobalMutationResult{}, err
	}
	if len(pending) > 0 && options.DryRun {
		return GlobalMutationResult{}, fmt.Errorf("global transaction recovery required before dry-run: %s", strings.Join(pending, ", "))
	}
	result := GlobalMutationResult{Warnings: []string{}}
	if len(pending) == 0 {
		plan, err := planGlobalChange(context, before, after, writes, options.Force)
		if err != nil {
			return result, err
		}
		result.Changed = len(plan.Changes) > 0 || !globalSnapshotsEqual(plan.StateBefore, plan.StateAfter)
		if options.DryRun || !result.Changed {
			return result, nil
		}
	}
	release, err := acquireGlobalLock(context)
	if err != nil {
		return result, err
	}
	defer release()
	if err := recoverGlobalTransactionsLocked(context); err != nil {
		return result, err
	}
	plan, err := planGlobalChange(context, before, after, writes, options.Force)
	if err != nil {
		return result, err
	}
	result.Changed = len(plan.Changes) > 0 || !globalSnapshotsEqual(plan.StateBefore, plan.StateAfter)
	if len(plan.Backups) > 0 {
		result.BackupPath, err = preserveGlobalBackups(context, plan.Backups)
		if err != nil {
			return result, err
		}
	}
	journalPath, err := saveGlobalJournal(context, plan)
	if err != nil {
		return result, err
	}
	for _, change := range plan.Changes {
		target, err := GlobalFilePath(context, change.RootID, change.RelativePath)
		if err == nil && globalMutationHook != nil {
			err = globalMutationHook("before-write", target)
		}
		if err == nil {
			current, readErr := readGlobalSnapshot(target)
			if readErr != nil {
				err = readErr
			} else if !globalSnapshotsEqual(current, change.Before) {
				err = fmt.Errorf("global destination changed since planning: %s", target)
			}
		}
		if err == nil {
			err = writeGlobalSnapshot(target, change.After)
		}
		if err != nil {
			rollbackErr := recoverGlobalJournal(context, journalPath)
			if rollbackErr != nil {
				return result, fmt.Errorf("global transaction failed: %w; restoration failed: %v", err, rollbackErr)
			}
			return result, fmt.Errorf("global transaction failed and restored: %w", err)
		}
	}
	statePath := filepath.Join(context.StateRoot, "state.json")
	if globalMutationHook != nil {
		err = globalMutationHook("before-state", statePath)
	}
	if err == nil {
		current, readErr := readGlobalSnapshot(statePath)
		if readErr != nil {
			err = readErr
		} else if !globalSnapshotsEqual(current, plan.StateBefore) {
			err = fmt.Errorf("global state changed before publication: %s", statePath)
		}
	}
	if err == nil {
		err = writeGlobalSnapshot(statePath, plan.StateAfter)
	}
	if err != nil {
		rollbackErr := recoverGlobalJournal(context, journalPath)
		return result, fmt.Errorf("publish global state: %w; restoration: %v", err, rollbackErr)
	}
	if err := removeGlobalJournal(journalPath); err != nil {
		return result, fmt.Errorf("global files installed but journal cleanup failed: %w", err)
	}
	for _, change := range plan.Changes {
		if change.After == nil {
			pruneGlobalEmptyDirectories(context, change.RootID, change.RelativePath)
		}
	}
	return result, nil
}

func planGlobalChange(context GlobalContext, before, after GlobalState, writes []GlobalWrite, force bool) (globalChangePlan, error) {
	oldFiles, err := validateGlobalState(context, before)
	if err != nil {
		return globalChangePlan{}, err
	}
	newFiles, err := validateGlobalState(context, after)
	if err != nil {
		return globalChangePlan{}, err
	}
	ownedDirectories := globalOwnedDirectories(context, before, oldFiles)
	current, err := ReadGlobalState(context)
	if err != nil {
		return globalChangePlan{}, err
	}
	if !globalStatesEqual(before, current) {
		return globalChangePlan{}, fmt.Errorf("global state changed since planning; retry the operation")
	}
	for _, installation := range after.Installations {
		for _, placement := range installation.Placements {
			owned := false
			for _, previous := range before.Installations {
				for _, priorPlacement := range previous.Placements {
					if priorPlacement.RootID == placement.RootID && strings.EqualFold(priorPlacement.RelativePath, placement.RelativePath) {
						if previous.ID != installation.ID {
							return globalChangePlan{}, fmt.Errorf("source collision between %s and %s", previous.ID, installation.ID)
						}
						owned = true
					}
				}
			}
			if !owned {
				target, err := GlobalFilePath(context, placement.RootID, placement.RelativePath)
				if err != nil {
					return globalChangePlan{}, err
				}
				if _, err := os.Lstat(target); err == nil {
					return globalChangePlan{}, fmt.Errorf("third-party directory conflicts with %s: %s", installation.ID, target)
				} else if !os.IsNotExist(err) {
					return globalChangePlan{}, err
				}
			}
		}
	}
	desired := make(map[string]GlobalWrite)
	for _, write := range writes {
		target, err := GlobalFilePath(context, write.RootID, write.RelativePath)
		if err != nil {
			return globalChangePlan{}, err
		}
		key := globalPathKey(target)
		owned, exists := newFiles[key]
		if !exists || owned.File.RootID != write.RootID || owned.File.RelativePath != write.RelativePath || owned.File.SHA256 != ComputeSHA256(write.Data) || owned.File.Mode != write.Mode {
			return globalChangePlan{}, fmt.Errorf("write does not match desired global manifest: %s", target)
		}
		if _, exists := desired[key]; exists {
			return globalChangePlan{}, fmt.Errorf("duplicate global write: %s", target)
		}
		desired[key] = write
	}
	touched := globalChangedInstallations(before, after)
	for key := range desired {
		touched[newFiles[key].Owner] = true
	}
	keys := make(map[string]bool)
	for key := range oldFiles {
		keys[key] = true
	}
	for key := range newFiles {
		keys[key] = true
	}
	sortedKeys := make([]string, 0, len(keys))
	for key := range keys {
		sortedKeys = append(sortedKeys, key)
	}
	sort.Strings(sortedKeys)
	plan := globalChangePlan{}
	for _, key := range sortedKeys {
		oldFile, wasOwned := oldFiles[key]
		newFile, willOwn := newFiles[key]
		file := oldFile
		if willOwn {
			file = newFile
		}
		if !touched[file.Owner] {
			continue
		}
		if !wasOwned && willOwn {
			if err := validateGlobalNewParentOwnership(context, file, ownedDirectories); err != nil {
				return globalChangePlan{}, err
			}
		}
		actual, err := readGlobalSnapshot(file.Path)
		if err != nil {
			return globalChangePlan{}, err
		}
		if wasOwned && willOwn && oldFile.Owner != newFile.Owner {
			return globalChangePlan{}, fmt.Errorf("source collision at %s between %s and %s", file.Path, oldFile.Owner, newFile.Owner)
		}
		if actual != nil && !wasOwned {
			return globalChangePlan{}, fmt.Errorf("third-party file conflicts with %s: %s", file.Owner, file.Path)
		}
		modified := wasOwned && actual != nil && (ComputeSHA256(actual.Data) != oldFile.File.SHA256 || !globalModesEqual(actual.Mode, oldFile.File.Mode))
		if modified && !force {
			return globalChangePlan{}, fmt.Errorf("managed file modified locally: %s", file.Path)
		}
		var next *globalSnapshot
		if willOwn {
			if write, exists := desired[key]; exists {
				next = &globalSnapshot{Data: write.Data, Mode: write.Mode}
			} else if actual != nil && ComputeSHA256(actual.Data) == newFile.File.SHA256 && globalModesEqual(actual.Mode, newFile.File.Mode) {
				next = actual
			} else {
				return globalChangePlan{}, fmt.Errorf("missing bytes for desired global file: %s", file.Path)
			}
		}
		if !globalSnapshotsEqual(actual, next) {
			if err := validateGlobalWritableParent(file.Path); err != nil {
				return globalChangePlan{}, err
			}
			change := globalChange{RootID: file.File.RootID, RelativePath: file.File.RelativePath, Before: actual, After: next}
			plan.Changes = append(plan.Changes, change)
			if modified {
				plan.Backups = append(plan.Backups, change)
			}
		}
	}
	plan.StateBefore, err = readGlobalSnapshot(filepath.Join(context.StateRoot, "state.json"))
	if err != nil {
		return globalChangePlan{}, err
	}
	if globalStatesEqual(before, after) && plan.StateBefore != nil {
		plan.StateAfter = plan.StateBefore
	} else {
		content, err := json.MarshalIndent(after, "", "  ")
		if err != nil {
			return globalChangePlan{}, err
		}
		plan.StateAfter = &globalSnapshot{Data: append(content, '\n'), Mode: 0600}
	}
	return plan, nil
}

func validateGlobalWritableParent(target string) error {
	current := filepath.Dir(target)
	for {
		info, err := os.Stat(current)
		if err == nil {
			if !info.IsDir() || runtime.GOOS != "windows" && (info.Mode().Perm()&0222 == 0 || info.Mode().Perm()&0111 == 0) {
				return fmt.Errorf("global destination parent is not writable: %s", current)
			}
			return nil
		}
		if !os.IsNotExist(err) {
			return err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return err
		}
		current = parent
	}
}

func globalOwnedDirectories(context GlobalContext, state GlobalState, files map[string]globalOwnedFile) map[string]string {
	owned := make(map[string]string)
	for _, installation := range state.Installations {
		for _, placement := range installation.Placements {
			owned[globalPathKey(filepath.Join(context.Roots[placement.RootID], filepath.FromSlash(placement.RelativePath)))] = installation.ID
		}
	}
	for _, file := range files {
		root := context.Roots[file.File.RootID]
		if file.File.RootID == "provenance" {
			root = filepath.Join(context.StateRoot, "provenance")
		}
		for directory := filepath.Dir(file.Path); directory != root && strings.HasPrefix(directory, root+string(filepath.Separator)); directory = filepath.Dir(directory) {
			owned[globalPathKey(directory)] = file.Owner
		}
	}
	return owned
}

func validateGlobalNewParentOwnership(context GlobalContext, file globalOwnedFile, owned map[string]string) error {
	root := context.Roots[file.File.RootID]
	if file.File.RootID == "provenance" {
		root = filepath.Join(context.StateRoot, "provenance")
	}
	for directory := filepath.Dir(file.Path); directory != root && strings.HasPrefix(directory, root+string(filepath.Separator)); directory = filepath.Dir(directory) {
		if _, err := os.Lstat(directory); err == nil {
			if owned[globalPathKey(directory)] != file.Owner {
				return fmt.Errorf("third-party directory conflicts with %s: %s", file.Owner, directory)
			}
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func globalStatesEqual(first, second GlobalState) bool {
	firstContent, firstErr := json.Marshal(first)
	secondContent, secondErr := json.Marshal(second)
	return firstErr == nil && secondErr == nil && bytes.Equal(firstContent, secondContent)
}

func globalChangedInstallations(before, after GlobalState) map[string]bool {
	previous := make(map[string][]byte)
	changed := make(map[string]bool)
	for _, installation := range before.Installations {
		previous[installation.ID], _ = json.Marshal(installation)
		changed[installation.ID] = true
	}
	for _, installation := range after.Installations {
		content, _ := json.Marshal(installation)
		changed[installation.ID] = !bytes.Equal(previous[installation.ID], content)
	}
	return changed
}

func globalSnapshotsEqual(first, second *globalSnapshot) bool {
	if first == nil || second == nil {
		return first == nil && second == nil
	}
	return globalModesEqual(first.Mode, second.Mode) && bytes.Equal(first.Data, second.Data)
}

func globalModesEqual(actual, expected uint32) bool {
	return globalModesEqualForOS(runtime.GOOS, actual, expected)
}

func globalModesEqualForOS(platform string, actual, expected uint32) bool {
	if platform == "windows" {
		return (actual&0200 != 0) == (expected&0200 != 0)
	}
	return actual == expected
}

func readGlobalSnapshot(target string) (*globalSnapshot, error) {
	if err := validateGlobalAbsolutePath(target); err != nil {
		return nil, err
	}
	info, err := os.Lstat(target)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("global destination is not a regular file: %s", target)
	}
	content, err := os.ReadFile(target)
	if err != nil {
		return nil, err
	}
	return &globalSnapshot{Data: content, Mode: uint32(info.Mode().Perm())}, nil
}

func writeGlobalSnapshot(target string, snapshot *globalSnapshot) error {
	if err := validateGlobalAbsolutePath(target); err != nil {
		return err
	}
	if snapshot == nil {
		if err := os.Remove(target); err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		return syncGlobalDirectory(filepath.Dir(target))
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	if err := validateGlobalAbsolutePath(target); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(target), ".sth-global-*")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer func() {
		file.Close()
		os.Remove(temporary)
	}()
	if _, err := file.Write(snapshot.Data); err != nil {
		return err
	}
	if err := file.Chmod(os.FileMode(snapshot.Mode)); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporary, target); err != nil {
		return err
	}
	return syncGlobalDirectory(filepath.Dir(target))
}

func restoreGlobalChanges(context GlobalContext, changes []globalChange) error {
	for index := len(changes) - 1; index >= 0; index-- {
		change := changes[index]
		target, err := GlobalFilePath(context, change.RootID, change.RelativePath)
		if err != nil {
			return err
		}
		if err := globalSnapshotHasNotDiverged(target, change.Before, change.After); err != nil {
			return err
		}
		if err := writeGlobalSnapshot(target, change.Before); err != nil {
			return fmt.Errorf("restore %s: %w", target, err)
		}
		if change.Before == nil {
			pruneGlobalEmptyDirectories(context, change.RootID, change.RelativePath)
		}
	}
	return nil
}

func preserveGlobalBackups(context GlobalContext, changes []globalChange) (string, error) {
	backupRoot := filepath.Join(context.StateRoot, "backups", fmt.Sprintf("%d-%d", time.Now().UTC().UnixNano(), os.Getpid()))
	if err := validateGlobalAbsolutePath(backupRoot); err != nil {
		return "", err
	}
	if err := os.MkdirAll(backupRoot, 0700); err != nil {
		return "", err
	}
	for _, change := range changes {
		if err := validateGlobalRelativePath(change.RootID + "/" + change.RelativePath); err != nil {
			return "", err
		}
		target := filepath.Join(backupRoot, change.RootID, filepath.FromSlash(change.RelativePath))
		if err := writeGlobalSnapshot(target, change.Before); err != nil {
			return "", err
		}
	}
	return backupRoot, nil
}

func pruneGlobalEmptyDirectories(context GlobalContext, rootID, relativePath string) {
	root := context.Roots[rootID]
	if rootID == "provenance" {
		root = filepath.Join(context.StateRoot, "provenance")
	}
	current := filepath.Dir(filepath.Join(root, filepath.FromSlash(relativePath)))
	for current != root && strings.HasPrefix(current, root+string(filepath.Separator)) {
		if err := validateGlobalAbsolutePath(current); err != nil {
			return
		}
		if err := os.Remove(current); err != nil {
			return
		}
		current = filepath.Dir(current)
	}
}

func acquireGlobalLock(context GlobalContext) (func(), error) {
	if err := validateGlobalAbsolutePath(context.StateRoot); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(context.StateRoot, 0700); err != nil {
		return nil, err
	}
	lockPath := filepath.Join(context.StateRoot, "lock")
	if err := validateGlobalAbsolutePath(lockPath); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := lockGlobalFile(file); err != nil {
		file.Close()
		return nil, err
	}
	return func() {
		unlockGlobalFile(file)
		file.Close()
	}, nil
}
