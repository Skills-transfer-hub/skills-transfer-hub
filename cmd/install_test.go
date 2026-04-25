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

func TestRun_installCreatesTargetFilesAndState(t *testing.T) {
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		switch r.URL.Path {
		case "/owner/repo/main/SKILLS.md":
			_, _ = w.Write([]byte(testCatalogMarkdown("| skill | alpha | skills | skills/alpha.md | Alpha |")))
		case "/owner/repo/main/skills/alpha.md":
			_, _ = w.Write([]byte("# Alpha\n"))
		default:
			stdhttp.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("LORESKILLS_GITHUB_RAW_BASE_URL", server.URL)

	writeProjectProfileForCommandTest(t, projectDir, fs.ProjectProfile{
		Provider:     "github",
		RepositoryID: "owner/repo",
		Ref:          "main",
		CatalogPath:  "SKILLS.md",
		TargetDir:    ".claude/resources",
	})

	result := runCLI(t, projectDir, t.TempDir(), "install", "alpha")

	if result.err != nil {
		t.Fatalf("Run() error = %v\nstderr=%s", result.err, result.stderr)
	}
	if !strings.Contains(result.stdout, "Ressource 'alpha' installée") {
		t.Fatalf("stdout %q does not contain install confirmation", result.stdout)
	}

	skillPath := filepath.Join(projectDir, ".claude", "resources", "skills", "alpha.md")
	data, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", skillPath, err)
	}
	if !strings.HasPrefix(string(data), "> ⚠️ ATTENTION : Skill géré par LoreSkills. Ne pas modifier manuellement.\n\n") {
		t.Fatalf("skill file %q does not contain managed header", data)
	}

	statePath := filepath.Join(projectDir, ".claude", "resources", "lore-state.json")
	stateData, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", statePath, err)
	}

	var state fs.InstalledResourceState
	if err := json.Unmarshal(stateData, &state); err != nil {
		t.Fatalf("json.Unmarshal(): %v", err)
	}
	if len(state.Entries) != 1 {
		t.Fatalf("len(state.Entries) = %d, want 1", len(state.Entries))
	}
	if got := state.Entries[0].CatalogID; got != fs.CatalogID("skill", "skills", "alpha") {
		t.Fatalf("state.Entries[0].CatalogID = %q", got)
	}
}

func TestRun_installRejectsAmbiguousResourceName(t *testing.T) {
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if r.URL.Path != "/owner/repo/main/SKILLS.md" {
			stdhttp.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(testCatalogMarkdown(
			"| skill | alpha | skills | skills/alpha.md | Alpha |",
			"| agent | alpha | agents | agents/alpha.md | Agent Alpha |",
		)))
	}))
	defer server.Close()
	t.Setenv("LORESKILLS_GITHUB_RAW_BASE_URL", server.URL)

	writeProjectProfileForCommandTest(t, projectDir, fs.ProjectProfile{
		Provider:     "github",
		RepositoryID: "owner/repo",
		Ref:          "main",
		CatalogPath:  "SKILLS.md",
		TargetDir:    ".claude/resources",
	})

	result := runCLI(t, projectDir, t.TempDir(), "install", "alpha")

	if result.err == nil {
		t.Fatal("Run() error = nil, want error")
	}
	if !strings.Contains(result.stderr, "ambiguë") {
		t.Fatalf("stderr %q does not contain ambiguous-resource details", result.stderr)
	}
}

