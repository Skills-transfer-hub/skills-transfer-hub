package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sth/internal/fs"
)

func TestJSONOutput_VersionEmitsObject(t *testing.T) {
	res := runCLI(t, "", "", "version", "--json")
	if res.err != nil {
		t.Fatalf("err = %v", res.err)
	}
	var payload struct {
		Version string `json:"version"`
		Commit  string `json:"commit"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &payload); err != nil {
		t.Fatalf("unmarshal: %v\nstdout=%s", err, res.stdout)
	}
	if payload.Version == "" {
		t.Fatalf("version empty in payload: %q", res.stdout)
	}
	if strings.Contains(res.stdout, versionByline) {
		t.Fatalf("--json leaked human-readable byline: %q", res.stdout)
	}
}

func TestJSONOutput_VersionTextDefault(t *testing.T) {
	res := runCLI(t, "", "", "version")
	if res.err != nil {
		t.Fatalf("err = %v", res.err)
	}
	if !strings.Contains(res.stdout, "STH") {
		t.Fatalf("text version missing: %q", res.stdout)
	}
}

func TestJSONOutput_ListEmitsInstalledArray(t *testing.T) {
	projectDir := t.TempDir()
	targetDir := ".claude/resources"
	writeProjectConfigV2ForTest(t, projectDir, fs.ProjectConfigV2{
		SchemaVersion: 2,
		Providers: []fs.ProviderEntry{
			{ID: "main", Provider: "github", RepositoryID: "owner/repo", CatalogRef: "main", CatalogPath: "SKILLS.md", TargetDir: targetDir},
		},
	})
	writeV2StateForStatusTest(t, projectDir, targetDir, fs.InstalledResourceStateV2{
		SchemaVersion: 2,
		Entries: []fs.InstalledResourceRecordV2{
			{
				ResourceName:    "alpha",
				ProviderID:      "main",
				ResolvedVersion: "abc",
				ManagedFiles:    []fs.ManagedFile{},
			},
		},
	})
	res := runCLI(t, projectDir, t.TempDir(), "list", "--json")
	if res.err != nil {
		t.Fatalf("err = %v\nstderr=%s", res.err, res.stderr)
	}
	var entries []listJSONEntry
	if err := json.Unmarshal([]byte(res.stdout), &entries); err != nil {
		t.Fatalf("unmarshal: %v\nstdout=%s", err, res.stdout)
	}
	if len(entries) != 1 {
		t.Fatalf("len=%d want 1", len(entries))
	}
	if entries[0].ResourceName != "alpha" || entries[0].ProviderID != "main" {
		t.Fatalf("unexpected entry: %+v", entries[0])
	}
	if entries[0].ResolvedVersion != "abc" {
		t.Fatalf("ResolvedVersion=%q want abc", entries[0].ResolvedVersion)
	}
}

func TestJSONOutput_ListEmptyArrayWhenNoProviders(t *testing.T) {
	projectDir := t.TempDir()
	writeProjectConfigV2ForTest(t, projectDir, fs.ProjectConfigV2{SchemaVersion: 2})
	res := runCLI(t, projectDir, t.TempDir(), "list", "--json")
	if res.err != nil {
		t.Fatalf("err = %v", res.err)
	}
	if strings.TrimSpace(res.stdout) != "[]" {
		t.Fatalf("stdout=%q want []", res.stdout)
	}
}

func TestJSONOutput_ListErrorEmitsErrorObject(t *testing.T) {
	// Corrupt project.json — list --json should emit {"error":...,"code":1}.
	projectDir := t.TempDir()
	dotSth := filepath.Join(projectDir, ".sth")
	if err := os.MkdirAll(dotSth, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dotSth, "project.json"), []byte("{not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := runCLI(t, projectDir, t.TempDir(), "list", "--json")
	if res.err == nil {
		t.Fatal("expected non-zero exit")
	}
	var payload struct {
		Error string `json:"error"`
		Code  int    `json:"code"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &payload); err != nil {
		t.Fatalf("unmarshal: %v\nstdout=%s", err, res.stdout)
	}
	if payload.Code != 1 {
		t.Fatalf("code=%d want 1", payload.Code)
	}
	if payload.Error == "" {
		t.Fatal("error empty")
	}
}
