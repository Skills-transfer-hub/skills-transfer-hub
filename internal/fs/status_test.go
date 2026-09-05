package fs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCommitQueryPath(t *testing.T) {
	cases := []struct {
		name         string
		sourcePath   string
		artifactKind string
		want         string
	}{
		{"skill bundle uses directory", "skills/alpha/SKILL.md", "skill", "skills/alpha"},
		{"skill by suffix without kind", "skills/alpha/SKILL.md", "", "skills/alpha"},
		{"single file resource uses file", "agents/beta.md", "agent", "agents/beta.md"},
		{"empty source path", "", "skill", ""},
		{"backslashes normalized", "skills\\alpha\\SKILL.md", "skill", "skills/alpha"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := CommitQueryPath(testCase.sourcePath, testCase.artifactKind); got != testCase.want {
				t.Fatalf("CommitQueryPath(%q, %q) = %q, want %q", testCase.sourcePath, testCase.artifactKind, got, testCase.want)
			}
		})
	}
}

func TestComputeResourceStatus_emptyResolvedVersionNotOutdated(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "SKILL.md")
	content := []byte("# Skill\n")
	if err := os.WriteFile(filePath, content, 0o644); err != nil {
		t.Fatal(err)
	}
	record := InstalledResourceRecordV2{
		ResourceName: "alpha",
		ManagedFiles: []ManagedFile{{Path: filePath, SHA256: ComputeSHA256(content)}},
	}
	status, _ := ComputeResourceStatus(record, "latest-sha")
	if status != ResourceStatusUpToDate {
		t.Fatalf("status = %q, want %q", status, ResourceStatusUpToDate)
	}
}

func TestComputeResourceStatus_outdatedWhenVersionsDiffer(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "SKILL.md")
	content := []byte("# Skill\n")
	if err := os.WriteFile(filePath, content, 0o644); err != nil {
		t.Fatal(err)
	}
	record := InstalledResourceRecordV2{
		ResourceName:    "alpha",
		ResolvedVersion: "installed-sha",
		ManagedFiles:    []ManagedFile{{Path: filePath, SHA256: ComputeSHA256(content)}},
	}
	status, _ := ComputeResourceStatus(record, "latest-sha")
	if status != ResourceStatusOutdated {
		t.Fatalf("status = %q, want %q", status, ResourceStatusOutdated)
	}
}

func TestComputeResourceStatusManagedRegionIgnoresUserContentOutsideRegion(t *testing.T) {
	directory := t.TempDir()
	filePath := filepath.Join(directory, "AGENTS.md")
	document, err := MergeManagedMarkdownRegion([]byte("User content\n"), "codex-instructions-v1", []byte("Managed content\n"))
	if err != nil {
		t.Fatal(err)
	}
	regionSHA256, err := ManagedMarkdownRegionSHA256(document, "codex-instructions-v1")
	if err != nil {
		t.Fatal(err)
	}
	changedOutsideRegion := append([]byte("Changed user content\n"), document[len("User content\n"):]...)
	if err := os.WriteFile(filePath, changedOutsideRegion, 0o644); err != nil {
		t.Fatal(err)
	}
	record := InstalledResourceRecordV2{
		ResourceName:   "instruction::rules::safe",
		ManagedRegions: []ManagedRegion{{Path: filePath, ID: "codex-instructions-v1", SHA256: regionSHA256}},
	}
	status, modifiedPaths := ComputeResourceStatus(record, "")
	if status != ResourceStatusUpToDate || len(modifiedPaths) != 0 {
		t.Fatalf("status = %q, modified = %v", status, modifiedPaths)
	}
}

