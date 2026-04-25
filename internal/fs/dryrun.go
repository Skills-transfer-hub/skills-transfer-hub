package fs

import (
	"encoding/json"
	"strings"
)

// ComputeDryRunChanges returns the list of file changes that would be made.
// existing may be nil (resource not yet installed).
// ignorePats are gitignore-style patterns; matching files are skipped.
func ComputeDryRunChanges(existing *InstalledResourceRecordV2, incomingFiles []ManagedFile, ignorePats []string) []DryRunChange {
	var changes []DryRunChange

	// Build index of existing managed files.
	existingIndex := map[string]string{} // path -> sha256
	if existing != nil {
		for _, mf := range existing.ManagedFiles {
			existingIndex[mf.Path] = mf.SHA256
		}
	}

	// Build index of incoming files, emit add/modify.
	incomingIndex := map[string]string{}
	for _, mf := range incomingFiles {
		if MatchesIgnorePattern(mf.Path, ignorePats) {
			continue
		}
		incomingIndex[mf.Path] = mf.SHA256
		oldSHA, exists := existingIndex[mf.Path]
		if !exists {
			changes = append(changes, DryRunChange{Action: "add", Path: mf.Path, NewSHA256: mf.SHA256})
		} else if oldSHA != mf.SHA256 {
			changes = append(changes, DryRunChange{Action: "modify", Path: mf.Path, OldSHA256: oldSHA, NewSHA256: mf.SHA256})
		}
	}

	// Deletions: files in existing but not in incoming.
	if existing != nil {
		for _, mf := range existing.ManagedFiles {
			if _, ok := incomingIndex[mf.Path]; !ok {
				changes = append(changes, DryRunChange{Action: "delete", Path: mf.Path, OldSHA256: mf.SHA256})
			}
		}
	}

	return changes
}

// FormatDryRunText returns human-readable dry-run output.
func FormatDryRunText(changes []DryRunChange) string {
	if len(changes) == 0 {
		return "No changes would be made."
	}
	var sb strings.Builder
	sb.WriteString("Dry run — no changes will be written.\n")
	for _, c := range changes {
		var prefix string
		switch c.Action {
		case "add":
			prefix = "[+]"
		case "modify":
			prefix = "[~]"
		case "delete":
			prefix = "[-]"
		}
		sb.WriteString(prefix + " " + c.Path + "\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

// FormatDryRunJSON returns a JSON-encoded dry-run change list.
func FormatDryRunJSON(changes []DryRunChange) ([]byte, error) {
	if changes == nil {
		changes = []DryRunChange{}
	}
	return json.MarshalIndent(changes, "", "  ")
}
