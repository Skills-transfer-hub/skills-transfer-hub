package fs

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateSkillRef_AcceptsWellFormedRef(t *testing.T) {
	category, name, err := ValidateSkillRef("prompts/code-review")
	if err != nil {
		t.Fatalf("ValidateSkillRef: %v", err)
	}
	if category != "prompts" || name != "code-review" {
		t.Errorf("got (%q, %q), want (prompts, code-review)", category, name)
	}
}

func TestValidateSkillRef_RejectsInvalidRefs(t *testing.T) {
	cases := []struct {
		ref             string
		expectedMessage string
	}{
		{"code-review", "Invalid format: use <category>/<name>"},
		{"a/b/c", "un seul séparateur"},
		{"prompts/", "segment invalide"},
		{"/code-review", "segment invalide"},
		{"prompts/code review", "segment invalide"},
		{"prompts/code$review", "segment invalide"},
		{"", "Invalid format: use <category>/<name>"},
	}

	for _, testCase := range cases {
		_, _, err := ValidateSkillRef(testCase.ref)
		if err == nil {
			t.Errorf("ValidateSkillRef(%q) : erreur attendue", testCase.ref)
			continue
		}
		if !strings.Contains(err.Error(), testCase.expectedMessage) {
			t.Errorf("ValidateSkillRef(%q) = %q, want contains %q", testCase.ref, err.Error(), testCase.expectedMessage)
		}
	}
}

func TestScaffoldSkill_CreatesSkillMDWithTemplate(t *testing.T) {
	root := t.TempDir()

	if err := ScaffoldSkill(root, "prompts", "code-review"); err != nil {
		t.Fatalf("ScaffoldSkill: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(root, "prompts", "code-review", "SKILL.md"))
	if err != nil {
		t.Fatalf("SKILL.md non créé : %v", err)
	}
	text := string(content)
	for _, expected := range []string{"name: code-review", "description:", "## Usage"} {
		if !strings.Contains(text, expected) {
			t.Errorf("SKILL.md sans %q :\n%s", expected, text)
		}
	}
}

func TestScaffoldSkill_RefusesExistingDirectory(t *testing.T) {
	root := t.TempDir()
	existingPath := filepath.Join(root, "prompts", "code-review")
	if err := os.MkdirAll(existingPath, 0o755); err != nil {
		t.Fatal(err)
	}
	sentinelPath := filepath.Join(existingPath, "keep.txt")
	if err := os.WriteFile(sentinelPath, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := ScaffoldSkill(root, "prompts", "code-review")
	if !errors.Is(err, ErrSkillAlreadyExists) {
		t.Fatalf("err = %v, want ErrSkillAlreadyExists", err)
	}

	content, readErr := os.ReadFile(sentinelPath)
	if readErr != nil || string(content) != "original" {
		t.Error("le contenu existant a été modifié")
	}
	if _, statErr := os.Stat(filepath.Join(existingPath, "SKILL.md")); statErr == nil {
		t.Error("SKILL.md écrit malgré le dossier existant")
	}
}

func TestScaffoldSkill_WorksInEmptyRepository(t *testing.T) {
	root := t.TempDir()

	if err := ScaffoldSkill(root, "brand-new_category", "skill_01"); err != nil {
		t.Fatalf("ScaffoldSkill dans un repo vide : %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "brand-new_category", "skill_01", "SKILL.md")); err != nil {
		t.Errorf("SKILL.md manquant : %v", err)
	}
}