func TestComputeResourceStatusManagedRegionDetectsRegionChange(t *testing.T) {
	directory := t.TempDir()
	filePath := filepath.Join(directory, "GEMINI.md")
	document, err := MergeManagedMarkdownRegion(nil, "gemini-instructions-v1", []byte("@./one.md\n"))
	if err != nil {
		t.Fatal(err)
	}
	regionSHA256, err := ManagedMarkdownRegionSHA256(document, "gemini-instructions-v1")
	if err != nil {
		t.Fatal(err)
	}
	changedDocument, err := ReplaceManagedMarkdownRegion(document, "gemini-instructions-v1", []byte("@./other.md\n"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, changedDocument, 0o644); err != nil {
		t.Fatal(err)
	}
	record := InstalledResourceRecordV2{
		ResourceName:   "instruction::rules::safe",
		ManagedRegions: []ManagedRegion{{Path: filePath, ID: "gemini-instructions-v1", SHA256: regionSHA256}},
	}
	status, modifiedPaths := ComputeResourceStatus(record, "")
	if status != ResourceStatusModifiedLocally || len(modifiedPaths) != 1 || modifiedPaths[0] != filePath {
		t.Fatalf("status = %q, modified = %v", status, modifiedPaths)
	}
}

func TestComputeResourceStatusManagedRegionDetectsMissingRegion(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "AGENTS.md")
	if err := os.WriteFile(filePath, []byte("User content only\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	record := InstalledResourceRecordV2{
		ResourceName:   "instruction::rules::safe",
		ManagedRegions: []ManagedRegion{{Path: filePath, ID: "codex-instructions-v1", SHA256: "missing"}},
	}
	status, modifiedPaths := ComputeResourceStatus(record, "")
	if status != ResourceStatusModifiedLocally || len(modifiedPaths) != 1 || modifiedPaths[0] != filePath {
		t.Fatalf("status = %q, modified = %v", status, modifiedPaths)
	}
}

func groupingResults() []StatusResult {
	return []StatusResult{
		{ResourceName: "agent::agents::shared", ProviderID: "internal", InstalledVersion: "abc", LatestVersion: "abc", Status: ResourceStatusUpToDate},
		{ResourceName: "agent::agents::shared", ProviderID: "internal", InstalledVersion: "abc", LatestVersion: "def", Status: ResourceStatusOutdated},
		{ResourceName: "agent::agents::shared", ProviderID: "internal", Status: ResourceStatusModifiedLocally, ModifiedFiles: []string{".codex/agents/shared.toml"}},
		{ResourceName: "agent::agents::shared", ProviderID: "public", InstalledVersion: "zzz", Status: ResourceStatusUpToDate},
		{ResourceName: "agent::agents::solo", ProviderID: "internal", InstalledVersion: "111", Status: ResourceStatusUpToDate},
	}
}

func TestGroupStatusesByResourceMergesTargets(t *testing.T) {
	grouped := GroupStatusesByResource(groupingResults())

	if len(grouped) != 3 {
		t.Fatalf("grouped = %+v, want one entry per (resource, provider)", grouped)
	}
	if grouped[0].ResourceName != "agent::agents::shared" || grouped[0].ProviderID != "internal" {
		t.Fatalf("first entry = %+v, want the internal shared resource first", grouped[0])
	}
	if grouped[0].Status != ResourceStatusModifiedLocally {
		t.Fatalf("merged status = %q, want the most severe (%q)", grouped[0].Status, ResourceStatusModifiedLocally)
	}
	if grouped[0].InstalledVersion != "abc" || grouped[0].LatestVersion != "abc" {
		t.Fatalf("merged versions = %q/%q, want the first non-empty ones", grouped[0].InstalledVersion, grouped[0].LatestVersion)
	}
	if len(grouped[0].ModifiedFiles) != 1 || grouped[0].ModifiedFiles[0] != ".codex/agents/shared.toml" {
		t.Fatalf("merged modified files = %v", grouped[0].ModifiedFiles)
	}
	if grouped[1].ProviderID != "public" {
		t.Fatalf("second entry = %+v, want the same resource from another provider kept apart", grouped[1])
	}
	if grouped[2].ResourceName != "agent::agents::solo" {
		t.Fatalf("third entry = %+v", grouped[2])
	}
}

func TestGroupStatusesByResourceKeepsPinAndDeduplicatesFiles(t *testing.T) {
	grouped := GroupStatusesByResource([]StatusResult{
		{ResourceName: "skill::skills::alpha", ProviderID: "internal", Status: ResourceStatusModifiedLocally, ModifiedFiles: []string{"a.md"}},
		{ResourceName: "skill::skills::alpha", ProviderID: "internal", Status: ResourceStatusModifiedLocally, ModifiedFiles: []string{"a.md", "b.md"}, Pinned: true, PinnedVersion: "v1"},
	})

	if len(grouped) != 1 {
		t.Fatalf("grouped = %+v, want a single entry", grouped)
	}
	if !grouped[0].Pinned || grouped[0].PinnedVersion != "v1" {
		t.Fatalf("pin = %v/%q, want the pin to survive the merge", grouped[0].Pinned, grouped[0].PinnedVersion)
	}
	if len(grouped[0].ModifiedFiles) != 2 {
		t.Fatalf("modified files = %v, want a deduplicated union", grouped[0].ModifiedFiles)
	}
}

func TestGroupStatusesByResourceOnEmptyInput(t *testing.T) {
	if grouped := GroupStatusesByResource(nil); len(grouped) != 0 {
		t.Fatalf("grouped = %+v, want empty", grouped)
	}
}
