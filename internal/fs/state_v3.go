package fs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"sth/internal/artifact"
)

const StateFileNameV3 = "sth-state-v3.json"

type InstalledResourceStateV3 struct {
	SchemaVersion int                         `json:"schema_version"`
	Resources     []InstalledResourceRecordV3 `json:"resources"`
}

type InstalledResourceSourceV3 struct {
	Dialect string `json:"dialect"`
	Path    string `json:"path"`
	Commit  string `json:"commit,omitempty"`
	SHA256  string `json:"sha256,omitempty"`
}

type InstalledTransformationV3 struct {
	TargetID             string                       `json:"target_id"`
	TargetKind           string                       `json:"target_kind"`
	TargetDialect        string                       `json:"target_dialect"`
	Transformer          string                       `json:"transformer"`
	TransformerVersion   string                       `json:"transformer_version"`
	Compatibility        artifact.CompatibilityLevel  `json:"compatibility"`
	Report               artifact.CompatibilityReport `json:"report"`
	LegacyStateDirectory string                       `json:"legacy_state_directory,omitempty"`
	Outputs              []InstalledOutputV3          `json:"outputs"`
}

type InstalledOutputV3 struct {
	Consumers []string        `json:"consumers"`
	Path      string          `json:"path"`
	SHA256    string          `json:"sha256"`
	Regions   []ManagedRegion `json:"regions,omitempty"`
}

type InstalledResourceRecordV3 struct {
	ResourceID      string                      `json:"resource_id"`
	ProviderID      string                      `json:"provider_id"`
	ArtifactKind    string                      `json:"artifact_kind"`
	ResolvedVersion string                      `json:"resolved_version,omitempty"`
	PreviousVersion string                      `json:"previous_version,omitempty"`
	InstalledAt     string                      `json:"installed_at"`
	Source          InstalledResourceSourceV3   `json:"source"`
	Plans           []InstalledTransformationV3 `json:"plans"`
	Outputs         []InstalledOutputV3         `json:"outputs"`
}

type StatusResultV3 struct {
	ResourceID       string         `json:"resource_id"`
	ResourceName     string         `json:"resource_name"`
	ProviderID       string         `json:"provider_id"`
	InstalledVersion string         `json:"installed_version,omitempty"`
	LatestVersion    string         `json:"latest_version,omitempty"`
	Status           ResourceStatus `json:"status"`
	ModifiedOutputs  []string       `json:"modified_outputs,omitempty"`
	Pinned           bool           `json:"pinned"`
	PinnedVersion    string         `json:"pinned_version,omitempty"`
}

func InstalledStateV3Path(projectRoot string) string {
	return filepath.Join(filepath.Clean(projectRoot), ".sth", StateFileNameV3)
}

func ReadInstalledStateV3(projectRoot string) (InstalledResourceStateV3, error) {
	state := InstalledResourceStateV3{SchemaVersion: 3, Resources: []InstalledResourceRecordV3{}}
	data, err := os.ReadFile(InstalledStateV3Path(projectRoot))
	if os.IsNotExist(err) {
		return state, nil
	}
	if err != nil {
		return state, fmt.Errorf("read v3 state: %w", err)
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return InstalledResourceStateV3{}, fmt.Errorf("parse v3 state: %w", err)
	}
	if state.SchemaVersion != 3 {
		return InstalledResourceStateV3{}, fmt.Errorf("unsupported state schema version: %d", state.SchemaVersion)
	}
	if state.Resources == nil {
		state.Resources = []InstalledResourceRecordV3{}
	}
	for resourceIndex := range state.Resources {
		if len(state.Resources[resourceIndex].Outputs) == 0 {
			state.Resources[resourceIndex].Outputs = coalescedOutputs(state.Resources[resourceIndex].Plans)
		}
	}
	return state, nil
}

