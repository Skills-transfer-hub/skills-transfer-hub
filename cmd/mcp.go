package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"sth/internal/auth"
	"sth/internal/cloud"
)

const mcpServerVersion = "1"
const mcpConfigPath = ".mcp.json"

const mcpCodexConfig = `[mcp_servers.sth]
command = "sth"
args = ["mcp"]`

const mcpGeminiConfig = `{
  "mcpServers": {
    "sth": { "command": "sth", "args": ["mcp"] }
  }
}`

type librarySearchArgs struct {
	Query string `json:"query,omitempty" jsonschema:"free-text matched against library item titles"`
	Kind  string `json:"kind,omitempty" jsonschema:"restrict to a kind: prompt or artifact"`
}

type libraryGetArgs struct {
	ID string `json:"id" jsonschema:"the library item id returned by library_search"`
}

type librarySaveArgs struct {
	Kind        string `json:"kind,omitempty" jsonschema:"prompt or artifact (defaults to prompt)"`
	Title       string `json:"title" jsonschema:"a short human-readable title"`
	Body        string `json:"body" jsonschema:"the full prompt or artifact content"`
	SourceTool  string `json:"source_tool,omitempty" jsonschema:"the tool it came from, e.g. claude"`
	SourceModel string `json:"source_model,omitempty" jsonschema:"the model, e.g. claude-opus-4"`
}

func (a *app) runMCP(args []string) error {
	if len(args) > 0 && isHelpFlag(args[0]) {
		a.writeStdout(commandHelpText("mcp"))
		return nil
	}
	if len(args) == 0 {
		return a.runMCPServe()
	}
	switch args[0] {
	case "install":
		return a.runMCPInstall(args[1:])
	default:
		return fmt.Errorf("sous-commande mcp inconnue : %q", args[0])
	}
}

func (a *app) runMCPServe() error {
	if _, err := auth.LoadAPIKey(); err != nil {
		return errors.New("aucune clé STH Cloud configurée — exécute `sth login` avant `sth mcp`.")
	}
	server := newLibraryMCPServer()
	return server.Run(context.Background(), &mcp.StdioTransport{})
}

func newLibraryMCPServer() *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "sth", Version: mcpServerVersion}, nil)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "library_search",
		Description: "Search the team STH library of reusable prompts and AI artifacts.",
	}, handleLibrarySearch)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "library_get",
		Description: "Fetch the full content of one STH library item by id and record it as reused.",
	}, handleLibraryGet)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "library_save",
		Description: "Save a prompt or an AI output to the team STH library.",
	}, handleLibrarySave)
	return server
}

func handleLibrarySearch(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input librarySearchArgs,
) (*mcp.CallToolResult, any, error) {
	matches, err := cloud.SearchLibrary(ctx, nil, input.Query, input.Kind)
	if err != nil {
		return mcpErrorResult(err), nil, nil
	}
	if len(matches) == 0 {
		return mcpTextResult("No matching library items."), nil, nil
	}
	lines := make([]string, 0, len(matches))
	for _, match := range matches {
		source := joinSource(match.SourceTool, match.SourceModel)
		line := fmt.Sprintf("- [%s] %s (id %s)", match.Kind, match.Title, match.ID)
		if source != "" {
			line += " — " + source
		}
		lines = append(lines, line)
	}
	return mcpTextResult(strings.Join(lines, "\n")), nil, nil
}

func handleLibraryGet(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input libraryGetArgs,
) (*mcp.CallToolResult, any, error) {
	item, err := cloud.GetLibrary(ctx, nil, input.ID)
	if err != nil {
		return mcpErrorResult(err), nil, nil
	}
	if reuseErr := cloud.RecordReuse(ctx, nil, item.ID); reuseErr != nil {
		fmt.Fprintln(os.Stderr, "sth mcp: réutilisation non enregistrée :", reuseErr)
	}
	source := joinSource(item.SourceTool, item.SourceModel)
	header := fmt.Sprintf("# %s [%s]", item.Title, item.Kind)
	if source != "" {
		header += "\nsource: " + source
	}
	return mcpTextResult(header + "\n\n" + item.Body), nil, nil
}

func handleLibrarySave(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input librarySaveArgs,
) (*mcp.CallToolResult, any, error) {
	kind := input.Kind
	if kind == "" {
		kind = "prompt"
	}
	id, err := cloud.SaveLibrary(ctx, nil, cloud.LibrarySaveInput{
		Kind:        kind,
		Title:       input.Title,
		Body:        input.Body,
		SourceTool:  input.SourceTool,
		SourceModel: input.SourceModel,
	})
	if err != nil {
		return mcpErrorResult(err), nil, nil
	}
	return mcpTextResult(fmt.Sprintf("Saved to the STH library (id %s).", id)), nil, nil
}

func mcpTextResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}

func mcpErrorResult(err error) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
	}
}

func joinSource(sourceTool string, sourceModel string) string {
	parts := make([]string, 0, 2)
	if sourceTool != "" {
		parts = append(parts, sourceTool)
	}
	if sourceModel != "" {
		parts = append(parts, sourceModel)
	}
	return strings.Join(parts, " · ")
}

func (a *app) runMCPInstall(_ []string) error {
	updated, err := writeClaudeMCPConfig(mcpConfigPath)
	if err != nil {
		return err
	}
	action := "créé"
	if updated {
		action = "mis à jour"
	}
	a.writeStdout(
		fmt.Sprintf("✔ %s — serveur MCP « sth » %s pour Claude Code.", mcpConfigPath, action),
		"",
		"Codex — ajoutez ceci à ~/.codex/config.toml :",
		mcpCodexConfig,
		"",
		"Gemini CLI — ajoutez ceci à ~/.gemini/settings.json :",
		mcpGeminiConfig,
		"",
		"Claude Desktop réutilise la même entrée mcpServers que Claude Code.",
		"Rappel : lancez `sth login` une fois pour vous identifier avec votre clé de licence.",
	)
	return nil
}

func writeClaudeMCPConfig(path string) (bool, error) {
	root := map[string]any{}
	existing, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(existing, &root); err != nil {
			return false, fmt.Errorf("%s existant illisible : %w", path, err)
		}
	case errors.Is(err, os.ErrNotExist):
	default:
		return false, err
	}

	servers, _ := root["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	_, alreadyPresent := servers["sth"]
	servers["sth"] = map[string]any{"command": "sth", "args": []string{"mcp"}}
	root["mcpServers"] = servers

	encoded, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return false, err
	}
	encoded = append(encoded, '\n')
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		return false, err
	}
	return alreadyPresent, nil
}
