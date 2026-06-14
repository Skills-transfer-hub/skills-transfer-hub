package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const protocolVersionDefault = "2024-11-05"

type LibraryMatch struct {
	ID          string
	Kind        string
	Title       string
	SourceTool  string
	SourceModel string
}

type LibraryDocument struct {
	ID          string
	Kind        string
	Title       string
	Body        string
	SourceTool  string
	SourceModel string
}

type LibrarySaveRequest struct {
	Kind        string
	Title       string
	Body        string
	SourceTool  string
	SourceModel string
}

type LibraryBackend interface {
	Search(ctx context.Context, query, kind string) ([]LibraryMatch, error)
	Get(ctx context.Context, identifier string) (LibraryDocument, error)
	Save(ctx context.Context, request LibrarySaveRequest) (string, error)
}

type Server struct {
	reader  io.Reader
	writer  io.Writer
	backend LibraryBackend
	name    string
	version string
}

func NewServer(reader io.Reader, writer io.Writer, backend LibraryBackend, version string) *Server {
	resolvedVersion := strings.TrimSpace(version)
	if resolvedVersion == "" {
		resolvedVersion = "dev"
	}
	return &Server{reader: reader, writer: writer, backend: backend, name: "sth", version: resolvedVersion}
}

func (server *Server) Serve(ctx context.Context) error {
	decoder := json.NewDecoder(server.reader)
	encoder := json.NewEncoder(server.writer)
	for {
		var request rpcRequest
		if err := decoder.Decode(&request); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		response := server.handle(ctx, request)
		if response == nil {
			continue
		}
		if err := encoder.Encode(response); err != nil {
			return err
		}
	}
}

func (server *Server) handle(ctx context.Context, request rpcRequest) *rpcResponse {
	if isNotification(request.ID) {
		return nil
	}
	switch request.Method {
	case "initialize":
		return server.success(request.ID, server.initializeResult(request.Params))
	case "ping":
		return server.success(request.ID, map[string]any{})
	case "tools/list":
		return server.success(request.ID, map[string]any{"tools": toolCatalog()})
	case "tools/call":
		return server.success(request.ID, server.callTool(ctx, request.Params))
	default:
		return server.failure(request.ID, -32601, fmt.Sprintf("method not found: %s", request.Method))
	}
}

func isNotification(identifier json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(identifier))
	return trimmed == "" || trimmed == "null"
}

func (server *Server) success(identifier json.RawMessage, result any) *rpcResponse {
	return &rpcResponse{JSONRPC: "2.0", ID: identifier, Result: result}
}

func (server *Server) failure(identifier json.RawMessage, code int, message string) *rpcResponse {
	return &rpcResponse{JSONRPC: "2.0", ID: identifier, Error: &rpcError{Code: code, Message: message}}
}

func (server *Server) initializeResult(params json.RawMessage) map[string]any {
	protocolVersion := protocolVersionDefault
	if len(params) > 0 {
		var decoded struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if err := json.Unmarshal(params, &decoded); err == nil && strings.TrimSpace(decoded.ProtocolVersion) != "" {
			protocolVersion = decoded.ProtocolVersion
		}
	}
	return map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{"tools": map[string]any{}},
		"serverInfo":      map[string]any{"name": server.name, "version": server.version},
	}
}

func toolCatalog() []toolDescriptor {
	return []toolDescriptor{
		{
			Name:        "library_search",
			Description: "Search the team STH library of reusable prompts and AI artifacts.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string", "description": "Free-text search over titles and bodies."},
					"kind":  map[string]any{"type": "string", "enum": []string{"prompt", "artifact"}, "description": "Optional kind filter."},
				},
			},
		},
		{
			Name:        "library_get",
			Description: "Fetch the full content of one STH library item by id and record it as reused.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"id": map[string]any{"type": "string", "description": "The library item id."}},
				"required":   []string{"id"},
			},
		},
		{
			Name:        "library_save",
			Description: "Save a prompt or an AI output to the team STH library.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"kind":         map[string]any{"type": "string", "enum": []string{"prompt", "artifact"}},
					"title":        map[string]any{"type": "string"},
					"body":         map[string]any{"type": "string"},
					"source_tool":  map[string]any{"type": "string", "description": "claude, chatgpt, gemini, …"},
					"source_model": map[string]any{"type": "string"},
				},
				"required": []string{"kind", "title", "body"},
			},
		},
	}
}

