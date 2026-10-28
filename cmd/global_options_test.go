package cmd

import (
	"reflect"
	"testing"
)

func TestGlobalOptionsTypedSelection(t *testing.T) {
	options, err := parseGlobalOptions("install", []string{"github:owner/repo", "-g", "--agent", "claude", "--agent=codex", "--skill", "one", "--skill=two", "--path", "skills", "--ref=release/one", "--json", "--dry-run"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(options.Agents, []string{"claude", "codex"}) || !reflect.DeepEqual(options.Skills, []string{"one", "two"}) || options.Path != "skills" || options.Ref != "release/one" || !options.JSON || !options.DryRun {
		t.Fatalf("unexpected options: %+v", options)
	}
}

func TestGlobalOptionsRejectInvalidSelection(t *testing.T) {
	for _, arguments := range [][]string{
		{"--global", "github:owner/repo", "--agent"},
		{"--global", "github:owner/repo", "--agent", "--json"},
		{"--global", "github:owner/repo", "--agent=", "--json"},
		{"--global", "github:owner/repo", "--wat"},
		{"--global", "github:owner/repo", "--agent", "codex", "--all-agents"},
		{"--global", "github:owner/repo", "--ref=one", "--ref=two"},
		{"--global", "github:owner/repo", "--force=true"},
	} {
		if _, err := parseGlobalOptions("install", arguments); err == nil || ExitCode(err) != 2 {
			t.Fatalf("%v: expected selection error, got %v", arguments, err)
		}
	}
	for _, operation := range []string{"list", "status", "update", "remove"} {
		if _, err := parseGlobalOptions(operation, []string{"--global", "--agent=codex"}); err == nil {
			t.Fatalf("%s accepted install-only option", operation)
		}
	}
	if _, err := parseGlobalOptions("update", []string{"self", "--global"}); err == nil || ExitCode(err) != 2 {
		t.Fatalf("global self update accepted: %v", err)
	}
}
