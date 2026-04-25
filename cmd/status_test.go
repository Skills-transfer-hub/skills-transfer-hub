package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"loreskills/internal/fs"

	stdhttp "net/http"
	"net/http/httptest"
)

// writeV2StateForStatusTest writes a v2 state file directly into the target dir.
func writeV2StateForStatusTest(t *testing.T, projectDir, targetDir string, state fs.InstalledResourceStateV2) {
	t.Helper()
	targetPath := filepath.Join(projectDir, filepath.FromSlash(targetDir))
	if err := os.MkdirAll(targetPath, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", targetPath, err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent(v2 state): %v", err)
	}
	if err := os.WriteFile(filepath.Join(targetPath, "lore-state-v2.json"), data, 0o644); err != nil {
		t.Fatalf("WriteFile(lore-state-v2.json): %v", err)
	}
}

func TestRunStatus_noResourcesInstalledMessage(t *testing.T) {
	projectDir := t.TempDir()
	writeProjectProfileForCommandTest(t, projectDir, fs.ProjectProfile{
		Provider:     "github",
		RepositoryID: "owner/repo",
		Ref:          "main",
		CatalogPath:  "SKILLS.md",
		TargetDir:    ".claude/resources",
	})
	result := runCLI(t, projectDir, t.TempDir(), "status")
	if result.err != nil {
		t.Fatalf("Run() error = %v", result.err)
	}
	assertContains(t, result.stdout, "No resources installed.")
}

func TestRunStatus_textOutput(t *testing.T) {
	projectDir := t.TempDir()
	targetDir := ".claude/resources"
	targetPath := filepath.Join(projectDir, filepath.FromSlash(targetDir))
	if err := os.MkdirAll(targetPath, 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte("# Guide\n")
	filePath := filepath.Join(targetPath, "skills", "my-skill.md")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, content, 0o644); err != nil {
		t.Fatal(err)
	}
	diskSHA := fs.ComputeSHA256(content)
	commitSHA := "abc1234567890abc1234567890"

	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if strings.Contains(r.URL.Path, "/commits/") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"sha":"` + commitSHA + `"}`))
			return
		}
		stdhttp.NotFound(w, r)
	}))
	defer server.Close()
	t.Setenv("LORESKILLS_GITHUB_API_BASE_URL", server.URL)

	writeProjectConfigV2ForTest(t, projectDir, fs.ProjectConfigV2{
		SchemaVersion: 2,
		Providers: []fs.ProviderEntry{
			{ID: "main", Provider: "github", RepositoryID: "owner/repo", CatalogRef: "main", CatalogPath: "SKILLS.md", TargetDir: targetDir},
		},
		Resources: []fs.ResourceEntry{},
	})
	writeV2StateForStatusTest(t, projectDir, targetDir, fs.InstalledResourceStateV2{
		SchemaVersion: 2,
		Entries: []fs.InstalledResourceRecordV2{
			{
				ResourceName:    "my-skill",
				ProviderID:      "main",
				ResolvedVersion: commitSHA,
				ManagedFiles:    []fs.ManagedFile{{Path: filepath.ToSlash(filePath), SHA256: diskSHA}},
			},
		},
	})

	result := runCLI(t, projectDir, t.TempDir(), "status")
	if result.err != nil {
		t.Fatalf("Run() error = %v\nstderr=%s", result.err, result.stderr)
	}
	assertContains(t, result.stdout, "my-skill")
	assertContains(t, result.stdout, "up-to-date")
}

func TestRunStatus_JSONOutput(t *testing.T) {
	projectDir := t.TempDir()
	targetDir := ".claude/resources"
	targetPath := filepath.Join(projectDir, filepath.FromSlash(targetDir))
	if err := os.MkdirAll(targetPath, 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte("# Guide\n")
	filePath := filepath.Join(targetPath, "skills", "my-skill.md")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, content, 0o644); err != nil {
		t.Fatal(err)
	}
	diskSHA := fs.ComputeSHA256(content)
	commitSHA := "abc123"

	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if strings.Contains(r.URL.Path, "/commits/") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"sha":"` + commitSHA + `"}`))
			return
		}
		stdhttp.NotFound(w, r)
	}))
	defer server.Close()
	t.Setenv("LORESKILLS_GITHUB_API_BASE_URL", server.URL)

	writeProjectConfigV2ForTest(t, projectDir, fs.ProjectConfigV2{
		SchemaVersion: 2,
		Providers: []fs.ProviderEntry{
			{ID: "main", Provider: "github", RepositoryID: "owner/repo", CatalogRef: "main", CatalogPath: "SKILLS.md", TargetDir: targetDir},
		},
		Resources: []fs.ResourceEntry{},
	})
	writeV2StateForStatusTest(t, projectDir, targetDir, fs.InstalledResourceStateV2{
		SchemaVersion: 2,
		Entries: []fs.InstalledResourceRecordV2{
			{
				ResourceName:    "my-skill",
				ProviderID:      "main",
				ResolvedVersion: commitSHA,
				ManagedFiles:    []fs.ManagedFile{{Path: filepath.ToSlash(filePath), SHA256: diskSHA}},
			},
		},
	})

	result := runCLI(t, projectDir, t.TempDir(), "status", "--json")
	if result.err != nil {
		t.Fatalf("Run() error = %v\nstderr=%s", result.err, result.stderr)
	}

	var results []fs.StatusResult
	if err := json.Unmarshal([]byte(result.stdout), &results); err != nil {
		t.Fatalf("json.Unmarshal: %v\nstdout=%s", err, result.stdout)
	}
	if len(results) != 1 {
		t.Fatalf("len(results)=%d want 1", len(results))
	}
	if results[0].ResourceName != "my-skill" {
		t.Errorf("ResourceName=%q want %q", results[0].ResourceName, "my-skill")
	}
	if results[0].Status != fs.StatusUpToDate {
		t.Errorf("Status=%q want %q", results[0].Status, fs.StatusUpToDate)
	}
}

