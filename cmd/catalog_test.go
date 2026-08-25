package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func chdirToTemp(t *testing.T) string {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	return dir
}

func writeSkillFixture(t *testing.T, dir, category, name, description string) {
	t.Helper()
	skillDir := filepath.Join(dir, category, name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: " + description + "\n---\n\nBody.\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogInit_GeneratesSkillsMarkdown(t *testing.T) {
	dir := chdirToTemp(t)
	writeSkillFixture(t, dir, "prompts", "code-review", "Review a diff")
	writeSkillFixture(t, dir, "prompts", "deep-research", "Research deeply")
	writeSkillFixture(t, dir, "conventions", "go-style", "Go conventions")

	var stdout bytes.Buffer
	application := newApp(nil, &stdout, io.Discard, nil)

	if err := application.runCatalog([]string{"init"}); err != nil {
		t.Fatalf("runCatalog: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "SKILLS.md"))
	if err != nil {
		t.Fatalf("SKILLS.md non créé : %v", err)
	}
	text := string(content)
	for _, expectedRow := range []string{
		"| skill | go-style | conventions | conventions/go-style/SKILL.md | Go conventions |",
		"| skill | code-review | prompts | prompts/code-review/SKILL.md | Review a diff |",
		"| skill | deep-research | prompts | prompts/deep-research/SKILL.md | Research deeply |",
	} {
		if !strings.Contains(text, expectedRow) {
			t.Errorf("ligne manquante %q dans :\n%s", expectedRow, text)
		}
	}
	if !strings.Contains(stdout.String(), "3 skill(s) across 2 category or categories") {
		t.Errorf("résumé inattendu : %q", stdout.String())
	}
}

func TestCatalogInit_SortsByCategoryThenName(t *testing.T) {
	dir := chdirToTemp(t)
	writeSkillFixture(t, dir, "zeta", "last", "z")
	writeSkillFixture(t, dir, "alpha", "second", "a2")
	writeSkillFixture(t, dir, "alpha", "first", "a1")

	application := newApp(nil, io.Discard, io.Discard, nil)
	if err := application.runCatalog([]string{"init"}); err != nil {
		t.Fatalf("runCatalog: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "SKILLS.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	firstIndex := strings.Index(text, "| skill | first | alpha |")
	secondIndex := strings.Index(text, "| skill | second | alpha |")
	lastIndex := strings.Index(text, "| skill | last | zeta |")
	if firstIndex == -1 || secondIndex == -1 || lastIndex == -1 {
		t.Fatalf("lignes manquantes :\n%s", text)
	}
	if !(firstIndex < secondIndex && secondIndex < lastIndex) {
		t.Errorf("ordre incorrect :\n%s", text)
	}
}

func TestCatalogInit_FailsWhenNoSkillsFound(t *testing.T) {
	dir := chdirToTemp(t)
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("pas un skill"), 0o644); err != nil {
		t.Fatal(err)
	}

	application := newApp(nil, io.Discard, io.Discard, nil)
	err := application.runCatalog([]string{"init"})
	if err == nil || !strings.Contains(err.Error(), "No skills found matching the expected structure") {
		t.Fatalf("err = %v, want No skills found", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "SKILLS.md")); statErr == nil {
		t.Error("SKILLS.md créé alors qu'aucun skill n'existe")
	}
}

func TestCatalogInit_PromptsBeforeOverwriteAndAbortsOnDecline(t *testing.T) {
	dir := chdirToTemp(t)
	writeSkillFixture(t, dir, "prompts", "code-review", "Review a diff")
	original := "contenu manuel à préserver"
	if err := os.WriteFile(filepath.Join(dir, "SKILLS.md"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	application := newApp(strings.NewReader("n\n"), &stdout, io.Discard, nil)

	err := application.runCatalog([]string{"init"})
	if err == nil {
		t.Fatal("erreur attendue quand l'utilisateur décline")
	}
	if !strings.Contains(stdout.String(), "SKILLS.md already exists") {
		t.Errorf("prompt manquant : %q", stdout.String())
	}
	content, readErr := os.ReadFile(filepath.Join(dir, "SKILLS.md"))
	if readErr != nil || string(content) != original {
		t.Error("SKILLS.md existant modifié malgré le refus")
	}
}

func TestCatalogInit_OverwritesOnConfirmation(t *testing.T) {
	dir := chdirToTemp(t)
	writeSkillFixture(t, dir, "prompts", "code-review", "Review a diff")
	if err := os.WriteFile(filepath.Join(dir, "SKILLS.md"), []byte("ancien"), 0o644); err != nil {
		t.Fatal(err)
	}

	application := newApp(strings.NewReader("y\n"), io.Discard, io.Discard, nil)
	if err := application.runCatalog([]string{"init"}); err != nil {
		t.Fatalf("runCatalog: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "SKILLS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "| skill | code-review |") {
		t.Errorf("SKILLS.md non régénéré :\n%s", string(content))
	}
}

func TestCatalogInit_ForceSkipsPrompt(t *testing.T) {
	dir := chdirToTemp(t)
	writeSkillFixture(t, dir, "prompts", "code-review", "Review a diff")
	if err := os.WriteFile(filepath.Join(dir, "SKILLS.md"), []byte("ancien"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	application := newApp(nil, &stdout, io.Discard, nil)
	if err := application.runCatalog([]string{"init", "--force"}); err != nil {
		t.Fatalf("runCatalog --force: %v", err)
	}

	if strings.Contains(stdout.String(), "Overwrite?") {
		t.Error("--force ne doit pas prompter")
	}
	content, err := os.ReadFile(filepath.Join(dir, "SKILLS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "| skill | code-review |") {
		t.Errorf("SKILLS.md non régénéré :\n%s", string(content))
	}
}

func TestCatalogInit_IgnoresFoldersWithoutSkillMD(t *testing.T) {
	dir := chdirToTemp(t)
	writeSkillFixture(t, dir, "prompts", "code-review", "Review a diff")
	if err := os.MkdirAll(filepath.Join(dir, "prompts", "not-a-skill"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "prompts", "not-a-skill", "notes.md"), []byte("notes"), 0o644); err != nil {
		t.Fatal(err)
	}

	application := newApp(nil, io.Discard, io.Discard, nil)
	if err := application.runCatalog([]string{"init"}); err != nil {
		t.Fatalf("runCatalog: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "SKILLS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "not-a-skill") {
		t.Errorf("dossier sans SKILL.md listé :\n%s", string(content))
	}
}

func TestCatalog_RejectsUnknownSubcommand(t *testing.T) {
	application := newApp(nil, io.Discard, io.Discard, nil)
	if err := application.runCatalog([]string{"publish"}); err == nil {
		t.Fatal("erreur attendue pour une sous-commande inconnue")
	}
	if err := application.runCatalog(nil); err == nil {
		t.Fatal("erreur attendue sans sous-commande")
	}
}
