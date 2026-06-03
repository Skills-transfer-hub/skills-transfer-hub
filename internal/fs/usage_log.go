package fs

import (
	"bufio"
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
			return "", fmt.Errorf("impossible de localiser le dossier de configuration utilisateur : %w", err)
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
		return fmt.Errorf("création du dossier du journal échouée : %w", err)
	}

	line, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("sérialisation de l'entrée du journal échouée : %w", err)
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("ouverture du journal échouée : %w", err)
	}
	if _, err := file.Write(append(line, '\n')); err != nil {
		file.Close()
		return fmt.Errorf("écriture dans le journal échouée : %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("fermeture du journal échouée : %w", err)
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
		return fmt.Errorf("lecture du journal pour rotation échouée : %w", err)
	}
	lines := splitNonEmptyLines(data)
	if len(lines) <= usageLogKeep {
		return nil
	}
	kept := lines[len(lines)-usageLogKeep:]
	var buffer bytes.Buffer
	for _, line := range kept {
		buffer.Write(line)
		buffer.WriteByte('\n')
	}
	return atomicWriteFile(path, buffer.Bytes(), 0o644)
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
		return nil, fmt.Errorf("lecture de %s échouée : %w", path, err)
	}

	lines := splitNonEmptyLines(data)
	records := make([]UsageLogRecord, 0, len(lines))
	for _, line := range lines {
		var record UsageLogRecord
		if err := json.Unmarshal(line, &record); err != nil {
			continue
		}
		records = append(records, record)
	}

	out := make([]UsageLogRecord, 0, len(records))
	for i := len(records) - 1; i >= 0; i-- {
		if limit > 0 && len(out) >= limit {
			break
		}
		out = append(out, records[i])
	}
	return out, nil
}

func splitNonEmptyLines(data []byte) [][]byte {
	var lines [][]byte
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		lineCopy := make([]byte, len(line))
		copy(lineCopy, line)
		lines = append(lines, lineCopy)
	}
	return lines
}
