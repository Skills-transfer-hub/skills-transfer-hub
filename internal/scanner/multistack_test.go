package scanner

import "testing"

func loreTree() []TreeEntry {
	// Reduced fixture mirroring C:\Users\hugo\project\Lore.Skills layout.
	return []TreeEntry{
		{Path: "README.md", Type: "blob"},
		{Path: "agents/code-reviewer.md", Type: "blob"},
		{Path: "agents/security-auditor.md", Type: "blob"},
		{Path: "commands/fix-issue.md", Type: "blob"},
		{Path: "commands/fix-pr/SKILL.md", Type: "blob"},
		{Path: "commands/fix-pr/references/mcp-tools.md", Type: "blob"},
		{Path: "angular/angular-component/SKILL.md", Type: "blob"},
		{Path: "angular/angular-component/references/component-patterns.md", Type: "blob"},
		{Path: "react/react-patterns/SKILL.md", Type: "blob"},
		{Path: "react/instructions/reactjs.instructions.md", Type: "blob"},
		{Path: "common/git-hooks/SKILL.md", Type: "blob"},
	}
}

func TestMultiStackScanner_DetectsManifestRoot(t *testing.T) {
	tree := []TreeEntry{{Path: "manifest.json", Type: "blob"}, {Path: "README.md", Type: "blob"}}
	if !(MultiStackScanner{}).Detect(tree) {
		t.Fatal("a root manifest.json should trigger multi-stack detection")
	}
}

func TestMultiStackScanner_DetectsByConventionalDirs(t *testing.T) {
	if !(MultiStackScanner{}).Detect(loreTree()) {
		t.Fatal("Detect should fire when 2+ conventional containers exist")
	}
}

func TestMultiStackScanner_IgnoresFlatRepos(t *testing.T) {
	tree := []TreeEntry{
		{Path: "react/form-wizard/SKILL.md", Type: "blob"},
		{Path: "agents/reviewer/SKILL.md", Type: "blob"},
	}
	// Pattern category/skill/SKILL.md only — no single-file agents, no instructions.
	if (MultiStackScanner{}).Detect(tree) {
		t.Fatal("flat anthropics-style repos must NOT match multi-stack")
	}
}

func TestMultiStackScanner_DiscoversAllFourKinds(t *testing.T) {
	artifacts, err := MultiStackScanner{}.Scan(loreTree())
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	counts := map[ArtifactKind]int{}
	for _, art := range artifacts {
		counts[art.Kind]++
	}
	if counts[KindAgent] != 2 {
		t.Errorf("agents: got %d, want 2", counts[KindAgent])
	}
	if counts[KindCommand] != 1 {
		t.Errorf("commands single-file: got %d, want 1 (fix-issue, fix-pr is the skill form)", counts[KindCommand])
	}
	if counts[KindSkill] != 4 {
		t.Errorf("skills: got %d, want 4 (commands/fix-pr + angular + react + common)", counts[KindSkill])
	}
	if counts[KindInstruction] != 1 {
		t.Errorf("instructions: got %d, want 1", counts[KindInstruction])
	}
}

func TestMultiStackScanner_SkillCategoryCapturesNesting(t *testing.T) {
	artifacts, _ := MultiStackScanner{}.Scan([]TreeEntry{
		{Path: "common/git-hooks/SKILL.md", Type: "blob"},
		{Path: "angular/angular-component/SKILL.md", Type: "blob"},
	})
	got := map[string]string{} // name -> category
	for _, art := range artifacts {
		got[art.Name] = art.Category
	}
	if got["git-hooks"] != "common" {
		t.Errorf("common/git-hooks should have category=common, got %q", got["git-hooks"])
	}
	if got["angular-component"] != "angular" {
		t.Errorf("angular/angular-component should have category=angular, got %q", got["angular-component"])
	}
}

func TestMultiStackScanner_CommandSkillFormShadowsSingleFile(t *testing.T) {
	artifacts, _ := MultiStackScanner{}.Scan([]TreeEntry{
		{Path: "commands/fix-pr/SKILL.md", Type: "blob"},
		{Path: "commands/fix-pr.md", Type: "blob"}, // would be a single-file command
	})
	for _, art := range artifacts {
		if art.Kind == KindCommand && art.Name == "fix-pr" {
			t.Fatalf("single-file command should be hidden when the skill form exists, got %+v", art)
		}
	}
}

func TestMultiStackScanner_InstructionsCarryLang(t *testing.T) {
	artifacts, _ := MultiStackScanner{}.Scan([]TreeEntry{
		{Path: "react/instructions/reactjs.instructions.md", Type: "blob"},
		{Path: "angular/instructions/angular-style.instructions.md", Type: "blob"},
	})
	if len(artifacts) != 2 {
		t.Fatalf("got %d artifacts, want 2", len(artifacts))
	}
	for _, art := range artifacts {
		if art.Kind != KindInstruction {
			t.Errorf("expected instruction kind, got %q", art.Kind)
		}
		if art.Lang == "" {
			t.Errorf("instruction artifact must carry Lang, got %+v", art)
		}
	}
}

