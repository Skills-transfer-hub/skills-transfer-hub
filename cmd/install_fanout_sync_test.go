package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"sth/internal/fs"
)

type artifactSynchronizationCase struct {
	name             string
	artifactKind     string
	artifactName     string
	folderName       string
	expectedByTarget map[string][]string
	expectedRegions  map[string]fs.ManagedRegion
	rootAssertions   map[string][]string
	changingRoots    []string
	forbiddenPaths   []string
}

func TestArtifactSynchronizationAcrossAllConsumers(t *testing.T) {
	testCases := []artifactSynchronizationCase{
		{
			name:         "agent",
			artifactKind: "agent",
			artifactName: "synchronized-agent",
			folderName:   "agents",
			expectedByTarget: map[string][]string{
				"claude":      {".claude/agents/synchronized-agent.md"},
				"codex":       {".codex/agents/synchronized-agent.toml"},
				"copilot":     {".github/agents/synchronized-agent.agent.md"},
				"antigravity": {".agents/agents/synchronized-agent.md"},
				"gemini":      {".gemini/agents/synchronized-agent.md"},
			},
			forbiddenPaths: []string{
				".codex/agents/synchronized-agent.md",
				".codex/agents/synchronized-agent.agent.md",
			},
		},
		{
			name:         "command",
			artifactKind: "command",
			artifactName: "synchronized-command",
			folderName:   "commands",
			expectedByTarget: map[string][]string{
				"claude": {".claude/commands/synchronized-command.md"},
				"codex":  {".agents/skills/synchronized-command/SKILL.md"},
				"copilot": {
					".claude/commands/synchronized-command.md",
					".github/prompts/synchronized-command.prompt.md",
				},
				"antigravity": {".agents/workflows/synchronized-command.md"},
				"gemini":      {".gemini/commands/synchronized-command.toml"},
			},
			forbiddenPaths: []string{
				".codex/commands/synchronized-command.md",
			},
		},
		{
			name:         "instruction",
			artifactKind: "instruction",
			artifactName: "synchronized-instruction",
			folderName:   "instructions",
			expectedByTarget: map[string][]string{
				"claude":      {".claude/rules/synchronized-instruction.md"},
				"codex":       {".codex/sth/instructions/synchronized-instruction.md"},
				"copilot":     {".github/instructions/synchronized-instruction.instructions.md"},
				"antigravity": {".agents/rules/synchronized-instruction.md"},
				"gemini":      {".gemini/sth/instructions/synchronized-instruction.md"},
			},
			expectedRegions: map[string]fs.ManagedRegion{
				"codex":  {Path: "AGENTS.md", ID: codexInstructionRegionID},
				"gemini": {Path: "GEMINI.md", ID: geminiInstructionRegionID},
			},
			rootAssertions: map[string][]string{
				"AGENTS.md": {codexInstructionRegionID, "SENTINEL_V1"},
				"GEMINI.md": {geminiInstructionRegionID, "@./.gemini/sth/instructions/synchronized-instruction.md"},
			},
			changingRoots: []string{"AGENTS.md"},
			forbiddenPaths: []string{
				".codex/instructions/synchronized-instruction.md",
				".gemini/instructions/synchronized-instruction.md",
				".gemini/rules/synchronized-instruction.md",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			projectDir := t.TempDir()
			fixture := newFanoutLifecycleFixture(t, testCase.artifactKind, testCase.artifactName, testCase.folderName)
			t.Setenv("STH_GITHUB_RAW_BASE_URL", fixture.server.URL)
			t.Setenv("STH_GITHUB_API_BASE_URL", fixture.server.URL)
			writeProjectConfigV2ForTest(t, projectDir, fanoutConfig(
				fanoutTarget("claude", ".claude"),
				fanoutTarget("codex", ".codex"),
				fanoutTarget("copilot", ".github"),
				fanoutTarget("antigravity", ".agents"),
				fanoutTarget("gemini", ".gemini"),
			))

			installResult := runCLI(t, projectDir, t.TempDir(), "install", testCase.artifactName)
			if installResult.err != nil {
				t.Fatalf("install error = %v\nstderr=%s", installResult.err, installResult.stderr)
			}
			assertSynchronizedDestinations(t, projectDir, testCase, "SENTINEL_V1", "")
			assertSynchronizedState(t, projectDir, testCase, "1111111111111111", "")

			fixture.setVersion("SENTINEL_V2", "2222222222222222")
			beforeDryRun := transactionalProjectFileSnapshot(t, projectDir)
			dryRunResult := runCLI(t, projectDir, t.TempDir(), "update", "--dry-run", "--json")
			if dryRunResult.err != nil {
				t.Fatalf("dry-run error = %v\nstdout=%s\nstderr=%s", dryRunResult.err, dryRunResult.stdout, dryRunResult.stderr)
			}
			assertDryRunChanges(t, dryRunResult.stdout, synchronizationDryRunPaths(testCase))
			afterDryRun := transactionalProjectFileSnapshot(t, projectDir)
			if !reflect.DeepEqual(afterDryRun, beforeDryRun) {
				t.Fatalf("dry-run mutated project files: before=%v after=%v", beforeDryRun, afterDryRun)
			}

			updateResult := runCLI(t, projectDir, t.TempDir(), "update")
			if updateResult.err != nil {
				t.Fatalf("update error = %v\nstderr=%s", updateResult.err, updateResult.stderr)
			}
			assertSynchronizedDestinations(t, projectDir, testCase, "SENTINEL_V2", "SENTINEL_V1")
			assertSynchronizedState(t, projectDir, testCase, "2222222222222222", "1111111111111111")

			statusResult := runCLI(t, projectDir, t.TempDir(), "status", "--json")
			if statusResult.err != nil {
				t.Fatalf("status error = %v\nstdout=%s\nstderr=%s", statusResult.err, statusResult.stdout, statusResult.stderr)
			}
			assertSynchronizedStatus(t, statusResult.stdout, testCase, "2222222222222222")

			resourceName := fs.CatalogID(testCase.artifactKind, testCase.folderName, testCase.artifactName)
			removeResult := runCLI(t, projectDir, t.TempDir(), "remove", resourceName, "--yes")
			if removeResult.err != nil {
				t.Fatalf("remove error = %v\nstderr=%s", removeResult.err, removeResult.stderr)
			}
			for _, relativePath := range uniqueSynchronizationPaths(testCase.expectedByTarget) {
				assertNoFile(t, filepath.Join(projectDir, filepath.FromSlash(relativePath)))
			}
			for rootPath := range testCase.rootAssertions {
				assertNoFile(t, filepath.Join(projectDir, filepath.FromSlash(rootPath)))
			}
			for _, relativePath := range testCase.forbiddenPaths {
				assertNoFile(t, filepath.Join(projectDir, filepath.FromSlash(relativePath)))
			}
			if entries := readStateV2(t, projectDir).Entries; len(entries) != 0 {
				t.Fatalf("state entries after remove = %+v", entries)
			}
		})
	}
}

