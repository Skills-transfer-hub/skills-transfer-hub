package cmd

import (
	"context"
	"fmt"
	"loreskills/internal/fs"
)

func (a *app) runInstall(skillName string) error {
	cfg, err := fs.ReadConfig()
	if err != nil {
		return err
	}

	content, err := a.client.DownloadSkill(context.Background(), cfg.RepoRawURL, skillName)
	if err != nil {
		return err
	}

	if err := fs.InstallSkill(cfg.TargetDir, skillName, content); err != nil {
		return err
	}

	fmt.Fprintf(a.stdout, "Skill '%s' installé dans %s.\n", skillName, cfg.TargetDir)
	return nil
}
