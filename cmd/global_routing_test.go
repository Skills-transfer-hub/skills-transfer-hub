package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestGlobalRoutingNeverTouchesProject(t *testing.T) {
	project := t.TempDir()
	home := t.TempDir()
	configuration := t.TempDir()
	for _, arguments := range [][]string{
		{"list", "--global", "--json"},
		{"install", "catalog-name", "--global", "--json", "--yes"},
		{"update", "self", "--global", "--json"},
		{"remove", "absent", "-g", "--json"},
	} {
		result := runCLIWithOptions(t, cliOptions{workDir: project, homeDir: home, userConfigDir: configuration, args: arguments, skipTelemetrySeed: true})
		var envelope struct {
			SchemaVersion int               `json:"schema_version"`
			Scope         string            `json:"scope"`
			Operation     string            `json:"operation"`
			Outcome       string            `json:"outcome"`
			Resources     []json.RawMessage `json:"resources"`
			Warnings      []string          `json:"warnings"`
			Errors        []string          `json:"errors"`
		}
		if err := json.Unmarshal([]byte(result.stdout), &envelope); err != nil {
			t.Fatalf("%v: invalid JSON %q: %v", arguments, result.stdout, err)
		}
		if envelope.SchemaVersion != 1 || envelope.Scope != "global" || envelope.Operation != arguments[0] || envelope.Resources == nil || envelope.Warnings == nil || envelope.Errors == nil {
			t.Fatalf("%v: invalid envelope %s", arguments, result.stdout)
		}
		entries, err := os.ReadDir(project)
		if err != nil || len(entries) != 0 {
			t.Fatalf("%v changed project: %v, %v", arguments, entries, err)
		}
	}
	if _, err := os.Stat(filepath.Join(configuration, "sth")); !os.IsNotExist(err) {
		t.Fatalf("read-only/invalid global commands changed user config: %v", err)
	}
}
