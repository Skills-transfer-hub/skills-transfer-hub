package cmd

import (
	"context"

	"sth/internal/fs"
)

func (a *app) resolveCommitForEntry(ctx context.Context, cfg fs.ProjectConfigV2, providerID string, entry fs.CatalogEntry, cache map[string]string) string {
	queryPath := fs.CommitQueryPath(entry.SourcePath, artifactKindOf(entry))
	if queryPath == "" {
		return ""
	}
	cacheKey := fs.LatestVersionKey(providerID, queryPath)
	if cached, ok := cache[cacheKey]; ok {
		return cached
	}
	sha, err := a.client.GetLatestCommitSHAForPath(ctx, cfg, providerID, queryPath)
	if err != nil {
		return ""
	}
	cache[cacheKey] = sha
	return sha
}

func (a *app) latestVersionsByPath(ctx context.Context, cfg fs.ProjectConfigV2, state fs.InstalledResourceStateV2) (map[string]string, error) {
	latestVersions := make(map[string]string)
	attempted := make(map[string]bool)
	var firstErr error
	for _, record := range state.Entries {
		queryPath := fs.CommitQueryPath(record.SourcePath, record.ArtifactKind)
		if queryPath == "" {
			continue
		}
		providerID := fs.RecordProviderID(cfg, record)
		cacheKey := fs.LatestVersionKey(providerID, queryPath)
		if attempted[cacheKey] {
			continue
		}
		attempted[cacheKey] = true
		sha, err := a.client.GetLatestCommitSHAForPath(ctx, cfg, providerID, queryPath)
		if err != nil {
			if isProviderConfigError(err) {
				continue
			}
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		latestVersions[cacheKey] = sha
	}
	return latestVersions, firstErr
}
