package writer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectAvailableTargets_EmptyProject(t *testing.T) {
	got := DetectAvailableTargets(t.TempDir())
	if len(got) != 0 {
		t.Fatalf("empty project should detect nothing, got %+v", got)
	}
}

func TestDetectAvailableTargets_AllRegisteredConsumers(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, ".claude"))
	mustMkdir(t, filepath.Join(dir, ".github"))
	mustWrite(t, filepath.Join(dir, ".github", "copilot-instructions.md"), "x")
	mustMkdir(t, filepath.Join(dir, ".codex"))
	mustMkdir(t, filepath.Join(dir, ".cursor"))
	mustMkdir(t, filepath.Join(dir, ".agents", "workflows"))
	mustMkdir(t, filepath.Join(dir, ".gemini", "commands"))

	got := DetectAvailableTargets(dir)
	kinds := map[string]bool{}
	for _, d := range got {
		kinds[d.Kind] = true
	}
	for _, want := range []string{"claude", "copilot", "codex", "cursor", "antigravity", "gemini"} {
		if !kinds[want] {
			t.Errorf("expected %s to be detected, got %+v", want, got)
		}
	}
}

func TestDetectAvailableTargets_RejectsBareGithubDir(t *testing.T) {
	// .github/ alone (no copilot-instructions.md, no instructions/) must NOT
	// trigger Copilot detection — the directory is too generic.
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, ".github"))
	mustWrite(t, filepath.Join(dir, ".github", "CODEOWNERS"), "x")

	for _, d := range DetectAvailableTargets(dir) {
		if d.Kind == "copilot" {
			t.Fatalf("bare .github/ should not match Copilot, got %+v", d)
		}
	}
}

func TestDetectAvailableTargets_RecognizesLegacyMarkers(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "CLAUDE.md"), "# Project Claude")
	mustWrite(t, filepath.Join(dir, "AGENTS.md"), "# Project Agents")
	mustWrite(t, filepath.Join(dir, ".cursorrules"), "rules")

	got := DetectAvailableTargets(dir)
	if len(got) != 3 {
		t.Fatalf("expected 3 detections from legacy markers, got %d (%+v)", len(got), got)
	}
}

func TestDetectAvailableTargets_Antigravity(t *testing.T) {
	ambiguous := t.TempDir()
	mustMkdir(t, filepath.Join(ambiguous, ".agents", "skills"))
	if detectsKind(DetectAvailableTargets(ambiguous), KindAntigravity) || detectsKind(DetectAvailableTargets(ambiguous), KindGemini) {
		t.Errorf(".agents/skills alone should remain ambiguous")
	}

	canonical := t.TempDir()
	mustMkdir(t, filepath.Join(canonical, ".agents", "agents"))
	if !detectsKind(DetectAvailableTargets(canonical), KindAntigravity) {
		t.Errorf(".agents/agents should detect antigravity")
	}

	legacy := t.TempDir()
	mustMkdir(t, filepath.Join(legacy, ".agent"))
	if !detectsKind(DetectAvailableTargets(legacy), KindAntigravity) {
		t.Errorf(".agent/ should detect antigravity")
	}
	if detectsKind(DetectAvailableTargets(legacy), KindGemini) {
		t.Errorf(".agent/ should not detect gemini")
	}
}

func TestDetectAvailableTargets_Gemini(t *testing.T) {
	gemini := t.TempDir()
	mustWrite(t, filepath.Join(gemini, "GEMINI.md"), "# Gemini")
	if !detectsKind(DetectAvailableTargets(gemini), KindGemini) {
		t.Errorf("GEMINI.md should detect gemini")
	}
	if detectsKind(DetectAvailableTargets(gemini), KindAntigravity) {
		t.Errorf("GEMINI.md should not detect antigravity")
	}
}

func detectsKind(targets []DetectedTarget, kind string) bool {
	for _, target := range targets {
		if target.Kind == kind {
			return true
		}
	}
	return false
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
