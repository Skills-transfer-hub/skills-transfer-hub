package cmd

import (
	"encoding/json"
	"fmt"
	"sort"

	"sth/internal/artifact"
	"sth/internal/writer"
)

type capabilitiesManifest struct {
	SchemaVersion int                         `json:"schema_version"`
	CLIVersion    string                      `json:"cli_version"`
	Targets       []string                    `json:"targets"`
	ArtifactKinds []artifact.Kind             `json:"artifact_kinds"`
	TargetSpecs   []writer.TargetSpec         `json:"target_specs"`
	Compatibility []writer.ArtifactCapability `json:"compatibility"`
}

func buildCapabilitiesManifest() capabilitiesManifest {
	targets := writer.SupportedTargetKinds()
	sort.Strings(targets)
	kinds := artifact.Kinds()
	sort.Slice(kinds, func(leftIndex, rightIndex int) bool { return kinds[leftIndex] < kinds[rightIndex] })
	specifications := writer.AllTargetSpecs()
	sort.Slice(specifications, func(leftIndex, rightIndex int) bool {
		return specifications[leftIndex].Kind < specifications[rightIndex].Kind
	})
	return capabilitiesManifest{SchemaVersion: 1, CLIVersion: valueOrDefault(version, "dev"), Targets: targets, ArtifactKinds: kinds, TargetSpecs: specifications, Compatibility: writer.ArtifactCapabilities()}
}

func (a *app) runCapabilities(args []string) error {
	options, err := parseCommandOptions(args, []string{"--json"}, nil)
	if err != nil {
		return err
	}
	if len(options.positional) != 0 {
		return fmt.Errorf("usage: sth capabilities [--json]")
	}
	manifest := buildCapabilitiesManifest()
	if options.flags["--json"] {
		data, marshalErr := json.MarshalIndent(manifest, "", "  ")
		if marshalErr != nil {
			return marshalErr
		}
		a.writeStdout(string(data))
		return nil
	}
	a.writeStdout(fmt.Sprintf("%d targets, %d artifact kinds", len(manifest.Targets), len(manifest.ArtifactKinds)))
	for _, capability := range manifest.Compatibility {
		a.writeStdout(fmt.Sprintf("%-12s %-12s %s", capability.TargetKind, capability.ArtifactKind, capability.Level))
	}
	return nil
}
