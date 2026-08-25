package writer

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

const codexManagedTOMLMarker = "# managed by sth\n"

var codexSkillNamePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?$`)

type CodexWriter struct{}

func NewCodexWriter() CodexWriter { return CodexWriter{} }

func (CodexWriter) Kind() string { return "codex" }

func (writer CodexWriter) Write(request WriteRequest) (WriteResult, error) {
	if strings.TrimSpace(request.InstalledPath) == "" {
		return WriteResult{}, fmt.Errorf("installed_path is required")
	}
	if err := os.MkdirAll(filepath.Dir(request.InstalledPath), 0o755); err != nil {
		return WriteResult{}, fmt.Errorf("failed to create directory %s: %w", filepath.Dir(request.InstalledPath), err)
	}

	content, renderErr := writer.Render(request)
	if renderErr != nil {
		return WriteResult{}, renderErr
	}

	if err := os.WriteFile(request.InstalledPath, content, 0o644); err != nil {
		return WriteResult{}, fmt.Errorf("failed to write %s: %w", request.InstalledPath, err)
	}
	return WriteResult{
		Path:   filepath.ToSlash(filepath.Clean(request.InstalledPath)),
		SHA256: computeSHA256(content),
	}, nil
}

func (CodexWriter) Render(request WriteRequest) ([]byte, error) {
	if strings.TrimSpace(request.InstalledPath) == "" {
		return nil, fmt.Errorf("installed_path is required")
	}
	if request.ArtifactKind == "instruction" {
		return renderInstructionFragment(request, "Codex")
	}
	if strings.EqualFold(filepath.Ext(request.InstalledPath), ".toml") {
		return renderCodexAgent(request)
	}
	if isCodexSkillPath(request.InstalledPath) {
		return renderCodexSkill(request)
	}
	content := request.Content
	if request.AddManagedHeader && strings.EqualFold(filepath.Ext(request.InstalledPath), ".md") {
		content = applyMinimalManagedMarker(request.InstalledPath, content)
	}
	return content, nil
}

func (CodexWriter) Plan(artifactKind, artifactName, _ string, sources []string) PlanResult {
	if artifactName == "" || len(sources) == 0 {
		return PlanResult{}
	}
	switch artifactKind {
	case "skill":
		return PlanResult{Files: relForSkillBundle(".agents/skills", artifactName, sources)}
	case "agent":
		return PlanResult{Files: []PlannedFile{{SourcePath: sources[0], RelativePath: ".codex/agents/" + artifactName + ".toml"}}}
	case "command":
		return PlanResult{Files: []PlannedFile{{SourcePath: sources[0], RelativePath: ".agents/skills/" + artifactName + "/SKILL.md"}}}
	case "instruction":
		return PlanResult{Files: []PlannedFile{{SourcePath: sources[0], RelativePath: ".codex/sth/instructions/" + artifactName + ".md"}}}
	}
	return PlanResult{Skipped: true, Reason: "unknown artifact kind: " + artifactKind}
}

func renderCodexAgent(request WriteRequest) ([]byte, error) {
	if !utf8.Valid(request.Content) {
		return nil, fmt.Errorf("invalid Codex agent: content is not UTF-8")
	}
	if strings.HasPrefix(string(request.Content), codexManagedTOMLMarker) {
		return request.Content, nil
	}
	bodyOffset := FrontmatterBodyOffset(request.Content)
	artifactName := codexRequestArtifactName(request)
	description := "Agent " + artifactName
	developerInstructions := strings.TrimSpace(string(request.Content))
	if bodyOffset >= 0 {
		frontmatter := string(request.Content[:bodyOffset])
		if sourceDescription := geminiFrontmatterValue(frontmatter, "description"); sourceDescription != "" {
			description = sourceDescription
		}
		developerInstructions = strings.TrimSpace(string(request.Content[bodyOffset:]))
	}

	var rendered strings.Builder
	if request.AddManagedHeader {
		rendered.WriteString(codexManagedTOMLMarker)
	}
	rendered.WriteString("name = ")
	rendered.WriteString(geminiTOMLString(artifactName))
	rendered.WriteByte('\n')
	rendered.WriteString("description = ")
	rendered.WriteString(geminiTOMLString(description))
	rendered.WriteByte('\n')
	rendered.WriteString("developer_instructions = ")
	rendered.WriteString(geminiTOMLString(developerInstructions))
	rendered.WriteByte('\n')
	return []byte(rendered.String()), nil
}

func renderCodexSkill(request WriteRequest) ([]byte, error) {
	if !utf8.Valid(request.Content) {
		return nil, fmt.Errorf("invalid Codex skill: content is not UTF-8")
	}
	artifactName := codexRequestArtifactName(request)
	if !codexSkillNamePattern.MatchString(artifactName) || strings.Contains(artifactName, "--") {
		return nil, fmt.Errorf("invalid Codex skill name: %s", artifactName)
	}
	bodyOffset := FrontmatterBodyOffset(request.Content)
	if request.ArtifactKind != "command" && bodyOffset >= 0 {
		frontmatter := string(request.Content[:bodyOffset])
		sourceName := geminiFrontmatterValue(frontmatter, "name")
		sourceDescription := geminiFrontmatterValue(frontmatter, "description")
		if sourceName != "" && sourceDescription != "" {
			if sourceName != artifactName {
				return nil, fmt.Errorf("Codex skill name %q differs from directory %q", sourceName, artifactName)
			}
			if descriptionErr := validateCodexSkillDescription(sourceDescription); descriptionErr != nil {
				return nil, descriptionErr
			}
			content := request.Content
			if request.AddManagedHeader {
				content = applyMinimalManagedMarker(request.InstalledPath, content)
			}
			return content, nil
		}
	}
	if request.ArtifactKind == "skill" {
		return nil, fmt.Errorf("invalid Codex skill: frontmatter name and description are required")
	}

	description := "Reusable command " + artifactName
	body := strings.TrimSpace(string(request.Content))
	if bodyOffset >= 0 {
		frontmatter := string(request.Content[:bodyOffset])
		if sourceDescription := geminiFrontmatterValue(frontmatter, "description"); sourceDescription != "" {
			description = sourceDescription
		}
		body = strings.TrimSpace(string(request.Content[bodyOffset:]))
	}
	if descriptionErr := validateCodexSkillDescription(description); descriptionErr != nil {
		return nil, descriptionErr
	}

	var rendered strings.Builder
	rendered.WriteString("---\nname: ")
	rendered.WriteString(geminiTOMLString(artifactName))
	rendered.WriteString("\ndescription: ")
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

func isCodexSkillPath(installedPath string) bool {
	portablePath := "/" + strings.TrimPrefix(filepath.ToSlash(filepath.Clean(installedPath)), "/")
	return strings.Contains(portablePath, "/.agents/skills/") && strings.EqualFold(filepath.Base(installedPath), "SKILL.md")
}

func codexArtifactName(installedPath string) string {
	if strings.EqualFold(filepath.Ext(installedPath), ".toml") {
		return strings.TrimSuffix(filepath.Base(installedPath), filepath.Ext(installedPath))
	}
	return filepath.Base(filepath.Dir(installedPath))
}

func codexRequestArtifactName(request WriteRequest) string {
	if request.ArtifactName != "" {
		return request.ArtifactName
	}
	return codexArtifactName(request.InstalledPath)
}

func validateCodexSkillDescription(description string) error {
	descriptionLength := utf8.RuneCountInString(description)
	if descriptionLength < 1 || descriptionLength > 1024 {
		return fmt.Errorf("invalid Codex skill description: length %d, expected between 1 and 1024", descriptionLength)
	}
	return nil
}
