package cmd

import (
	"strings"
	"testing"

	stdhttp "net/http"
	"net/http/httptest"

	"sth/internal/fs"
)

func TestVerbose_HTTPLineOnList(t *testing.T) {
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		_, _ = w.Write([]byte(testCatalogMarkdown("| skill | alpha | skills | skills/alpha.md | Alpha |")))
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)

	projectDir := t.TempDir()
	writeProjectProfileForCommandTest(t, projectDir, fs.ProjectProfile{
		Provider:     "github",
		RepositoryID: "owner/repo",
		Ref:          "main",
		CatalogPath:  "SKILLS.md",
		TargetDir:    ".claude/resources",
	})

	res := runCLI(t, projectDir, t.TempDir(), "list", "--verbose")
	if res.err != nil {
		t.Fatalf("err = %v\nstderr=%s", res.err, res.stderr)
	}
	if !strings.Contains(res.stderr, "[verbose] GET ") {
		t.Fatalf("stderr missing [verbose] HTTP line: %q", res.stderr)
	}
	if !strings.Contains(res.stderr, "→ 200") {
		t.Fatalf("stderr missing status code: %q", res.stderr)
	}
	// Path verbose should fire for project.json.
	if !strings.Contains(res.stderr, "[verbose] path project.json") {
		t.Fatalf("stderr missing [verbose] path: %q", res.stderr)
	}
	// stdout must not contain verbose lines (FR-012).
	if strings.Contains(res.stdout, "[verbose]") {
		t.Fatalf("stdout leaked [verbose]: %q", res.stdout)
	}
}

func TestVerbose_DebugIsAlias(t *testing.T) {
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		_, _ = w.Write([]byte(testCatalogMarkdown("| skill | alpha | skills | skills/alpha.md | Alpha |")))
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)

	projectDir := t.TempDir()
	writeProjectProfileForCommandTest(t, projectDir, fs.ProjectProfile{
		Provider:     "github",
		RepositoryID: "owner/repo",
		Ref:          "main",
		CatalogPath:  "SKILLS.md",
		TargetDir:    ".claude/resources",
	})
	res := runCLI(t, projectDir, t.TempDir(), "list", "--debug")
	if res.err != nil {
		t.Fatalf("err = %v", res.err)
	}
	if !strings.Contains(res.stderr, "[verbose]") {
		t.Fatalf("--debug did not enable verbose: %q", res.stderr)
	}
}

func TestVerbose_AbsentByDefault(t *testing.T) {
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		_, _ = w.Write([]byte(testCatalogMarkdown("| skill | alpha | skills | skills/alpha.md | Alpha |")))
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)

	projectDir := t.TempDir()
	writeProjectProfileForCommandTest(t, projectDir, fs.ProjectProfile{
		Provider:     "github",
		RepositoryID: "owner/repo",
		Ref:          "main",
		CatalogPath:  "SKILLS.md",
		TargetDir:    ".claude/resources",
	})
	res := runCLI(t, projectDir, t.TempDir(), "list")
	if res.err != nil {
		t.Fatalf("err = %v", res.err)
	}
	if strings.Contains(res.stderr, "[verbose]") {
		t.Fatalf("stderr leaked [verbose] without flag: %q", res.stderr)
	}
}
