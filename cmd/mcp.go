package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"sth/internal/auth"
	"sth/internal/cloud"
	"sth/internal/fs"
)

const mcpServerVersion = "1"
const mcpConfigPath = ".mcp.json"

var errLocalAssetNotFound = errors.New("local STH asset not found")
var errLocalLibraryItemNotFound = errors.New("local library item not found")

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

type assetSearchArgs struct {
	Query  string `json:"query,omitempty" jsonschema:"free-text matched against package and library assets"`
	Family string `json:"family,omitempty" jsonschema:"restrict to package or library"`
	Kind   string `json:"kind,omitempty" jsonschema:"restrict to skill, agent, command, instruction, prompt or artifact"`
}

type assetGetArgs struct {
	ID string `json:"id" jsonschema:"the asset id returned by asset_search"`
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
		return fmt.Errorf("unknown mcp subcommand: %q", args[0])
	}
}

func (a *app) runMCPServe() error {
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
	mcp.AddTool(server, &mcp.Tool{
		Name:        "asset_search",
		Description: "Search deployable packages and collaborative library assets through one STH read model.",
	}, handleAssetSearch)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "asset_get",
		Description: "Fetch one package or library asset returned by asset_search.",
	}, handleAssetGet)
	return server
}

func handleAssetSearch(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input assetSearchArgs,
) (*mcp.CallToolResult, any, error) {
	localMatches, err := localAssetSearch(input)
	if err != nil {
		return mcpErrorResult(err), nil, nil
	}
	if !mcpCloudConnected() {
		return renderAssetSearch(localMatches, nil), nil, nil
	}
	sharedMatches, sharedErr := cloud.SearchAssets(ctx, nil, input.Query, input.Family, input.Kind)
	matches := mergeAssetSearchResults(localMatches, sharedMatches)
	return renderAssetSearch(matches, sharedErr), nil, nil
}

func mergeAssetSearchResults(localMatches, sharedMatches []cloud.AssetSummary) []cloud.AssetSummary {
	matches := make([]cloud.AssetSummary, 0, len(localMatches)+len(sharedMatches))
	seenAssets := make(map[string]struct{}, len(localMatches)+len(sharedMatches))
	for _, match := range localMatches {
		key := match.Family + "\x00" + match.ID
		seenAssets[key] = struct{}{}
		matches = append(matches, match)
	}
	for _, match := range sharedMatches {
		key := match.Family + "\x00" + match.ID
		if _, seen := seenAssets[key]; seen {
			continue
		}
		seenAssets[key] = struct{}{}
		matches = append(matches, match)
	}
	return matches
}

func renderAssetSearch(matches []cloud.AssetSummary, sharedErr error) *mcp.CallToolResult {
	if len(matches) == 0 {
		return mcpTextResultWithSharedWarning("No matching STH assets.", sharedErr)
	}
	lines := make([]string, 0, len(matches))
	for _, match := range matches {
		line := fmt.Sprintf("- [%s/%s] %s (id %s)", match.Family, match.Kind, match.Name, match.ID)
		if strings.TrimSpace(match.Description) != "" {
			line += " — " + strings.TrimSpace(match.Description)
		}
		lines = append(lines, line)
	}
	return mcpTextResultWithSharedWarning(strings.Join(lines, "\n"), sharedErr)
}

func handleAssetGet(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input assetGetArgs,
) (*mcp.CallToolResult, any, error) {
	asset, localErr := localAssetGet(input.ID)
	if localErr == nil {
		return renderAssetGet(asset), nil, nil
	}
	if !errors.Is(localErr, errLocalAssetNotFound) {
		return mcpErrorResult(localErr), nil, nil
	}
	if !mcpCloudConnected() {
		return mcpErrorResult(localErr), nil, nil
	}
	asset, err := cloud.GetAsset(ctx, nil, input.ID)
	if err != nil {
		return mcpErrorResult(err), nil, nil
	}
	return renderAssetGet(asset), nil, nil
}

func renderAssetGet(asset cloud.AssetSummary) *mcp.CallToolResult {
	header := fmt.Sprintf("# %s [%s/%s]", asset.Name, asset.Family, asset.Kind)
	details := make([]string, 0, 2)
	if asset.SourcePath != "" {
		details = append(details, "source: "+asset.SourcePath)
	}
	if asset.Status != "" {
		details = append(details, "status: "+asset.Status)
	}
	content := strings.TrimSpace(asset.Body)
	if content == "" {
		content = strings.TrimSpace(asset.Description)
	}
	parts := []string{header}
	if len(details) > 0 {
		parts = append(parts, strings.Join(details, "\n"))
	}
	if content != "" {
		parts = append(parts, content)
	}
	return mcpTextResult(strings.Join(parts, "\n\n"))
}

