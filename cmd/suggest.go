package cmd

import (
	"fmt"
	"sort"
	"strings"
)

const suggestionMaxDistance = 3

func editDistance(first, second string) int {
	firstRunes := []rune(strings.ToLower(first))
	secondRunes := []rune(strings.ToLower(second))
	previous := make([]int, len(secondRunes)+1)
	current := make([]int, len(secondRunes)+1)
	for column := 0; column <= len(secondRunes); column++ {
		previous[column] = column
	}
	for row := 1; row <= len(firstRunes); row++ {
		current[0] = row
		for column := 1; column <= len(secondRunes); column++ {
			cost := 1
			if firstRunes[row-1] == secondRunes[column-1] {
				cost = 0
			}
			current[column] = min3(
				current[column-1]+1,
				previous[column]+1,
				previous[column-1]+cost,
			)
		}
		copy(previous, current)
	}
	return previous[len(secondRunes)]
}

func min3(first, second, third int) int {
	smallest := first
	if second < smallest {
		smallest = second
	}
	if third < smallest {
		smallest = third
	}
	return smallest
}

type commandCandidate struct {
	name     string
	distance int
}

func commandSuggestions(input string) []string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return nil
	}
	candidates := make([]commandCandidate, 0, len(commandCatalog))
	for _, spec := range commandCatalog {
		if spec.hidden {
			continue
		}
		distance := editDistance(trimmed, spec.name)
		isPrefix := strings.HasPrefix(spec.name, strings.ToLower(trimmed))
		if distance > suggestionMaxDistance && !isPrefix {
			continue
		}
		candidates = append(candidates, commandCandidate{name: spec.name, distance: distance})
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.Slice(candidates, func(firstIndex, secondIndex int) bool {
		if candidates[firstIndex].distance != candidates[secondIndex].distance {
			return candidates[firstIndex].distance < candidates[secondIndex].distance
		}
		return candidates[firstIndex].name < candidates[secondIndex].name
	})
	if len(candidates) > 3 {
		candidates = candidates[:3]
	}
	names := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		names = append(names, fmt.Sprintf("`sth %s`", candidate.name))
	}
	verb := "Did you mean"
	if len(names) == 1 {
		verb = "Did you mean"
	}
	return []string{verb + ": " + strings.Join(names, ", ") + "?"}
}
