package fs

import (
	"fmt"
	"os"
	"strings"
)

// DeleteManagedFiles deletes each managed file for a record.
// Missing files are reported in failedPaths with "(missing)" prefix but not treated as errors.
// Permission/lock errors are reported in failedPaths as real errors.
func DeleteManagedFiles(record InstalledResourceRecordV2) (deletedPaths []string, failedPaths []string) {
	for _, mf := range record.ManagedFiles {
		err := os.Remove(mf.Path)
		if err == nil {
			deletedPaths = append(deletedPaths, mf.Path)
			continue
		}
		if os.IsNotExist(err) {
			failedPaths = append(failedPaths, "(missing) "+mf.Path)
		} else {
			failedPaths = append(failedPaths, mf.Path)
		}
	}
	return
}

// RemoveResourceFromState removes the named resource record from sth-state-v2.json.
// Returns an error when the name is not found.
func RemoveResourceFromState(targetDir string, resourceName string) error {
	state, err := ReadInstalledStateV2(targetDir)
	if err != nil {
		return err
	}

	found := false
	filtered := state.Entries[:0]
	for _, rec := range state.Entries {
		if rec.ResourceName == resourceName {
			found = true
		} else {
			filtered = append(filtered, rec)
		}
	}
	if !found {
		return fmt.Errorf("ressource %q introuvable dans l'état", resourceName)
	}

	state.Entries = filtered
	return WriteInstalledStateV2(targetDir, state)
}

// isMissingFailure reports whether a failedPath is a "(missing)" warning rather than a real error.
func isMissingFailure(p string) bool {
	return strings.HasPrefix(p, "(missing) ")
}
