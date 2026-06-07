package writer

import (
	"reflect"
	"testing"
)

func TestPlanSkillPlacement_HubRule(t *testing.T) {
	cases := []struct {
		name         string
		selected     []string
		wantFolders  []string
		wantCodexCfg bool
		wantCodexDir string
	}{
		{"claude", []string{KindClaude}, []string{claudeSkillsDir}, false, ""},
		{"antigravity", []string{KindAntigravity}, []string{agentsSkillsDir}, false, ""},
		{"copilot", []string{KindCopilot}, []string{claudeSkillsDir}, false, ""},
		{"cursor+copilot", []string{KindCursor, KindCopilot}, []string{claudeSkillsDir}, false, ""},
		{"claude+antigravity", []string{KindClaude, KindAntigravity}, []string{claudeSkillsDir, agentsSkillsDir}, false, ""},
		{"cursor+copilot+antigravity", []string{KindCursor, KindCopilot, KindAntigravity}, []string{claudeSkillsDir, agentsSkillsDir}, false, ""},
		{"codex", []string{KindCodex}, []string{claudeSkillsDir}, true, claudeSkillsDir},
		{"all", []string{KindClaude, KindCursor, KindCopilot, KindCodex, KindAntigravity}, []string{claudeSkillsDir, agentsSkillsDir}, true, claudeSkillsDir},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			placement := PlanSkillPlacement(testCase.selected)
			if !reflect.DeepEqual(placement.Folders, testCase.wantFolders) {
				t.Errorf("Folders = %v, want %v", placement.Folders, testCase.wantFolders)
			}
			if placement.NeedsCodexCfg != testCase.wantCodexCfg {
				t.Errorf("NeedsCodexCfg = %v, want %v", placement.NeedsCodexCfg, testCase.wantCodexCfg)
			}
			if placement.CodexFolder != testCase.wantCodexDir {
				t.Errorf("CodexFolder = %q, want %q", placement.CodexFolder, testCase.wantCodexDir)
			}
		})
	}
}

func TestPlanSkillPlacement_ReadsFrom(t *testing.T) {
	placement := PlanSkillPlacement([]string{KindCursor, KindCopilot, KindAntigravity})
	want := map[string]string{
		KindCursor:      claudeSkillsDir,
		KindCopilot:     claudeSkillsDir,
		KindAntigravity: agentsSkillsDir,
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