func handleLibrarySearch(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input librarySearchArgs,
) (*mcp.CallToolResult, any, error) {
	localMatches, err := searchLocalLibrary(input)
	if err != nil {
		return mcpErrorResult(err), nil, nil
	}
	if !mcpCloudConnected() {
		return renderLibrarySearch(localMatches, nil, nil), nil, nil
	}
	sharedMatches, sharedErr := cloud.SearchLibrary(ctx, nil, input.Query, input.Kind)
	return renderLibrarySearch(localMatches, sharedMatches, sharedErr), nil, nil
}

func searchLocalLibrary(input librarySearchArgs) ([]fs.LocalLibraryItem, error) {
	items, err := fs.ReadLocalLibrary(".")
	if err != nil {
		return nil, err
	}
	matches := make([]fs.LocalLibraryItem, 0, len(items))
	query := strings.ToLower(strings.TrimSpace(input.Query))
	for _, item := range items {
		if input.Kind != "" && item.Kind != input.Kind {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(item.Title+" "+item.Body), query) {
			continue
		}
		matches = append(matches, item)
	}
	return matches, nil
}

func renderLibrarySearch(localMatches []fs.LocalLibraryItem, sharedMatches []cloud.LibraryMatch, sharedErr error) *mcp.CallToolResult {
	lines := make([]string, 0, len(localMatches)+len(sharedMatches))
	seenItems := make(map[string]struct{}, len(localMatches)+len(sharedMatches))
	for _, match := range localMatches {
		seenItems[match.ID] = struct{}{}
		lines = append(lines, fmt.Sprintf("- [%s] %s (id %s) — local", match.Kind, match.Title, match.ID))
	}
	for _, match := range sharedMatches {
		if _, seen := seenItems[match.ID]; seen {
			continue
		}
		seenItems[match.ID] = struct{}{}
		source := joinSource(match.SourceTool, match.SourceModel)
		line := fmt.Sprintf("- [%s] %s (id %s)", match.Kind, match.Title, match.ID)
		if source != "" {
			line += " — " + source
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return mcpTextResultWithSharedWarning("No matching library items.", sharedErr)
	}
	return mcpTextResultWithSharedWarning(strings.Join(lines, "\n"), sharedErr)
}

func handleLibraryGet(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input libraryGetArgs,
) (*mcp.CallToolResult, any, error) {
	localItem, localErr := findLocalLibraryItem(input.ID)
	if localErr == nil {
		return renderLocalLibraryGet(localItem), nil, nil
	}
	if !errors.Is(localErr, errLocalLibraryItemNotFound) {
		return mcpErrorResult(localErr), nil, nil
	}
	if !mcpCloudConnected() {
		return mcpErrorResult(localErr), nil, nil
	}
	item, err := cloud.GetLibrary(ctx, nil, input.ID)
	if err != nil {
		return mcpErrorResult(err), nil, nil
	}
	if reuseErr := cloud.RecordReuse(ctx, nil, item.ID); reuseErr != nil {
		fmt.Fprintln(os.Stderr, "sth mcp: reuse was not recorded:", reuseErr)
	}
	source := joinSource(item.SourceTool, item.SourceModel)
	header := fmt.Sprintf("# %s [%s]", item.Title, item.Kind)
	if source != "" {
		header += "\nsource: " + source
	}
	return mcpTextResult(header + "\n\n" + item.Body), nil, nil
}

func findLocalLibraryItem(itemID string) (fs.LocalLibraryItem, error) {
	items, err := fs.ReadLocalLibrary(".")
	if err != nil {
		return fs.LocalLibraryItem{}, err
	}
	itemID = strings.TrimSpace(itemID)
	for _, item := range items {
		if item.ID == itemID {
			return item, nil
		}
	}
	return fs.LocalLibraryItem{}, errLocalLibraryItemNotFound
}

func renderLocalLibraryGet(item fs.LocalLibraryItem) *mcp.CallToolResult {
	return mcpTextResult(fmt.Sprintf("# %s [%s]\nsource: local\n\n%s", item.Title, item.Kind, item.Body))
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
	if !mcpCloudConnected() {
		item, err := saveLocalLibraryItem(input, kind)
		if err != nil {
			return mcpErrorResult(err), nil, nil
		}
		return mcpTextResult(fmt.Sprintf("Saved to the local STH library (id %s).", item.ID)), nil, nil
	}
	id, err := cloud.SaveLibrary(ctx, nil, cloud.LibrarySaveInput{
		Kind:        kind,
		Title:       input.Title,
		Body:        input.Body,
		SourceTool:  input.SourceTool,
		SourceModel: input.SourceModel,
	})
	if err != nil {
		item, localErr := saveLocalLibraryItem(input, kind)
		if localErr != nil {
			return mcpErrorResult(fmt.Errorf("shared library save failed: %v; local fallback failed: %w", err, localErr)), nil, nil
		}
		text := fmt.Sprintf("Shared STH library unavailable: %v\n\nSaved to the local STH library (id %s).", err, item.ID)
		return mcpTextResult(text), nil, nil
	}
	return mcpTextResult(fmt.Sprintf("Saved to the STH library (id %s).", id)), nil, nil
}

func saveLocalLibraryItem(input librarySaveArgs, kind string) (fs.LocalLibraryItem, error) {
	tags := []string{"mcp"}
	if sourceTool := strings.TrimSpace(input.SourceTool); sourceTool != "" {
		tags = append(tags, sourceTool)
	}
	return fs.SaveLocalLibraryItem(".", fs.LocalLibrarySaveInput{
		Kind: kind, Title: input.Title, Body: input.Body, Tags: tags,
	})
}

func mcpCloudConnected() bool {
	if os.Getenv("STH_MCP_LOCAL_ONLY") == "1" {
		return false
	}
	_, err := auth.LoadAPIKey()
	return err == nil
}

func localAssetSearch(input assetSearchArgs) ([]cloud.AssetSummary, error) {
	assets := []cloud.AssetSummary{}
	if input.Family == "" || input.Family == "package" {
		project, err := readBridgeProject()
		if err != nil {
			return nil, err
		}
		resources, err := readBridgeResources(project.Config)
		if err != nil {
			return nil, err
		}
		for _, resource := range resources {
			assets = append(assets, cloud.AssetSummary{
				ID: resource.ResourceRef, Family: "package", Kind: resource.Kind,
				Name: resource.ResourceRef, Status: resource.Status,
			})
		}
	}
	if input.Family == "" || input.Family == "library" {
		items, err := fs.ReadLocalLibrary(".")
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			assets = append(assets, cloud.AssetSummary{
				ID: item.ID, Family: "library", Kind: item.Kind, Name: item.Title,
				Description: localAssetExcerpt(item.Body), Body: item.Body, Status: "available",
			})
		}
	}
	query := strings.ToLower(strings.TrimSpace(input.Query))
	filtered := make([]cloud.AssetSummary, 0, len(assets))
	for _, asset := range assets {
		if input.Kind != "" && asset.Kind != input.Kind {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(asset.Name+" "+asset.Description), query) {
			continue
		}
		filtered = append(filtered, asset)
	}
	return filtered, nil
}