func TestPickScanner_HintWins(t *testing.T) {
	if PickScanner(nil, "flat").Name() != "flat" {
		t.Fatal("hint=flat should force FlatSkillScanner")
	}
	if PickScanner(loreTree(), "flat").Name() != "flat" {
		t.Fatal("hint=flat must override autodetect")
	}
	if PickScanner(nil, "multi-stack").Name() != "multi-stack" {
		t.Fatal("hint=multi-stack should force MultiStackScanner")
	}
}

func TestPickScanner_AutoFallback(t *testing.T) {
	if PickScanner(loreTree(), "").Name() != "multi-stack" {
		t.Fatal("auto: Lore-shaped tree should resolve to multi-stack")
	}
	flatTree := []TreeEntry{{Path: "react/form-wizard/SKILL.md", Type: "blob"}}
	if PickScanner(flatTree, "").Name() != "flat" {
		t.Fatal("auto: flat-shaped tree should resolve to flat")
	}
}

func TestExtractFrontmatter_AgentStyle(t *testing.T) {
	content := []byte(`---
name: code-reviewer
description: Reviews Angular/TS code
tools: Read, Glob, Grep
model: sonnet
---

You are a senior Angular reviewer.
`)
	fm, ok := ExtractFrontmatter(content)
	if !ok {
		t.Fatal("frontmatter expected")
	}
	if fm.Name != "code-reviewer" {
		t.Errorf("Name = %q, want code-reviewer", fm.Name)
	}
	if fm.Description != "Reviews Angular/TS code" {
		t.Errorf("Description = %q", fm.Description)
	}
}

func TestExtractFrontmatter_InstructionApplyTo(t *testing.T) {
	content := []byte("---\ndescription: 'ReactJS development standards'\napplyTo: '**/*.tsx, **/*.jsx'\n---\nbody")
	fm, ok := ExtractFrontmatter(content)
	if !ok {
		t.Fatal("frontmatter expected")
	}
	if len(fm.ApplyTo) != 2 || fm.ApplyTo[0] != "**/*.tsx" || fm.ApplyTo[1] != "**/*.jsx" {
		t.Errorf("ApplyTo = %v, want [**/*.tsx **/*.jsx]", fm.ApplyTo)
	}
}

func TestExtractFrontmatter_NoFrontmatter(t *testing.T) {
	if _, ok := ExtractFrontmatter([]byte("# Heading\nbody\n")); ok {
		t.Fatal("plain markdown should return ok=false")
	}
}

func TestExtractFrontmatter_BracketedGlobList(t *testing.T) {
	content := []byte("---\nglobs: ['**/*.go', '**/*.proto']\n---\nbody")
	fm, _ := ExtractFrontmatter(content)
	if len(fm.ApplyTo) != 2 || fm.ApplyTo[0] != "**/*.go" {
		t.Errorf("bracketed globs not parsed, got %v", fm.ApplyTo)
	}
}

func TestExtractFrontmatter_CRLFLineEndings(t *testing.T) {
	// Regression: bufio.Scanner strips CRLF and LF identically, so a fixed
	// `+1` per line drifts the offset by one byte per CRLF line and
	// truncates the extracted block (or misses the closing fence on small
	// files). Manual bytes.IndexByte walk fixes both.
	content := []byte("---\r\nname: crlf-skill\r\ndescription: Windows-edited file\r\n---\r\nbody\r\n")
	fm, ok := ExtractFrontmatter(content)
	if !ok {
		t.Fatal("frontmatter must be detected even on CRLF-terminated files")
	}
	if fm.Name != "crlf-skill" {
		t.Errorf("Name = %q, want crlf-skill", fm.Name)
	}
	if fm.Description != "Windows-edited file" {
		t.Errorf("Description = %q", fm.Description)
	}
}

func TestInferSkillLang_HeuristicCases(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"react/form-wizard/SKILL.md", ""},               // no lang container, category=react
		{"skills/react/form-wizard/SKILL.md", "react"},   // skips "skills"
		{"agents/react/code-reviewer/SKILL.md", "react"}, // skips "agents"
		{"common/git-hooks/SKILL.md", ""},                // "common" is not a lang
		{"skills/form-wizard/SKILL.md", ""},              // no lang under "skills"
		{"angular/angular-component/SKILL.md", ""},       // category equals top, no lang
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			art, ok := skillFromPath(tc.path)
			if !ok {
				t.Fatalf("skillFromPath returned !ok for %q", tc.path)
			}
			if art.Lang != tc.want {
				t.Errorf("Lang = %q, want %q", art.Lang, tc.want)
			}
		})
	}
}
