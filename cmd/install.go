package cmd

import (
	"fmt"
	"loreskills/internal/fetch"
	"loreskills/internal/fs"
)

func runInstall(skillName string) error {
	cfg, err := fs.ReadConfig()
	if err != nil {
		return fmt.Errorf("install : %w", err)
	}

	url := cfg.RepoRawURL + "/" + skillName + ".md"

	content, err := fetch.Download(url)
	if err != nil {
		return fmt.Errorf("install : %w", err)
	}

	if err := fs.EnsureTargetDir(cfg.TargetDir); err != nil {
		return fmt.Errorf("install : %w", err)
	}

	if err := fs.SaveSkill(cfg.TargetDir, skillName, content); err != nil {
		return fmt.Errorf("install : %w", err)
	}

	if err := fs.UpdateState(cfg.TargetDir, skillName); err != nil {
		return fmt.Errorf("install : %w", err)
	}

	fmt.Printf("✅ Skill '%s' installé dans %s/\n", skillName, cfg.TargetDir)
	return nil
}