func assertSynchronizedDestinations(t *testing.T, projectDir string, testCase artifactSynchronizationCase, expectedSentinel, forbiddenSentinel string) {
	t.Helper()
	for _, relativePath := range uniqueSynchronizationPaths(testCase.expectedByTarget) {
		installedPath := filepath.Join(projectDir, filepath.FromSlash(relativePath))
		content, err := os.ReadFile(installedPath)
		if err != nil {
			t.Fatalf("read %s: %v", relativePath, err)
		}
		if !strings.Contains(string(content), expectedSentinel) {
			t.Errorf("%s does not contain %s: %s", relativePath, expectedSentinel, content)
		}
		if forbiddenSentinel != "" && strings.Contains(string(content), forbiddenSentinel) {
			t.Errorf("%s still contains %s: %s", relativePath, forbiddenSentinel, content)
		}
	}
	for _, relativePath := range testCase.forbiddenPaths {
		assertNoFile(t, filepath.Join(projectDir, filepath.FromSlash(relativePath)))
	}
	for rootPath, expectedParts := range testCase.rootAssertions {
		content, err := os.ReadFile(filepath.Join(projectDir, filepath.FromSlash(rootPath)))
		if err != nil {
			t.Fatalf("read %s: %v", rootPath, err)
		}
		for _, expectedPart := range expectedParts {
			effectiveExpectedPart := expectedPart
			if expectedPart == "SENTINEL_V1" {
				effectiveExpectedPart = expectedSentinel
			}
			if !strings.Contains(string(content), effectiveExpectedPart) {
				t.Errorf("%s does not contain %s: %s", rootPath, effectiveExpectedPart, content)
			}
		}
		if forbiddenSentinel != "" && strings.Contains(string(content), forbiddenSentinel) {
			t.Errorf("%s still contains %s: %s", rootPath, forbiddenSentinel, content)
		}
	}
}

