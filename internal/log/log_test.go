package log

import (
	"bytes"
	"strings"
	"testing"
)

func TestNoopWhenDisabled(t *testing.T) {
	var buf bytes.Buffer
	v := NewVerbose(&buf, false)
	v.HTTP("GET", "https://example.com/", 200)
	v.Path("project.json", "/abs/path")
	v.Decision("decided %s", "x")
	if buf.Len() != 0 {
		t.Fatalf("expected no output when disabled, got %q", buf.String())
	}
}

func TestNoopWhenWriterNil(t *testing.T) {
	v := NewVerbose(nil, true) // should still no-op safely
	v.HTTP("GET", "x", 0)
	v.Path("a", "b")
	v.Decision("x")
}

func TestHTTPWithStatus(t *testing.T) {
	var buf bytes.Buffer
	v := NewVerbose(&buf, true)
	v.HTTP("GET", "https://example.com/foo", 200)
	out := buf.String()
	if !strings.Contains(out, "[verbose] GET https://example.com/foo → 200") {
		t.Fatalf("missing expected line: %q", out)
	}
}

func TestHTTPWithoutStatus(t *testing.T) {
	var buf bytes.Buffer
	v := NewVerbose(&buf, true)
	v.HTTP("", "https://example.com/foo", 0)
	out := buf.String()
	if !strings.Contains(out, "[verbose] GET https://example.com/foo\n") {
		t.Fatalf("missing expected line without status: %q", out)
	}
}

func TestPath(t *testing.T) {
	var buf bytes.Buffer
	v := NewVerbose(&buf, true)
	v.Path("project.json", "/tmp/.sth/project.json")
	if !strings.Contains(buf.String(), "[verbose] path project.json → /tmp/.sth/project.json") {
		t.Fatalf("missing path line: %q", buf.String())
	}
}

func TestDecision(t *testing.T) {
	var buf bytes.Buffer
	v := NewVerbose(&buf, true)
	v.Decision("retried after %s", "401")
	if !strings.Contains(buf.String(), "[verbose] retried after 401") {
		t.Fatalf("missing decision line: %q", buf.String())
	}
}
