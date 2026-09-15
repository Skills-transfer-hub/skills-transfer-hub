package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"sth/internal/artifact"
	"sth/internal/writer"
)

type artifactInspection struct {
	Definition  artifact.Definition          `json:"definition"`
	Diagnostics []artifact.Diagnostic        `json:"diagnostics"`
	Plans       []artifact.CompatibilityPlan `json:"plans"`
}

func (a *app) runValidate(args []string) error {
	options, err := parseCommandOptions(args, []string{"--json", "--strict"}, []string{"--target"})
	if err != nil {
		return err
	}
	if len(options.positional) > 1 {
		return fmt.Errorf("usage: sth validate [selector|path] [--target <target>] [--strict] [--json]")
	}
	selector := ""
	if len(options.positional) == 1 {
		selector = options.positional[0]
	}
	targetKinds := optionValues(options, "--target")
	inspections, inspectErr := inspectProjectArtifacts(selector, targetKinds, len(targetKinds) > 0)
	if inspectErr != nil {
		return inspectErr
	}
	if options.flags["--json"] {
		data, marshalErr := json.MarshalIndent(inspections, "", "  ")
		if marshalErr != nil {
			return marshalErr
		}
		a.writeStdout(string(data))
	} else {
		for _, inspection := range inspections {
			a.writeStdout(fmt.Sprintf("%s  %s  %s", inspection.Definition.ID, inspection.Definition.SourceDialect, inspection.Definition.SourcePath))
			for _, diagnostic := range inspection.Diagnostics {
				a.writeStderr(fmt.Sprintf("%s %s %s: %s", diagnostic.Severity, diagnostic.Path, diagnostic.Code, diagnostic.Message))
			}
			for _, plan := range inspection.Plans {
				a.writeStdout(fmt.Sprintf("  → %s: %s", plan.TargetDialect, plan.Level))
			}
		}
	}
	for _, inspection := range inspections {
		if artifact.HasErrors(inspection.Diagnostics, options.flags["--strict"]) {
			return fmt.Errorf("artifact validation failed")
		}
	}
	return nil
}

func (a *app) runInspect(args []string) error {
	options, err := parseCommandOptions(args, []string{"--json"}, []string{"--target"})
	if err != nil {
		return err
	}
	if len(options.positional) != 1 {
		return fmt.Errorf("usage: sth inspect <selector|path> [--target <target>] [--json]")
	}
	inspections, inspectErr := inspectProjectArtifacts(options.positional[0], optionValues(options, "--target"), true)
	if inspectErr != nil {
		return inspectErr
	}
	if options.flags["--json"] {
		data, marshalErr := json.MarshalIndent(inspections, "", "  ")
		if marshalErr != nil {
			return marshalErr
		}
		a.writeStdout(string(data))
		return nil
	}
	for _, inspection := range inspections {
		a.writeStdout(fmt.Sprintf("%s (%s)", inspection.Definition.ID, inspection.Definition.SourceDialect))
		a.writeStdout(fmt.Sprintf("  source: %s", inspection.Definition.SourcePath))
		a.writeStdout(fmt.Sprintf("  sha256: %s", inspection.Definition.SourceSHA256))
		for _, plan := range inspection.Plans {
			a.writeStdout(fmt.Sprintf("  %s: %s", plan.TargetDialect, plan.Level))
			for _, warning := range plan.Report.Warnings {
				a.writeStderr("warning: " + warning)
			}
		}
	}
	return nil
}

