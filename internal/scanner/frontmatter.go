package scanner

import (
	"bytes"
	"strings"
)

// Frontmatter is a minimal subset of YAML frontmatter we read for artifact
// discovery. It deliberately does NOT cover full YAML (no anchors, no flow
// scalars, no block scalars). The four fields below are the only ones STH
// needs to enrich an Artifact; everything else is preserved on-disk by the
// writer when it copies the source file.
type Frontmatter struct {
	Name        string
	Description string
	ApplyTo     []string // Copilot-style file globs (single string or comma-list)
	Lang        string   // optional, rarely used by source repos
}

// ExtractFrontmatter returns the frontmatter block from a Markdown file when
// the file starts with `---` on line 1. The boolean is false when there is no
// frontmatter. The returned Frontmatter is best-effort: only top-level scalar
// keys and one-line value/list patterns are recognized.
func ExtractFrontmatter(content []byte) (Frontmatter, bool) {
	bomLen := 0
	if bytes.HasPrefix(content, []byte{0xEF, 0xBB, 0xBF}) {
		bomLen = 3
	}
	rest := content[bomLen:]
	var openLen int
	switch {
	case bytes.HasPrefix(rest, []byte("---\n")):
		openLen = 4
	case bytes.HasPrefix(rest, []byte("---\r\n")):
		openLen = 5
	default:
		return Frontmatter{}, false
	}

	// Manual scan over body so the byte offset stays correct under both LF
	// and CRLF (bufio.Scanner strips both terminators identically, which
	// makes a fixed `+1` offset drift on CRLF files). Also avoids the 64KB
	// line-length cap bufio.Scanner ships with by default.
	body := rest[openLen:]
	end := -1
	for offset := 0; offset < len(body); {
		nl := bytes.IndexByte(body[offset:], '\n')
		var line []byte
		var nextOffset int
		if nl == -1 {
			line = body[offset:]
			nextOffset = len(body)
		} else {
			line = body[offset : offset+nl]
			nextOffset = offset + nl + 1
		}
		if bytes.Equal(bytes.TrimRight(line, "\r"), []byte("---")) {
			end = offset
			break
		}
		if nl == -1 {
			break
		}
		offset = nextOffset
	}
	if end < 0 {
		return Frontmatter{}, false
	}

	return parseFrontmatterLines(string(body[:end])), true
}

func parseFrontmatterLines(block string) Frontmatter {
	fm := Frontmatter{}
	for _, raw := range strings.Split(block, "\n") {
		line := strings.TrimRight(raw, "\r")
		if line == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		// Top-level key: skip indented lines (nested values not supported here).
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		colon := strings.IndexByte(line, ':')
		if colon < 0 {
			continue
		}
		key := strings.TrimSpace(line[:colon])
		value := strings.TrimSpace(line[colon+1:])
		value = stripQuotes(value)
		switch key {
		case "name":
			fm.Name = value
		case "description":
			fm.Description = value
		case "applyTo":
			fm.ApplyTo = splitGlobList(value)
		case "globs":
			if len(fm.ApplyTo) == 0 {
				fm.ApplyTo = splitGlobList(value)
			}
		case "paths":
			if len(fm.ApplyTo) == 0 {
				fm.ApplyTo = splitGlobList(value)
			}
		case "lang":
			fm.Lang = value
		}
	}
	return fm
}

func stripQuotes(value string) string {
	if len(value) >= 2 {
		first, last := value[0], value[len(value)-1]
		if (first == '\'' && last == '\'') || (first == '"' && last == '"') {
			return value[1 : len(value)-1]
		}
	}
	return value
}

// splitGlobList parses values like "**/*.tsx, **/*.jsx" or "['a','b']" into
// a clean string slice. Full YAML sequence / block syntaxes are not supported.
func splitGlobList(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		value = value[1 : len(value)-1]
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		clean := stripQuotes(strings.TrimSpace(part))
		if clean != "" {
			out = append(out, clean)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
