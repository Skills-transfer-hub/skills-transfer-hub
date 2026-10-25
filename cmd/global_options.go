package cmd

import (
	"errors"
	"fmt"
	"strings"
)

type globalOptions struct {
	Agents     []string
	Skills     []string
	Path       string
	Ref        string
	Positional []string
	AllAgents  bool
	DryRun     bool
	JSON       bool
	Force      bool
	Yes        bool
	Help       bool
}

type globalCommandError struct {
	code    int
	message string
}

func (e *globalCommandError) Error() string {
	return e.message
}

func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var commandError *globalCommandError
	if errors.As(err, &commandError) {
		return commandError.code
	}
	return 1
}

func globalSelectionError(format string, arguments ...any) error {
	return &globalCommandError{code: 2, message: fmt.Sprintf(format, arguments...)}
}

func hasGlobalScope(arguments []string) bool {
	for _, argument := range arguments {
		if argument == "--" {
			return false
		}
		if argument == "--global" || argument == "-g" || strings.HasPrefix(argument, "--global=") {
			return true
		}
	}
	return false
}

func parseGlobalOptions(operation string, arguments []string) (globalOptions, error) {
	options := globalOptions{}
	seen := map[string]bool{}
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if argument == "--" {
			options.Positional = append(options.Positional, arguments[index+1:]...)
			break
		}
		if !strings.HasPrefix(argument, "-") {
			options.Positional = append(options.Positional, argument)
			continue
		}
		name, value, hasValue := strings.Cut(argument, "=")
		switch name {
		case "--agent", "--skill", "--path", "--ref":
			if operation != "install" {
				return options, globalSelectionError("%s is only supported by global install", name)
			}
			if !hasValue {
				index++
				if index >= len(arguments) || strings.HasPrefix(arguments[index], "-") {
					return options, globalSelectionError("%s requires a value", name)
				}
				value = arguments[index]
			}
			if strings.TrimSpace(value) == "" {
				return options, globalSelectionError("%s requires a nonempty value", name)
			}
			if (name == "--path" || name == "--ref") && seen[name] {
				return options, globalSelectionError("%s may only be specified once", name)
			}
			seen[name] = true
			switch name {
			case "--agent":
				options.Agents = append(options.Agents, value)
			case "--skill":
				options.Skills = append(options.Skills, value)
			case "--path":
				options.Path = value
			case "--ref":
				options.Ref = value
			}
		case "--global", "-g", "--all-agents", "--dry-run", "--json", "--force", "--yes", "--help", "-h":
			if hasValue {
				return options, globalSelectionError("%s does not accept a value", name)
			}
			switch name {
			case "--all-agents":
				if operation != "install" {
					return options, globalSelectionError("--all-agents is only supported by global install")
				}
				options.AllAgents = true
			case "--dry-run", "--force", "--yes":
				if operation == "list" || operation == "status" {
					return options, globalSelectionError("%s is not supported by global %s", name, operation)
				}
				options.DryRun = options.DryRun || name == "--dry-run"
				options.Force = options.Force || name == "--force"
				options.Yes = options.Yes || name == "--yes"
			case "--json":
				options.JSON = true
			case "--help", "-h":
				options.Help = true
			}
		default:
			return options, globalSelectionError("unknown global %s option: %s", operation, argument)
		}
	}
	if options.AllAgents && len(options.Agents) > 0 {
		return options, globalSelectionError("--all-agents and --agent are mutually exclusive")
	}
	if options.Help {
		return options, nil
	}
	switch operation {
	case "install", "remove":
		if len(options.Positional) != 1 {
			return options, globalSelectionError("usage: sth %s <source-or-skill> --global", operation)
		}
	case "list", "status", "update":
		if len(options.Positional) > 1 || (operation == "list" && len(options.Positional) != 0) {
			return options, globalSelectionError("usage: sth %s --global", operation)
		}
	default:
		return options, globalSelectionError("--global is supported by install, list, status, update and remove")
	}
	if operation == "update" && len(options.Positional) == 1 && options.Positional[0] == "self" {
		return options, globalSelectionError("use sth update self without --global; address a skill named self by its canonical ID")
	}
	return options, nil
}
