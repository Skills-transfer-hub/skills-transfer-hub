package cmd

import (
	"errors"
	"fmt"
	"strings"

	"sth/internal/fs"
)

func (a *app) runNew(args []string) error {
	if len(args) == 1 && isHelpFlag(args[0]) {
		a.writeStdout(commandHelpText("new"))
		return nil
	}
	if len(args) != 1 {
		return errors.New("usage: sth new <category>/<name>")
	}

	category, name, err := fs.ValidateSkillRef(args[0])
	if err != nil {
		return err
	}

	if err := fs.ScaffoldSkill(".", category, name); err != nil {
		if errors.Is(err, fs.ErrSkillAlreadyExists) {
			return fmt.Errorf("Skill already exists at %s/%s", category, name)
		}
		return err
	}

	a.writeStdout(fmt.Sprintf("Created %s/%s/SKILL.md", category, name))
	if strings.Contains(name, "_") {
		a.writeStdout(fmt.Sprintf("Suggestion: use %s for Agent Skills portability; the created folder remains %s.", strings.ReplaceAll(name, "_", "-"), name))
	}
	return nil
}
