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

// usageLogMaxBytes caps the on-disk size of the send-log before it is trimmed
// back to usageLogKeep lines. The file holds one tiny JSON object per CLI
// command, so 256 KB is thousands of entries — far more than usageLogKeep — and
// the trim rewrite therefore runs only very rarely.
const usageLogMaxBytes = 256 * 1024

// usageLogKeep is the number of most-recent entries retained after a trim.
// Matches the server-side MAX_LIMIT for /v1/usage/mine so the local journal and
// the server view cap at the same depth.
const usageLogKeep = 500

// usageLogMu serialises the read-trim-rewrite cycle in AppendUsageLogEntry.
// Plain appends rely on O_APPEND (atomic for the small lines we write), but the
// occasional trim does a full Read-Modify-Write that two concurrent processes
// could otherwise interleave. Process-level mutex is enough — the file is
// user-scoped, not shared across machines.
var usageLogMu sync.Mutex

// UsageLogRecord is one locally-recorded telemetry send. It mirrors the wire
// shape of sthhttp.UsageMineRecord (internal/http/cloud.go) so cmd/telemetry.go
// can render both through the same formatter. Kept in this package (not in
// internal/http) because internal/http already imports internal/fs — putting
// the type there would invert the dependency.
type UsageLogRecord struct {
	Ts          string `json:"ts"`
	CLIVersion  string `json:"cli_version"`
	CommandName string `json:"command_name"`
}

// UsageLogPath returns the absolute path of the local send-log, applying the
// STH_USER_CONFIG_DIR override when set so tests and CI sandbox it the same way
// settings.json is sandboxed. Lives next to settings.json.
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

// AppendUsageLogEntry appends one record to the local send-log as a JSON line.
// Callers treat it as best-effort (the telemetry path discards the error) so it
// must never block a CLI command. After appending it trims the file back to
// usageLogKeep lines when it grows past usageLogMaxBytes.
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

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("ouverture du journal échouée : %w", err)
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		f.Close()
		return fmt.Errorf("écriture dans le journal échouée : %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("fermeture du journal échouée : %w", err)
	}

	return trimUsageLogIfNeeded(path)
}

// trimUsageLogIfNeeded rewrites the log keeping only the last usageLogKeep lines
// once it exceeds usageLogMaxBytes. Best-effort: a Stat or read failure is
// returned but never corrupts the existing file (atomicWriteFile swaps in
// place). Called under usageLogMu.
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
	var b bytes.Buffer
	for _, l := range kept {
		b.Write(l)
		b.WriteByte('\n')
	}
	return atomicWriteFile(path, b.Bytes(), 0o644)
}

// ReadUsageLog returns up to limit of the most-recent send-log entries,
// newest-first (matching the server's /v1/usage/mine ordering). An absent file
// yields an empty slice and no error — "nothing sent yet" is a normal state.
// Malformed lines are skipped silently, mirroring ReadUserSettings' tolerance:
// a partially-corrupt journal must never break `sth telemetry output`.
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
	for _, l := range lines {
		var r UsageLogRecord
		if err := json.Unmarshal(l, &r); err != nil {
			continue // tolerate a corrupt line, keep the rest
		}
		records = append(records, r)
	}

	// Newest-first: walk the appended (oldest-first) slice in reverse, capped
	// at limit. limit <= 0 means "no cap".
	out := make([]UsageLogRecord, 0, len(records))
	for i := len(records) - 1; i >= 0; i-- {
		if limit > 0 && len(out) >= limit {
			break
		}
		out = append(out, records[i])
	}
	return out, nil
}

// splitNonEmptyLines splits raw JSONL into trimmed, non-empty line slices.
func splitNonEmptyLines(data []byte) [][]byte {
	var lines [][]byte
	scanner := bufio.NewScanner(bytes.NewReader(data))
	// Allow long lines defensively, though our records are tiny.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		// scanner.Bytes() is reused across Scan() calls — copy before keeping.
		cp := make([]byte, len(line))
		copy(cp, line)
		lines = append(lines, cp)
	}
	return lines
}
