package fs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const usageLogFileName = "usage_log.jsonl"

const usageLogMaxBytes = 256 * 1024

const usageLogKeep = 500

var usageLogMu sync.Mutex

type UsageLogRecord struct {
	Ts          string `json:"ts"`
	CLIVersion  string `json:"cli_version"`
	CommandName string `json:"command_name"`
}

func UsageLogPath() (string, error) {
	baseDir := strings.TrimSpace(os.Getenv("STH_USER_CONFIG_DIR"))
	if baseDir == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("unable to locate the user configuration directory: %w", err)
		}
		baseDir = dir
	}
	return filepath.Join(filepath.Clean(baseDir), userSettingsDirName, usageLogFileName), nil
}

func AppendUsageLogEntry(rec UsageLogRecord) error {
	usageLogMu.Lock()
	defer usageLogMu.Unlock()

	path, err := UsageLogPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	line, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("failed to serialize log entry: %w", err)
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open log: %w", err)
	}
	if _, err := file.Write(append(line, '\n')); err != nil {
		file.Close()
		return fmt.Errorf("failed to write to log: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("failed to close log: %w", err)
	}

	return trimUsageLogIfNeeded(path)
}

func trimUsageLogIfNeeded(path string) error {
	info, err := os.Stat(path)
	if err != nil || info.Size() <= usageLogMaxBytes {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read log for rotation: %w", err)
	}
	lines := splitNonEmptyLines(data)
	if len(lines) <= usageLogKeep {
		return nil
	}
	kept := lines[len(lines)-usageLogKeep:]
	trimmed := append(bytes.Join(kept, []byte{'\n'}), '\n')
	return atomicWriteFile(path, trimmed, 0o644)
}

func ReadUsageLog(limit int) ([]UsageLogRecord, error) {
	path, err := UsageLogPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return []UsageLogRecord{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}

	lines := splitNonEmptyLines(data)
	capacity := len(lines)
	if limit > 0 && limit < capacity {
		capacity = limit
	}
	out := make([]UsageLogRecord, 0, capacity)
	for i := len(lines) - 1; i >= 0; i-- {
		if limit > 0 && len(out) >= limit {
			break
		}
		var record UsageLogRecord
		if err := json.Unmarshal(lines[i], &record); err != nil {
			continue
		}
		out = append(out, record)
	}
	return out, nil
}

func splitNonEmptyLines(data []byte) [][]byte {
	var lines [][]byte
	for _, part := range bytes.Split(data, []byte{'\n'}) {
		line := bytes.TrimSpace(part)
		if len(line) > 0 {
			lines = append(lines, line)
		}
	}
	return lines
}
