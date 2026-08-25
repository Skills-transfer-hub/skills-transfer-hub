package tui

import (
	"bytes"
	"strings"
	"testing"
)

func TestProgressPrintPersistsOutputAndKeepsRendering(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	progress := NewProgress(&stdout, &stderr, true)
	progress.Start(2)
	progress.Step("First")
	if !progress.Print("Resource updated.\n") {
		t.Fatal("Print returned false")
	}
	progress.Step("Second")
	progress.Stop()

	output := stdout.String()
	if !strings.Contains(output, "Resource updated.") {
		t.Fatalf("persistent output missing: %q", output)
	}
	if !strings.Contains(output, "Second") {
		t.Fatalf("progress stopped before the second step: %q", output)
	}
	if strings.Contains(output, "✔") {
		t.Fatalf("neutral stop rendered success: %q", output)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestProgressCompleteRendersSuccess(t *testing.T) {
	var stdout bytes.Buffer
	progress := NewProgress(&stdout, &bytes.Buffer{}, true)
	progress.Start(1)
	progress.Step("Done")
	progress.Complete()
	if !strings.Contains(stdout.String(), "✔") {
		t.Fatalf("success marker missing: %q", stdout.String())
	}
}
