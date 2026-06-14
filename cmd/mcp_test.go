package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRunMCPInstall_writesProjectConfig(t *testing.T) {
	workDir := t.TempDir()
	result := runCLI(t, workDir, "", "mcp", "install")
	if result.err != nil {
		t.Fatalf("mcp install error: %v (stderr=%s)", result.err, result.stderr)
	}
	assertContains(t, result.stdout, ".mcp.json")
	assertContains(t, result.stdout, "config.toml")

	data, err := os.ReadFile(filepath.Join(workDir, ".mcp.json"))
	if err != nil {
		t.Fatalf("read .mcp.json: %v", err)
	}
	var document struct {
		MCPServers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("invalid .mcp.json: %v", err)
	}
	server, ok := document.MCPServers["sth"]
	if !ok {
		t.Fatalf("missing sth server: %s", data)
	}
	if server.Command != "sth" || len(server.Args) != 1 || server.Args[0] != "mcp" {
		t.Fatalf("unexpected server config: %+v", server)
	}
}

func TestRunMCPInstall_preservesExistingServers(t *testing.T) {
	workDir := t.TempDir()
	existing := `{"mcpServers":{"other":{"command":"other","args":[]}}}`
	if err := os.WriteFile(filepath.Join(workDir, ".mcp.json"), []byte(existing), 0o644); err != nil {
		t.Fatalf("seed .mcp.json: %v", err)
	}
	result := runCLI(t, workDir, "", "mcp", "install")
	if result.err != nil {
		t.Fatalf("mcp install error: %v", result.err)
	}
	data, _ := os.ReadFile(filepath.Join(workDir, ".mcp.json"))
	var document struct {
		MCPServers map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("invalid .mcp.json: %v", err)
	}
	if _, ok := document.MCPServers["other"]; !ok {
		t.Fatalf("existing server dropped: %s", data)
	}
	if _, ok := document.MCPServers["sth"]; !ok {
		t.Fatalf("sth server missing: %s", data)
	}
}

func TestRunMCP_unknownSubcommand(t *testing.T) {
	result := runCLI(t, "", "", "mcp", "bogus")
	if result.err == nil {
		t.Fatal("expected error for unknown mcp subcommand")
	}
	assertContains(t, result.stderr, "sous-commande inconnue")
}
