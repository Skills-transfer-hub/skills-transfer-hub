package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sth/internal/fs"
)

func TestProviderAddedLinesAnnouncesSourceCountOnlyWhenMultiple(t *testing.T) {
	gitEntry := fs.ProviderEntry{ID: "internal", Provider: "gitlab", RepositoryID: "group/skills", CatalogRef: "main"}

	single := providerAddedLines(fs.ProjectConfigV2{Providers: []fs.ProviderEntry{gitEntry}}, gitEntry)
	if len(single) != 1 {
		t.Fatalf("lines = %v, want a single line for a single source", single)
	}
	if single[0] != "✓ Source 'internal' added: gitlab group/skills@main" {
		t.Fatalf("headline = %q", single[0])
	}

	multiple := providerAddedLines(fs.ProjectConfigV2{Providers: []fs.ProviderEntry{{ID: "public"}, gitEntry}}, gitEntry)
	if len(multiple) != 2 {
		t.Fatalf("lines = %v, want a headline plus the source count", multiple)
	}
	if !strings.Contains(multiple[1], "2 sources are now configured") || !strings.Contains(multiple[1], "sth provider list") {
		t.Fatalf("count line = %q", multiple[1])
	}
}

func TestProviderAddedLinesDescribesSourceWithoutRepository(t *testing.T) {
	cloudEntry := fs.ProviderEntry{ID: "sth-cloud", Provider: "sth-cloud", TargetDir: ".claude/skills"}
	lines := providerAddedLines(fs.ProjectConfigV2{Providers: []fs.ProviderEntry{cloudEntry}}, cloudEntry)
	if len(lines) != 1 || lines[0] != "✓ Source 'sth-cloud' added and attached to this project." {
		t.Fatalf("lines = %v", lines)
	}
}

func TestLoginAnnouncesTheAttachedSource(t *testing.T) {
	env := setupCloudLoginEnv(t, cloudLoginEnv{})
	app, stdout, _ := newCloudLoginApp("", false, env.server)

	if err := app.runLogin([]string{"lsk_live_valid"}); err != nil {
		t.Fatalf("runLogin() error = %v", err)
	}
	assertContains(t, stdout.String(), "Key stored in the OS keychain.")
	assertContains(t, stdout.String(), "✓ Source 'sth-cloud' added and attached to this project.")

	config, err := fs.ReadProjectConfigV2()
	if err != nil {
		t.Fatalf("ReadProjectConfigV2() error = %v", err)
	}
	if _, found := fs.ProviderByID(config, "sth-cloud"); !found {
		t.Fatalf("providers = %+v, want an sth-cloud entry", config.Providers)
	}
}

func TestLoginStaysSilentWhenSourceIsAlreadyAttached(t *testing.T) {
	env := setupCloudLoginEnv(t, cloudLoginEnv{})
	seedCloudProvider(t, env.projectDir)
	app, stdout, _ := newCloudLoginApp("", false, env.server)

	if err := app.runLogin([]string{"lsk_live_valid"}); err != nil {
		t.Fatalf("runLogin() error = %v", err)
	}
	assertNotContains(t, stdout.String(), "Source '")

	config, err := fs.ReadProjectConfigV2()
	if err != nil {
		t.Fatalf("ReadProjectConfigV2() error = %v", err)
	}
	if len(config.Providers) != 1 {
		t.Fatalf("providers = %+v, want the seeded entry untouched", config.Providers)
	}
}

func TestEnsureCloudProviderEntryKeepsIdentifiersUnique(t *testing.T) {
	env := setupCloudLoginEnv(t, cloudLoginEnv{})
	config := fs.ProjectConfigV2{
		SchemaVersion: 2,
		Providers: []fs.ProviderEntry{
			{ID: "sth-cloud", Provider: "github", RepositoryID: "owner/repo", CatalogRef: "main", CatalogPath: "SKILLS.md", TargetDir: ".claude/skills"},
		},
		Resources: []fs.ResourceEntry{},
	}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(env.projectDir, ".sth"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(env.projectDir, ".sth", "project.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	attachment, err := ensureCloudProviderEntry()
	if err != nil {
		t.Fatalf("ensureCloudProviderEntry() error = %v", err)
	}
	if !attachment.added {
		t.Fatal("attachment.added = false, want a freshly created entry")
	}
	if attachment.entry.ID != "sth-cloud-2" {
		t.Fatalf("entry ID = %q, want sth-cloud-2 (sth-cloud is taken by a git source)", attachment.entry.ID)
	}

	written, err := fs.ReadProjectConfigV2()
	if err != nil {
		t.Fatal(err)
	}
	if len(written.Providers) != 2 {
		t.Fatalf("providers = %+v, want the git source plus the cloud source", written.Providers)
	}
	gitSource, found := fs.ProviderByID(written, "sth-cloud")
	if !found || gitSource.Provider != "github" {
		t.Fatalf("existing source = %+v, want the github one untouched", gitSource)
	}
}
