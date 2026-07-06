package cmd

import (
	"errors"
	"fmt"
	"path"
	"sort"

	"sth/internal/fs"
)

func (a *app) printBundleChangelog(resourceName string, bundleContents map[string][]byte, previousVersion string) {
	if previousVersion == "" {
		return
	}
	changelogContent, found := findBundleChangelog(bundleContents)
	if !found {
		return
	}

	truncatedContent, wasTruncated := fs.TruncateChangelog(changelogContent)
	entries, parseErr := fs.ParseChangelog(truncatedContent)
	if parseErr != nil {
		if errors.Is(parseErr, fs.ErrUnrecognizedChangelogFormat) {
			a.writeStderr(fmt.Sprintf("Warning: Could not parse changelog for %s", resourceName))
		}
		return
	}

	newEntries := fs.EntriesSince(entries, previousVersion)
	if len(newEntries) == 0 {
		return
	}

	section := fs.RenderChangelogSection(resourceName, newEntries)
	if wasTruncated {
		section += "(changelog truncated)\n"
	}
	a.writeStdout(section)
}

func findBundleChangelog(bundleContents map[string][]byte) ([]byte, bool) {
	candidates := make([]string, 0)
	for sourcePath := range bundleContents {
		if path.Base(sourcePath) == "CHANGELOG.md" {
			candidates = append(candidates, sourcePath)
		}
	}
	if len(candidates) == 0 {
		return nil, false
	}
	sort.Strings(candidates)
	return bundleContents[candidates[0]], true
}
