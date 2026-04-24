package fs

// PinnedVersion returns the pinned_version for resourceName from cfg, or "" when not pinned.
func PinnedVersion(cfg ProjectConfigV2, resourceName string) string {
	for _, r := range cfg.Resources {
		if r.ResourceName == resourceName {
			return r.PinnedVersion
		}
	}
	return ""
}