func assertSynchronizedState(t *testing.T, projectDir string, testCase artifactSynchronizationCase, resolvedVersion, previousVersion string) {
	t.Helper()
	state := readStateV2(t, projectDir)
	if len(state.Entries) != len(testCase.expectedByTarget) {
		t.Fatalf("state entries = %d, want %d: %+v", len(state.Entries), len(testCase.expectedByTarget), state.Entries)
	}
	recordsByTarget := make(map[string]fs.InstalledResourceRecordV2, len(state.Entries))
	managedPaths := make(map[string]struct{})
	for _, record := range state.Entries {
		recordsByTarget[record.TargetID] = record
		if record.ArtifactKind != testCase.artifactKind {
			t.Errorf("target %s artifact kind = %q, want %q", record.TargetID, record.ArtifactKind, testCase.artifactKind)
		}
		if record.ResolvedVersion != resolvedVersion || record.PreviousVersion != previousVersion {
			t.Errorf("target %s versions = %q/%q, want %q/%q", record.TargetID, record.ResolvedVersion, record.PreviousVersion, resolvedVersion, previousVersion)
		}
		for _, managedFile := range record.ManagedFiles {
			managedPaths[managedFile.Path] = struct{}{}
			content, err := os.ReadFile(filepath.Join(projectDir, filepath.FromSlash(managedFile.Path)))
			if err != nil {
				t.Errorf("target %s managed path %s: %v", record.TargetID, managedFile.Path, err)
				continue
			}
			if managedFile.SHA256 != fs.ComputeSHA256(content) {
				t.Errorf("target %s state hash does not match physical file %s", record.TargetID, managedFile.Path)
			}
		}
	}
	for targetKind, expectedPaths := range testCase.expectedByTarget {
		record, found := recordsByTarget[targetKind]
		if !found {
			t.Errorf("state has no record for target %s", targetKind)
			continue
		}
		if len(record.ManagedFiles) != len(expectedPaths) {
			t.Errorf("target %s managed files = %+v, want %v", targetKind, record.ManagedFiles, expectedPaths)
			continue
		}
		actualPaths := make(map[string]struct{}, len(record.ManagedFiles))
		for _, managedFile := range record.ManagedFiles {
			actualPaths[managedFile.Path] = struct{}{}
		}
		for _, expectedPath := range expectedPaths {
			if _, found := actualPaths[expectedPath]; !found {
				t.Errorf("target %s has no managed path %q: %+v", targetKind, expectedPath, record.ManagedFiles)
			}
		}
		expectedRegion, hasExpectedRegion := testCase.expectedRegions[targetKind]
		if !hasExpectedRegion {
			if len(record.ManagedRegions) != 0 {
				t.Errorf("target %s managed regions = %+v, want none", targetKind, record.ManagedRegions)
			}
			continue
		}
		if len(record.ManagedRegions) != 1 {
			t.Errorf("target %s managed regions = %+v, want one", targetKind, record.ManagedRegions)
			continue
		}
		actualRegion := record.ManagedRegions[0]
		if actualRegion.Path != expectedRegion.Path || actualRegion.ID != expectedRegion.ID {
			t.Errorf("target %s managed region = %+v, want %+v", targetKind, actualRegion, expectedRegion)
			continue
		}
		document, err := os.ReadFile(filepath.Join(projectDir, filepath.FromSlash(actualRegion.Path)))
		if err != nil {
			t.Errorf("target %s managed region path %s: %v", targetKind, actualRegion.Path, err)
			continue
		}
		regionSHA256, err := fs.ManagedMarkdownRegionSHA256(document, actualRegion.ID)
		if err != nil || actualRegion.SHA256 != regionSHA256 {
			t.Errorf("target %s managed region SHA = %s, want %s, error=%v", targetKind, actualRegion.SHA256, regionSHA256, err)
		}
	}
	expectedUniquePaths := uniqueSynchronizationPaths(testCase.expectedByTarget)
	if len(managedPaths) != len(expectedUniquePaths) {
		t.Errorf("unique managed paths = %d, want %d: %v", len(managedPaths), len(expectedUniquePaths), managedPaths)
	}
}

