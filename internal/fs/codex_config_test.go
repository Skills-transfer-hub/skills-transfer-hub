package fs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureCodexSkillsConfig_CreatesAndIdempotent(t *testing.T) {
	dir := t.TempDir()
	skillPath := ".claude/skills/form-wizard/SKILL.md"

	changed, err := EnsureCodexSkillsConfig(dir, skillPath)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Errorf("first call should report changed")
	}
	configPath := filepath.Join(dir, ".codex", "config.toml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "[[skills.config]]") || !strings.Contains(content, `path = "`+skillPath+`"`) {
		t.Errorf("config missing skill entry: %q", content)
	}

	changed, err = EnsureCodexSkillsConfig(dir, skillPath)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Errorf("second call should be a no-op")
	}
	again, _ := os.ReadFile(configPath)
	if strings.Count(string(again), "[[skills.config]]") != 1 {
		t.Errorf("idempotency broken, blocks: %d", strings.Count(string(again), "[[skills.config]]"))
	}
}

func TestEnsureCodexSkillsConfig_AppendsDistinctAndPreserves(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".codex")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "config.toml")
	if err := os.WriteFile(configPath, []byte("model = \"gpt-5\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := EnsureCodexSkillsConfig(dir, ".claude/skills/a/SKILL.md"); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureCodexSkillsConfig(dir, ".agents/skills/b/SKILL.md"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(configPath)
	content := string(data)
	if !strings.Contains(content, `model = "gpt-5"`) {
		t.Errorf("existing content not preserved: %q", content)
	}
	if strings.Count(content, "[[skills.config]]") != 2 {
		t.Errorf("expected 2 distinct blocks, got %d", strings.Count(content, "[[skills.config]]"))
	}
}