func ReadInstalledStateV3OrLegacy(projectRoot string, targetDirectories []string) (InstalledResourceStateV3, error) {
	if _, err := os.Stat(InstalledStateV3Path(projectRoot)); err == nil {
		return ReadInstalledStateV3(projectRoot)
	} else if !os.IsNotExist(err) {
		return InstalledResourceStateV3{}, err
	}
	state := InstalledResourceStateV3{SchemaVersion: 3, Resources: []InstalledResourceRecordV3{}}
	seenDirectories := make(map[string]struct{})
	for _, targetDirectory := range targetDirectories {
		cleanDirectory := filepath.Clean(targetDirectory)
		if _, duplicate := seenDirectories[cleanDirectory]; duplicate {
			continue
		}
		seenDirectories[cleanDirectory] = struct{}{}
		legacyState, err := ReadInstalledStateV2(targetDirectory)
		if err != nil {
			return InstalledResourceStateV3{}, err
		}
		mergeInstalledStateV2(&state, targetDirectory, legacyState)
	}
	return state, nil
}

func WriteInstalledStateV3(projectRoot string, state InstalledResourceStateV3) error {
	state.SchemaVersion = 3
	if state.Resources == nil {
		state.Resources = []InstalledResourceRecordV3{}
	}
	sortInstalledStateV3(&state)
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("serialize v3 state: %w", err)
	}
	statePath := InstalledStateV3Path(projectRoot)
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		return fmt.Errorf("create v3 state directory: %w", err)
	}
	if err := atomicWriteFile(statePath, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write v3 state: %w", err)
	}
	return nil
}

func PromoteInstalledStateV3(projectRoot string, targetDirectories []string) error {
	if _, err := os.Stat(InstalledStateV3Path(projectRoot)); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	state := InstalledResourceStateV3{SchemaVersion: 3, Resources: []InstalledResourceRecordV3{}}
	for _, targetDirectory := range targetDirectories {
		legacyState, err := ReadInstalledStateV2(targetDirectory)
		if err != nil {
			return err
		}
		mergeInstalledStateV2(&state, targetDirectory, legacyState)
	}
	return WriteInstalledStateV3(projectRoot, state)
}

func MergeInstalledStateV3FromV2(projectRoot, targetDirectory string, legacyState InstalledResourceStateV2) error {
	if _, err := os.Stat(filepath.Join(projectRoot, ".sth", "project.json")); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if _, err := os.Stat(InstalledStateV3Path(projectRoot)); os.IsNotExist(err) {
		targetDirectories := []string{targetDirectory}
		if config, configErr := ReadProjectConfigV2ReadOnly(); configErr == nil {
			for _, provider := range config.Providers {
				targetDirectories = append(targetDirectories, provider.TargetDir)
			}
		}
		if promoteErr := PromoteInstalledStateV3(projectRoot, targetDirectories); promoteErr != nil {
			return promoteErr
		}
	} else if err != nil {
		return err
	}
	state, err := ReadInstalledStateV3(projectRoot)
	if err != nil {
		return err
	}
	mergeInstalledStateV2(&state, targetDirectory, legacyState)
	return WriteInstalledStateV3(projectRoot, state)
}

func mergeInstalledStateV2(state *InstalledResourceStateV3, targetDirectory string, legacyState InstalledResourceStateV2) {
	cleanTargetDirectory := filepath.ToSlash(filepath.Clean(targetDirectory))
	for resourceIndex := range state.Resources {
		plans := state.Resources[resourceIndex].Plans[:0]
		for _, plan := range state.Resources[resourceIndex].Plans {
			if plan.LegacyStateDirectory != cleanTargetDirectory {
				plans = append(plans, plan)
			}
		}
		state.Resources[resourceIndex].Plans = plans
	}
	for _, record := range legacyState.Entries {
		resourceIndex := findResourceV3(*state, record.ResourceName, record.ProviderID)
		if resourceIndex < 0 {
			state.Resources = append(state.Resources, InstalledResourceRecordV3{ResourceID: record.ResourceName, ProviderID: record.ProviderID, ArtifactKind: record.ArtifactKind, ResolvedVersion: record.ResolvedVersion, PreviousVersion: record.PreviousVersion, InstalledAt: record.InstalledAt, Source: InstalledResourceSourceV3{Dialect: valueOrFallback(record.SourceDialect, "sth.catalog.v1"), Path: record.SourcePath, Commit: record.ResolvedVersion, SHA256: record.SourceSHA256}, Plans: []InstalledTransformationV3{}, Outputs: []InstalledOutputV3{}})
			resourceIndex = len(state.Resources) - 1
		}
		resource := &state.Resources[resourceIndex]
		resource.ArtifactKind = valueOrFallback(record.ArtifactKind, resource.ArtifactKind)
		resource.ResolvedVersion = valueOrFallback(record.ResolvedVersion, resource.ResolvedVersion)
		resource.PreviousVersion = record.PreviousVersion
		resource.InstalledAt = valueOrFallback(record.InstalledAt, resource.InstalledAt)
		resource.Source = InstalledResourceSourceV3{Dialect: valueOrFallback(record.SourceDialect, "sth.catalog.v1"), Path: record.SourcePath, Commit: record.ResolvedVersion, SHA256: record.SourceSHA256}
		compatibility := artifact.CompatibilityLevel(valueOrFallback(record.Compatibility, string(artifact.CompatibilityExact)))
		plan := InstalledTransformationV3{TargetID: record.TargetID, TargetKind: record.TargetKind, TargetDialect: valueOrFallback(record.TargetDialect, inferredTargetDialect(record)), Transformer: "sth", TransformerVersion: valueOrFallback(record.TransformerVersion, "1"), Compatibility: compatibility, Report: record.CompatibilityReport, LegacyStateDirectory: cleanTargetDirectory, Outputs: outputsFromV2(record)}
		resource.Plans = append(resource.Plans, plan)
	}
	resources := state.Resources[:0]
	for _, resource := range state.Resources {
		if len(resource.Plans) > 0 {
			resources = append(resources, resource)
		}
	}
	state.Resources = resources
	for resourceIndex := range state.Resources {
		state.Resources[resourceIndex].Outputs = coalescedOutputs(state.Resources[resourceIndex].Plans)
	}
	sortInstalledStateV3(state)
}

