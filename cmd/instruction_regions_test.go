package cmd

import (
	stdhttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"sth/internal/fs"
)

func TestInstructionRegionsPreserveUserContentAndOtherContributors(t *testing.T) {
	projectDir := t.TempDir()
	codexUserContent := []byte("Codex user content without trailing newline")
	geminiUserContent := []byte("@./user.md")
	if err := os.WriteFile(filepath.Join(projectDir, "AGENTS.md"), codexUserContent, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "GEMINI.md"), geminiUserContent, 0o640); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(stdhttp.HandlerFunc(func(responseWriter stdhttp.ResponseWriter, request *stdhttp.Request) {
		switch request.URL.Path {
		case "/owner/repo/main/SKILLS.md":
			catalog := testCatalogMarkdown(
				"| instruction | instruction-alpha | instructions | instructions/instruction-alpha.md | Alpha |\n" +
					"| instruction | instruction-beta | instructions | instructions/instruction-beta.md | Beta |",
			)
			_, _ = responseWriter.Write([]byte(catalog))
		case "/owner/repo/main/instructions/instruction-alpha.md":
			_, _ = responseWriter.Write([]byte("---\ndescription: Alpha\n---\nALPHA_SENTINEL\n"))
		case "/owner/repo/main/instructions/instruction-beta.md":
			_, _ = responseWriter.Write([]byte("---\ndescription: Beta\n---\nBETA_SENTINEL\n"))
		case "/repos/owner/repo/commits":
			responseWriter.Header().Set("Content-Type", "application/json")
			_, _ = responseWriter.Write([]byte(`[{"sha":"1111111111111111"}]`))
		default:
			stdhttp.NotFound(responseWriter, request)
		}
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)
	t.Setenv("STH_GITHUB_API_BASE_URL", server.URL)
	writeProjectConfigV2ForTest(t, projectDir, fanoutConfig(
		fanoutTarget("codex", ".codex"),
		fanoutTarget("gemini", ".gemini"),
	))

	for _, artifactName := range []string{"instruction-alpha", "instruction-beta"} {
		result := runCLI(t, projectDir, t.TempDir(), "install", artifactName, "--allow", "exact,lossy")
		if result.err != nil {
			t.Fatalf("install %s: %v\nstderr=%s", artifactName, result.err, result.stderr)
		}
	}

	agentsContent := readProjectFile(t, projectDir, "AGENTS.md")
	if !strings.HasPrefix(agentsContent, string(codexUserContent)) || !strings.Contains(agentsContent, "ALPHA_SENTINEL") || !strings.Contains(agentsContent, "BETA_SENTINEL") {
		t.Fatalf("AGENTS.md after installs = %q", agentsContent)
	}
	if strings.Index(agentsContent, "instruction-alpha") > strings.Index(agentsContent, "instruction-beta") {
		t.Fatalf("AGENTS.md contributions are not sorted: %s", agentsContent)
	}
	geminiContent := readProjectFile(t, projectDir, "GEMINI.md")
	alphaImport := "@./.gemini/sth/instructions/instruction-alpha.md"
	betaImport := "@./.gemini/sth/instructions/instruction-beta.md"
	if !strings.HasPrefix(geminiContent, string(geminiUserContent)) || !strings.Contains(geminiContent, alphaImport) || !strings.Contains(geminiContent, betaImport) {
		t.Fatalf("GEMINI.md after installs = %q", geminiContent)
	}
	if strings.Contains(geminiContent, "@./user.md<!--") {
		t.Fatalf("GEMINI.md user import was joined with the managed region: %q", geminiContent)
	}
	if strings.Index(geminiContent, alphaImport) > strings.Index(geminiContent, betaImport) {
		t.Fatalf("GEMINI.md imports are not sorted: %s", geminiContent)
	}
	assertInstructionRegionStateHashes(t, projectDir)

	alphaResourceName := fs.CatalogID("instruction", "instructions", "instruction-alpha")
	removeAlpha := runCLI(t, projectDir, t.TempDir(), "remove", alphaResourceName, "--yes")
	if removeAlpha.err != nil {
		t.Fatalf("remove alpha: %v\nstderr=%s", removeAlpha.err, removeAlpha.stderr)
	}
	agentsContent = readProjectFile(t, projectDir, "AGENTS.md")
	if !strings.HasPrefix(agentsContent, string(codexUserContent)) || strings.Contains(agentsContent, "ALPHA_SENTINEL") || !strings.Contains(agentsContent, "BETA_SENTINEL") {
		t.Fatalf("AGENTS.md after alpha removal = %q", agentsContent)
	}
	geminiContent = readProjectFile(t, projectDir, "GEMINI.md")
	if !strings.HasPrefix(geminiContent, string(geminiUserContent)) || strings.Contains(geminiContent, alphaImport) || !strings.Contains(geminiContent, betaImport) {
		t.Fatalf("GEMINI.md after alpha removal = %q", geminiContent)
	}
	assertInstructionRegionStateHashes(t, projectDir)

	betaResourceName := fs.CatalogID("instruction", "instructions", "instruction-beta")
	removeBeta := runCLI(t, projectDir, t.TempDir(), "remove", betaResourceName, "--yes")
	if removeBeta.err != nil {
		t.Fatalf("remove beta: %v\nstderr=%s", removeBeta.err, removeBeta.stderr)
	}
	if content := []byte(readProjectFile(t, projectDir, "AGENTS.md")); string(content) != string(codexUserContent) {
		t.Fatalf("AGENTS.md user content changed: %q", content)
	}
	if content := []byte(readProjectFile(t, projectDir, "GEMINI.md")); string(content) != string(geminiUserContent) {
		t.Fatalf("GEMINI.md user content changed: %q", content)
	}
	if fileInfo, err := os.Stat(filepath.Join(projectDir, "AGENTS.md")); err != nil || runtime.GOOS != "windows" && fileInfo.Mode().Perm() != 0o600 {
		t.Fatalf("AGENTS.md mode = %v, error=%v", fileInfo, err)
	}
	if fileInfo, err := os.Stat(filepath.Join(projectDir, "GEMINI.md")); err != nil || runtime.GOOS != "windows" && fileInfo.Mode().Perm() != 0o640 {
		t.Fatalf("GEMINI.md mode = %v, error=%v", fileInfo, err)
	}
	if entries := readStateV2(t, projectDir).Entries; len(entries) != 0 {
		t.Fatalf("state entries after removals = %+v", entries)
	}
}

func TestInstructionRegionInstallRollsBackOnMalformedMarkers(t *testing.T) {
	projectDir := t.TempDir()
	malformedAgentsContent := []byte("User content\n<!-- sth:managed:codex-instructions-v1:start -->\nstale\n")
	if err := os.WriteFile(filepath.Join(projectDir, "AGENTS.md"), malformedAgentsContent, 0o600); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(stdhttp.HandlerFunc(func(responseWriter stdhttp.ResponseWriter, request *stdhttp.Request) {
		switch request.URL.Path {
		case "/owner/repo/main/SKILLS.md":
			_, _ = responseWriter.Write([]byte(testCatalogMarkdown("| instruction | instruction-alpha | instructions | instructions/instruction-alpha.md | Alpha |")))
		case "/owner/repo/main/instructions/instruction-alpha.md":
			_, _ = responseWriter.Write([]byte("---\ndescription: Alpha\n---\nALPHA_SENTINEL\n"))
		case "/repos/owner/repo/commits":
			responseWriter.Header().Set("Content-Type", "application/json")
			_, _ = responseWriter.Write([]byte(`[{"sha":"1111111111111111"}]`))
		default:
			stdhttp.NotFound(responseWriter, request)
		}
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)
	t.Setenv("STH_GITHUB_API_BASE_URL", server.URL)
	writeProjectConfigV2ForTest(t, projectDir, fanoutConfig(fanoutTarget("codex", ".codex")))
	beforeInstall := transactionalProjectFileSnapshot(t, projectDir)

	result := runCLI(t, projectDir, t.TempDir(), "install", "instruction-alpha", "--allow", "exact,lossy")
	if result.err == nil {
		t.Fatalf("install succeeded with malformed markers\nstdout=%s\nstderr=%s", result.stdout, result.stderr)
	}
	if !strings.Contains(result.err.Error(), fs.ErrManagedMarkdownRegionMalformed.Error()) {
		t.Fatalf("install error = %v, want malformed managed region", result.err)
	}
	afterInstall := transactionalProjectFileSnapshot(t, projectDir)
	if len(afterInstall) != len(beforeInstall) {
		t.Fatalf("project files changed after rollback: before=%v after=%v", beforeInstall, afterInstall)
	}
	for path, beforeContent := range beforeInstall {
		if afterInstall[path] != beforeContent {
			t.Fatalf("file %s changed after rollback: before=%q after=%q", path, beforeContent, afterInstall[path])
		}
	}
	if fileInfo, err := os.Stat(filepath.Join(projectDir, "AGENTS.md")); err != nil || runtime.GOOS != "windows" && fileInfo.Mode().Perm() != 0o600 {
		t.Fatalf("AGENTS.md mode = %v, error=%v", fileInfo, err)
	}
}

func TestInstructionRegionContributionsDeduplicateSharedFragment(t *testing.T) {
	fragmentPath := ".codex/sth/instructions/shared.md"
	managedRegion := fs.ManagedRegion{Path: "AGENTS.md", ID: codexInstructionRegionID}
	state := fs.InstalledResourceStateV2{SchemaVersion: 2, Entries: []fs.InstalledResourceRecordV2{
		{ResourceName: "instruction::instructions::shared", TargetID: "codex-primary", ManagedFiles: []fs.ManagedFile{{Path: fragmentPath}}, ManagedRegions: []fs.ManagedRegion{managedRegion}},
		{ResourceName: "instruction::instructions::shared", TargetID: "codex-secondary", ManagedFiles: []fs.ManagedFile{{Path: fragmentPath}}, ManagedRegions: []fs.ManagedRegion{managedRegion}},
	}}
	definition := instructionRegionDefinition{path: "AGENTS.md", id: codexInstructionRegionID, targetKind: "codex"}
	body, contributionCount, err := buildInstructionRegionBody(t.TempDir(), state, definition, map[string][]byte{
		fragmentPath: []byte(instructionFragmentManagedMarker + "SHARED_SENTINEL\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if contributionCount != 1 || strings.Count(string(body), "SHARED_SENTINEL") != 1 {
		t.Fatalf("shared fragment contributions = %d, body=%q", contributionCount, body)
	}
}

func TestInstructionForceTransferPreservesNewRegionOwner(t *testing.T) {
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(responseWriter stdhttp.ResponseWriter, request *stdhttp.Request) {
		switch request.URL.Path {
		case "/owner/repo/main/SKILLS.md":
			catalog := testCatalogMarkdown(
				"| instruction | shared | alpha | instructions/alpha.md | Alpha |",
				"| instruction | shared | beta | instructions/beta.md | Beta |",
			)
			_, _ = responseWriter.Write([]byte(catalog))
		case "/owner/repo/main/instructions/alpha.md":
			_, _ = responseWriter.Write([]byte("FIRST_OWNER_SENTINEL\n"))
		case "/owner/repo/main/instructions/beta.md":
			_, _ = responseWriter.Write([]byte("SECOND_OWNER_SENTINEL\n"))
		case "/repos/owner/repo/commits":
			responseWriter.Header().Set("Content-Type", "application/json")
			_, _ = responseWriter.Write([]byte(`[{"sha":"1111111111111111"}]`))
		default:
			stdhttp.NotFound(responseWriter, request)
		}
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)
	t.Setenv("STH_GITHUB_API_BASE_URL", server.URL)
	writeProjectConfigV2ForTest(t, projectDir, fanoutConfig(fanoutTarget("codex", ".codex")))

	firstInstall := runCLI(t, projectDir, t.TempDir(), "install", "alpha/shared", "--allow", "exact,lossy")
	if firstInstall.err != nil {
		t.Fatalf("first install: %v\nstderr=%s", firstInstall.err, firstInstall.stderr)
	}
	blockedInstall := runCLI(t, projectDir, t.TempDir(), "install", "beta/shared", "--allow", "exact,lossy")
	if blockedInstall.err == nil {
		t.Fatal("second owner install succeeded without --force")
	}
	forcedInstall := runCLI(t, projectDir, t.TempDir(), "install", "beta/shared", "--force", "--allow", "exact,lossy")
	if forcedInstall.err != nil {
		t.Fatalf("forced install: %v\nstderr=%s", forcedInstall.err, forcedInstall.stderr)
	}

	fragmentContent := readProjectFile(t, projectDir, ".codex/sth/instructions/shared.md")
	agentsContent := readProjectFile(t, projectDir, "AGENTS.md")
	if !strings.Contains(fragmentContent, "SECOND_OWNER_SENTINEL") || strings.Contains(fragmentContent, "FIRST_OWNER_SENTINEL") {
		t.Fatalf("fragment after transfer = %q", fragmentContent)
	}
	if !strings.Contains(agentsContent, "SECOND_OWNER_SENTINEL") || strings.Contains(agentsContent, "FIRST_OWNER_SENTINEL") {
		t.Fatalf("AGENTS.md after transfer = %q", agentsContent)
	}

	firstResourceName := fs.CatalogID("instruction", "alpha", "shared")
	secondResourceName := fs.CatalogID("instruction", "beta", "shared")
	state := readStateV2(t, projectDir)
	firstOwnerFound := false
	secondOwnerFound := false
	for _, record := range state.Entries {
		switch record.ResourceName {
		case firstResourceName:
			firstOwnerFound = true
			if len(record.ManagedFiles) != 0 || len(record.ManagedRegions) != 0 {
				t.Fatalf("first owner still has claims: %+v", record)
			}
		case secondResourceName:
			secondOwnerFound = true
			if len(record.ManagedFiles) != 1 || len(record.ManagedRegions) != 1 {
				t.Fatalf("second owner claims = %+v", record)
			}
		}
	}
	if !firstOwnerFound || !secondOwnerFound {
		t.Fatalf("ownership records missing: %+v", state.Entries)
	}

	removeFirst := runCLI(t, projectDir, t.TempDir(), "remove", firstResourceName, "--yes")
	if removeFirst.err != nil {
		t.Fatalf("remove first owner: %v\nstderr=%s", removeFirst.err, removeFirst.stderr)
	}
	if fragmentContent = readProjectFile(t, projectDir, ".codex/sth/instructions/shared.md"); !strings.Contains(fragmentContent, "SECOND_OWNER_SENTINEL") {
		t.Fatalf("new fragment changed after old owner removal: %q", fragmentContent)
	}
	if agentsContent = readProjectFile(t, projectDir, "AGENTS.md"); !strings.Contains(agentsContent, "SECOND_OWNER_SENTINEL") {
		t.Fatalf("new region changed after old owner removal: %q", agentsContent)
	}
}

func TestNormalizedInstructionBodyRemovesFrontmatterAndManagedMarker(t *testing.T) {
	content := []byte("---\r\ndescription: Runtime\r\napplyTo: '**/*.go'\r\n---\r\n<!-- managed by sth -->\nRuntime body\n")
	if actual := normalizedInstructionBody(content); actual != "Runtime body" {
		t.Fatalf("body = %q", actual)
	}
}

func readProjectFile(t *testing.T, projectDir, relativePath string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(projectDir, filepath.FromSlash(relativePath)))
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func assertInstructionRegionStateHashes(t *testing.T, projectDir string) {
	t.Helper()
	state := readStateV2(t, projectDir)
	for _, record := range state.Entries {
		if len(record.ManagedRegions) != 1 {
			t.Fatalf("record %s/%s managed regions = %+v", record.ResourceName, record.TargetID, record.ManagedRegions)
		}
		region := record.ManagedRegions[0]
		document, err := os.ReadFile(filepath.Join(projectDir, filepath.FromSlash(region.Path)))
		if err != nil {
			t.Fatal(err)
		}
		regionSHA256, err := fs.ManagedMarkdownRegionSHA256(document, region.ID)
		if err != nil || region.SHA256 != regionSHA256 {
			t.Fatalf("record %s/%s region SHA = %s, want %s, error=%v", record.ResourceName, record.TargetID, region.SHA256, regionSHA256, err)
		}
	}
}
