package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func readMCPConfig(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	return root
}

func TestWriteClaudeMCPConfigCreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".mcp.json")
	updated, err := writeClaudeMCPConfig(path)
	if err != nil {
		t.Fatalf("writeClaudeMCPConfig: %v", err)
	}
	if updated {
		t.Fatalf("expected updated=false for a fresh file")
	}
	servers := readMCPConfig(t, path)["mcpServers"].(map[string]any)
	sth := servers["sth"].(map[string]any)
	if sth["command"] != "sth" {
		t.Fatalf("command = %v", sth["command"])
	}
	args := sth["args"].([]any)
	if len(args) != 1 || args[0] != "mcp" {
		t.Fatalf("args = %v", args)
	}
}

func TestWriteClaudeMCPConfigPreservesOtherServers(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".mcp.json")
	seed := []byte(`{"mcpServers":{"other":{"command":"foo"}}}`)
	if err := os.WriteFile(path, seed, 0o644); err != nil {
		t.Fatal(err)
	}
	updated, err := writeClaudeMCPConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if updated {
		t.Fatalf("expected updated=false when sth was absent")
	}
	servers := readMCPConfig(t, path)["mcpServers"].(map[string]any)
	if _, ok := servers["other"]; !ok {
		t.Fatalf("existing server dropped")
	}
	if _, ok := servers["sth"]; !ok {
		t.Fatalf("sth not added")
	}
}

func TestWriteClaudeMCPConfigReportsExistingSth(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".mcp.json")
	seed := []byte(`{"mcpServers":{"sth":{"command":"sth","args":["mcp"]}}}`)
	if err := os.WriteFile(path, seed, 0o644); err != nil {
		t.Fatal(err)
	}
	updated, err := writeClaudeMCPConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if !updated {
		t.Fatalf("expected updated=true when sth already present")
	}
}

func toolText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if result.IsError {
		t.Fatalf("tool returned error: %v", result.Content)
	}
	var builder strings.Builder
	for _, content := range result.Content {
		text, ok := content.(*mcp.TextContent)
		if !ok {
			t.Fatalf("unexpected content type %T", content)
		}
		builder.WriteString(text.Text)
	}
	return builder.String()
}

func TestLibraryMCPServerToolsEndToEnd(t *testing.T) {
	sawMCPHeader := false
	backend := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("X-STH-Client") == "mcp" {
			sawMCPHeader = true
		}
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/v1/library":
			_, _ = writer.Write([]byte(`{"items":[{"id":"11111111-1111-1111-1111-111111111111","kind":"prompt","title":"Cold email","source_tool":"claude","source_model":"claude-opus-4"}]}`))
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/events"):
			writer.WriteHeader(http.StatusAccepted)
			_, _ = writer.Write([]byte(`{"status":"logged"}`))
		case request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/v1/library/"):
			_, _ = writer.Write([]byte(`{"item":{"id":"11111111-1111-1111-1111-111111111111","kind":"prompt","title":"Cold email","body":"Write a friendly email.","source_tool":"claude","source_model":"claude-opus-4"}}`))
		case request.Method == http.MethodPost && request.URL.Path == "/v1/library":
			writer.WriteHeader(http.StatusCreated)
			_, _ = writer.Write([]byte(`{"id":"22222222-2222-2222-2222-222222222222"}`))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer backend.Close()

	t.Setenv("STH_CLOUD_BASE_URL", backend.URL)
	t.Setenv("STH_CLOUD_API_KEY", "lsk_live_testkey")

	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := newLibraryMCPServer().Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer serverSession.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer session.Close()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(tools.Tools) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(tools.Tools))
	}

	searchResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "library_search",
		Arguments: map[string]any{"query": "email"},
	})
	if err != nil {
		t.Fatalf("call library_search: %v", err)
	}
	if got := toolText(t, searchResult); !strings.Contains(got, "Cold email") {
		t.Fatalf("search text = %q", got)
	}

	getResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "library_get",
		Arguments: map[string]any{"id": "11111111-1111-1111-1111-111111111111"},
	})
	if err != nil {
		t.Fatalf("call library_get: %v", err)
	}
	if got := toolText(t, getResult); !strings.Contains(got, "Write a friendly email.") {
		t.Fatalf("get text = %q", got)
	}

	saveResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "library_save",
		Arguments: map[string]any{"title": "New idea", "body": "some content"},
	})
	if err != nil {
		t.Fatalf("call library_save: %v", err)
	}
	if got := toolText(t, saveResult); !strings.Contains(got, "22222222-2222-2222-2222-222222222222") {
		t.Fatalf("save text = %q", got)
	}

	if !sawMCPHeader {
		t.Fatalf("backend never saw the X-STH-Client: mcp header")
	}
}