func coalescedOutputs(plans []InstalledTransformationV3) []InstalledOutputV3 {
	outputs := make([]InstalledOutputV3, 0)
	indexes := make(map[string]int)
	for _, plan := range plans {
		for _, output := range plan.Outputs {
			key := output.Path + "\x00" + output.SHA256
			outputIndex, exists := indexes[key]
			if !exists {
				indexes[key] = len(outputs)
				outputs = append(outputs, InstalledOutputV3{Consumers: uniqueStrings(append([]string(nil), output.Consumers...)), Path: output.Path, SHA256: output.SHA256, Regions: append([]ManagedRegion(nil), output.Regions...)})
				continue
			}
			outputs[outputIndex].Consumers = uniqueStrings(append(outputs[outputIndex].Consumers, output.Consumers...))
			for _, region := range output.Regions {
				regionExists := false
				for _, existingRegion := range outputs[outputIndex].Regions {
					if existingRegion.Path == region.Path && existingRegion.ID == region.ID && existingRegion.SHA256 == region.SHA256 {
						regionExists = true
						break
					}
				}
				if !regionExists {
					outputs[outputIndex].Regions = append(outputs[outputIndex].Regions, region)
				}
			}
		}
	}
	sort.Slice(outputs, func(leftIndex, rightIndex int) bool { return outputs[leftIndex].Path < outputs[rightIndex].Path })
	return outputs
}

func outputsFromV2(record InstalledResourceRecordV2) []InstalledOutputV3 {
	consumer := valueOrFallback(record.TargetKind, record.TargetID)
	consumers := append([]string(nil), record.Consumers...)
	if len(consumers) == 0 {
		consumers = compactConsumers(consumer)
	}
	outputs := make([]InstalledOutputV3, 0, len(record.ManagedFiles)+len(record.ManagedRegions))
	indexes := map[string]int{}
	for _, managedFile := range record.ManagedFiles {
		indexes[managedFile.Path] = len(outputs)
		outputs = append(outputs, InstalledOutputV3{Consumers: append([]string(nil), consumers...), Path: managedFile.Path, SHA256: managedFile.SHA256})
	}
	for _, managedRegion := range record.ManagedRegions {
		if outputIndex, exists := indexes[managedRegion.Path]; exists {
			outputs[outputIndex].Regions = append(outputs[outputIndex].Regions, managedRegion)
			continue
		}
		indexes[managedRegion.Path] = len(outputs)
		outputs = append(outputs, InstalledOutputV3{Consumers: append([]string(nil), consumers...), Path: managedRegion.Path, SHA256: managedRegion.SHA256, Regions: []ManagedRegion{managedRegion}})
	}
	return outputs
}

func compactConsumers(consumer string) []string {
	if strings.TrimSpace(consumer) == "" {
		return []string{}
	}
	return []string{consumer}
}

