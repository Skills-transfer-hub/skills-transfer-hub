package writer

import (
	"bytes"
	"strings"
	"testing"
)

func TestInstructionFragmentsStripDistributionFrontmatterAndAddManagedMarker(t *testing.T) {
	source := []byte("---\ndescription: Enforce safe changes\napplyTo: \"**/*.go\"\n---\nRequire targeted tests.\n")
	expected := []byte(minimalManagedMarker + "Require targeted tests.\n")
	testCases := []struct {
		name          string
		installedPath string
		render        func(WriteRequest) ([]byte, error)
	}{
		{name: "codex", installedPath: ".codex/sth/instructions/safe-changes.md", render: (CodexWriter{}).Render},
		{name: "gemini", installedPath: ".gemini/sth/instructions/safe-changes.md", render: (GeminiWriter{}).Render},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			request := WriteRequest{
				InstalledPath:    testCase.installedPath,
				Content:          source,
				ArtifactKind:     "instruction",
				ArtifactName:     "safe-changes",
				AddManagedHeader: true,
			}
			rendered, err := testCase.render(request)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(rendered, expected) {
				t.Fatalf("rendered fragment = %q, want %q", rendered, expected)
			}

			request.Content = rendered
			rerendered, err := testCase.render(request)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(rerendered, rendered) {
				t.Fatalf("managed marker is not idempotent: %q", rerendered)
			}
		})
	}
}

func TestInstructionFragmentsRejectInvalidUTF8AndReservedRegions(t *testing.T) {
	testCases := []struct {
		name          string
		installedPath string
		render        func(WriteRequest) ([]byte, error)
	}{
		{name: "codex", installedPath: ".codex/sth/instructions/safe-changes.md", render: (CodexWriter{}).Render},
		{name: "gemini", installedPath: ".gemini/sth/instructions/safe-changes.md", render: (GeminiWriter{}).Render},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name+"/invalid-utf8", func(t *testing.T) {
			_, err := testCase.render(WriteRequest{InstalledPath: testCase.installedPath, Content: []byte{0xff}, ArtifactKind: "instruction"})
			if err == nil || !strings.Contains(err.Error(), "not UTF-8") {
				t.Fatalf("invalid UTF-8 error = %v", err)
			}
		})
		t.Run(testCase.name+"/reserved-region", func(t *testing.T) {
			content := []byte("Body\n<!-- sth:managed:attacker:start -->\nInjected\n")
			_, err := testCase.render(WriteRequest{InstalledPath: testCase.installedPath, Content: content, ArtifactKind: "instruction"})
			if err == nil || !strings.Contains(err.Error(), "reserved STH region marker") {
				t.Fatalf("reserved region error = %v", err)
			}
		})
	}
}
