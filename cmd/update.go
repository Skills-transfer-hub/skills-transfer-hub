package cmd

import (
	"context"
	"fmt"
	"loreskills/internal/fs"
)

func (a *app) runUpdate() error {
	cfg, err := fs.ReadConfig()
	if err != nil {
		return err
	}

	skills, err := fs.ListInstalledSkills(cfg.TargetDir)
	if err != nil {
		return err
	}

	if len(skills) == 0 {
		fmt.Fprintln(a.stdout, "Aucun skill à mettre à jour.")
		return nil
	}

	failures := 0
	for _, skillName := range skills {
		content, err := a.client.DownloadSkill(context.Background(), cfg.RepoRawURL, skillName)
		if err != nil {
			fmt.Fprintf(a.stderr, "Échec de mise à jour pour '%s' : %v\n", skillName, err)
			failures++
			continue
		}

		if err := fs.SaveSkill(cfg.TargetDir, skillName, content); err != nil {
			fmt.Fprintf(a.stderr, "Échec de mise à jour pour '%s' : %v\n", skillName, err)
			failures++
			continue
		}

		if err := fs.UpdateState(cfg.TargetDir, skillName); err != nil {
			fmt.Fprintf(a.stderr, "Échec de mise à jour pour '%s' : %v\n", skillName, err)
			failures++
			continue
		}

		fmt.Fprintf(a.stdout, "Skill '%s' mis à jour.\n", skillName)
	}

	if failures > 0 {
		return fmt.Errorf("mise à jour incomplète : %d skill(s) en échec", failures)
	}

	return nil
}
