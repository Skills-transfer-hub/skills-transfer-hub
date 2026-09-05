package cmd

import (
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"sth/internal/fs"
)

type multiProviderServer struct {
	server        *httptest.Server
	mutex         sync.Mutex
	sharedContent map[string]string
	publicBroken  bool
}

func newMultiProviderServer(t *testing.T) *multiProviderServer {
	t.Helper()
	fixture := &multiProviderServer{
		sharedContent: map[string]string{
			"public":   "---\ndescription: Shared agent\n---\nPUBLIC_SENTINEL\n",
			"internal": "---\ndescription: Shared agent\n---\nINTERNAL_SENTINEL\n",
		},
	}
	fixture.server = httptest.NewServer(stdhttp.HandlerFunc(fixture.serveHTTP))
	t.Cleanup(fixture.server.Close)
	t.Setenv("STH_GITHUB_RAW_BASE_URL", fixture.server.URL)
	t.Setenv("STH_GITHUB_API_BASE_URL", fixture.server.URL)
	return fixture
}

func (fixture *multiProviderServer) setSharedContent(repository, content string) {
	fixture.mutex.Lock()
	defer fixture.mutex.Unlock()
	fixture.sharedContent[repository] = content
}

func (fixture *multiProviderServer) breakPublicRepository() {
	fixture.mutex.Lock()
	defer fixture.mutex.Unlock()
	fixture.publicBroken = true
}

func (fixture *multiProviderServer) serveHTTP(responseWriter stdhttp.ResponseWriter, request *stdhttp.Request) {
	fixture.mutex.Lock()
	broken := fixture.publicBroken
	publicShared := fixture.sharedContent["public"]
	internalShared := fixture.sharedContent["internal"]
	fixture.mutex.Unlock()

	if broken && strings.Contains(request.URL.Path, "owner/public") {
		responseWriter.WriteHeader(stdhttp.StatusInternalServerError)
		return
	}

	switch request.URL.Path {
	case "/owner/public/main/SKILLS.md":
		_, _ = responseWriter.Write([]byte(testCatalogMarkdown(
			"| agent | shared | agents | agents/shared.md | Shared |",
			"| agent | public-only | agents | agents/public-only.md | Public only |",
		)))
	case "/owner/internal/main/SKILLS.md":
		_, _ = responseWriter.Write([]byte(testCatalogMarkdown(
			"| agent | shared | agents | agents/shared.md | Shared |",
			"| agent | internal-only | agents | agents/internal-only.md | Internal only |",
		)))
	case "/owner/public/main/agents/shared.md":
		_, _ = responseWriter.Write([]byte(publicShared))
	case "/owner/internal/main/agents/shared.md":
		_, _ = responseWriter.Write([]byte(internalShared))
	case "/owner/public/main/agents/public-only.md":
		_, _ = responseWriter.Write([]byte("---\ndescription: Public only\n---\nPUBLIC_ONLY\n"))
	case "/owner/internal/main/agents/internal-only.md":
		_, _ = responseWriter.Write([]byte("---\ndescription: Internal only\n---\nINTERNAL_ONLY\n"))
	case "/repos/owner/public/commits", "/repos/owner/internal/commits":
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`[{"sha":"1111111111111111"}]`))
	default:
		stdhttp.NotFound(responseWriter, request)
	}
}

func multiProviderConfig() fs.ProjectConfigV2 {
	return fs.ProjectConfigV2{
		SchemaVersion: 2,
		Providers: []fs.ProviderEntry{
			{ID: "public", Provider: "github", RepositoryID: "owner/public", CatalogRef: "main", CatalogPath: "SKILLS.md", TargetDir: ".sth"},
			{ID: "internal", Provider: "github", RepositoryID: "owner/internal", CatalogRef: "main", CatalogPath: "SKILLS.md", TargetDir: ".sth"},
		},
		Resources: []fs.ResourceEntry{},
		Targets:   []fs.TargetEntry{fanoutTarget("claude", ".claude")},
	}
}

func TestListAggregatesEveryProvider(t *testing.T) {
	newMultiProviderServer(t)
	projectDir := t.TempDir()
	writeProjectConfigV2ForTest(t, projectDir, multiProviderConfig())

	result := runCLI(t, projectDir, t.TempDir(), "list")
	if result.err != nil {
		t.Fatalf("list error = %v\nstderr=%s", result.err, result.stderr)
	}
	assertContains(t, result.stdout, "public-only")
	assertContains(t, result.stdout, "internal-only")
	assertContains(t, result.stdout, "[public]")
	assertContains(t, result.stdout, "[internal]")
}

