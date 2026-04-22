package cmd

import "testing"

func TestRun_versionShowsBuildMetadata(t *testing.T) {
	result := runCLI(t, "", "", "version")

	if result.err != nil {
		t.Fatalf("Run() error = %v", result.err)
	}
	assertContains(t, result.stdout, asciiGuide)
	assertContains(t, result.stdout, "version:")
	assertContains(t, result.stdout, "commit:")
	assertContains(t, result.stdout, "build_date:")
	assertContains(t, result.stdout, "target:")
	assertEmpty(t, result.stderr)
}