func inspectProjectArtifacts(selector string, targetKinds []string, diagnoseTargets bool) ([]artifactInspection, error) {
	projectRoot, selectorValue, err := artifactProjectRootAndSelector(selector)
	if err != nil {
		return nil, err
	}
	definitions, err := artifact.ScanProject(projectRoot)
	if err != nil {
		return nil, err
	}
	definitions = filterArtifactDefinitions(definitions, selectorValue)
	if len(definitions) == 0 {
		return nil, fmt.Errorf("no artifact matched %q", selector)
	}
	if len(targetKinds) == 0 {
		targetKinds = writer.SupportedTargetKinds()
	}
	for _, targetKind := range targetKinds {
		if _, exists := writer.SpecForKind(targetKind); !exists {
			return nil, fmt.Errorf("unknown target: %s", targetKind)
		}
	}
	inspections := make([]artifactInspection, 0, len(definitions))
	for _, definition := range definitions {
		diagnostics := writer.ValidateForKind(targetKinds[0], definition)
		plans := make([]artifact.CompatibilityPlan, 0, len(targetKinds))
		for _, targetKind := range targetKinds {
			plan := writer.PlanCompatibility(definition, targetKind)
			plan.Outputs = plannedDefinitionOutputs(definition, targetKind)
			if diagnoseTargets && plan.Level == artifact.CompatibilityUnsupported {
				diagnostics = append(diagnostics, artifact.Diagnostic{Severity: artifact.SeverityError, Code: "unsupported_target", Path: definition.SourcePath, Field: targetKind, Message: strings.Join(plan.Report.Warnings, "; ")})
			} else if diagnoseTargets && plan.Level == artifact.CompatibilityNativeOnly && !strings.HasPrefix(string(definition.SourceDialect), targetKind+".") {
				diagnostics = append(diagnostics, artifact.Diagnostic{Severity: artifact.SeverityError, Code: "native_only", Path: definition.SourcePath, Field: targetKind, Message: strings.Join(plan.Report.Warnings, "; ")})
			} else if diagnoseTargets && plan.Level == artifact.CompatibilityLossy {
				diagnostics = append(diagnostics, artifact.Diagnostic{Severity: artifact.SeverityWarning, Code: "lossy_conversion", Path: definition.SourcePath, Field: targetKind, Message: "conversion requires explicit approval"})
			}
			plans = append(plans, plan)
		}
		inspections = append(inspections, artifactInspection{Definition: definition, Diagnostics: diagnostics, Plans: plans})
	}
	return inspections, nil
}

func artifactProjectRootAndSelector(selector string) (string, string, error) {
	if strings.TrimSpace(selector) == "" {
		return ".", "", nil
	}
	info, err := os.Stat(selector)
	if err != nil {
		if os.IsNotExist(err) {
			return ".", selector, nil
		}
		return "", "", err
	}
	if info.IsDir() {
		return selector, "", nil
	}
	absolutePath, err := filepath.Abs(selector)
	if err != nil {
		return "", "", err
	}
	if resolvedPath, resolveErr := filepath.EvalSymlinks(absolutePath); resolveErr == nil {
		absolutePath = resolvedPath
	}
	workingRoot, err := filepath.Abs(".")
	if err != nil {
		return "", "", err
	}
	if resolvedRoot, resolveErr := filepath.EvalSymlinks(workingRoot); resolveErr == nil {
		workingRoot = resolvedRoot
	}
	relativePath, err := filepath.Rel(workingRoot, absolutePath)
	if err != nil || strings.HasPrefix(relativePath, "..") {
		return filepath.Dir(absolutePath), filepath.Base(absolutePath), nil
	}
	return workingRoot, filepath.ToSlash(relativePath), nil
}

func filterArtifactDefinitions(definitions []artifact.Definition, selector string) []artifact.Definition {
	if selector == "" {
		return definitions
	}
	filtered := make([]artifact.Definition, 0)
	for _, definition := range definitions {
		if definition.ID == selector || definition.Name == selector || definition.SourcePath == filepath.ToSlash(selector) {
			filtered = append(filtered, definition)
		}
	}
	return filtered
}

func plannedDefinitionOutputs(definition artifact.Definition, targetKind string) []artifact.PlannedOutput {
	target, exists := writer.WriterForKind(targetKind)
	if !exists {
		return nil
	}
	sourcePaths := make([]string, 0, len(definition.Files))
	for _, file := range definition.Files {
		sourcePaths = append(sourcePaths, file.Path)
	}
	plan := target.Plan(string(definition.Kind), definition.Name, "", sourcePaths)
	if plan.Skipped {
		return nil
	}
	outputs := make([]artifact.PlannedOutput, 0, len(plan.Files))
	for _, file := range plan.Files {
		outputs = append(outputs, artifact.PlannedOutput{Consumers: []string{targetKind}, Path: file.RelativePath})
	}
	if definition.Kind == artifact.KindInstruction {
		switch targetKind {
		case writer.KindCodex, writer.KindCopilot, writer.KindCursor:
			outputs = append(outputs, artifact.PlannedOutput{Consumers: []string{targetKind}, Path: "AGENTS.md"})
		case writer.KindGemini:
			outputs = append(outputs, artifact.PlannedOutput{Consumers: []string{targetKind}, Path: "GEMINI.md"})
		}
	}
	sort.Slice(outputs, func(leftIndex, rightIndex int) bool { return outputs[leftIndex].Path < outputs[rightIndex].Path })
	return outputs
}
