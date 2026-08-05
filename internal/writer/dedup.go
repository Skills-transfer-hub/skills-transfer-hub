package writer

const (
	claudeSkillsDir = ".claude/skills"
	agentsSkillsDir = ".agents/skills"
)

type SkillPlacement struct {
	Folders   []string
	ReadsFrom map[string]string
}

func PlanSkillPlacement(selectedKinds []string) SkillPlacement {
	selected := make(map[string]bool, len(selectedKinds))
	for _, kind := range selectedKinds {
		selected[kind] = true
	}

	hasClaudeSkills := selected[KindClaude] || selected[KindCursor] || selected[KindCopilot]
	hasAgentSkills := selected[KindCodex] || selected[KindAntigravity] || selected[KindGemini]

	placement := SkillPlacement{ReadsFrom: map[string]string{}}

	if hasClaudeSkills {
		placement.Folders = append(placement.Folders, claudeSkillsDir)
		for _, kind := range []string{KindClaude, KindCursor, KindCopilot} {
			if selected[kind] {
				placement.ReadsFrom[kind] = claudeSkillsDir
			}
		}
	}
	if hasAgentSkills {
		placement.Folders = append(placement.Folders, agentsSkillsDir)
		for _, kind := range []string{KindCodex, KindAntigravity, KindGemini} {
			if selected[kind] {
				placement.ReadsFrom[kind] = agentsSkillsDir
			}
		}
	}

	return placement
}

func WriterForSkillsFolder(folder string) (Target, bool) {
	switch folder {
	case claudeSkillsDir:
		return ClaudeWriter{}, true
	case agentsSkillsDir:
		return AntigravityWriter{}, true
	}
	return nil, false
}
