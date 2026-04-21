package http

import (
	"context"
	"strings"
	"testing"

	stdhttp "net/http"
	"net/http/httptest"
)

func newTestServer() *httptest.Server {
	return httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		switch r.URL.Path {
		case "/alpha.md":
			_, _ = w.Write([]byte("# Alpha\n"))
		case "/missing.md":
			stdhttp.NotFound(w, r)
		default:
			w.WriteHeader(stdhttp.StatusInternalServerError)
		}
	}))
}

func TestClient_DownloadSkill_success(t *testing.T) {
	server := newTestServer()
	defer server.Close()

	client := NewClient(server.Client())
	data, err := client.DownloadSkill(context.Background(), server.URL, "alpha")
	if err != nil {
		t.Fatalf("DownloadSkill() error = %v", err)
	}
	if string(data) != "# Alpha\n" {
		t.Fatalf("data = %q, want %q", data, "# Alpha\n")
	}
}

func TestClient_DownloadSkill_notFound(t *testing.T) {
	server := newTestServer()
	defer server.Close()

	client := NewClient(server.Client())
	_, err := client.DownloadSkill(context.Background(), server.URL, "missing")
	if err == nil {
		t.Fatal("DownloadSkill() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Fatalf("error %q does not contain 404", err)
	}
}

func TestClient_DownloadSkill_unexpectedStatus(t *testing.T) {
	server := newTestServer()
	defer server.Close()

	client := NewClient(server.Client())
	_, err := client.DownloadSkill(context.Background(), server.URL, "broken")
	if err == nil {
		t.Fatal("DownloadSkill() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("error %q does not contain 500", err)
	}
}
