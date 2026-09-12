package fs

import (
	"fmt"
	"os"
	"strings"
)

func DeleteManagedFiles(record InstalledResourceRecordV2) (deletedPaths []string, failedPaths []string) {
	for _, managedFile := range record.ManagedFiles {
		err := os.Remove(managedFile.Path)
		if err == nil {
			deletedPaths = append(deletedPaths, managedFile.Path)
			continue
		}
		if os.IsNotExist(err) {
			failedPaths = append(failedPaths, "(missing) "+managedFile.Path)
		} else {
			failedPaths = append(failedPaths, managedFile.Path)
		}
	}
	return
}

func RemoveResourceFromState(targetDir string, resourceName string, providerIDs ...string) error {
	state, err := ReadInstalledStateV2(targetDir)
	if err != nil {
		return err
	}

	found := false
	filtered := state.Entries[:0]
	for _, record := range state.Entries {
		if record.ResourceName == resourceName && (len(providerIDs) == 0 || record.ProviderID == providerIDs[0]) {
			found = true
		} else {
			filtered = append(filtered, record)
		}
	}
	if !found {
		return fmt.Errorf("resource %q not found in state", resourceName)
	}

	state.Entries = filtered
	return WriteInstalledStateV2(targetDir, state)
}

func isMissingFailure(failedPath string) bool {
	return strings.HasPrefix(failedPath, "(missing) ")
}
