package cmd

import (
	"context"
	"errors"
	"slices"
	"sort"
	"strings"

	"sth/internal/fs"
	sthhttp "sth/internal/http"
)

func (a *app) resolveGlobalInstallCandidates(state fs.GlobalState, source sthhttp.GlobalSource, options globalOptions) ([]sthhttp.GlobalCandidate, map[string]sthhttp.GlobalResolution, error) {
	existing := []fs.GlobalInstallation{}
	for _, installation := range state.Installations {
		if globalHTTPSource(installation.Source) != source || options.Ref != "" {
			continue
		}
		if options.Path != "" && installation.Source.Subdir != options.Path && !strings.HasPrefix(installation.Source.Subdir, options.Path+"/") {
			continue
		}
		existing = append(existing, installation)
	}
	needDiscovery := len(options.Skills) == 0 || slices.Contains(options.Skills, "*")
	for _, requested := range options.Skills {
		found := false
		for _, installation := range existing {
			found = found || requested == installation.SkillName
		}
		needDiscovery = needDiscovery || !found
	}
	byPath := map[string]sthhttp.GlobalCandidate{}
	resolutions := map[string]sthhttp.GlobalResolution{}
	if needDiscovery {
		resolution, err := a.client.ResolveGlobalSource(context.Background(), source, options.Ref)
		if err != nil {
			return nil, nil, err
		}
		candidates, err := a.client.DiscoverGlobalSkills(context.Background(), resolution, options.Path)
		if err != nil {
			var selectionError *sthhttp.GlobalSelectionError
			if len(existing) == 0 || !errors.As(err, &selectionError) || selectionError.Message != "no skills found in the selected repository path" {
				return nil, nil, err
			}
		}
		for _, candidate := range candidates {
			byPath[candidate.Path] = candidate
			resolutions[candidate.Path] = resolution
		}
	}
	for _, installation := range existing {
		byPath[installation.Source.Subdir] = sthhttp.GlobalCandidate{Name: installation.SkillName, Path: installation.Source.Subdir}
		resolutions[installation.Source.Subdir] = sthhttp.GlobalResolution{Source: source, RequestedRef: installation.RequestedRef, Revision: installation.ResolvedRevision}
	}
	if options.Path == "" {
		hasCanonical := false
		for candidatePath := range byPath {
			hasCanonical = hasCanonical || strings.HasPrefix(candidatePath, "skills/")
		}
		if hasCanonical {
			for candidatePath := range byPath {
				if !strings.HasPrefix(candidatePath, "skills/") {
					delete(byPath, candidatePath)
					delete(resolutions, candidatePath)
				}
			}
		}
	}
	candidates := make([]sthhttp.GlobalCandidate, 0, len(byPath))
	for _, candidate := range byPath {
		candidates = append(candidates, candidate)
	}
	sort.Slice(candidates, func(first, second int) bool { return candidates[first].Path < candidates[second].Path })
	return candidates, resolutions, nil
}
