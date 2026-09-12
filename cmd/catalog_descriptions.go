package cmd

import (
	"context"
	"strings"

	"sth/internal/fs"
	"sth/internal/scanner"
)

func (a *app) describeCatalogSkills(ctx context.Context, config fs.ProjectConfigV2, entries []fs.CatalogEntry) {
	for index := range entries {
		entry := &entries[index]
		if artifactKindOf(*entry) != "skill" {
			continue
		}
		description := strings.TrimSpace(entry.Description)
		if description != "" && description != "Discovered from the repository structure" {
			continue
		}
		entry.Description = "Description inaccessible — réessayez avec `sth list` ou consultez le dépôt"
		profile, err := a.sourceProfile(config, fs.CatalogEntryProviderID(*entry))
		if err != nil {
			continue
		}
		content, err := a.client.DownloadCatalogEntry(ctx, profile, *entry)
		if err != nil {
			continue
		}
		entry.Description = "Description non fournie par l’auteur — consultez le dépôt avant installation"
		metadata, valid := scanner.ExtractFrontmatter(content)
		if valid && strings.TrimSpace(metadata.Description) != "" {
			entry.Description = strings.Join(strings.Fields(metadata.Description), " ")
		}
	}
}
