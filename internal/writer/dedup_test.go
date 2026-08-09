package writer

import (
	"reflect"
	"testing"
)

func TestPlanSkillPlacement_HubRule(t *testing.T) {
	cases := []struct {
		name        string
		selected    []string
		wantFolders []string
	}{
		{"claude", []string{KindClaude}, []string{claudeSkillsDir}},
		{"antigravity", []string{KindAntigravity}, []string{agentsSkillsDir}},
		{"copilot", []string{KindCopilot}, []string{claudeSkillsDir}},
		{"cursor+copilot", []string{KindCursor, KindCopilot}, []string{claudeSkillsDir}},
		{"claude+antigravity", []string{KindClaude, KindAntigravity}, []string{claudeSkillsDir, agentsSkillsDir}},
		{"cursor+copilot+antigravity", []string{KindCursor, KindCopilot, KindAntigravity}, []string{claudeSkillsDir, agentsSkillsDir}},
		{"codex", []string{KindCodex}, []string{agentsSkillsDir}},
		{"gemini", []string{KindGemini}, []string{agentsSkillsDir}},
		{"codex+antigravity", []string{KindCodex, KindAntigravity}, []string{agentsSkillsDir}},
		{"codex+antigravity+gemini", []string{KindCodex, KindAntigravity, KindGemini}, []string{agentsSkillsDir}},
		{"all", []string{KindClaude, KindCursor, KindCopilot, KindCodex, KindAntigravity, KindGemini}, []string{claudeSkillsDir, agentsSkillsDir}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			placement := PlanSkillPlacement(testCase.selected)
			if !reflect.DeepEqual(placement.Folders, testCase.wantFolders) {
				t.Errorf("Folders = %v, want %v", placement.Folders, testCase.wantFolders)
			}
		})
	}
}

func TestPlanSkillPlacement_ReadsFrom(t *testing.T) {
	placement := PlanSkillPlacement([]string{KindCursor, KindCopilot, KindCodex, KindAntigravity, KindGemini})
	want := map[string]string{
		KindCursor:      claudeSkillsDir,
		KindCopilot:     claudeSkillsDir,
		KindCodex:       agentsSkillsDir,
		KindAntigravity: agentsSkillsDir,
		KindGemini:      agentsSkillsDir,
	}
	if !reflect.DeepEqual(placement.ReadsFrom, want) {
		t.Errorf("ReadsFrom = %v, want %v", placement.ReadsFrom, want)
	}
}

func TestPlanSkillPlacement_Deterministic(t *testing.T) {
	selected := []string{KindAntigravity, KindCodex, KindCursor}
	first := PlanSkillPlacement(selected)
	second := PlanSkillPlacement(selected)
	if !reflect.DeepEqual(first, second) {
		t.Errorf("non-deterministic: %+v vs %+v", first, second)
	}
}

func TestWriterForSkillsFolder(t *testing.T) {
	claude, ok := WriterForSkillsFolder(claudeSkillsDir)
	if !ok || claude.Kind() != KindClaude {
		t.Errorf("%s should map to claude writer, got %v ok=%v", claudeSkillsDir, claude, ok)
	}
	antigravity, ok := WriterForSkillsFolder(agentsSkillsDir)
	if !ok || antigravity.Kind() != KindAntigravity {
		t.Errorf("%s should map to antigravity writer, got %v ok=%v", agentsSkillsDir, antigravity, ok)
	}
	if _, ok := WriterForSkillsFolder(".github/skills"); ok {
		t.Errorf(".github/skills should not map to a hub writer")
	}
}