func TestRunStatus_pinnedAnnotation(t *testing.T) {
	projectDir := t.TempDir()
	targetDir := ".claude/resources"
	targetPath := filepath.Join(projectDir, filepath.FromSlash(targetDir))
	if err := os.MkdirAll(targetPath, 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte("# Guide\n")
	filePath := filepath.Join(targetPath, "skills", "pinned-skill.md")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, content, 0o644); err != nil {
		t.Fatal(err)
	}
	diskSHA := fs.ComputeSHA256(content)
	commitSHA := "latest-sha"

	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if strings.Contains(r.URL.Path, "/commits/") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"sha":"` + commitSHA + `"}`))
			return
		}
		stdhttp.NotFound(w, r)
	}))
	defer server.Close()
	t.Setenv("LORESKILLS_GITHUB_API_BASE_URL", server.URL)

	writeProjectConfigV2ForTest(t, projectDir, fs.ProjectConfigV2{
		SchemaVersion: 2,
		Providers: []fs.ProviderEntry{
			{ID: "main", Provider: "github", RepositoryID: "owner/repo", CatalogRef: "main", CatalogPath: "SKILLS.md", TargetDir: targetDir},
		},
		Resources: []fs.ResourceEntry{
			{ResourceName: "pinned-skill", ProviderID: "main", PinnedVersion: "v1.0.0"},
		},
	})
	writeV2StateForStatusTest(t, projectDir, targetDir, fs.InstalledResourceStateV2{
		SchemaVersion: 2,
		Entries: []fs.InstalledResourceRecordV2{
			{
				ResourceName:    "pinned-skill",
				ProviderID:      "main",
				ResolvedVersion: commitSHA,
				ManagedFiles:    []fs.ManagedFile{{Path: filepath.ToSlash(filePath), SHA256: diskSHA}},
			},
		},
	})

	result := runCLI(t, projectDir, t.TempDir(), "status")
	if result.err != nil {
		t.Fatalf("Run() error = %v\nstderr=%s", result.err, result.stderr)
	}
	assertContains(t, result.stdout, "pinned-skill")
	assertContains(t, result.stdout, "pinned at v1.0.0")
}

// T017: Faster status — commit SHA replaces per-resource content download.
func TestRunStatus_pinnedNewerVersionNotice(t *testing.T) {
	projectDir := t.TempDir()
	targetDir := ".claude/resources"
	targetPath := filepath.Join(projectDir, filepath.FromSlash(targetDir))
	if err := os.MkdirAll(targetPath, 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte("# Guide\n")
	filePath := filepath.Join(targetPath, "skills", "pinned-skill.md")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, content, 0o644); err != nil {
		t.Fatal(err)
	}
	diskSHA := fs.ComputeSHA256(content)
	installedSHA := "old-commit-sha"
	latestSHA := "new-commit-sha-abc"

	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if strings.Contains(r.URL.Path, "/commits/") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"sha":"` + latestSHA + `"}`))
			return
		}
		stdhttp.NotFound(w, r)
	}))
	defer server.Close()
	t.Setenv("LORESKILLS_GITHUB_API_BASE_URL", server.URL)

	writeProjectConfigV2ForTest(t, projectDir, fs.ProjectConfigV2{
		SchemaVersion: 2,
		Providers: []fs.ProviderEntry{
			{ID: "main", Provider: "github", RepositoryID: "owner/repo", CatalogRef: "main", CatalogPath: "SKILLS.md", TargetDir: targetDir},
		},
		Resources: []fs.ResourceEntry{
			{ResourceName: "pinned-skill", ProviderID: "main", PinnedVersion: "v1.0.0"},
		},
	})
	writeV2StateForStatusTest(t, projectDir, targetDir, fs.InstalledResourceStateV2{
		SchemaVersion: 2,
		Entries: []fs.InstalledResourceRecordV2{
			{
				ResourceName:    "pinned-skill",
				ProviderID:      "main",
				ResolvedVersion: installedSHA,
				ManagedFiles:    []fs.ManagedFile{{Path: filepath.ToSlash(filePath), SHA256: diskSHA}},
			},
		},
	})

	result := runCLI(t, projectDir, t.TempDir(), "status")
	if result.err != nil {
		t.Fatalf("Run() error = %v\nstderr=%s", result.err, result.stderr)
	}
	assertContains(t, result.stdout, "pinned-skill")
	assertContains(t, result.stdout, "Newer version available")
	assertContains(t, result.stdout, "v1.0.0")
}

