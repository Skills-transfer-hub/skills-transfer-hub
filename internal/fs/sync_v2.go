package fs

import (
	"fmt"
	"os"
	"path/filepath"
)

func SyncManagedFilesV2(targetDir string, record InstalledResourceRecordV2, previous []ManagedFile) error {
	kept := make(map[string]struct{}, len(record.ManagedFiles))
	for _, managedFile := range record.ManagedFiles {
		kept[managedFile.Path] = struct{}{}
	}
	for _, old := range previous {
		if _, stillThere := kept[old.Path]; stillThere {
			continue
		}
		if err := os.Remove(filepath.Clean(filepath.FromSlash(old.Path))); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("suppression de %s échouée : %w", old.Path, err)
		}
	}
	return UpsertInstalledResourceV2(targetDir, record)
}

func FindInstalledRecordsV2(targetDir, resourceName string) ([]InstalledResourceRecordV2, error) {
	state, err := ReadInstalledStateV2(targetDir)
	if err != nil {
		return nil, err
	}
	matches := make([]InstalledResourceRecordV2, 0, len(state.Entries))
	for _, record := range state.Entries {
		if record.ResourceName == resourceName {
			matches = append(matches, record)
		}
	}
	return matches, nil
}

func InstalledRecordV2For(targetDir, resourceName, targetID string) (InstalledResourceRecordV2, bool, error) {
	state, err := ReadInstalledStateV2(targetDir)
	if err != nil {
		return InstalledResourceRecordV2{}, false, err
	}
	for _, record := range state.Entries {
		if record.ResourceName == resourceName && record.TargetID == targetID {
			return record, true, nil
		}
	}
	return InstalledResourceRecordV2{}, false, nil
}
