package cmd

import "testing"

func TestRun_stockholmShowsHiddenArt(t *testing.T) {
	result := runCLI(t, "", "", "stockholm")

	if result.err != nil {
		t.Fatalf("Run() error = %v", result.err)
	}
	assertContains(t, result.stdout, "______")
	assertNotContains(t, helpText(), "stockholm")
	assertEmpty(t, result.stderr)
}