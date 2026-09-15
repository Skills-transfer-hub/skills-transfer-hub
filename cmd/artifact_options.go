package cmd

import (
	"fmt"
	"strings"
)

type commandOptions struct {
	flags      map[string]bool
	values     map[string][]string
	positional []string
}

func parseCommandOptions(args []string, booleanNames, valueNames []string) (commandOptions, error) {
	booleanSet := make(map[string]struct{}, len(booleanNames))
	for _, name := range booleanNames {
		booleanSet[name] = struct{}{}
	}
	valueSet := make(map[string]struct{}, len(valueNames))
	for _, name := range valueNames {
		valueSet[name] = struct{}{}
	}
	options := commandOptions{flags: map[string]bool{}, values: map[string][]string{}}
	for argumentIndex := 0; argumentIndex < len(args); argumentIndex++ {
		argument := args[argumentIndex]
		if _, exists := booleanSet[argument]; exists {
			options.flags[argument] = true
			continue
		}
		name, inlineValue, hasInlineValue := strings.Cut(argument, "=")
		if _, exists := valueSet[name]; exists {
			value := inlineValue
			if !hasInlineValue {
				argumentIndex++
				if argumentIndex >= len(args) {
					return commandOptions{}, fmt.Errorf("%s requires a value", name)
				}
				value = args[argumentIndex]
			}
			if strings.TrimSpace(value) == "" {
				return commandOptions{}, fmt.Errorf("%s requires a value", name)
			}
			options.values[name] = append(options.values[name], value)
			continue
		}
		if strings.HasPrefix(argument, "-") {
			return commandOptions{}, fmt.Errorf("unknown flag: %s", argument)
		}
		options.positional = append(options.positional, argument)
	}
	return options, nil
}

func optionValues(options commandOptions, name string) []string {
	values := make([]string, 0)
	for _, rawValue := range options.values[name] {
		for _, value := range strings.Split(rawValue, ",") {
			trimmedValue := strings.TrimSpace(value)
			if trimmedValue != "" {
				values = append(values, trimmedValue)
			}
		}
	}
	return values
}