func inferredTargetDialect(record InstalledResourceRecordV2) string {
	targetKind := valueOrFallback(record.TargetKind, record.TargetID)
	if targetKind == "" {
		targetKind = "claude"
	}
	artifactKind := valueOrFallback(record.ArtifactKind, "skill")
	return targetKind + "." + artifactKind + ".v1"
}

func findResourceV3(state InstalledResourceStateV3, resourceID, providerID string) int {
	for resourceIndex, resource := range state.Resources {
		if resource.ResourceID == resourceID && resource.ProviderID == providerID {
			return resourceIndex
		}
	}
	return -1
}

func sortInstalledStateV3(state *InstalledResourceStateV3) {
	sort.Slice(state.Resources, func(leftIndex, rightIndex int) bool {
		if state.Resources[leftIndex].ProviderID != state.Resources[rightIndex].ProviderID {
			return state.Resources[leftIndex].ProviderID < state.Resources[rightIndex].ProviderID
		}
		return state.Resources[leftIndex].ResourceID < state.Resources[rightIndex].ResourceID
	})
	for resourceIndex := range state.Resources {
		sort.Slice(state.Resources[resourceIndex].Plans, func(leftIndex, rightIndex int) bool {
			return state.Resources[resourceIndex].Plans[leftIndex].TargetID < state.Resources[resourceIndex].Plans[rightIndex].TargetID
		})
		for planIndex := range state.Resources[resourceIndex].Plans {
			sort.Slice(state.Resources[resourceIndex].Plans[planIndex].Outputs, func(leftIndex, rightIndex int) bool {
				return state.Resources[resourceIndex].Plans[planIndex].Outputs[leftIndex].Path < state.Resources[resourceIndex].Plans[planIndex].Outputs[rightIndex].Path
			})
		}
		sort.Slice(state.Resources[resourceIndex].Outputs, func(leftIndex, rightIndex int) bool {
			return state.Resources[resourceIndex].Outputs[leftIndex].Path < state.Resources[resourceIndex].Outputs[rightIndex].Path
		})
	}
}

func valueOrFallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func ComputeStatusesV3(state InstalledResourceStateV3, projectRoot string) []StatusResultV3 {
	results := make([]StatusResultV3, 0, len(state.Resources))
	for _, resource := range state.Resources {
		status := ResourceStatusUpToDate
		modified := make([]string, 0)
		partiallyConverted := false
		for _, plan := range resource.Plans {
			if plan.Compatibility == artifact.CompatibilityLossy {
				partiallyConverted = true
			}
			if plan.Compatibility == artifact.CompatibilityUnsupported || plan.Compatibility == artifact.CompatibilityNativeOnly && len(plan.Outputs) == 0 {
				status = ResourceStatusNotInstallable
			}
		}
		outputs := resource.Outputs
		if len(outputs) == 0 {
			outputs = coalescedOutputs(resource.Plans)
		}
		for _, output := range outputs {
			absolutePath := output.Path
			if !filepath.IsAbs(absolutePath) {
				absolutePath = filepath.Join(projectRoot, filepath.FromSlash(output.Path))
			}
			if len(output.Regions) > 0 {
				for _, region := range output.Regions {
					data, err := os.ReadFile(absolutePath)
					if err != nil {
						modified = append(modified, output.Path)
						continue
					}
					actual, regionErr := ManagedMarkdownRegionSHA256(data, region.ID)
					if regionErr != nil || actual != region.SHA256 {
						modified = append(modified, output.Path)
					}
				}
				continue
			}
			data, err := os.ReadFile(absolutePath)
			if err != nil || ComputeSHA256(data) != output.SHA256 {
				modified = append(modified, output.Path)
			}
		}
		if len(modified) > 0 {
			status = ResourceStatusModifiedLocally
		} else if status == ResourceStatusUpToDate && partiallyConverted {
			status = ResourceStatusPartiallyConverted
		}
		results = append(results, StatusResultV3{ResourceID: resource.ResourceID, ResourceName: resource.ResourceID, ProviderID: resource.ProviderID, InstalledVersion: resource.ResolvedVersion, Status: status, ModifiedOutputs: uniqueStrings(modified)})
	}
	return results
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	unique := make([]string, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	sort.Strings(unique)
	return unique
}