func localAssetGet(id string) (cloud.AssetSummary, error) {
	id = strings.TrimSpace(id)
	assets, err := localAssetSearch(assetSearchArgs{})
	if err != nil {
		return cloud.AssetSummary{}, err
	}
	for _, asset := range assets {
		if asset.ID == id {
			if asset.Family == "package" {
				asset.SourcePath = filepath.ToSlash(filepath.Join(".sth", "project.json"))
			}
			return asset, nil
		}
	}
	return cloud.AssetSummary{}, errLocalAssetNotFound
}

func localAssetExcerpt(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= 240 {
		return value
	}
	return string(runes[:240])
}

func mcpTextResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}

func mcpTextResultWithSharedWarning(text string, sharedErr error) *mcp.CallToolResult {
	if sharedErr == nil {
		return mcpTextResult(text)
	}
	return mcpTextResult(fmt.Sprintf("%s\n\nShared STH data unavailable: %v", text, sharedErr))
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
	action := "created"
	if updated {
		action = "updated"
	}
	a.writeStdout(
		fmt.Sprintf("✔ %s — MCP server \"sth\" %s for Claude Code.", mcpConfigPath, action),
		"",
		"Codex — add this to ~/.codex/config.toml:",
		mcpCodexConfig,
		"",
		"Gemini CLI — add this to ~/.gemini/settings.json:",
		mcpGeminiConfig,
		"",
		"Claude Desktop uses the same mcpServers entry as Claude Code.",
		"Without a connection, the server remains local. Run `sth login` only to access your organization's shared assets.",
	)
	return nil
}

func writeClaudeMCPConfig(path string) (bool, error) {
	root := map[string]any{}
	existing, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(existing, &root); err != nil {
			return false, fmt.Errorf("existing %s cannot be read: %w", path, err)
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
