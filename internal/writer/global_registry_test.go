package writer

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestGlobalConsumerSelectionExpandsFamiliesAndRejectsInvalidSelection(t *testing.T) {
	consumers, warnings, err := ResolveGlobalConsumers([]string{"copilot", "antigravity", "copilot-cli"}, false)
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{"antigravity-cli", "antigravity-ide", "copilot-cli", "copilot-vscode"}
	if !reflect.DeepEqual(consumers, expected) {
		t.Fatalf("consumers = %v, want %v", consumers, expected)
	}
	if len(warnings) == 0 || !strings.Contains(strings.Join(warnings, " "), "discovery-unverified") {
		t.Fatalf("documented support must not imply runtime certification: %v", warnings)
	}
	for _, selection := range [][]string{nil, {"unknown"}, {"codex-cloud"}} {
		if _, _, err := ResolveGlobalConsumers(selection, false); err == nil {
			t.Fatalf("selection %v should fail", selection)
		}
	}
	if _, _, err := ResolveGlobalConsumers([]string{"claude"}, true); err == nil {
		t.Fatal("all-agents and explicit agents must conflict")
	}
}

func TestGlobalAllAgentsPlansFourPhysicalCopiesWithoutWriting(t *testing.T) {
	home := t.TempDir()
	consumers, _, err := ResolveGlobalConsumers(nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(consumers) != 8 {
		t.Fatalf("consumers = %v, want eight surfaces", consumers)
	}
	destinations, _, err := PlanGlobalDestinations(home, "ponytail", consumers)
	if err != nil {
		t.Fatal(err)
	}
	expectedRoots := map[string]string{
		"agents":          filepath.Join(home, ".agents", "skills"),
		"claude":          filepath.Join(home, ".claude", "skills"),
		"antigravity-ide": filepath.Join(home, ".gemini", "config", "skills"),
		"antigravity-cli": filepath.Join(home, ".gemini", "antigravity-cli", "skills"),
	}
	if !reflect.DeepEqual(GlobalRoots(home), expectedRoots) {
		t.Fatalf("roots = %v, want %v", GlobalRoots(home), expectedRoots)
	}
	if len(destinations) != 4 {
		t.Fatalf("destinations = %+v, want four copies", destinations)
	}
	for _, destination := range destinations {
		if destination.Root != expectedRoots[destination.RootID] || destination.RelativePath != "ponytail" {
			t.Fatalf("unexpected destination %+v", destination)
		}
		if len(destination.RequestedConsumers) == 0 || len(destination.PossibleReaders) == 0 {
			t.Fatalf("missing consumers or readers: %+v", destination)
		}
	}
	entries, err := os.ReadDir(home)
	if err != nil || len(entries) != 0 {
		t.Fatalf("planning wrote files: %v, %v", entries, err)
	}
}

func TestGlobalSharedRootExposesUnrequestedReaders(t *testing.T) {
	destinations, _, err := PlanGlobalDestinations(t.TempDir(), "ponytail", []string{"codex"})
	if err != nil {
		t.Fatal(err)
	}
	if len(destinations) != 1 || !reflect.DeepEqual(destinations[0].RequestedConsumers, []string{"codex"}) {
		t.Fatalf("destinations = %+v", destinations)
	}
	expectedReaders := []string{"codex", "copilot-cli", "copilot-vscode", "cursor", "gemini"}
	if !reflect.DeepEqual(destinations[0].PossibleReaders, expectedReaders) {
		t.Fatalf("possible readers = %v, want %v", destinations[0].PossibleReaders, expectedReaders)
	}
}

func TestGlobalPlanRejectsUnsafeNamesAndRelativeHome(t *testing.T) {
	for _, name := range []string{"", "../escape", "/absolute", "nested/skill", "nested\\skill", "UPPER", "trailing.", "con", "lpt1", "nul", strings.Repeat("a", 65)} {
		if _, _, err := PlanGlobalDestinations(t.TempDir(), name, []string{"codex"}); err == nil {
			t.Errorf("accepted unsafe skill name %q", name)
		}
	}
	if _, _, err := PlanGlobalDestinations("relative", "ponytail", []string{"codex"}); err == nil {
		t.Fatal("accepted relative home")
	}
}

func TestGlobalPlanWarnsAboutNativeCoexistenceAndPreservesIt(t *testing.T) {
	home := t.TempDir()
	nativeSkill := filepath.Join(home, ".gemini", "skills", "ponytail", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(nativeSkill), 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte("third-party skill")
	if err := os.WriteFile(nativeSkill, content, 0o600); err != nil {
		t.Fatal(err)
	}
	destinations, warnings, err := PlanGlobalDestinations(home, "ponytail", []string{"gemini"})
	if err != nil {
		t.Fatal(err)
	}
	if len(destinations) != 1 || destinations[0].RootID != "agents" {
		t.Fatalf("destinations = %+v", destinations)
	}
	warningText := strings.Join(warnings, " ")
	if !strings.Contains(warningText, filepath.Dir(nativeSkill)) || !strings.Contains(warningText, "precedence") {
		t.Fatalf("missing coexistence and known priority: %v", warnings)
	}
	after, err := os.ReadFile(nativeSkill)
	if err != nil || !reflect.DeepEqual(after, content) {
		t.Fatalf("native skill changed: %q, %v", after, err)
	}
}

func TestGlobalPlanWarnsAboutDuplicateReaders(t *testing.T) {
	_, warnings, err := PlanGlobalDestinations(t.TempDir(), "ponytail", []string{"claude", "codex", "cursor", "copilot-vscode"})
	if err != nil {
		t.Fatal(err)
	}
	warningText := strings.Join(warnings, " ")
	for _, expected := range []string{"cursor", "copilot-vscode", "duplicate", "unverified"} {
		if !strings.Contains(warningText, expected) {
			t.Fatalf("warnings lack %q: %v", expected, warnings)
		}
	}
}

func TestGlobalPlanDoesNotFollowSymlinksInReadOnlyRoots(t *testing.T) {
	home := t.TempDir()
	external := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(home, ".cursor", "skills")); err != nil {
		t.Skip(err)
	}
	_, warnings, err := PlanGlobalDestinations(home, "ponytail", []string{"cursor"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(warnings, " "), "symbolic") {
		t.Fatalf("uninspected symbolic read root must be disclosed: %v", warnings)
	}
}

func TestGlobalRegistryDoesNotExposeMutableState(t *testing.T) {
	consumers := GlobalConsumers()
	consumers[0].ReadRoots[0] = "escape"
	consumers[0].Supported = false
	fresh := GlobalConsumers()
	if fresh[0].ReadRoots[0] != "claude" || !fresh[0].Supported {
		t.Fatal("registry was mutated by caller")
	}
}
