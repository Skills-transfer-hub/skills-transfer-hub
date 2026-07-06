package fs

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var ErrUnrecognizedChangelogFormat = errors.New("changelog format not recognized")

const changelogMaxBytes = 50 * 1024

type ChangelogEntry struct {
	Version string
	Date    string
	Body    string
}

var changelogHeadingPattern = regexp.MustCompile(`^##\s+\[([^\]]+)\](?:\s+-\s+(\S+))?\s*$`)

func ParseChangelog(content []byte) ([]ChangelogEntry, error) {
	lines := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
	entries := make([]ChangelogEntry, 0)
	var currentBody []string
	for _, line := range lines {
		matches := changelogHeadingPattern.FindStringSubmatch(line)
		if matches == nil {
			if len(entries) > 0 {
				currentBody = append(currentBody, line)
			}
			continue
		}
		if len(entries) > 0 {
			entries[len(entries)-1].Body = strings.TrimSpace(strings.Join(currentBody, "\n"))
		}
		entries = append(entries, ChangelogEntry{Version: matches[1], Date: matches[2]})
		currentBody = nil
	}
	if len(entries) == 0 {
		return nil, ErrUnrecognizedChangelogFormat
	}
	entries[len(entries)-1].Body = strings.TrimSpace(strings.Join(currentBody, "\n"))
	return entries, nil
}

func EntriesSince(entries []ChangelogEntry, previousVersion string) []ChangelogEntry {
	for index, entry := range entries {
		if entry.Version == previousVersion {
			return entries[:index]
		}
	}
	return entries
}

func RenderChangelogSection(resourceName string, entries []ChangelogEntry) string {
	if len(entries) == 0 {
		return ""
	}
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("What's new in %s:\n", resourceName))
	for _, entry := range entries {
		builder.WriteString("\n")
		if entry.Date != "" {
			builder.WriteString(fmt.Sprintf("## [%s] - %s\n", entry.Version, entry.Date))
		} else {
			builder.WriteString(fmt.Sprintf("## [%s]\n", entry.Version))
		}
		if entry.Body != "" {
			builder.WriteString(entry.Body)
			builder.WriteString("\n")
		}
	}
	return builder.String()
}

func TruncateChangelog(content []byte) ([]byte, bool) {
	if len(content) <= changelogMaxBytes {
		return content, false
	}
	return content[:changelogMaxBytes], true
}
