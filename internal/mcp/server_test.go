package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type fakeBackend struct {
	saved      LibrarySaveRequest
	getCalls   []string
	searchHits []LibraryMatch
}

func (backend *fakeBackend) Search(ctx context.Context, query, kind string) ([]LibraryMatch, error) {
	return backend.searchHits, nil
}

func (backend *fakeBackend) Get(ctx context.Context, identifier string) (LibraryDocument, error) {
	backend.getCalls = append(backend.getCalls, identifier)
	return LibraryDocument{ID: identifier, Kind: "prompt", Title: "Cold email", Body: "Write a cold email"}, nil
}

func (backend *fakeBackend) Save(ctx context.Context, request LibrarySaveRequest) (string, error) {
	backend.saved = request
	return "new123", nil
}

func runSession(t *testing.T, backend LibraryBackend, requests ...string) []map[string]any {
	t.Helper()
	input := strings.Join(requests, "\n") + "\n"
	var output strings.Builder
	server := NewServer(strings.NewReader(input), &output, backend, "test")
	if err := server.Serve(context.Background()); err != nil {
		t.Fatalf("serve error: %v", err)
	}
	var responses []map[string]any
	decoder := json.NewDecoder(strings.NewReader(output.String()))
	for decoder.More() {
		var response map[string]any
		if err := decoder.Decode(&response); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		responses = append(responses, response)
	}
	return responses
}

func resultOf(t *testing.T, response map[string]any) map[string]any {
	t.Helper()
	result, ok := response["result"].(map[string]any)
	if !ok {
		t.Fatalf("missing result: %+v", response)
	}
	return result
}

func firstText(t *testing.T, result map[string]any) string {
	t.Helper()
	content, ok := result["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatalf("missing content: %+v", result)
	}
	entry := content[0].(map[string]any)
	return entry["text"].(string)
}

func TestServe_initializeEchoesProtocol(t *testing.T) {
	responses := runSession(t, &fakeBackend{},
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`,
	)
	if len(responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(responses))
	}
	result := resultOf(t, responses[0])
	if result["protocolVersion"] != "2025-06-18" {
		t.Fatalf("expected echoed protocol version, got %v", result["protocolVersion"])
	}
	serverInfo, _ := result["serverInfo"].(map[string]any)
	if serverInfo["name"] != "sth" {
		t.Fatalf("unexpected serverInfo: %+v", serverInfo)
	}
}

func TestServe_notificationProducesNoResponse(t *testing.T) {
	responses := runSession(t, &fakeBackend{},
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
	)
	if len(responses) != 0 {
		t.Fatalf("expected no response to a notification, got %d", len(responses))
	}
}

func TestServe_toolsListReturnsThreeTools(t *testing.T) {
	responses := runSession(t, &fakeBackend{},
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
	)
	result := resultOf(t, responses[0])
	tools, ok := result["tools"].([]any)
	if !ok || len(tools) != 3 {
		t.Fatalf("expected 3 tools, got %+v", result["tools"])
	}
}

func TestServe_librarySaveInvokesBackend(t *testing.T) {
	backend := &fakeBackend{}
	responses := runSession(t, backend,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"library_save","arguments":{"kind":"prompt","title":"Cold email","body":"Write..."}}}`,
	)
	if backend.saved.Title != "Cold email" || backend.saved.Kind != "prompt" {
		t.Fatalf("save not invoked correctly: %+v", backend.saved)
	}
	if !strings.Contains(firstText(t, resultOf(t, responses[0])), "new123") {
		t.Fatalf("expected saved id in response")
	}
}

func TestServe_libraryGetReturnsBody(t *testing.T) {
	backend := &fakeBackend{}
	responses := runSession(t, backend,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"library_get","arguments":{"id":"abc"}}}`,
	)
	if len(backend.getCalls) != 1 || backend.getCalls[0] != "abc" {
		t.Fatalf("get not invoked with id abc: %+v", backend.getCalls)
	}
	if !strings.Contains(firstText(t, resultOf(t, responses[0])), "Write a cold email") {
		t.Fatalf("expected body in response")
	}
}

func TestServe_unknownMethodReturnsError(t *testing.T) {
	responses := runSession(t, &fakeBackend{},
		`{"jsonrpc":"2.0","id":5,"method":"does/not/exist"}`,
	)
	errorObject, ok := responses[0]["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error response, got %+v", responses[0])
	}
	if errorObject["code"].(float64) != -32601 {
		t.Fatalf("expected -32601, got %v", errorObject["code"])
	}
}
