package fs

// CheckOwnershipConflict returns a non-nil *OwnershipConflict when filePath is
// already managed by a different resource in state.
func CheckOwnershipConflict(state *InstalledResourceStateV2, newOwner, filePath string) *OwnershipConflict {
	for _, rec := range state.Entries {
		if rec.ResourceName == newOwner {
			continue
		}
		for _, mf := range rec.ManagedFiles {
			if mf.Path == filePath {
				return &OwnershipConflict{
					FilePath:      filePath,
					ExistingOwner: rec.ResourceName,
					NewOwner:      newOwner,
				}
			}
		}
	}
	return nil
}

// TransferOwnership removes filePath from its current owner's ManagedFiles in state.
// It is a no-op when filePath is not found.
func TransferOwnership(state *InstalledResourceStateV2, newOwner, filePath string) {
	for i, rec := range state.Entries {
		if rec.ResourceName == newOwner {
			continue
		}
		for j, mf := range rec.ManagedFiles {
			if mf.Path == filePath {
				state.Entries[i].ManagedFiles = append(
					rec.ManagedFiles[:j],
					rec.ManagedFiles[j+1:]...,
				)
				return
			}
		}
	}
}
