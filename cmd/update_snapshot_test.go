package cmd

import (
	"fmt"
	stdhttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateUsesResolvedGitHubCommitForEveryDownload(t *testing.T) {
	const currentCommit = "abcdef0123456789abcdef0123456789abcdef01"
	projectDir := t.TempDir()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
		switch request.URL.Path {
		case "/repos/owner/repo/commits/main":
			_, _ = fmt.Fprintf(writer, `{"sha":%q}`, currentCommit)
		case "/repos/owner/repo/commits":
			_, _ = fmt.Fprintf(writer, `[{"sha":%q}]`, currentCommit)
		case "/owner/repo/main/SKILLS.md", "/owner/repo/" + currentCommit + "/SKILLS.md":
			_, _ = writer.Write([]byte(testCatalogMarkdown("| skill | alpha | skills | skills/alpha/SKILL.md | Alpha |")))
		case "/repos/owner/repo/git/trees/main", "/repos/owner/repo/git/trees/" + currentCommit:
			_, _ = writer.Write([]byte(`{"tree":[{"path":"skills/alpha/SKILL.md","type":"blob"}]}`))
		case "/owner/repo/main/skills/alpha/SKILL.md":
			_, _ = writer.Write([]byte("---\nname: alpha\ndescription: Old version\n---\n# Old version\n"))
		case "/owner/repo/" + currentCommit + "/skills/alpha/SKILL.md":
			_, _ = writer.Write([]byte("---\nname: alpha\ndescription: Current version\n---\n# Current version\n"))
		default:
			stdhttp.NotFound(writer, request)
		}
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)
	t.Setenv("STH_GITHUB_API_BASE_URL", server.URL)
	writeProjectConfigV2ForTest(t, projectDir, fanoutConfig(fanoutTarget("claude", ".claude")))

	installResult := runCLI(t, projectDir, t.TempDir(), "install", "alpha")
	if installResult.err != nil {
		t.Fatalf("install error = %v\nstderr=%s", installResult.err, installResult.stderr)
	}

	updateResult := runCLI(t, projectDir, t.TempDir(), "update", "--json")
	if updateResult.err != nil {
		t.Fatalf("update error = %v\nstderr=%s", updateResult.err, updateResult.stderr)
	}
	installedPath := filepath.Join(projectDir, ".claude", "skills", "alpha", "SKILL.md")
	content, err := os.ReadFile(installedPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "Current version") {
		t.Fatalf("update installed stale branch content: %q", content)
	}
	if !strings.Contains(updateResult.stdout, currentCommit) {
		t.Fatalf("update did not record resolved commit: %s", updateResult.stdout)
	}
}
