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
	if cached, ok := cache[queryPath]; ok {
		return cached
	}
	sha, err := a.client.GetLatestCommitSHAForPath(ctx, cfg, providerID, queryPath)
	if err != nil {
		return ""
	}
	cache[queryPath] = sha
	return sha
}

func (a *app) latestVersionsByPath(ctx context.Context, cfg fs.ProjectConfigV2, state fs.InstalledResourceStateV2) (map[string]string, error) {
	latestByPath := make(map[string]string)
	attempted := make(map[string]bool)
	var firstErr error
	for _, record := range state.Entries {
		queryPath := fs.CommitQueryPath(record.SourcePath, record.ArtifactKind)
		if queryPath == "" || attempted[queryPath] {
			continue
		}
		attempted[queryPath] = true
		sha, err := a.client.GetLatestCommitSHAForPath(ctx, cfg, record.ProviderID, queryPath)
		if err != nil {
			if isProviderConfigError(err) {
				continue
			}
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		latestByPath[queryPath] = sha
	}
	return latestByPath, firstErr
}
