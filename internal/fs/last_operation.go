package fs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const lastOperationFileName = "last-operation.json"

// LastOperationStatus is the recorded outcome of an STH command.
type LastOperationStatus string

const (
	LastOperationStatusOK    LastOperationStatus = "ok"
	LastOperationStatusError LastOperationStatus = "error"
)

// LastOperation is a tiny observability record written by install/update/remove
// after they finish. It exists exclusively to power the `sth buddy` Error / Done
// states. It contains no secrets and may be deleted at any time.
type LastOperation struct {
	Op        string              `json:"op"`
	Status    LastOperationStatus `json:"status"`
	Resource  string              `json:"resource,omitempty"`
	Message   string              `json:"message,omitempty"`
	Timestamp string              `json:"timestamp"`
}

// LastOperationPath returns the absolute path of .sth/last-operation.json
// resolved against the current working directory.
func LastOperationPath() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("impossible de localiser le projet courant : %w", err)
	}
	return filepath.Join(wd, projectConfigDirName, lastOperationFileName), nil
}

// WriteLastOperation persists op atomically to .sth/last-operation.json.
// If the .sth directory is missing the call is a no-op (no project to track).
func WriteLastOperation(op LastOperation) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	dir := filepath.Join(wd, projectConfigDirName)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}

	if op.Timestamp == "" {
		op.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	data, err := json.MarshalIndent(op, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(filepath.Join(dir, lastOperationFileName), data, 0o644)
}

// ReadLastOperation loads .sth/last-operation.json. Returns nil without error
// when the file does not exist.
func ReadLastOperation() (*LastOperation, error) {
	path, err := LastOperationPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var op LastOperation
	if err := json.Unmarshal(data, &op); err != nil {
		return nil, fmt.Errorf("last-operation.json malformé : %w", err)
	}
	return &op, nil
}