func TestListSurvivesUnreachableProvider(t *testing.T) {
	fixture := newMultiProviderServer(t)
	fixture.breakPublicRepository()
	projectDir := t.TempDir()
	writeProjectConfigV2ForTest(t, projectDir, multiProviderConfig())

	result := runCLI(t, projectDir, t.TempDir(), "list")
	if result.err != nil {
		t.Fatalf("list error = %v\nstderr=%s", result.err, result.stderr)
	}
	assertContains(t, result.stdout, "internal-only")
	assertNotContains(t, result.stdout, "public-only")
	assertContains(t, result.stderr, "provider public unavailable")
}

func TestInstallReportsAmbiguousResourceAcrossProviders(t *testing.T) {
	newMultiProviderServer(t)
	projectDir := t.TempDir()
	writeProjectConfigV2ForTest(t, projectDir, multiProviderConfig())

	result := runCLI(t, projectDir, t.TempDir(), "install", "shared")
	if result.err == nil {
		t.Fatalf("install error = nil, want ambiguity error\nstdout=%s", result.stdout)
	}
	assertContains(t, result.err.Error(), "ambiguous resource")
	assertContains(t, result.err.Error(), "internal:agents/shared")
}

func TestInstallHonorsProviderFlagAndQualifiedRef(t *testing.T) {
	newMultiProviderServer(t)
	for _, arguments := range [][]string{
		{"install", "--provider", "internal", "shared"},
		{"install", "internal:shared"},
		{"install", "--provider=internal", "agents/shared"},
	} {
		projectDir := t.TempDir()
		writeProjectConfigV2ForTest(t, projectDir, multiProviderConfig())

		result := runCLI(t, projectDir, t.TempDir(), arguments...)
		if result.err != nil {
			t.Fatalf("%v error = %v\nstderr=%s", arguments, result.err, result.stderr)
		}
		content := readProjectFile(t, projectDir, ".claude/agents/shared.md")
		if !strings.Contains(content, "INTERNAL_SENTINEL") {
			t.Fatalf("%v installed content = %q, want the internal provider version", arguments, content)
		}
		state := readStateV2(t, projectDir)
		if len(state.Entries) != 1 || state.Entries[0].ProviderID != "internal" {
			t.Fatalf("%v state = %+v, want provider_id internal", arguments, state.Entries)
		}
	}
}

func TestInstallRejectsUnknownProviderFlag(t *testing.T) {
	newMultiProviderServer(t)
	projectDir := t.TempDir()
	writeProjectConfigV2ForTest(t, projectDir, multiProviderConfig())

	result := runCLI(t, projectDir, t.TempDir(), "install", "--provider", "nope", "shared")
	if result.err == nil {
		t.Fatal("install error = nil, want unknown provider error")
	}
	assertContains(t, result.err.Error(), "unknown provider")
}

func TestUpdateRefreshesFromTheOriginProvider(t *testing.T) {
	fixture := newMultiProviderServer(t)
	projectDir := t.TempDir()
	writeProjectConfigV2ForTest(t, projectDir, multiProviderConfig())

	installResult := runCLI(t, projectDir, t.TempDir(), "install", "internal:shared")
	if installResult.err != nil {
		t.Fatalf("install error = %v\nstderr=%s", installResult.err, installResult.stderr)
	}

	fixture.setSharedContent("internal", "---\ndescription: Shared agent\n---\nINTERNAL_SENTINEL_V2\n")
	updateResult := runCLI(t, projectDir, t.TempDir(), "update")
	if updateResult.err != nil {
		t.Fatalf("update error = %v\nstderr=%s", updateResult.err, updateResult.stderr)
	}

	content := readProjectFile(t, projectDir, ".claude/agents/shared.md")
	if !strings.Contains(content, "INTERNAL_SENTINEL_V2") {
		t.Fatalf("updated content = %q, want the internal provider version", content)
	}
	state := readStateV2(t, projectDir)
	if len(state.Entries) != 1 || state.Entries[0].ProviderID != "internal" {
		t.Fatalf("state after update = %+v, want provider_id internal", state.Entries)
	}
}

func newGitLabProviderServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(responseWriter stdhttp.ResponseWriter, request *stdhttp.Request) {
		switch request.URL.EscapedPath() {
		case "/projects/group%2Fskills/repository/files/SKILLS.md/raw":
			_, _ = responseWriter.Write([]byte(testCatalogMarkdown(
				"| agent | gitlab-agent | agents | agents/gitlab-agent.md | GitLab |",
			)))
		case "/projects/group%2Fskills/repository/files/agents%2Fgitlab-agent.md/raw":
			_, _ = responseWriter.Write([]byte("---\ndescription: GitLab\n---\nGITLAB_SENTINEL\n"))
		case "/projects/group%2Fskills/repository/commits":
			responseWriter.Header().Set("Content-Type", "application/json")
			_, _ = responseWriter.Write([]byte(`[{"id":"g1t1ab00000000000"}]`))
		default:
			stdhttp.NotFound(responseWriter, request)
		}
	}))
	t.Cleanup(server.Close)
	t.Setenv("STH_GITLAB_API_BASE_URL", server.URL)
	return server
}

func TestInstallResolvesVersionForNonGithubProvider(t *testing.T) {
	newMultiProviderServer(t)
	newGitLabProviderServer(t)

	config := multiProviderConfig()
	config.Providers = append(config.Providers, fs.ProviderEntry{
		ID:           "gitlab",
		Provider:     "gitlab",
		RepositoryID: "group/skills",
		CatalogRef:   "main",
		CatalogPath:  "SKILLS.md",
		TargetDir:    ".sth",
	})
	projectDir := t.TempDir()
	writeProjectConfigV2ForTest(t, projectDir, config)

	result := runCLI(t, projectDir, t.TempDir(), "install", "gitlab-agent")
	if result.err != nil {
		t.Fatalf("install error = %v\nstderr=%s", result.err, result.stderr)
	}

	content := readProjectFile(t, projectDir, ".claude/agents/gitlab-agent.md")
	if !strings.Contains(content, "GITLAB_SENTINEL") {
		t.Fatalf("installed content = %q", content)
	}
	state := readStateV2(t, projectDir)
	if len(state.Entries) != 1 {
		t.Fatalf("state = %+v, want a single record", state.Entries)
	}
	if state.Entries[0].ProviderID != "gitlab" {
		t.Fatalf("provider_id = %q, want gitlab", state.Entries[0].ProviderID)
	}
	if state.Entries[0].ResolvedVersion != "g1t1ab00000000000" {
		t.Fatalf("resolved_version = %q, want the GitLab commit id", state.Entries[0].ResolvedVersion)
	}
}

func TestStatusGroupsOneLinePerResource(t *testing.T) {
	newMultiProviderServer(t)
	projectDir := t.TempDir()
	config := multiProviderConfig()
	config.Targets = []fs.TargetEntry{fanoutTarget("claude", ".claude"), fanoutTarget("codex", ".codex")}
	writeProjectConfigV2ForTest(t, projectDir, config)

	installResult := runCLI(t, projectDir, t.TempDir(), "install", "internal:shared")
	if installResult.err != nil {
		t.Fatalf("install error = %v\nstderr=%s", installResult.err, installResult.stderr)
	}
	if entries := readStateV2(t, projectDir).Entries; len(entries) != 2 {
		t.Fatalf("state entries = %d, want one per target", len(entries))
	}

	statusResult := runCLI(t, projectDir, t.TempDir(), "status")
	if statusResult.err != nil {
		t.Fatalf("status error = %v\nstderr=%s", statusResult.err, statusResult.stderr)
	}
	occurrences := strings.Count(statusResult.stdout, "agent::agents::shared")
	if occurrences != 1 {
		t.Fatalf("status printed the resource %d times, want once:\n%s", occurrences, statusResult.stdout)
	}

	listResult := runCLI(t, projectDir, t.TempDir(), "list", "--json")
	if listResult.err != nil {
		t.Fatalf("list --json error = %v", listResult.err)
	}
	var entries []listJSONEntry
	if err := json.Unmarshal([]byte(strings.TrimSpace(listResult.stdout)), &entries); err != nil {
		t.Fatalf("list --json output is not valid JSON: %v\n%s", err, listResult.stdout)
	}
	if len(entries) != 1 || entries[0].ProviderID != "internal" {
		t.Fatalf("list --json = %+v, want a single grouped entry from internal", entries)
	}
}
