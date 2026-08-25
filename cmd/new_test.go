package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNew_ScaffoldsSkill(t *testing.T) {
	dir := chdirToTemp(t)

	var stdout bytes.Buffer
	application := newApp(nil, &stdout, io.Discard, nil)

	if err := application.runNew([]string{"prompts/code-review"}); err != nil {
		t.Fatalf("runNew: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "prompts", "code-review", "SKILL.md"))
	if err != nil {
		t.Fatalf("SKILL.md non créé : %v", err)
	}
	if !strings.Contains(string(content), "name: code-review") {
		t.Errorf("front matter incorrect :\n%s", string(content))
	}
	if !strings.Contains(stdout.String(), "Created prompts/code-review/SKILL.md") {
		t.Errorf("sortie inattendue : %q", stdout.String())
	}
}

func TestNew_ScaffoldedSkillIsDiscoveredByCatalogInit(t *testing.T) {
	dir := chdirToTemp(t)

	application := newApp(nil, io.Discard, io.Discard, nil)
	if err := application.runNew([]string{"prompts/code-review"}); err != nil {
		t.Fatalf("runNew: %v", err)
	}
	if err := application.runCatalog([]string{"init"}); err != nil {
		t.Fatalf("runCatalog après runNew : %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "SKILLS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "| skill | code-review | prompts |") {
		t.Errorf("skill scaffoldé absent du catalogue :\n%s", string(content))
	}
}

func TestNew_RefusesExistingSkill(t *testing.T) {
	chdirToTemp(t)

	application := newApp(nil, io.Discard, io.Discard, nil)
	if err := application.runNew([]string{"prompts/code-review"}); err != nil {
		t.Fatal(err)
	}

	err := application.runNew([]string{"prompts/code-review"})
	if err == nil || !strings.Contains(err.Error(), "Skill already exists at prompts/code-review") {
		t.Fatalf("err = %v, want Skill already exists", err)
	}
}

func TestNew_RejectsInvalidRefs(t *testing.T) {
	chdirToTemp(t)
	application := newApp(nil, io.Discard, io.Discard, nil)

	cases := []struct {
		args            []string
		expectedMessage string
	}{
		{[]string{"code-review"}, "Invalid format: use <category>/<name>"},
		{[]string{"prompts/bad name"}, "invalid segment"},
		{nil, "usage: sth new"},
		{[]string{"a/b", "c/d"}, "usage: sth new"},
	}

	for _, testCase := range cases {
		err := application.runNew(testCase.args)
		if err == nil {
			t.Errorf("runNew(%v) : erreur attendue", testCase.args)
			continue
		}
		if !strings.Contains(err.Error(), testCase.expectedMessage) {
			t.Errorf("runNew(%v) = %q, want contains %q", testCase.args, err.Error(), testCase.expectedMessage)
		}
	}
}
