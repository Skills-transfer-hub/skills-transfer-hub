package fs

import (
	"os"
	"strings"
	"testing"
)

func sandboxUserConfig(t *testing.T) {
	t.Helper()
	t.Setenv("STH_USER_CONFIG_DIR", t.TempDir())
}

func TestReadUsageLog_AbsentFile_EmptyNoError(t *testing.T) {
	sandboxUserConfig(t)
	records, err := ReadUsageLog(50)
	if err != nil {
		t.Fatalf("absent log must not error, got %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("absent log must be empty, got %d", len(records))
	}
}

func TestAppendUsageLog_RoundTripNewestFirst(t *testing.T) {
	sandboxUserConfig(t)

	for _, command := range []string{"first", "second", "third"} {
		if err := AppendUsageLogEntry(UsageLogRecord{Ts: "2026-06-03T00:00:00Z", CLIVersion: "1.0.0", CommandName: command}); err != nil {
			t.Fatalf("append %q: %v", command, err)
		}
	}

	records, err := ReadUsageLog(50)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 {
		t.Fatalf("want 3 records, got %d", len(records))
	}
	if records[0].CommandName != "third" || records[2].CommandName != "first" {
		t.Fatalf("want newest-first order, got %q..%q", records[0].CommandName, records[2].CommandName)
	}
}

func TestReadUsageLog_RespectsLimit(t *testing.T) {
	sandboxUserConfig(t)
	for i := 0; i < 5; i++ {
		if err := AppendUsageLogEntry(UsageLogRecord{Ts: "2026-06-03T00:00:00Z", CLIVersion: "1.0.0", CommandName: "list"}); err != nil {
			t.Fatal(err)
		}
	}
	records, err := ReadUsageLog(2)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("limit=2 must cap to 2, got %d", len(records))
	}
}

func TestReadUsageLog_SkipsMalformedLines(t *testing.T) {
	sandboxUserConfig(t)
	if err := AppendUsageLogEntry(UsageLogRecord{Ts: "2026-06-03T00:00:00Z", CLIVersion: "1.0.0", CommandName: "good"}); err != nil {
		t.Fatal(err)
	}
	path, err := UsageLogPath()
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("{not json\n\n"); err != nil {
		t.Fatal(err)
	}
	file.Close()
	if err := AppendUsageLogEntry(UsageLogRecord{Ts: "2026-06-03T00:00:00Z", CLIVersion: "1.0.0", CommandName: "good2"}); err != nil {
		t.Fatal(err)
	}

	records, err := ReadUsageLog(50)
	if err != nil {
		t.Fatalf("malformed line must not break read, got %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("want 2 valid records skipping junk, got %d", len(records))
	}
}

func TestAppendUsageLog_TrimsRingBuffer(t *testing.T) {
	sandboxUserConfig(t)
	pad := strings.Repeat("x", 1024)
	total := (usageLogMaxBytes / 1024) + usageLogKeep + 50
	for i := 0; i < total; i++ {
		if err := AppendUsageLogEntry(UsageLogRecord{Ts: "2026-06-03T00:00:00Z", CLIVersion: "1.0.0", CommandName: pad}); err != nil {
			t.Fatal(err)
		}
	}
	records, err := ReadUsageLog(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) > usageLogKeep {
		t.Fatalf("ring buffer must cap at %d, got %d", usageLogKeep, len(records))
	}
	if len(records) == 0 {
		t.Fatal("trim must not empty the log")
	}
}
