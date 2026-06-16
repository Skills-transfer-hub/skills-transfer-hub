package cmd

import (
	"bytes"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"sth/internal/fs"
)

func TestInstallProgress_NilSafe(t *testing.T) {
	var progress *installProgress
	progress.start(installStepCount)
	progress.step("Téléchargement…")
	progress.note("Configuration…")
	progress.stop()
}

func TestInstallProgress_NonTTYStepLines(t *testing.T) {
	var stdout, stderr bytes.Buffer
	progress := newInstallProgress(&stdout, &stderr, false)
	progress.start(installStepCount)
	progress.step("Récupération du catalogue…")
	progress.step("Téléchargement de alpha…")
	progress.stop()

	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty in non-TTY mode", stdout.String())
	}
	want := "→ [1/4] Récupération du catalogue…\n→ [2/4] Téléchargement de alpha…\n"
	if stderr.String() != want {
		t.Fatalf("stderr = %q, want %q", stderr.String(), want)
	}
}

func TestInstallProgress_NoteDoesNotAdvanceCounter(t *testing.T) {
	var stdout, stderr bytes.Buffer
	progress := newInstallProgress(&stdout, &stderr, false)
	progress.start(installStepCount)
	progress.step("Installation des fichiers…")
	progress.note("Configuration de Codex…")
	progress.step("Finalisation…")

	want := "→ [1/4] Installation des fichiers…\n→ Configuration de Codex…\n→ [2/4] Finalisation…\n"
	if stderr.String() != want {
		t.Fatalf("stderr = %q, want %q", stderr.String(), want)
	}
}

func singleSkillServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		switch r.URL.Path {
		case "/owner/repo/main/SKILLS.md":
			_, _ = w.Write([]byte(testCatalogMarkdown("| skill | alpha | skills | skills/alpha.md | Alpha |")))
		case "/owner/repo/main/skills/alpha.md":
			_, _ = w.Write([]byte("# Alpha\n"))
		default:
			stdhttp.NotFound(w, r)
		}
	}))
}

func TestRun_installEmitsProgressOnStderr(t *testing.T) {
	projectDir := t.TempDir()
	server := singleSkillServer(t)
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

	result := runCLI(t, projectDir, t.TempDir(), "install", "alpha")
	if result.err != nil {
		t.Fatalf("install error = %v\nstderr=%s", result.err, result.stderr)
	}
	assertContains(t, result.stderr, "[1/4] Récupération du catalogue")
	assertContains(t, result.stderr, "[2/4] Téléchargement de alpha")
	assertContains(t, result.stderr, "[4/4] Finalisation")
	assertContains(t, result.stdout, "Ressource 'alpha' installée")
}

func TestRun_installDryRunEmitsNoProgress(t *testing.T) {
	projectDir := t.TempDir()
	server := singleSkillServer(t)
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

	result := runCLI(t, projectDir, t.TempDir(), "install", "alpha", "--dry-run")
	if result.err != nil {
		t.Fatalf("install --dry-run error = %v\nstderr=%s", result.err, result.stderr)
	}
	assertNotContains(t, result.stderr, "→ [")
}

func TestDownloadBundleContents_DownloadsInParallel(t *testing.T) {
	projectDir := t.TempDir()
	var inFlight, maxInFlight int32
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		switch {
		case r.URL.Path == "/owner/repo/main/SKILLS.md":
			_, _ = w.Write([]byte(testCatalogMarkdown("| skill | alpha | skills | skills/alpha/SKILL.md | Alpha |")))
		case r.URL.Path == "/repos/owner/repo/git/trees/main":
			_, _ = w.Write([]byte(`{"tree":[` +
				`{"path":"skills/alpha/SKILL.md","type":"blob"},` +
				`{"path":"skills/alpha/a.py","type":"blob"},` +
				`{"path":"skills/alpha/b.py","type":"blob"},` +
				`{"path":"skills/alpha/c.py","type":"blob"},` +
				`{"path":"skills/alpha/d.py","type":"blob"}]}`))
		case strings.HasPrefix(r.URL.Path, "/owner/repo/main/skills/alpha/"):
			current := atomic.AddInt32(&inFlight, 1)
			for {
				previous := atomic.LoadInt32(&maxInFlight)
				if current <= previous || atomic.CompareAndSwapInt32(&maxInFlight, previous, current) {
					break
				}
			}
			time.Sleep(40 * time.Millisecond)
			atomic.AddInt32(&inFlight, -1)
			_, _ = w.Write([]byte("# content\n"))
		default:
			stdhttp.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("STH_GITHUB_RAW_BASE_URL", server.URL)
	t.Setenv("STH_GITHUB_API_BASE_URL", server.URL)

	writeProjectConfigV2ForTest(t, projectDir, fanoutConfig(fanoutTarget("claude", ".claude")))

	result := runCLI(t, projectDir, t.TempDir(), "install", "alpha")
	if result.err != nil {
		t.Fatalf("install error = %v\nstderr=%s", result.err, result.stderr)
	}
	if got := atomic.LoadInt32(&maxInFlight); got < 2 {
		t.Fatalf("max concurrent downloads = %d, want >= 2", got)
	}
}