func assertDryRunChanges(t *testing.T, output string, expectedPaths []string) {
	t.Helper()
	var changes []fs.DryRunChange
	jsonOutput := output
	if err := json.Unmarshal([]byte(jsonOutput), &changes); err != nil {
		t.Errorf("dry-run output is not valid JSON: %v\n%s", err, output)
		arrayStart := strings.Index(output, "[")
		if arrayStart < 0 {
			return
		}
		jsonOutput = output[arrayStart:]
		if fallbackErr := json.Unmarshal([]byte(jsonOutput), &changes); fallbackErr != nil {
			t.Errorf("dry-run output contains no valid JSON array: %v\n%s", fallbackErr, output)
			return
		}
	}
	if len(changes) != len(expectedPaths) {
		t.Errorf("dry-run changes = %d, want one per physical path (%d): %+v", len(changes), len(expectedPaths), changes)
	}
	changedPaths := make(map[string]struct{}, len(changes))
	for _, change := range changes {
		if change.Action != "modify" {
			t.Errorf("dry-run action for %s = %q, want modify", change.Path, change.Action)
		}
		if _, duplicate := changedPaths[change.Path]; duplicate {
			t.Errorf("dry-run reports physical path %s more than once", change.Path)
		}
		changedPaths[change.Path] = struct{}{}
	}
	for _, expectedPath := range expectedPaths {
		if _, found := changedPaths[expectedPath]; !found {
			t.Errorf("dry-run has no change for %s: %+v", expectedPath, changes)
		}
	}
}

func assertSynchronizedStatus(t *testing.T, output string, testCase artifactSynchronizationCase, expectedVersion string) {
	t.Helper()
	var statuses []fs.StatusResult
	if err := json.Unmarshal([]byte(output), &statuses); err != nil {
		t.Fatalf("status output is not valid JSON: %v\n%s", err, output)
	}
	if len(statuses) != 1 {
		t.Fatalf("status entries = %d, want 1 grouped entry for the %d targets: %+v", len(statuses), len(testCase.expectedByTarget), statuses)
	}
	expectedResourceName := fs.CatalogID(testCase.artifactKind, testCase.folderName, testCase.artifactName)
	for _, status := range statuses {
		if status.ResourceName != expectedResourceName {
			t.Errorf("status resource = %q, want %q", status.ResourceName, expectedResourceName)
		}
		if status.InstalledVersion != expectedVersion || status.LatestVersion != expectedVersion {
			t.Errorf("status versions = %q/%q, want %q", status.InstalledVersion, status.LatestVersion, expectedVersion)
		}
		if status.Status != fs.ResourceStatusUpToDate {
			t.Errorf("status = %q, want %q: %+v", status.Status, fs.ResourceStatusUpToDate, status)
		}
	}
}

func uniqueSynchronizationPaths(expectedByTarget map[string][]string) []string {
	paths := make([]string, 0, len(expectedByTarget))
	seenPaths := make(map[string]struct{}, len(expectedByTarget))
	for _, expectedPaths := range expectedByTarget {
		for _, expectedPath := range expectedPaths {
			if _, seen := seenPaths[expectedPath]; seen {
				continue
			}
			seenPaths[expectedPath] = struct{}{}
			paths = append(paths, expectedPath)
		}
	}
	return paths
}

func synchronizationDryRunPaths(testCase artifactSynchronizationCase) []string {
	paths := uniqueSynchronizationPaths(testCase.expectedByTarget)
	paths = append(paths, testCase.changingRoots...)
	return paths
}