func (server *Server) callTool(ctx context.Context, params json.RawMessage) callResult {
	var call struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &call); err != nil {
		return errorResult(fmt.Sprintf("invalid tool call: %v", err))
	}
	switch call.Name {
	case "library_search":
		return server.runSearch(ctx, call.Arguments)
	case "library_get":
		return server.runGet(ctx, call.Arguments)
	case "library_save":
		return server.runSave(ctx, call.Arguments)
	default:
		return errorResult(fmt.Sprintf("unknown tool: %s", call.Name))
	}
}

func (server *Server) runSearch(ctx context.Context, arguments json.RawMessage) callResult {
	var input struct {
		Query string `json:"query"`
		Kind  string `json:"kind"`
	}
	_ = json.Unmarshal(arguments, &input)
	matches, err := server.backend.Search(ctx, input.Query, input.Kind)
	if err != nil {
		return errorResult(err.Error())
	}
	if len(matches) == 0 {
		return textResult("No matching library items.")
	}
	var builder strings.Builder
	for _, match := range matches {
		builder.WriteString(fmt.Sprintf("- [%s] %s (id %s)", match.Kind, match.Title, match.ID))
		source := strings.TrimSpace(match.SourceTool + " " + match.SourceModel)
		if source != "" {
			builder.WriteString(" — " + source)
		}
		builder.WriteString("\n")
	}
	return textResult(strings.TrimRight(builder.String(), "\n"))
}

func (server *Server) runGet(ctx context.Context, arguments json.RawMessage) callResult {
	var input struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(arguments, &input); err != nil || strings.TrimSpace(input.ID) == "" {
		return errorResult("a library item id is required")
	}
	document, err := server.backend.Get(ctx, strings.TrimSpace(input.ID))
	if err != nil {
		return errorResult(err.Error())
	}
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("# %s [%s]\n", document.Title, document.Kind))
	source := strings.TrimSpace(document.SourceTool + " " + document.SourceModel)
	if source != "" {
		builder.WriteString("source: " + source + "\n")
	}
	builder.WriteString("\n")
	builder.WriteString(document.Body)
	return textResult(builder.String())
}

func (server *Server) runSave(ctx context.Context, arguments json.RawMessage) callResult {
	var input struct {
		Kind        string `json:"kind"`
		Title       string `json:"title"`
		Body        string `json:"body"`
		SourceTool  string `json:"source_tool"`
		SourceModel string `json:"source_model"`
	}
	if err := json.Unmarshal(arguments, &input); err != nil {
		return errorResult(fmt.Sprintf("invalid arguments: %v", err))
	}
	kind := strings.TrimSpace(input.Kind)
	if kind == "" {
		kind = "prompt"
	}
	if strings.TrimSpace(input.Title) == "" || strings.TrimSpace(input.Body) == "" {
		return errorResult("title and body are required")
	}
	identifier, err := server.backend.Save(ctx, LibrarySaveRequest{
		Kind:        kind,
		Title:       strings.TrimSpace(input.Title),
		Body:        input.Body,
		SourceTool:  strings.TrimSpace(input.SourceTool),
		SourceModel: strings.TrimSpace(input.SourceModel),
	})
	if err != nil {
		return errorResult(err.Error())
	}
	return textResult(fmt.Sprintf("Saved to the STH library (id %s).", identifier))
}

func textResult(text string) callResult {
	return callResult{Content: []textContent{{Type: "text", Text: text}}}
}

func errorResult(text string) callResult {
	return callResult{Content: []textContent{{Type: "text", Text: text}}, IsError: true}
}
