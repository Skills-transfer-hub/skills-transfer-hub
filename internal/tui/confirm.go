package tui

import "strings"

type ConfirmConfig struct {
	Question   string
	DefaultYes bool
}

func Confirm(session *Session, config ConfirmConfig) (bool, error) {
	options := []Option{
		{Label: "Yes", Aliases: []string{"o", "oui", "y", "yes"}},
		{Label: "No", Aliases: []string{"n", "non", "no"}},
	}
	recommended := 1
	if config.DefaultYes {
		recommended = 0
	}
	title := strings.TrimRight(strings.TrimSpace(config.Question), " \t.?")
	index, err := Select(session, SelectConfig{Title: title + " ?", Options: options, Recommended: recommended})
	if err != nil {
		return false, err
	}
	return index == 0, nil
}
