package writer

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	metadata "sth/internal/frontmatter"
)

const geminiManagedTOMLMarker = "# managed by sth\n"

type GeminiWriter struct{}

func NewGeminiWriter() GeminiWriter { return GeminiWriter{} }

func (GeminiWriter) Kind() string { return "gemini" }

func (GeminiWriter) Plan(artifactKind, artifactName, _ string, sources []string) PlanResult {
	if artifactName == "" || len(sources) == 0 {
		return PlanResult{}
	}

	switch artifactKind {
	case "skill":
		return PlanResult{Files: relForSkillBundle(".agents/skills", artifactName, sources)}
	case "agent":
		return PlanResult{Files: []PlannedFile{{SourcePath: sources[0], RelativePath: ".gemini/agents/" + artifactName + ".md"}}}
	case "command":
		return PlanResult{Files: []PlannedFile{{SourcePath: sources[0], RelativePath: ".gemini/commands/" + artifactName + ".toml"}}}
	case "instruction":
		return PlanResult{Files: []PlannedFile{{SourcePath: sources[0], RelativePath: ".gemini/sth/instructions/" + artifactName + ".md"}}}
	}

	return PlanResult{Skipped: true, Reason: "unknown artifact kind: " + artifactKind}
}

func (GeminiWriter) Write(request WriteRequest) (WriteResult, error) {
	if strings.TrimSpace(request.InstalledPath) == "" {
		return WriteResult{}, fmt.Errorf("installed_path is required")
	}
	if err := os.MkdirAll(filepath.Dir(request.InstalledPath), 0o755); err != nil {
		return WriteResult{}, fmt.Errorf("failed to create directory %s: %w", filepath.Dir(request.InstalledPath), err)
	}

	content, err := (GeminiWriter{}).Render(request)
	if err != nil {
		return WriteResult{}, err
	}
	if err := os.WriteFile(request.InstalledPath, content, 0o644); err != nil {
		return WriteResult{}, fmt.Errorf("failed to write %s: %w", request.InstalledPath, err)
	}

	return WriteResult{
		Path:   filepath.ToSlash(filepath.Clean(request.InstalledPath)),
		SHA256: computeSHA256(content),
	}, nil
}

func (GeminiWriter) Render(request WriteRequest) ([]byte, error) {
	return renderGeminiContent(request)
}

func RenderGeminiCommand(content []byte) ([]byte, error) {
	if !utf8.Valid(content) {
		return nil, fmt.Errorf("invalid Gemini command: content is not UTF-8")
	}

	description, prompt := geminiCommandParts(content)
	var rendered strings.Builder
	if description != "" {
		rendered.WriteString("description = ")
		rendered.WriteString(geminiTOMLString(description))
		rendered.WriteByte('\n')
	}
	rendered.WriteString("prompt = ")
	rendered.WriteString(geminiTOMLString(prompt))
	rendered.WriteByte('\n')
	return []byte(rendered.String()), nil
}

func renderGeminiContent(request WriteRequest) ([]byte, error) {
	if request.ArtifactKind == "instruction" {
		return renderInstructionFragment(request, "Gemini")
	}
	content := request.Content
	if strings.EqualFold(filepath.Ext(request.InstalledPath), ".toml") {
		if bytes.HasPrefix(content, []byte(geminiManagedTOMLMarker)) {
			return content, nil
		}
		rendered, err := RenderGeminiCommand(content)
		if err != nil {
			return nil, fmt.Errorf("failed to render Gemini file %s: %w", request.InstalledPath, err)
		}
		if request.AddManagedHeader {
			return append([]byte(geminiManagedTOMLMarker), rendered...), nil
		}
		return rendered, nil
	}
	if request.AddManagedHeader && strings.EqualFold(filepath.Ext(request.InstalledPath), ".md") {
		content = applyMinimalManagedMarker(request.InstalledPath, content)
	}
	return content, nil
}

func geminiCommandParts(content []byte) (string, string) {
	bodyOffset := FrontmatterBodyOffset(content)
	if bodyOffset < 0 {
		return "", strings.TrimSpace(string(content))
	}
	frontmatter := string(content[:bodyOffset])
	description := geminiFrontmatterValue(frontmatter, "description")
	prompt := strings.TrimSpace(string(content[bodyOffset:]))
	return description, prompt
}

func geminiFrontmatterValue(content, key string) string {
	value, valid := metadata.String([]byte(content), key)
	if !valid {
		return ""
	}
	return value
}

func geminiTOMLString(value string) string {
	var encoded strings.Builder
	encoded.WriteByte('"')
	for _, character := range value {
		switch character {
		case '\b':
			encoded.WriteString("\\b")
		case '\t':
			encoded.WriteString("\\t")
		case '\n':
			encoded.WriteString("\\n")
		case '\f':
			encoded.WriteString("\\f")
		case '\r':
			encoded.WriteString("\\r")
		case '"':
			encoded.WriteString("\\\"")
		case '\\':
			encoded.WriteString("\\\\")
		default:
			if character < 0x20 || character == 0x7f {
				encoded.WriteString(fmt.Sprintf("\\u%04X", character))
			} else {
				encoded.WriteRune(character)
			}
		}
	}
	encoded.WriteByte('"')
	return encoded.String()
}
