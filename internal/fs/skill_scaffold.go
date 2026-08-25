package fs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var ErrSkillAlreadyExists = errors.New("skill already exists")

func ValidateSkillRef(ref string) (string, string, error) {
	trimmed := strings.TrimSpace(ref)
	if !strings.Contains(trimmed, "/") {
		return "", "", errors.New("Invalid format: use <category>/<name>")
	}
	segments := strings.Split(trimmed, "/")
	if len(segments) != 2 {
		return "", "", errors.New("Invalid format: use <category>/<name> (exactly one \"/\" separator expected)")
	}
	category, name := segments[0], segments[1]
	for _, segment := range segments {
		if !isValidSkillSegment(segment) {
			return "", "", fmt.Errorf(
				"invalid segment %q: allowed characters are letters, digits, hyphens (-), and underscores (_)",
				segment,
			)
		}
	}
	return category, name, nil
}

func isValidSkillSegment(segment string) bool {
	if segment == "" {
		return false
	}
	for _, character := range segment {
		switch {
		case character >= 'a' && character <= 'z':
		case character >= 'A' && character <= 'Z':
		case character >= '0' && character <= '9':
		case character == '-' || character == '_':
		default:
			return false
		}
	}
	return true
}

func ScaffoldSkill(root, category, name string) error {
	skillDir := filepath.Join(root, category, name)
	if _, statErr := os.Stat(skillDir); statErr == nil {
		return fmt.Errorf("%w : %s", ErrSkillAlreadyExists, filepath.ToSlash(filepath.Join(category, name)))
	}
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", skillDir, err)
	}
	skillMDPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillMDPath, []byte(skillTemplate(name)), 0o644); err != nil {
		return fmt.Errorf("failed to write %s: %w", skillMDPath, err)
	}
	return nil
}

func skillTemplate(name string) string {
	return fmt.Sprintf(`---
name: %s
description: TODO — describe what this skill does and when to use it
---

## Usage

TODO — describe how to invoke this skill.
`, name)
}
