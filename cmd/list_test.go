package cmd

import (
	"testing"

	stdhttp "net/http"
	"net/http/httptest"
	"sth/internal/fs"
)

func TestRun_listShowsRemoteCatalogEntries(t *testing.T) {
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if r.URL.Path != "/owner/repo/main/SKILLS.md" {
			stdhttp.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(testCatalogMarkdown(
			"| skill | alpha | skills | skills/alpha.md | Alpha |",
			"| agent | bravo | agents | agents/bravo.md | Bravo |",
		)))
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)

	writeProjectProfileForCommandTest(t, projectDir, fs.ProjectProfile{
		Provider:     "github",
		RepositoryID: "owner/repo",
		Ref:          "main",
		CatalogPath:  "SKILLS.md",
		TargetDir:    ".claude/resources",
	})

	result := runCLI(t, projectDir, t.TempDir(), "list")

	if result.err != nil {
		t.Fatalf("Run() error = %v", result.err)
	}
	assertContains(t, result.stdout, "2 resource(s) available")
	assertContains(t, result.stdout, "[skill] alpha (folder: skills)")
	assertContains(t, result.stdout, "[agent] bravo (folder: agents)")
	assertEmpty(t, result.stderr)
}

func TestRun_listFallsBackToStructureScanWhenCatalogMissing(t *testing.T) {
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		switch r.URL.Path {
		case "/owner/repo/main/SKILLS.md":
			stdhttp.NotFound(w, r)
		case "/repos/owner/repo/git/trees/main":
			if r.URL.Query().Get("recursive") != "1" {
				stdhttp.Error(w, "missing recursive flag", stdhttp.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"tree":[{"path":"react/form-wizard/SKILL.md","type":"blob"},{"path":"agents/reviewer/SKILL.md","type":"blob"},{"path":"react/form-wizard/README.md","type":"blob"}]}`))
		default:
			stdhttp.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)
	t.Setenv("STH_GITHUB_API_BASE_URL", server.URL)

	writeProjectProfileForCommandTest(t, projectDir, fs.ProjectProfile{
		Provider:     "github",
		RepositoryID: "owner/repo",
		Ref:          "main",
		CatalogPath:  "SKILLS.md",
		TargetDir:    ".claude/resources",
	})

	result := runCLI(t, projectDir, t.TempDir(), "list")

	if result.err != nil {
		t.Fatalf("Run() error = %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}
	assertContains(t, result.stdout, "2 resource(s) available")
	assertContains(t, result.stdout, "[bundle] react/form-wizard")
	assertContains(t, result.stdout, "[bundle] agents/reviewer")
	assertEmpty(t, result.stderr)
}
