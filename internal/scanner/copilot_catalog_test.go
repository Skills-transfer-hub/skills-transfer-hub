package scanner

import "testing"

func TestCopilotAgentFilenameIsNotPartOfItsName(t *testing.T) {
	tree := []TreeEntry{{Path: "skills/alpha/SKILL.md", Type: "blob"}, {Path: "agents/api-architect.agent.md", Type: "blob"}}
	artifacts, err := PickScanner(tree, "").Scan(tree)
	if err != nil {
		t.Fatal(err)
	}
	for _, definition := range artifacts {
		if definition.Kind == KindAgent {
			if definition.Name != "api-architect" || definition.Entrypoint != "agents/api-architect.agent.md" {
				t.Fatalf("wrong Copilot reference: %+v", definition)
			}
			return
		}
	}
	t.Fatal("Copilot agent missing")
}
