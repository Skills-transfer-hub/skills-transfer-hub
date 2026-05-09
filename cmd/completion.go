package cmd

import (
	"fmt"
	"strings"
)

// runCompletion generates a shell completion script for one of the supported
// shells and prints it to stdout. Hidden commands (those absent from
// commandCatalog or marked Hidden=true) are skipped — they MUST NOT show up
// in completion (FR-025).
func (a *app) runCompletion(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage : sth completion <bash|zsh|powershell>")
	}
	shell := strings.ToLower(strings.TrimSpace(args[0]))
	switch shell {
	case "bash":
		a.writeStdout(renderBashCompletion(visibleCommands()))
	case "zsh":
		a.writeStdout(renderZshCompletion(visibleCommands()))
	case "powershell":
		a.writeStdout(renderPowerShellCompletion(visibleCommands()))
	default:
		return fmt.Errorf("Unsupported shell: %s. Supported: bash, zsh, powershell.", shell)
	}
	return nil
}

func visibleCommands() []commandSpec {
	out := make([]commandSpec, 0, len(commandCatalog))
	for _, spec := range commandCatalog {
		if spec.hidden {
			continue
		}
		out = append(out, spec)
	}
	return out
}

func renderBashCompletion(cmds []commandSpec) string {
	var b strings.Builder
	b.WriteString("# bash completion for sth — source this file to enable.\n")
	b.WriteString("_sth_completion() {\n")
	b.WriteString("    local cur prev cmd\n")
	b.WriteString("    COMPREPLY=()\n")
	b.WriteString("    cur=\"${COMP_WORDS[COMP_CWORD]}\"\n")
	b.WriteString("    cmd=\"${COMP_WORDS[1]}\"\n")
	b.WriteString("    if [ \"$COMP_CWORD\" -eq 1 ]; then\n")
	names := make([]string, 0, len(cmds))
	for _, c := range cmds {
		names = append(names, c.name)
	}
	fmt.Fprintf(&b, "        COMPREPLY=( $(compgen -W \"%s\" -- \"$cur\") )\n", strings.Join(names, " "))
	b.WriteString("        return 0\n")
	b.WriteString("    fi\n")
	b.WriteString("    case \"$cmd\" in\n")
	for _, c := range cmds {
		if len(c.flags) == 0 {
			continue
		}
		fmt.Fprintf(&b, "        %s)\n            COMPREPLY=( $(compgen -W \"%s\" -- \"$cur\") )\n            ;;\n", c.name, strings.Join(c.flags, " "))
	}
	b.WriteString("    esac\n")
	b.WriteString("}\n")
	b.WriteString("complete -F _sth_completion sth\n")
	return b.String()
}

func renderZshCompletion(cmds []commandSpec) string {
	var b strings.Builder
	b.WriteString("#compdef sth\n")
	b.WriteString("_sth() {\n")
	b.WriteString("    local -a commands\n")
	b.WriteString("    commands=(\n")
	for _, c := range cmds {
		summary := strings.ReplaceAll(c.summary, "'", "")
		fmt.Fprintf(&b, "        '%s:%s'\n", c.name, summary)
	}
	b.WriteString("    )\n")
	b.WriteString("    if (( CURRENT == 2 )); then\n")
	b.WriteString("        _describe 'command' commands\n")
	b.WriteString("        return\n")
	b.WriteString("    fi\n")
	b.WriteString("    case \"${words[2]}\" in\n")
	for _, c := range cmds {
		if len(c.flags) == 0 {
			continue
		}
		fmt.Fprintf(&b, "        %s) _values 'flag' %s ;;\n", c.name, joinSingleQuoted(c.flags))
	}
	b.WriteString("    esac\n")
	b.WriteString("}\n")
	b.WriteString("compdef _sth sth\n")
	return b.String()
}

func renderPowerShellCompletion(cmds []commandSpec) string {
	var b strings.Builder
	b.WriteString("# PowerShell completion for sth — dot-source this script to enable.\n")
	b.WriteString("Register-ArgumentCompleter -CommandName sth -ScriptBlock {\n")
	b.WriteString("    param($wordToComplete, $commandAst, $cursorPosition)\n")
	b.WriteString("    $tokens = $commandAst.CommandElements\n")
	b.WriteString("    if ($tokens.Count -le 2) {\n")
	names := make([]string, 0, len(cmds))
	for _, c := range cmds {
		names = append(names, "'"+c.name+"'")
	}
	fmt.Fprintf(&b, "        @(%s) | Where-Object { $_ -like \"$wordToComplete*\" } | ForEach-Object { [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_) }\n", strings.Join(names, ", "))
	b.WriteString("        return\n")
	b.WriteString("    }\n")
	b.WriteString("    $cmd = $tokens[1].Value\n")
	b.WriteString("    switch ($cmd) {\n")
	for _, c := range cmds {
		if len(c.flags) == 0 {
			continue
		}
		flagList := make([]string, 0, len(c.flags))
		for _, f := range c.flags {
			flagList = append(flagList, "'"+f+"'")
		}
		fmt.Fprintf(&b, "        '%s' { @(%s) | Where-Object { $_ -like \"$wordToComplete*\" } | ForEach-Object { [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_) } }\n", c.name, strings.Join(flagList, ", "))
	}
	b.WriteString("    }\n")
	b.WriteString("}\n")
	return b.String()
}

func joinSingleQuoted(values []string) string {
	parts := make([]string, 0, len(values))
	for _, v := range values {
		parts = append(parts, "'"+v+"'")
	}
	return strings.Join(parts, " ")
}