func TestRunStatus_pinnedJSONHasPinnedFields(t *testing.T) {
	projectDir := t.TempDir()
	targetDir := ".claude/resources"
	targetPath := filepath.Join(projectDir, filepath.FromSlash(targetDir))
	if err := os.MkdirAll(targetPath, 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte("# Guide\n")
	filePath := filepath.Join(targetPath, "skills", "pinned-skill.md")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, content, 0o644); err != nil {
		t.Fatal(err)
	}
	diskSHA := fs.ComputeSHA256(content)
	commitSHA := "abc123"

	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if strings.Contains(r.URL.Path, "/commits/") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"sha":"` + commitSHA + `"}`))
			return
		}
		stdhttp.NotFound(w, r)
	}))
	defer server.Close()
	t.Setenv("LORESKILLS_GITHUB_API_BASE_URL", server.URL)

	writeProjectConfigV2ForTest(t, projectDir, fs.ProjectConfigV2{
		SchemaVersion: 2,
		Providers: []fs.ProviderEntry{
			{ID: "main", Provider: "github", RepositoryID: "owner/repo", CatalogRef: "main", CatalogPath: "SKILLS.md", TargetDir: targetDir},
		},
		Resources: []fs.ResourceEntry{
			{ResourceName: "pinned-skill", ProviderID: "main", PinnedVersion: "v2.3.0"},
		},
	})
	writeV2StateForStatusTest(t, projectDir, targetDir, fs.InstalledResourceStateV2{
		SchemaVersion: 2,
		Entries: []fs.InstalledResourceRecordV2{
			{
				ResourceName:    "pinned-skill",
				ProviderID:      "main",
				ResolvedVersion: commitSHA,
				ManagedFiles:    []fs.ManagedFile{{Path: filepath.ToSlash(filePath), SHA256: diskSHA}},
			},
		},
	})

	result := runCLI(t, projectDir, t.TempDir(), "status", "--json")
	if result.err != nil {
		t.Fatalf("Run() error = %v\nstderr=%s", result.err, result.stderr)
	}

	var results []fs.StatusResult
	if err := json.Unmarshal([]byte(result.stdout), &results); err != nil {
		t.Fatalf("json.Unmarshal: %v\nstdout=%s", err, result.stdout)
	}
	if len(results) != 1 {
		t.Fatalf("len(results)=%d want 1", len(results))
	}
	if !results[0].Pinned {
		t.Errorf("Pinned=%v want true", results[0].Pinned)
	}
	if results[0].PinnedVersion != "v2.3.0" {
		t.Errorf("PinnedVersion=%q want %q", results[0].PinnedVersion, "v2.3.0")
	}
}

func TestRunStatus_networkErrorExitCode2(t *testing.T) {
	projectDir := t.TempDir()
	targetDir := ".claude/resources"
	targetPath := filepath.Join(projectDir, filepath.FromSlash(targetDir))
	if err := os.MkdirAll(targetPath, 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte("# Guide\n")
	filePath := filepath.Join(targetPath, "skills", "my-skill.md")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, content, 0o644); err != nil {
		t.Fatal(err)
	}
	diskSHA := fs.ComputeSHA256(content)

	t.Setenv("LORESKILLS_GITHUB_API_BASE_URL", "http://127.0.0.1:19999")

	writeProjectConfigV2ForTest(t, projectDir, fs.ProjectConfigV2{
		SchemaVersion: 2,
		Providers: []fs.ProviderEntry{
			{ID: "main", Provider: "github", RepositoryID: "owner/repo", CatalogRef: "main", CatalogPath: "SKILLS.md", TargetDir: targetDir},
		},
		Resources: []fs.ResourceEntry{},
	})
	writeV2StateForStatusTest(t, projectDir, targetDir, fs.InstalledResourceStateV2{
		SchemaVersion: 2,
		Entries: []fs.InstalledResourceRecordV2{
			{
				ResourceName:    "my-skill",
				ProviderID:      "main",
				ResolvedVersion: "some-sha",
				ManagedFiles:    []fs.ManagedFile{{Path: filepath.ToSlash(filePath), SHA256: diskSHA}},
			},
		},
	})

	result := runCLI(t, projectDir, t.TempDir(), "status")
	if result.err == nil {
		t.Fatal("expected error on network failure, got nil")
	}
}