func TestRun_installSyncsWholeBundleAndState(t *testing.T) {
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		switch r.URL.Path {
		case "/owner/repo/main/SKILLS.md":
			stdhttp.NotFound(w, r)
		case "/repos/owner/repo/git/trees/main":
			_, _ = w.Write([]byte(`{"tree":[{"path":"react/form-wizard/SKILL.md","type":"blob"},{"path":"react/form-wizard/templates/prompt.md","type":"blob"},{"path":"react/form-wizard/bin/tool.js","type":"blob"}]}`))
		case "/owner/repo/main/react/form-wizard/SKILL.md":
			_, _ = w.Write([]byte("# Form Wizard\n"))
		case "/owner/repo/main/react/form-wizard/templates/prompt.md":
			_, _ = w.Write([]byte("Prompt body\n"))
		case "/owner/repo/main/react/form-wizard/bin/tool.js":
			_, _ = w.Write([]byte("console.log('bundle');\n"))
		default:
			stdhttp.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("LORESKILLS_GITHUB_RAW_BASE_URL", server.URL)
	t.Setenv("LORESKILLS_GITHUB_API_BASE_URL", server.URL)

	writeProjectProfileForCommandTest(t, projectDir, fs.ProjectProfile{
		Provider:     "github",
		RepositoryID: "owner/repo",
		Ref:          "main",
		CatalogPath:  "SKILLS.md",
		TargetDir:    ".claude/resources",
	})

	result := runCLI(t, projectDir, t.TempDir(), "install", "react/form-wizard")

	if result.err != nil {
		t.Fatalf("Run() error = %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}
	assertContains(t, result.stdout, "Ressource 'form-wizard' installée")
	assertContains(t, result.stdout, ".claude/resources/react/form-wizard")

	for _, relativePath := range []string{
		".claude/resources/react/form-wizard/SKILL.md",
		".claude/resources/react/form-wizard/templates/prompt.md",
		".claude/resources/react/form-wizard/bin/tool.js",
	} {
		fullPath := filepath.Join(projectDir, filepath.FromSlash(relativePath))
		if _, err := os.Stat(fullPath); err != nil {
			t.Fatalf("Stat(%s): %v", fullPath, err)
		}
	}

	statePath := filepath.Join(projectDir, ".claude", "resources", "lore-state.json")
	stateData, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", statePath, err)
	}

	var state fs.InstalledResourceState
	if err := json.Unmarshal(stateData, &state); err != nil {
		t.Fatalf("json.Unmarshal(): %v", err)
	}
	if len(state.Entries) != 1 {
		t.Fatalf("len(state.Entries) = %d, want 1", len(state.Entries))
	}
	if got := state.Entries[0].ManagedRoot; got != ".claude/resources/react/form-wizard" {
		t.Fatalf("state.Entries[0].ManagedRoot = %q, want .claude/resources/react/form-wizard", got)
	}
	if len(state.Entries[0].ManagedFiles) != 3 {
		t.Fatalf("len(state.Entries[0].ManagedFiles) = %d, want 3", len(state.Entries[0].ManagedFiles))
	}
}

// T020 — .loreskillsignore excludes resource file from being written to disk.
func TestRunInstall_loreskillsignoreExcludesFile(t *testing.T) {
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		switch r.URL.Path {
		case "/owner/repo/main/SKILLS.md":
			_, _ = w.Write([]byte(testCatalogMarkdown("| skill | gamma | skills | skills/gamma.md | Gamma |")))
		case "/owner/repo/main/skills/gamma.md":
			_, _ = w.Write([]byte("# Gamma\n"))
		default:
			stdhttp.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("LORESKILLS_GITHUB_RAW_BASE_URL", server.URL)

	writeProjectProfileForCommandTest(t, projectDir, fs.ProjectProfile{
		Provider:     "github",
		RepositoryID: "owner/repo",
		Ref:          "main",
		CatalogPath:  "SKILLS.md",
		TargetDir:    ".claude/resources",
	})

	ignoreContent := "skills/\n"
	if err := os.WriteFile(filepath.Join(projectDir, ".loreskillsignore"), []byte(ignoreContent), 0644); err != nil {
		t.Fatalf("write .loreskillsignore: %v", err)
	}

	result := runCLI(t, projectDir, t.TempDir(), "install", "gamma")
	if result.err != nil {
		t.Fatalf("Run() error = %v\nstderr=%s", result.err, result.stderr)
	}

	skillPath := filepath.Join(projectDir, ".claude", "resources", "skills", "gamma.md")
	if _, err := os.Stat(skillPath); err == nil {
		t.Fatalf("expected skills/gamma.md to NOT exist (excluded by .loreskillsignore), but it does")
	}

	v2Path := filepath.Join(projectDir, ".claude", "resources", "lore-state-v2.json")
	v2Data, err := os.ReadFile(v2Path)
	if err != nil {
		t.Fatalf("ReadFile(lore-state-v2.json): %v", err)
	}
	var v2state fs.InstalledResourceStateV2
	if err := json.Unmarshal(v2Data, &v2state); err != nil {
		t.Fatalf("json.Unmarshal v2 state: %v", err)
	}
	found := false
	wantName := fs.CatalogID("skill", "skills", "gamma")
	for _, rec := range v2state.Entries {
		if rec.ResourceName == wantName {
			found = true
			if len(rec.ManagedFiles) != 0 {
				t.Fatalf("v2 ManagedFiles = %v, want empty (excluded)", rec.ManagedFiles)
			}
		}
	}
	if !found {
		t.Fatalf("v2 state has no entry for %s; entries=%v", wantName, v2state.Entries)
	}
	if !strings.Contains(result.stderr, "loreskillsignore") {
		t.Fatalf("stderr %q does not contain loreskillsignore warning", result.stderr)
	}
}

// writeV2StateForInstallConflictTest writes a v2 state file into the target dir.
func writeV2StateForInstallConflictTest(t *testing.T, projectDir, targetDir string, state fs.InstalledResourceStateV2) {
	t.Helper()
	targetPath := filepath.Join(projectDir, filepath.FromSlash(targetDir))
	if err := os.MkdirAll(targetPath, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", targetPath, err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent: %v", err)
	}
	if err := os.WriteFile(filepath.Join(targetPath, "lore-state-v2.json"), data, 0o644); err != nil {
		t.Fatalf("WriteFile(lore-state-v2.json): %v", err)
	}
}

// T022 — conflict without --force: install fails with an error.
func TestRunInstall_ownershipConflictBlocksInstall(t *testing.T) {
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		switch r.URL.Path {
		case "/owner/repo/main/SKILLS.md":
			_, _ = w.Write([]byte(testCatalogMarkdown("| skill | beta | skills | skills/beta.md | Beta |")))
		case "/owner/repo/main/skills/beta.md":
			_, _ = w.Write([]byte("# Beta\n"))
		default:
			stdhttp.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("LORESKILLS_GITHUB_RAW_BASE_URL", server.URL)

	writeProjectProfileForCommandTest(t, projectDir, fs.ProjectProfile{
		Provider:     "github",
		RepositoryID: "owner/repo",
		Ref:          "main",
		CatalogPath:  "SKILLS.md",
		TargetDir:    ".claude/resources",
	})

	existingOwner := fs.CatalogID("skill", "skills", "existing")
	conflictPath := ".claude/resources/skills/beta.md"
	writeV2StateForInstallConflictTest(t, projectDir, ".claude/resources", fs.InstalledResourceStateV2{
		SchemaVersion: 2,
		Entries: []fs.InstalledResourceRecordV2{
			{
				ResourceName: existingOwner,
				ManagedFiles: []fs.ManagedFile{{Path: conflictPath, SHA256: "deadbeef"}},
			},
		},
	})

	result := runCLI(t, projectDir, t.TempDir(), "install", "beta")
	if result.err == nil {
		t.Fatal("Run() error = nil, want conflict error")
	}
	errMsg := result.err.Error()
	if !strings.Contains(errMsg, "conflict") {
		t.Fatalf("error %q does not mention conflict", errMsg)
	}
	if !strings.Contains(errMsg, conflictPath) {
		t.Fatalf("error %q does not mention file path", errMsg)
	}
	if !strings.Contains(errMsg, existingOwner) {
		t.Fatalf("error %q does not mention existing owner", errMsg)
	}
	if !strings.Contains(errMsg, "--force") {
		t.Fatalf("error %q does not mention --force", errMsg)
	}
}

// T022 — --force transfers ownership in v2 state and emits a warning.
func TestRunInstall_forceTransfersOwnership(t *testing.T) {
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		switch r.URL.Path {
		case "/owner/repo/main/SKILLS.md":
			_, _ = w.Write([]byte(testCatalogMarkdown("| skill | beta | skills | skills/beta.md | Beta |")))
		case "/owner/repo/main/skills/beta.md":
			_, _ = w.Write([]byte("# Beta\n"))
		default:
			stdhttp.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("LORESKILLS_GITHUB_RAW_BASE_URL", server.URL)

	writeProjectProfileForCommandTest(t, projectDir, fs.ProjectProfile{
		Provider:     "github",
		RepositoryID: "owner/repo",
		Ref:          "main",
		CatalogPath:  "SKILLS.md",
		TargetDir:    ".claude/resources",
	})

	existingOwner := fs.CatalogID("skill", "skills", "existing")
	newOwner := fs.CatalogID("skill", "skills", "beta")
	conflictPath := ".claude/resources/skills/beta.md"
	writeV2StateForInstallConflictTest(t, projectDir, ".claude/resources", fs.InstalledResourceStateV2{
		SchemaVersion: 2,
		Entries: []fs.InstalledResourceRecordV2{
			{
				ResourceName: existingOwner,
				ManagedFiles: []fs.ManagedFile{{Path: conflictPath, SHA256: "deadbeef"}},
			},
		},
	})

	result := runCLI(t, projectDir, t.TempDir(), "install", "--force", "beta")
	if result.err != nil {
		t.Fatalf("Run() error = %v\nstderr=%s", result.err, result.stderr)
	}
	if !strings.Contains(result.stdout, "warning: ownership of") {
		t.Fatalf("stdout %q does not contain ownership transfer warning", result.stdout)
	}

	v2Path := filepath.Join(projectDir, ".claude", "resources", "lore-state-v2.json")
	v2Data, err := os.ReadFile(v2Path)
	if err != nil {
		t.Fatalf("ReadFile(lore-state-v2.json): %v", err)
	}
	var v2state fs.InstalledResourceStateV2
	if err := json.Unmarshal(v2Data, &v2state); err != nil {
		t.Fatalf("json.Unmarshal v2 state: %v", err)
	}
	for _, rec := range v2state.Entries {
		if rec.ResourceName == existingOwner {
			for _, mf := range rec.ManagedFiles {
				if mf.Path == conflictPath {
					t.Fatalf("existingOwner %q still has %q in managed_files after --force", existingOwner, conflictPath)
				}
			}
		}
	}
	foundNew := false
	for _, rec := range v2state.Entries {
		if rec.ResourceName == newOwner {
			for _, mf := range rec.ManagedFiles {
				if mf.Path == conflictPath {
					foundNew = true
				}
			}
		}
	}
	if !foundNew {
		t.Fatalf("newOwner %q does not have %q in managed_files after --force", newOwner, conflictPath)
	}
}
