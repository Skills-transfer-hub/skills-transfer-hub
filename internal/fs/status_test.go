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
