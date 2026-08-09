package writer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMaterializeGeminiCommandPreviewMatchesWrite(t *testing.T) {
	projectRoot := t.TempDir()
	source := []byte("---\ndescription: Deploy safely\n---\nReturn GEMINI_SENTINEL.\n")
	fetch := func(string) ([]byte, error) {
		return source, nil
	}

	preview, err := MaterializeArtifact(projectRoot, GeminiWriter{}, "command", "deploy", "", []string{"commands/deploy.md"}, fetch, true, false)
	if err != nil {
		t.Fatal(err)
	}
	live, err := MaterializeArtifact(projectRoot, GeminiWriter{}, "command", "deploy", "", []string{"commands/deploy.md"}, fetch, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Files) != 1 || len(live.Files) != 1 || preview.Files[0].SHA256 != live.Files[0].SHA256 {
		t.Fatalf("preview = %+v, live = %+v", preview.Files, live.Files)
	}
	content, err := os.ReadFile(filepath.Join(projectRoot, ".gemini", "commands", "deploy.toml"))
	if err != nil {
		t.Fatal(err)
	}
	expectedSHA := computeSHA256(content)
	if live.Files[0].SHA256 != expectedSHA {
		t.Fatalf("live SHA = %s, want %s", live.Files[0].SHA256, expectedSHA)
	}
}

func TestMaterializeCopilotCommandAcrossCLIRootAndIDERoot(t *testing.T) {
	projectRoot := t.TempDir()
	source := []byte("---\ndescription: Deploy safely\n---\nDeploy now.\n")
	fetch := func(string) ([]byte, error) {
		return source, nil
	}

	result, err := MaterializeArtifact(projectRoot, CopilotWriter{}, "command", "deploy", "", []string{"commands/deploy.md"}, fetch, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 2 {
		t.Fatalf("materialized files = %+v, want two", result.Files)
	}
	for _, relativePath := range []string{".claude/commands/deploy.md", ".github/prompts/deploy.prompt.md"} {
		if _, err := os.Stat(filepath.Join(projectRoot, filepath.FromSlash(relativePath))); err != nil {
			t.Errorf("materialized path %s: %v", relativePath, err)
		}
	}
}

func TestMaterializeCursorPreviewMatchesWrite(t *testing.T) {
	projectRoot := t.TempDir()
	fetch := func(string) ([]byte, error) {
		return []byte("Cursor rule\n"), nil
	}
	preview, err := MaterializeArtifact(projectRoot, CursorWriter{}, "instruction", "style", "", []string{"style.md"}, fetch, true, false)
	if err != nil {
		t.Fatal(err)
	}
	live, err := MaterializeArtifact(projectRoot, CursorWriter{}, "instruction", "style", "", []string{"style.md"}, fetch, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Files[0].SHA256 != live.Files[0].SHA256 {
		t.Fatalf("preview SHA = %s, live SHA = %s", preview.Files[0].SHA256, live.Files[0].SHA256)
	}
}

func TestMaterializeRejectsDestinationOutsideProjectBeforeFetch(t *testing.T) {
	fetchCalled := false
	fetch := func(string) ([]byte, error) {
		fetchCalled = true
		return []byte("body"), nil
	}
	_, err := MaterializeArtifact(t.TempDir(), AntigravityWriter{}, "agent", "../../../../outside", "", []string{"agent.md"}, fetch, true, true)
	if err == nil {
		t.Fatal("expected path validation error")
	}
	if fetchCalled {
		t.Fatal("invalid destination fetched source content")
	}
}

func TestMaterializeRejectsEmptyArtifactNameBeforeFetch(t *testing.T) {
	fetchCalled := false
	fetch := func(string) ([]byte, error) {
		fetchCalled = true
		return []byte("body"), nil
	}
	_, err := MaterializeArtifact(t.TempDir(), ClaudeWriter{}, "agent", "  ", "", []string{"agent.md"}, fetch, true, true)
	if err == nil {
		t.Fatal("expected empty artifact name error")
	}
	if fetchCalled {
		t.Fatal("empty artifact name fetched source content")
	}
}

func TestMaterializeRejectsDestinationOutsideWriterRootBeforeFetch(t *testing.T) {
	projectRoot := t.TempDir()
	fetchCalled := false
	fetch := func(string) ([]byte, error) {
		fetchCalled = true
		return []byte("body"), nil
	}
	_, err := MaterializeArtifact(projectRoot, ClaudeWriter{}, "agent", "../../README", "", []string{"agent.md"}, fetch, true, true)
	if err == nil {
		t.Fatal("expected writer root validation error")
	}
	if fetchCalled {
		t.Fatal("invalid destination fetched source content")
	}
	if _, err := os.Stat(filepath.Join(projectRoot, "README.md")); !os.IsNotExist(err) {
		t.Fatalf("escaped destination exists: %v", err)
	}
}

func TestMaterializeRejectsWriterRootSymlinkOutsideProjectBeforeFetch(t *testing.T) {
	projectRoot := t.TempDir()
	outsideRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectRoot, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideRoot, filepath.Join(projectRoot, ".claude", "agents")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	fetchCalled := false
	fetch := func(string) ([]byte, error) {
		fetchCalled = true
		return []byte("body"), nil
	}
	_, err := MaterializeArtifact(projectRoot, ClaudeWriter{}, "agent", "reviewer", "", []string{"agent.md"}, fetch, true, true)
	if err == nil {
		t.Fatal("expected symlink validation error")
	}
	if fetchCalled {
		t.Fatal("symlink destination fetched source content")
	}
	if _, err := os.Stat(filepath.Join(outsideRoot, "reviewer.md")); !os.IsNotExist(err) {
		t.Fatalf("outside destination exists: %v", err)
	}
}
