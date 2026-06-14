package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"sth/internal/cloud"
	"sth/internal/fs"
	"sth/internal/mcp"
)

const mcpConfigPath = ".mcp.json"

func (a *app) runMCP(args []string) error {
	if len(args) > 0 && isHelpFlag(args[0]) {
		a.writeStdout(commandHelpText("mcp"))
		return nil
	}
	if len(args) > 0 && args[0] == "install" {
		return a.runMCPInstall(args[1:])
	}
	if len(args) > 0 {
		return fmt.Errorf("sous-commande inconnue pour mcp : %q (essaie `sth mcp` ou `sth mcp install`)", args[0])
	}

	backend := newLibraryBackend()
	server := mcp.NewServer(a.stdin, a.stdout, backend, valueOrDefault(version, "dev"))
	return server.Serve(context.Background())
}

func (a *app) runMCPInstall(args []string) error {
	if len(args) > 0 && isHelpFlag(args[0]) {
		a.writeStdout(commandHelpText("mcp"))
		return nil
	}
	if err := mergeMCPServerConfig(mcpConfigPath); err != nil {
		return fmt.Errorf("écriture %s impossible : %w", mcpConfigPath, err)
	}
	a.writeStdout(
		fmt.Sprintf("Claude Code : %s mis à jour (serveur MCP « sth »).", mcpConfigPath),
		"",
		"Codex (~/.codex/config.toml) — ajoute :",
		"  [mcp_servers.sth]",
		"  command = \"sth\"",
		"  args = [\"mcp\"]",
		"",
		"Gemini (~/.gemini/settings.json) — ajoute sous \"mcpServers\" :",
		"  \"sth\": { \"command\": \"sth\", \"args\": [\"mcp\"] }",
		"",
		"Prérequis : exécute `sth login` une fois pour autoriser l'accès à la bibliothèque.",
	)
	return nil
}

func mergeMCPServerConfig(path string) error {
	document := map[string]any{}
	if existing, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(existing, &document)
	}
	servers, ok := document["mcpServers"].(map[string]any)
	if !ok {
		servers = map[string]any{}
	}
	servers["sth"] = map[string]any{
		"command": "sth",
		"args":    []string{"mcp"},
	}
	document["mcpServers"] = servers
	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(encoded, '\n'), 0o644)
}

type libraryBackend struct {
	orgID string
}

func newLibraryBackend() *libraryBackend {
	orgID := ""
	if selection, err := fs.ReadActiveOrg(); err == nil && selection != nil {
		orgID = selection.OrgID
	}
	return &libraryBackend{orgID: orgID}
}

func (backend *libraryBackend) Search(ctx context.Context, query, kind string) ([]mcp.LibraryMatch, error) {
	items, err := cloud.SearchLibrary(ctx, nil, backend.orgID, query, kind)
	if err != nil {
		return nil, err
	}
	matches := make([]mcp.LibraryMatch, 0, len(items))
	for _, item := range items {
		matches = append(matches, mcp.LibraryMatch{
			ID:          item.ID,
			Kind:        item.Kind,
			Title:       item.Title,
			SourceTool:  item.SourceTool,
			SourceModel: item.SourceModel,
		})
	}
	return matches, nil
}

func (backend *libraryBackend) Get(ctx context.Context, identifier string) (mcp.LibraryDocument, error) {
	detail, err := cloud.GetLibraryItem(ctx, nil, backend.orgID, identifier)
	if err != nil {
		return mcp.LibraryDocument{}, err
	}
	_ = cloud.RecordLibraryEvent(ctx, nil, backend.orgID, identifier, "reuse")
	return mcp.LibraryDocument{
		ID:          detail.Item.ID,
		Kind:        detail.Item.Kind,
		Title:       detail.Item.Title,
		Body:        detail.Item.Body,
		SourceTool:  detail.Item.SourceTool,
		SourceModel: detail.Item.SourceModel,
	}, nil
}

func (backend *libraryBackend) Save(ctx context.Context, request mcp.LibrarySaveRequest) (string, error) {
	return cloud.SaveLibraryItem(ctx, nil, backend.orgID, cloud.LibrarySaveInput{
		Kind:        request.Kind,
		Title:       request.Title,
		Body:        request.Body,
		SourceTool:  request.SourceTool,
		SourceModel: request.SourceModel,
	})
}
