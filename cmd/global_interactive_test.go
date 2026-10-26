package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGlobalTextReportsConflictPath(t *testing.T) {
	fixture := newGlobalFixture(t)
	if _, err := fixture.run(t, "install", "github:example/skills", "--agent=codex"); err != nil {
		t.Fatal(err)
	}
	managedPath := filepath.Join(fixture.home, ".agents", "skills", "ponytail", "SKILL.md")
	if err := os.WriteFile(managedPath, []byte("local edit"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := runCLIWithOptions(t, cliOptions{workDir: fixture.project, homeDir: fixture.home, userConfigDir: fixture.configuration, args: []string{"update", "ponytail", "--global"}, skipTelemetrySeed: true})
	if result.err == nil {
		t.Fatal("local change accepted")
	}
	assertContains(t, result.stderr, "modified locally")
	assertContains(t, result.stderr, filepath.Join("ponytail", "SKILL.md"))
}

func TestGlobalInteractiveSelection(t *testing.T) {
	fixture := newGlobalAcceptanceFixture(t)
	result := runCLIWithOptions(t, cliOptions{workDir: fixture.project, homeDir: fixture.home, userConfigDir: fixture.configuration, input: "codex\nreview\n", args: []string{"install", "github:example/skills", "--global"}, interactive: func() bool { return true }, skipTelemetrySeed: true})
	if result.err != nil {
		t.Fatal(result.err)
	}
	state := fixture.state(t)
	if len(state.Installations) != 1 || state.Installations[0].SkillName != "review" || len(state.Installations[0].RequestedConsumers) != 1 || state.Installations[0].RequestedConsumers[0] != "codex" {
		t.Fatalf("unexpected guided selection: %+v", state)
	}
	assertContains(t, result.stderr, "--all-agents")
	assertContains(t, result.stderr, "skills/review")
	assertContains(t, result.stdout, "codex")
}

func TestGlobalYesAndJSONDoNotInventSelection(t *testing.T) {
	fixture := newGlobalAcceptanceFixture(t)
	for _, arguments := range [][]string{
		{"install", "github:example/skills", "--global", "--yes"},
		{"install", "github:example/skills", "--global", "--json"},
		{"install", "github:example/skills", "--global", "--yes", "--agent", "codex"},
	} {
		result := runCLIWithOptions(t, cliOptions{workDir: fixture.project, homeDir: fixture.home, userConfigDir: fixture.configuration, input: "all\n*\n", args: arguments, interactive: func() bool { return true }, skipTelemetrySeed: true})
		if ExitCode(result.err) != 2 {
			t.Fatalf("%v invented selection: %v", arguments, result.err)
		}
	}
	entries, err := os.ReadDir(fixture.home)
	if err != nil || len(entries) != 0 {
		t.Fatalf("invalid selection wrote home: %v %v", entries, err)
	}
}

func TestGlobalHelpAndCompletion(t *testing.T) {
	for _, command := range []string{"install", "list", "status", "update", "remove"} {
		assertContains(t, commandHelpText(command), "--global")
	}
	for _, flag := range []string{"--all-agents", "--agent", "--skill", "--path", "--ref"} {
		assertContains(t, commandHelpText("install"), flag)
		for _, script := range []string{renderBashCompletion(visibleCommands()), renderZshCompletion(visibleCommands()), renderPowerShellCompletion(visibleCommands())} {
			assertContains(t, script, flag)
		}
	}
}
