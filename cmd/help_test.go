package cmd

import "testing"

func TestRun_helpCommandShowsGeneralHelp(t *testing.T) {
	result := runCLI(t, "", "", "help")

	if result.err != nil {
		t.Fatalf("Run() error = %v", result.err)
	}
	assertContains(t, result.stdout, asciiGuide)
	assertContains(t, result.stdout, "Commandes disponibles")
	assertContains(t, result.stdout, "help")
	assertContains(t, result.stdout, "version")
	assertEmpty(t, result.stderr)
}

func TestRun_helpCommandShowsCommandSpecificHelp(t *testing.T) {
	result := runCLI(t, "", "", "help", "install")

	if result.err != nil {
		t.Fatalf("Run() error = %v", result.err)
	}
	assertContains(t, result.stdout, asciiGuide)
	assertContains(t, result.stdout, "loreskills install <skill>")
	assertContains(t, result.stdout, "Telecharge et installe un skill")
	assertContains(t, result.stdout, "loreskills install mon-skill")
	assertEmpty(t, result.stderr)
}