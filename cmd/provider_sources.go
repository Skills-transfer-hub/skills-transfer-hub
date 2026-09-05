package cmd

import (
	"context"
	"fmt"
	"strings"

	"sth/internal/fs"
	"sth/internal/tui"
)

type providerSource struct {
	id      string
	profile fs.ProjectProfile
}

type providerFailure struct {
	providerID string
	err        error
}

type aggregatedCatalog struct {
	entries  []fs.CatalogEntry
	failures []providerFailure
}

func (a *app) providerSources(cfg fs.ProjectConfigV2) ([]providerSource, []providerFailure, error) {
	if len(cfg.Providers) == 0 {
		profile, err := fs.ReadProjectProfile()
		if err != nil {
			return nil, nil, err
		}
		return []providerSource{{id: fs.FallbackProviderID, profile: *profile}}, nil, nil
	}

	sources := make([]providerSource, 0, len(cfg.Providers))
	failures := make([]providerFailure, 0)
	for _, provider := range fs.ProviderIDs(cfg) {
		profile, err := fs.ProviderProfile(cfg, provider)
		if err != nil {
			failures = append(failures, providerFailure{providerID: provider, err: err})
			continue
		}
		sources = append(sources, providerSource{id: provider, profile: profile})
	}
	return sources, failures, nil
}

func (a *app) sourceProfile(cfg fs.ProjectConfigV2, providerID string) (fs.ProjectProfile, error) {
	if len(cfg.Providers) == 0 {
		profile, err := fs.ReadProjectProfile()
		if err != nil {
			return fs.ProjectProfile{}, err
		}
		return *profile, nil
	}
	return fs.ProviderProfile(cfg, providerID)
}

func (a *app) aggregateCatalog(ctx context.Context, cfg fs.ProjectConfigV2) (aggregatedCatalog, error) {
	sources, failures, err := a.providerSources(cfg)
	if err != nil {
		return aggregatedCatalog{}, err
	}
	catalog := aggregatedCatalog{entries: make([]fs.CatalogEntry, 0), failures: failures}

	for _, source := range sources {
		var entries []fs.CatalogEntry
		listErr := a.withAuthRecovery(ctx, func() error {
			var innerErr error
			entries, innerErr = a.client.ListCatalog(ctx, source.profile)
			return innerErr
		})
		if listErr != nil {
			catalog.failures = append(catalog.failures, providerFailure{providerID: source.id, err: listErr})
			continue
		}
		for index := range entries {
			entries[index].ProviderID = source.id
		}
		catalog.entries = append(catalog.entries, entries...)
	}

	if len(catalog.entries) == 0 && len(catalog.failures) > 0 {
		return catalog, catalogFailureError(catalog.failures)
	}
	return catalog, nil
}

func catalogFailureError(failures []providerFailure) error {
	if len(failures) == 1 {
		return fmt.Errorf("provider %s: %w", failures[0].providerID, failures[0].err)
	}
	messages := make([]string, 0, len(failures))
	for _, failure := range failures {
		messages = append(messages, fmt.Sprintf("%s: %v", failure.providerID, failure.err))
	}
	return fmt.Errorf("no provider is reachable — %s", strings.Join(messages, "; "))
}

func (a *app) reportProviderFailures(failures []providerFailure) {
	for _, failure := range failures {
		a.writeStderr(fmt.Sprintf("warning: provider %s unavailable — %v", failure.providerID, failure.err))
	}
}

func (a *app) resolveCatalogEntry(catalog aggregatedCatalog, resourceRef, providerFlag string) (fs.CatalogEntry, error) {
	entry, err := fs.ResolveCatalogEntryForProvider(catalog.entries, resourceRef, providerFlag)
	if err == nil {
		return entry, nil
	}

	ambiguous, isAmbiguous := err.(*fs.AmbiguousCatalogRefError)
	if !isAmbiguous || !a.isInteractiveSession() {
		return fs.CatalogEntry{}, err
	}
	return a.promptCatalogEntryChoice(ambiguous)
}

func (a *app) promptCatalogEntryChoice(ambiguous *fs.AmbiguousCatalogRefError) (fs.CatalogEntry, error) {
	options := make([]tui.Option, 0, len(ambiguous.Candidates))
	for _, candidate := range ambiguous.Candidates {
		providerID := fs.CatalogEntryProviderID(candidate)
		options = append(options, tui.Option{
			Label:       providerID,
			Description: fmt.Sprintf("%s/%s", candidate.FolderName, candidate.Name),
			Aliases:     []string{providerID},
		})
	}

	session := tui.NewSession(a.stdin, a.stdout)
	a.writeStdout(fmt.Sprintf("'%s' exists in several catalogs.", ambiguous.ResourceRef))
	index, err := tui.Select(session, tui.SelectConfig{
		Title:       "Source provider",
		Options:     options,
		Recommended: 0,
	})
	if err != nil {
		return fs.CatalogEntry{}, ambiguous
	}
	return ambiguous.Candidates[index], nil
}
