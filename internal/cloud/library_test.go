package cloud

import (
	"context"
	"encoding/json"
	"io"
	stdhttp "net/http"
	"testing"
)

func TestSearchLibrary_OK(t *testing.T) {
	setupTestEnv(t)
	newTestServer(t, func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if r.URL.Path != "/v1/library" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer lsk_live_TESTKEY" {
			t.Errorf("missing bearer header")
		}
		if r.URL.Query().Get("kind") != "prompt" {
			t.Errorf("missing kind filter: %q", r.URL.RawQuery)
		}
		_, _ = io.WriteString(w, `{"items":[{"id":"abc","kind":"prompt","title":"Cold email","source_tool":"claude"}]}`)
	})
	items, err := SearchLibrary(context.Background(), nil, "", "find", "prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 || items[0].ID != "abc" || items[0].Title != "Cold email" {
		t.Fatalf("unexpected items: %+v", items)
	}
}

func TestSearchLibrary_Unauthorized(t *testing.T) {
	setupTestEnv(t)
	newTestServer(t, func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		w.WriteHeader(stdhttp.StatusUnauthorized)
	})
	_, err := SearchLibrary(context.Background(), nil, "", "", "")
	if err != ErrUnauthorized {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestGetLibraryItem_OK(t *testing.T) {
	setupTestEnv(t)
	newTestServer(t, func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if r.URL.Path != "/v1/library/abc" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if r.Header.Get("X-STH-Org") != "org-1" {
			t.Errorf("missing org header: %q", r.Header.Get("X-STH-Org"))
		}
		_, _ = io.WriteString(w, `{"item":{"id":"abc","kind":"prompt","title":"Cold email","body":"Write a cold email"},"download_url":""}`)
	})
	detail, err := GetLibraryItem(context.Background(), nil, "org-1", "abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail.Item.Body != "Write a cold email" {
		t.Fatalf("unexpected detail: %+v", detail)
	}
}

func TestGetLibraryItem_NotFound(t *testing.T) {
	setupTestEnv(t)
	newTestServer(t, func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		w.WriteHeader(stdhttp.StatusNotFound)
	})
	_, err := GetLibraryItem(context.Background(), nil, "", "missing")
	if err != ErrLibraryNotFound {
		t.Fatalf("expected ErrLibraryNotFound, got %v", err)
	}
}

func TestSaveLibraryItem_OK(t *testing.T) {
	setupTestEnv(t)
	newTestServer(t, func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if r.Method != stdhttp.MethodPost || r.URL.Path != "/v1/library" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var decoded LibrarySaveInput
		if err := json.NewDecoder(r.Body).Decode(&decoded); err != nil {
			t.Errorf("decode body: %v", err)
		}
		if decoded.Title != "Cold email" || decoded.Kind != "prompt" {
			t.Errorf("unexpected body: %+v", decoded)
		}
		w.WriteHeader(stdhttp.StatusCreated)
		_, _ = io.WriteString(w, `{"id":"new123"}`)
	})
	identifier, err := SaveLibraryItem(context.Background(), nil, "", LibrarySaveInput{Kind: "prompt", Title: "Cold email", Body: "Write..."})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if identifier != "new123" {
		t.Fatalf("unexpected id: %q", identifier)
	}
}

func TestRecordLibraryEvent_OK(t *testing.T) {
	setupTestEnv(t)
	var capturedPath string
	var capturedEvent string
	newTestServer(t, func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		capturedPath = r.URL.Path
		var decoded struct {
			EventType string `json:"event_type"`
		}
		_ = json.NewDecoder(r.Body).Decode(&decoded)
		capturedEvent = decoded.EventType
		w.WriteHeader(stdhttp.StatusAccepted)
	})
	if err := RecordLibraryEvent(context.Background(), nil, "", "abc", "reuse"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedPath != "/v1/library/abc/events" || capturedEvent != "reuse" {
		t.Fatalf("unexpected event call: path=%q event=%q", capturedPath, capturedEvent)
	}
}
