package cmd

import (
	"strings"
	"testing"

	"sth/internal/fs"
)

func TestProviderListShowsSourcesAndDefault(t *testing.T) {
	newMultiProviderServer(t)
	projectDir := t.TempDir()
	config := multiProviderConfig()
	config.Providers = append(config.Providers, fs.ProviderEntry{ID: "broken", Provider: "github", CatalogRef: "main", CatalogPath: "SKILLS.md", TargetDir: ".sth"})
	writeProjectConfigV2ForTest(t, projectDir, config)

	installResult := runCLI(t, projectDir, t.TempDir(), "install", "--allow", "exact,lossy", "internal:shared")
	if installResult.err != nil {
		t.Fatalf("install error = %v\nstderr=%s", installResult.err, installResult.stderr)
	}

	result := runCLI(t, projectDir, t.TempDir(), "provider", "list")
	if result.err != nil {
		t.Fatalf("provider list error = %v\nstderr=%s", result.err, result.stderr)
	}
	assertContains(t, result.stdout, "3 provider(s):")
	assertContains(t, result.stdout, "* public")
	assertContains(t, result.stdout, "misconfigured")
	assertContains(t, result.stdout, "owner/internal@main")
	for _, line := range strings.Split(result.stdout, "\n") {
		if strings.Contains(line, "internal ") && !strings.Contains(line, "1 resource(s)") {
			t.Fatalf("internal provider line = %q, want 1 resource(s)", line)
		}
	}
}

func TestProviderAddValidatesAndPersists(t *testing.T) {
	newMultiProviderServer(t)
	newGitLabProviderServer(t)
	projectDir := t.TempDir()
	writeProjectConfigV2ForTest(t, projectDir, multiProviderConfig())

	result := runCLI(t, projectDir, t.TempDir(), "provider", "add", "gitlab", "--type", "gitlab", "--repo", "group/skills")
	if result.err != nil {
		t.Fatalf("provider add error = %v\nstderr=%s", result.err, result.stderr)
	}
	assertContains(t, result.stdout, "✓ Source 'gitlab' added: gitlab group/skills@main")
	assertContains(t, result.stdout, "3 sources are now configured")

	config := readProjectConfigForTest(t, projectDir)
	added, found := fs.ProviderByID(config, "gitlab")
	if !found {
		t.Fatalf("providers = %+v, want a gitlab entry", config.Providers)
	}
	if added.RepositoryID != "group/skills" || added.CatalogRef != "main" || added.CatalogPath != "SKILLS.md" {
		t.Fatalf("added provider = %+v", added)
	}

	listResult := runCLI(t, projectDir, t.TempDir(), "list")
	if listResult.err != nil {
		t.Fatalf("list error = %v", listResult.err)
	}
	assertContains(t, listResult.stdout, "gitlab-agent")
}

func TestProviderAddRejectsDuplicateAndUnreachable(t *testing.T) {
	newMultiProviderServer(t)
	projectDir := t.TempDir()
	writeProjectConfigV2ForTest(t, projectDir, multiProviderConfig())

	duplicate := runCLI(t, projectDir, t.TempDir(), "provider", "add", "internal", "--type", "github", "--repo", "owner/internal")
	if duplicate.err == nil {
		t.Fatal("provider add error = nil, want duplicate error")
	}
	assertContains(t, duplicate.err.Error(), "already exists")

	unreachable := runCLI(t, projectDir, t.TempDir(), "provider", "add", "ghost", "--type", "github", "--repo", "owner/ghost")
	if unreachable.err == nil {
		t.Fatal("provider add error = nil, want unreachable error")
	}
	assertContains(t, unreachable.err.Error(), "not reachable")

	skipped := runCLI(t, projectDir, t.TempDir(), "provider", "add", "ghost", "--type", "github", "--repo", "owner/ghost", "--skip-check")
	if skipped.err != nil {
		t.Fatalf("provider add --skip-check error = %v", skipped.err)
	}
}

func TestProviderRemoveProtectsInstalledResources(t *testing.T) {
	newMultiProviderServer(t)
	projectDir := t.TempDir()
	writeProjectConfigV2ForTest(t, projectDir, multiProviderConfig())

	installResult := runCLI(t, projectDir, t.TempDir(), "install", "--allow", "exact,lossy", "internal:shared")
	if installResult.err != nil {
		t.Fatalf("install error = %v\nstderr=%s", installResult.err, installResult.stderr)
	}

	protected := runCLI(t, projectDir, t.TempDir(), "provider", "remove", "internal")
	if protected.err == nil {
		t.Fatal("provider remove error = nil, want protection error")
	}
	assertContains(t, protected.err.Error(), "still owns 1 installed resource(s)")

	forced := runCLI(t, projectDir, t.TempDir(), "provider", "remove", "internal", "--force")
	if forced.err != nil {
		t.Fatalf("provider remove --force error = %v", forced.err)
	}
	config := readProjectConfigForTest(t, projectDir)
	if _, found := fs.ProviderByID(config, "internal"); found {
		t.Fatalf("providers = %+v, want internal removed", config.Providers)
	}

	unknown := runCLI(t, projectDir, t.TempDir(), "provider", "remove", "internal")
	if unknown.err == nil {
		t.Fatal("provider remove error = nil, want unknown provider error")
	}
	assertContains(t, unknown.err.Error(), "unknown provider")
}
