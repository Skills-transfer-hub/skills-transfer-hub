package cmd

import (
	"fmt"
	"sort"
	"strings"

	"sth/internal/fs"
)

type installedSelection struct {
	stateDir     string
	providerID   string
	resourceName string
	records      []fs.InstalledResourceRecordV2
}

func installedSelections(config fs.ProjectConfigV2) ([]installedSelection, error) {
	directories, err := projectStateDirs(config)
	if err != nil {
		return nil, err
	}
	selections := make([]installedSelection, 0)
	indexes := map[string]int{}
	for _, directory := range directories {
		state, err := fs.ReadInstalledStateV2(directory)
		if err != nil {
			return nil, err
		}
		for _, record := range state.Entries {
			providerID := fs.RecordProviderID(config, record)
			key := directory + "\x00" + providerID + "\x00" + record.ResourceName
			index, exists := indexes[key]
			if !exists {
				index = len(selections)
				indexes[key] = index
				selections = append(selections, installedSelection{stateDir: directory, providerID: providerID, resourceName: record.ResourceName})
			}
			selections[index].records = append(selections[index].records, record)
		}
	}
	return selections, nil
}

func resolveInstalledSelection(selections []installedSelection, reference, providerFlag string) (installedSelection, error) {
	providerID := providerFlag
	if prefix, suffix, found := strings.Cut(reference, ":"); found && !strings.HasPrefix(suffix, ":") {
		if providerID != "" && providerID != prefix {
			return installedSelection{}, fmt.Errorf("conflicting provider references; use one source ID")
		}
		providerID, reference = prefix, suffix
	}
	matches := make([]installedSelection, 0)
	for _, selection := range selections {
		if providerID != "" && !strings.EqualFold(providerID, selection.providerID) {
			continue
		}
		_, folder, name, canonical := fs.ParseCatalogID(selection.resourceName)
		if strings.EqualFold(reference, selection.resourceName) || canonical && (strings.EqualFold(reference, name) || strings.EqualFold(reference, folder+"/"+name)) {
			matches = append(matches, selection)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) == 0 {
		return installedSelection{}, fmt.Errorf("resource %q not found; run `sth status` to see installed skills", reference)
	}
	choices := make([]string, 0, len(matches))
	for _, match := range matches {
		choices = append(choices, match.providerID+":"+match.resourceName)
	}
	sort.Strings(choices)
	return installedSelection{}, fmt.Errorf("ambiguous resource %q; use one of: %s", reference, strings.Join(choices, ", "))
}
