package fs

import "testing"

func TestDisplayVersion(t *testing.T) {
	original := AlphaPhase
	t.Cleanup(func() { AlphaPhase = original })

	AlphaPhase = true
	if got := DisplayVersion("v1.2.3"); got != "Alpha v1.2.3" {
		t.Fatalf("alpha on: got %q, want %q", got, "Alpha v1.2.3")
	}
	if got := DisplayVersion("  v1.2.3  "); got != "Alpha v1.2.3" {
		t.Fatalf("alpha on trims: got %q", got)
	}
	if got := DisplayVersion(""); got != "" {
		t.Fatalf("empty stays empty, got %q", got)
	}

	AlphaPhase = false
	if got := DisplayVersion("v1.2.3"); got != "v1.2.3" {
		t.Fatalf("alpha off: got %q, want %q", got, "v1.2.3")
	}
}
