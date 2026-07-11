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
