package scanner

import "testing"

func TestSplitFlatSkillPath_acceptsCategoryFolderSkillPath(t *testing.T) {
	category, name, err := SplitFlatSkillPath("react/form-wizard/SKILL.md")
	if err != nil {
		t.Fatalf("SplitFlatSkillPath() error = %v", err)
	}
	if category != "react" || name != "form-wizard" {
		t.Fatalf("got (%q, %q), want (react, form-wizard)", category, name)
	}
}

func TestSplitFlatSkillPath_rejectsInvalidLayouts(t *testing.T) {
	cases := []string{
		"react/SKILL.md",
		"../react/form-wizard/SKILL.md",
		"react/form-wizard/readme.md",
		"",
		"a/b/c/SKILL.md",
		"/abs/skill/SKILL.md",
	}
	for _, p := range cases {
		if _, _, err := SplitFlatSkillPath(p); err == nil {
			t.Fatalf("SplitFlatSkillPath(%q) error = nil, want error", p)
		}
	}
}

func TestFlatSkillScanner_Detect(t *testing.T) {
	scan := FlatSkillScanner{}
	if !scan.Detect([]TreeEntry{{Path: "react/form-wizard/SKILL.md", Type: "blob"}}) {
		t.Fatal("Detect should return true when a flat skill is present")
	}
	if scan.Detect([]TreeEntry{{Path: "README.md", Type: "blob"}}) {
		t.Fatal("Detect should return false on unrelated trees")
	}
}

func TestFlatSkillScanner_ScanDiscoversSkillBundles(t *testing.T) {
	artifacts, err := FlatSkillScanner{}.Scan([]TreeEntry{
		{Path: "react/form-wizard/SKILL.md", Type: "blob"},
		{Path: "agents/reviewer/SKILL.md", Type: "blob"},
		{Path: "react/form-wizard/README.md", Type: "blob"},
	})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(artifacts) != 2 {
		t.Fatalf("len(artifacts) = %d, want 2", len(artifacts))
	}
	// Sorted by (category, name): agents/reviewer first, then react/form-wizard.
	if artifacts[0].Category != "agents" || artifacts[0].Name != "reviewer" {
		t.Fatalf("artifacts[0] = %+v, want category=agents name=reviewer", artifacts[0])
	}
	if artifacts[0].Kind != KindSkill {
		t.Fatalf("artifacts[0].Kind = %q, want %q", artifacts[0].Kind, KindSkill)
	}
	if artifacts[1].Entrypoint != "react/form-wizard/SKILL.md" {
		t.Fatalf("artifacts[1].Entrypoint = %q, want react/form-wizard/SKILL.md", artifacts[1].Entrypoint)
	}
}

func TestFlatSkillScanner_ScanEmpty(t *testing.T) {
	if _, err := (FlatSkillScanner{}).Scan(nil); err == nil {
		t.Fatal("Scan() on empty tree should return error")
	}
}

func TestFlatSkillScanner_ScanDeduplicates(t *testing.T) {
	artifacts, err := FlatSkillScanner{}.Scan([]TreeEntry{
		{Path: "react/form-wizard/SKILL.md", Type: "blob"},
		{Path: "react/form-wizard/SKILL.md", Type: "blob"},
	})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("len(artifacts) = %d, want 1 (dedup)", len(artifacts))
	}
}
