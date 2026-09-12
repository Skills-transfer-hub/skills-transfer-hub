package writer

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

type CopilotWriter struct{}

func NewCopilotWriter() CopilotWriter { return CopilotWriter{} }

func (CopilotWriter) Kind() string { return "copilot" }

func (writer CopilotWriter) Write(request WriteRequest) (WriteResult, error) {
	content, renderErr := writer.Render(request)
	if renderErr != nil {
		return WriteResult{}, renderErr
	}
	if err := os.MkdirAll(filepath.Dir(request.InstalledPath), 0o755); err != nil {
		return WriteResult{}, fmt.Errorf("failed to create directory %s: %w", filepath.Dir(request.InstalledPath), err)
	}
	if err := os.WriteFile(request.InstalledPath, content, 0o644); err != nil {
		return WriteResult{}, fmt.Errorf("failed to write %s: %w", request.InstalledPath, err)
	}
	return WriteResult{
		Path:   filepath.ToSlash(filepath.Clean(request.InstalledPath)),
		SHA256: computeSHA256(content),
	}, nil
}

func (CopilotWriter) Render(request WriteRequest) ([]byte, error) {
	if strings.TrimSpace(request.InstalledPath) == "" {
		return nil, fmt.Errorf("installed_path is required")
	}
	if isCopilotPromptPath(request.InstalledPath) {
		if !request.AddManagedHeader && isCopilotPromptPath(request.SourcePath) {
			return request.Content, nil
		}
		return renderCopilotPrompt(request)
	}

	content := request.Content
	if request.AddManagedHeader && strings.EqualFold(filepath.Ext(request.InstalledPath), ".md") {
		if isSharedClaudeCommandPath(request.InstalledPath) {
			content = applyClaudeManagedHeader(request.InstalledPath, content)
		} else {
			content = applyMinimalManagedMarker(request.InstalledPath, content)
		}
	}
	return content, nil
}

func (CopilotWriter) Plan(artifactKind, artifactName, _ string, sources []string) PlanResult {
	if artifactName == "" || len(sources) == 0 {
		return PlanResult{}
	}

	switch artifactKind {
	case "skill":
		return PlanResult{Files: relForSkillBundle(".agents/skills", artifactName, sources)}
	case "agent":
		return PlanResult{Files: []PlannedFile{{SourcePath: sources[0], RelativePath: ".github/agents/" + artifactName + ".agent.md"}}}
	case "command":
		return PlanResult{Files: []PlannedFile{{SourcePath: sources[0], RelativePath: ".github/prompts/" + artifactName + ".prompt.md"}}}
	case "instruction":
		return PlanResult{Files: []PlannedFile{{SourcePath: sources[0], RelativePath: ".github/sth/instructions/" + artifactName + ".md"}}}
	case "rule":
		return PlanResult{Files: []PlannedFile{{SourcePath: sources[0], RelativePath: ".github/instructions/" + artifactName + ".instructions.md"}}}
	case "workflow":
		return PlanResult{Skipped: true, Reason: "Copilot workflows are outside the V1 artifact contract"}
	}

	return PlanResult{Skipped: true, Reason: "unknown artifact kind: " + artifactKind}
}

const minimalManagedMarker = "<!-- managed by sth -->\n"

func applyMinimalManagedMarker(_ string, content []byte) []byte {
	marker := []byte(minimalManagedMarker)
	if bodyStart := FrontmatterBodyOffset(content); bodyStart >= 0 {
		if bytes.HasPrefix(content[bodyStart:], marker) {
			return content
		}
		output := make([]byte, 0, len(content)+len(marker))
		output = append(output, content[:bodyStart]...)
		output = append(output, marker...)
		output = append(output, content[bodyStart:]...)
		return output
	}
	if bytes.HasPrefix(content, marker) {
		return content
	}
	return append(marker, content...)
}

func isSharedClaudeCommandPath(installedPath string) bool {
	portablePath := "/" + strings.TrimPrefix(filepath.ToSlash(filepath.Clean(installedPath)), "/")
	return strings.Contains(portablePath, "/.claude/commands/")
}

func isCopilotPromptPath(installedPath string) bool {
	portablePath := "/" + strings.TrimPrefix(filepath.ToSlash(filepath.Clean(installedPath)), "/")
	return strings.Contains(portablePath, "/.github/prompts/") && strings.HasSuffix(strings.ToLower(portablePath), ".prompt.md")
}

func renderCopilotPrompt(request WriteRequest) ([]byte, error) {
	if !utf8.Valid(request.Content) {
		return nil, fmt.Errorf("invalid Copilot prompt: content is not UTF-8")
	}
	bodyOffset := FrontmatterBodyOffset(request.Content)
	description := "Reusable prompt " + copilotPromptName(request.InstalledPath)
	body := strings.TrimSpace(string(request.Content))
	if bodyOffset >= 0 {
		frontmatter := string(request.Content[:bodyOffset])
		if sourceDescription := geminiFrontmatterValue(frontmatter, "description"); sourceDescription != "" {
			description = sourceDescription
		}
		body = strings.TrimSpace(string(request.Content[bodyOffset:]))
	}
	body = strings.TrimSpace(strings.TrimPrefix(body, strings.TrimSpace(minimalManagedMarker)))

	var rendered strings.Builder
	rendered.WriteString("---\ndescription: ")
	rendered.WriteString(geminiTOMLString(description))
	rendered.WriteString("\n---\n")
	if request.AddManagedHeader {
		rendered.WriteString(minimalManagedMarker)
	}
	if body != "" {
		rendered.WriteString(body)
		rendered.WriteByte('\n')
	}
	return []byte(rendered.String()), nil
}

func copilotPromptName(installedPath string) string {
	baseName := filepath.Base(installedPath)
	return strings.TrimSuffix(baseName, ".prompt.md")
}
