package cmd

import (
	"strings"
	"testing"
)

func TestRun_helpCommandShowsGeneralHelp(t *testing.T) {
	result := runCLI(t, "", "", "help")

	if result.err != nil {
		t.Fatalf("Run() error = %v", result.err)
	}
	assertContains(t, result.stdout, "Available commands")
	assertContains(t, result.stdout, "list")
	assertContains(t, result.stdout, "help")
	assertContains(t, result.stdout, "version")
	assertNotContains(t, result.stdout, "stockholm")
	assertEmpty(t, result.stderr)
}

func TestRun_helpCommandShowsCommandSpecificHelp(t *testing.T) {
	result := runCLI(t, "", "", "help", "install")

	if result.err != nil {
		t.Fatalf("Run() error = %v", result.err)
	}
	assertContains(t, result.stdout, "sth install <resource-ref>")
	assertContains(t, result.stdout, "Install a remote catalog resource")
	assertContains(t, result.stdout, "sth install skills/analyse-risque")
	assertEmpty(t, result.stderr)
}

func TestHelpTextUsesCommandCatalogExamplesInOrder(t *testing.T) {
	help := helpText()
	parts := strings.Split(help, "\n\nExamples:\n")
	if len(parts) != 2 {
		t.Fatalf("help text %q does not contain exemples section", help)
	}

	got := strings.Split(parts[1], "\n")
	want := make([]string, 0, len(commandCatalog))
	for _, spec := range commandCatalog {
		if spec.hidden || len(spec.examples) == 0 {
			continue
		}
		want = append(want, "  "+spec.examples[0])
	}

	if len(got) != len(want) {
		t.Fatalf("got %d example lines, want %d: %q", len(got), len(want), parts[1])
	}

	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("example line %d = %q, want %q", index, got[index], want[index])
		}
	}
}
