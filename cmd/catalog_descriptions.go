package cmd

import (
	"context"
	"strings"
	"sync"

	"sth/internal/fs"
	"sth/internal/scanner"
)

func (a *app) describeCatalogSkills(ctx context.Context, config fs.ProjectConfigV2, entries []fs.CatalogEntry) {
	if len(entries) <= 20 {
		for index := range entries {
			a.describeCatalogSkill(ctx, config, &entries[index])
		}
		return
	}
	var workers sync.WaitGroup
	queue := make(chan int)
	for worker := 0; worker < 8; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range queue {
				a.describeCatalogSkill(ctx, config, &entries[index])
			}
		}()
	}
	for index := range entries {
		queue <- index
	}
	close(queue)
	workers.Wait()
}

func (a *app) describeCatalogSkill(ctx context.Context, config fs.ProjectConfigV2, entry *fs.CatalogEntry) {
	if artifactKindOf(*entry) != "skill" {
		return
	}
	description := strings.TrimSpace(entry.Description)
	if description != "" && description != "Discovered from the repository structure" {
		return
	}
	entry.Description = "Description inaccessible — réessayez avec `sth list` ou consultez le dépôt"
	profile, err := a.sourceProfile(config, fs.CatalogEntryProviderID(*entry))
	if err != nil {
		return
	}
	content, err := a.client.DownloadCatalogEntry(ctx, profile, *entry)
	if err != nil {
		return
	}
	entry.Description = "Description non fournie par l’auteur — consultez le dépôt avant installation"
	metadata, valid := scanner.ExtractFrontmatter(content)
	if valid && strings.TrimSpace(metadata.Description) != "" {
		entry.Description = strings.Join(strings.Fields(metadata.Description), " ")
	}
}
