package fs

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

const keepAChangelogFixture = `# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

### Added
- Work in progress

## [1.3.0] - 2026-07-01

### Added
- Support for nested field arrays

## [1.2.1] - 2026-06-20

### Fixed
- Validation message not clearing on reset

## [1.2.0] - 2026-06-01

### Added
- Initial nested support
`

func TestParseChangelog_ParsesKeepAChangelogSections(t *testing.T) {
	entries, err := ParseChangelog([]byte(keepAChangelogFixture))
	if err != nil {
		t.Fatalf("ParseChangelog: %v", err)
	}

	if len(entries) != 4 {
		t.Fatalf("len(entries) = %d, want 4", len(entries))
	}
	if entries[0].Version != "Unreleased" || entries[0].Date != "" {
		t.Errorf("entries[0] = %+v, want Unreleased sans date", entries[0])
	}
	if entries[1].Version != "1.3.0" || entries[1].Date != "2026-07-01" {
		t.Errorf("entries[1] = %+v, want 1.3.0 / 2026-07-01", entries[1])
	}
	if !strings.Contains(entries[1].Body, "### Added") || !strings.Contains(entries[1].Body, "nested field arrays") {
		t.Errorf("entries[1].Body = %q", entries[1].Body)
	}
	if !strings.Contains(entries[3].Body, "Initial nested support") {
		t.Errorf("entries[3].Body = %q", entries[3].Body)
	}
}

func TestParseChangelog_RejectsUnrecognizedFormat(t *testing.T) {
	_, err := ParseChangelog([]byte("# Notes\n\nv3 : plein de choses\nv2 : moins de choses\n"))
	if !errors.Is(err, ErrUnrecognizedChangelogFormat) {
		t.Fatalf("err = %v, want ErrUnrecognizedChangelogFormat", err)
	}
}

func TestParseChangelog_HandlesCRLF(t *testing.T) {
	fixture := strings.ReplaceAll("## [1.0.0] - 2026-01-01\n\n- First\n", "\n", "\r\n")
	entries, err := ParseChangelog([]byte(fixture))
	if err != nil {
		t.Fatalf("ParseChangelog: %v", err)
	}
	if len(entries) != 1 || entries[0].Version != "1.0.0" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestEntriesSince_ReturnsEntriesNewerThanPrevious(t *testing.T) {
	entries, err := ParseChangelog([]byte(keepAChangelogFixture))
	if err != nil {
		t.Fatal(err)
	}

	newEntries := EntriesSince(entries, "1.2.1")
	if len(newEntries) != 2 {
		t.Fatalf("len(newEntries) = %d, want 2", len(newEntries))
	}
	if newEntries[0].Version != "Unreleased" || newEntries[1].Version != "1.3.0" {
		t.Errorf("newEntries = %+v", newEntries)
	}
}

func TestEntriesSince_ReturnsAllWhenPreviousNotFound(t *testing.T) {
	entries, err := ParseChangelog([]byte(keepAChangelogFixture))
	if err != nil {
		t.Fatal(err)
	}

	newEntries := EntriesSince(entries, "abc1234deadbeef")
	if len(newEntries) != len(entries) {
		t.Errorf("len = %d, want %d (toutes les entrées)", len(newEntries), len(entries))
	}
}

func TestRenderChangelogSection_FormatsHeaderAndEntries(t *testing.T) {
	entries := []ChangelogEntry{
		{Version: "1.3.0", Date: "2026-07-01", Body: "### Added\n- Nested arrays"},
		{Version: "Unreleased", Body: "- WIP"},
	}

	rendered := RenderChangelogSection("react-forms", entries)

	if !strings.HasPrefix(rendered, "What's new in react-forms:\n") {
		t.Errorf("header manquant : %q", rendered)
	}
	if !strings.Contains(rendered, "## [1.3.0] - 2026-07-01") {
		t.Errorf("heading versionné manquant : %q", rendered)
	}
	if !strings.Contains(rendered, "## [Unreleased]\n") {
		t.Errorf("heading Unreleased manquant : %q", rendered)
	}
}

func TestRenderChangelogSection_EmptyEntriesRenderNothing(t *testing.T) {
	if rendered := RenderChangelogSection("react-forms", nil); rendered != "" {
		t.Errorf("rendered = %q, want empty", rendered)
	}
}

func TestTruncateChangelog_CapsAt50KB(t *testing.T) {
	small := bytes.Repeat([]byte("a"), 50*1024)
	if _, wasTruncated := TruncateChangelog(small); wasTruncated {
		t.Error("50 KB exact ne doit pas être tronqué")
	}

	large := bytes.Repeat([]byte("a"), 50*1024+1)
	truncated, wasTruncated := TruncateChangelog(large)
	if !wasTruncated {
		t.Fatal("au-delà de 50 KB doit être tronqué")
	}
	if len(truncated) != 50*1024 {
		t.Errorf("len(truncated) = %d, want %d", len(truncated), 50*1024)
	}
}
