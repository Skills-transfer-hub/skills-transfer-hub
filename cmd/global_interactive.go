package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"

	sthhttp "sth/internal/http"
	"sth/internal/writer"
)

func (a *app) chooseGlobalAgents(input *bufio.Reader, options globalOptions) (globalOptions, error) {
	if len(options.Agents) > 0 || options.AllAgents || !a.isInteractiveErrorSession() || options.JSON || options.Yes {
		return options, nil
	}
	executables := map[string]string{"claude": "claude", "codex": "codex", "cursor": "cursor", "gemini": "gemini", "copilot-cli": "copilot", "antigravity-cli": "antigravity"}
	detected := []string{}
	a.writeStderr("Select global agents by ID, separated by commas; 'all' selects --all-agents:")
	for _, consumer := range writer.GlobalConsumers() {
		label := consumer.ID
		if executable := executables[consumer.ID]; executable != "" {
			if _, err := exec.LookPath(executable); err == nil {
				detected = append(detected, consumer.ID)
				label += " (executable detected)"
			}
		}
		a.writeStderr("  " + label)
	}
	if len(detected) > 0 {
		a.writeStderr("Enter selects detected executables: " + strings.Join(detected, ", "))
	}
	selection, err := readGlobalSelection(input)
	if err != nil {
		return options, err
	}
	switch selection {
	case "all":
		options.AllAgents = true
	case "":
		options.Agents = detected
	default:
		options.Agents = splitGlobalSelection(selection)
	}
	return options, nil
}

func (a *app) chooseGlobalSkills(input *bufio.Reader, candidates []sthhttp.GlobalCandidate, options globalOptions) ([]sthhttp.GlobalCandidate, error) {
	if len(options.Skills) > 0 || len(candidates) < 2 || !a.isInteractiveErrorSession() || options.JSON || options.Yes {
		return sthhttp.SelectGlobalSkills(candidates, options.Skills)
	}
	a.writeStderr("Select skills by name or number, separated by commas; '*' selects all:")
	for index, candidate := range candidates {
		a.writeStderr(fmt.Sprintf("  %d. %s (%s)", index+1, candidate.Name, candidate.Path))
	}
	selection, err := readGlobalSelection(input)
	if err != nil {
		return nil, err
	}
	choices := splitGlobalSelection(selection)
	if len(choices) == 0 {
		return nil, globalSelectionError("choose at least one skill")
	}
	selected := []sthhttp.GlobalCandidate{}
	seen := map[string]bool{}
	for _, choice := range choices {
		var matches []sthhttp.GlobalCandidate
		if number, parseErr := strconv.Atoi(choice); parseErr == nil {
			if number < 1 || number > len(candidates) {
				return nil, globalSelectionError("invalid skill number %q", choice)
			}
			matches = []sthhttp.GlobalCandidate{candidates[number-1]}
		} else {
			matches, err = sthhttp.SelectGlobalSkills(candidates, []string{choice})
			if err != nil {
				return nil, err
			}
		}
		for _, candidate := range matches {
			if !seen[candidate.Path] {
				seen[candidate.Path] = true
				selected = append(selected, candidate)
			}
		}
	}
	return selected, nil
}

func readGlobalSelection(input *bufio.Reader) (string, error) {
	selection, err := input.ReadString('\n')
	if err != nil && (err != io.EOF || selection == "") {
		return "", globalSelectionError("selection required: %v", err)
	}
	return strings.TrimSpace(selection), nil
}

func splitGlobalSelection(selection string) []string {
	return strings.FieldsFunc(selection, func(character rune) bool { return character == ',' || character == ' ' || character == '\t' })
}
