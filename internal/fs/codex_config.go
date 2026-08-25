package fs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func EnsureCodexSkillsConfig(projectRoot, skillPath string) (bool, error) {
	configDir := filepath.Join(projectRoot, ".codex")
	configPath := filepath.Join(configDir, "config.toml")

	existing, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("failed to read %s: %w", configPath, err)
	}

	entryLine := `path = "` + filepath.ToSlash(skillPath) + `"`
	if strings.Contains(string(existing), entryLine) {
		return false, nil
	}

	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return false, fmt.Errorf("failed to create directory %s: %w", configDir, err)
	}

	var builder strings.Builder
	builder.Write(existing)
	if len(existing) > 0 && !strings.HasSuffix(string(existing), "\n") {
		builder.WriteString("\n")
	}
	if len(existing) > 0 {
		builder.WriteString("\n")
	}
	builder.WriteString("[[skills.config]]\n")
	builder.WriteString(entryLine + "\n")

	if err := atomicWriteFile(configPath, []byte(builder.String()), 0o644); err != nil {
		return false, fmt.Errorf("failed to write %s: %w", configPath, err)
	}
	return true, nil
}
