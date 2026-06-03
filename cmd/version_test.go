package cmd

import "testing"

func TestRun_versionShowsMinimalSignature(t *testing.T) {
	result := runCLI(t, "", "", "version")

	if result.err != nil {
		t.Fatalf("Run() error = %v", result.err)
	}
	assertContains(t, result.stdout, "STH Alpha dev")
	assertContains(t, result.stdout, versionByline)
	assertNotContains(t, result.stdout, "commit:")
	assertNotContains(t, result.stdout, "build_date:")
	assertEmpty(t, result.stderr)
}