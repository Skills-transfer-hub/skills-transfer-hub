package writer

const (
	claudeSkillsDir = ".claude/skills"
	agentsSkillsDir = ".agents/skills"
)

type SkillPlacement struct {
	Folders       []string
	ReadsFrom     map[string]string
	CodexFolder   string
	NeedsCodexCfg bool
}

func PlanSkillPlacement(selectedKinds []string) SkillPlacement {
	selected := make(map[string]bool, len(selectedKinds))
	for _, kind := range selectedKinds {
		selected[kind] = true
	}

	hasNonAntigravity := selected[KindClaude] || selected[KindCursor] || selected[KindCopilot] || selected[KindCodex]
	hasAntigravity := selected[KindAntigravity]

	placement := SkillPlacement{ReadsFrom: map[string]string{}}

	if hasNonAntigravity {
		placement.Folders = append(placement.Folders, claudeSkillsDir)
		for _, kind := range []string{KindClaude, KindCursor, KindCopilot, KindCodex} {
			if selected[kind] {
				placement.ReadsFrom[kind] = claudeSkillsDir
			}
		}
	}
	if hasAntigravity {
		placement.Folders = append(placement.Folders, agentsSkillsDir)
		placement.ReadsFrom[KindAntigravity] = agentsSkillsDir
	}
	if selected[KindCodex] {
		placement.NeedsCodexCfg = true
		placement.CodexFolder = claudeSkillsDir
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
