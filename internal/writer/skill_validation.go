package writer

import (
	"fmt"
	"strings"
	"unicode/utf8"

	metadata "sth/internal/frontmatter"
)

func validateSkillConsumerMetadata(targetKind, artifactName string, content []byte) error {
	switch targetKind {
	case KindCodex:
		return validateCodexSkillConsumerMetadata(artifactName, content)
	case KindCopilot:
		return validateCopilotSkillConsumerMetadata(artifactName, content)
	case KindGemini:
		return validateGeminiSkillConsumerMetadata(artifactName, content)
	case KindAntigravity:
		return validateAntigravitySkillConsumerMetadata(artifactName, content)
	default:
		return nil
	}
}

func validateCodexSkillConsumerMetadata(artifactName string, content []byte) error {
	_, err := renderCodexSkill(WriteRequest{
		Content:      content,
		ArtifactKind: "skill",
		ArtifactName: artifactName,
	})
	return err
}

func validateCopilotSkillConsumerMetadata(artifactName string, content []byte) error {
	name, nameErr := requiredSkillMetadataString(content, "Copilot", "name")
	if nameErr != nil {
		return nameErr
	}
	if nameErr := validateAgentSkillName("Copilot", name); nameErr != nil {
		return nameErr
	}
	if name != artifactName {
		return fmt.Errorf("invalid Copilot skill: frontmatter name %q differs from directory %q", name, artifactName)
	}
	description, descriptionErr := requiredSkillMetadataString(content, "Copilot", "description")
	if descriptionErr != nil {
		return descriptionErr
	}
	return validateAgentSkillDescription("Copilot", description)
}

func validateGeminiSkillConsumerMetadata(artifactName string, content []byte) error {
	name, nameErr := requiredSkillMetadataString(content, "Gemini", "name")
	if nameErr != nil {
		return nameErr
	}
	if name != artifactName {
		return fmt.Errorf("invalid Gemini skill: frontmatter name %q differs from directory %q", name, artifactName)
	}
	if nameErr := validateAgentSkillName("Gemini", name); nameErr != nil {
		return nameErr
	}
	description, descriptionErr := requiredSkillMetadataString(content, "Gemini", "description")
	if descriptionErr != nil {
		return descriptionErr
	}
	return validateAgentSkillDescription("Gemini", description)
}

func validateAntigravitySkillConsumerMetadata(artifactName string, content []byte) error {
	if nameErr := validateAgentSkillName("Antigravity", artifactName); nameErr != nil {
		return nameErr
	}
	description, descriptionErr := requiredSkillMetadataString(content, "Antigravity", "description")
	if descriptionErr != nil {
		return descriptionErr
	}
	if descriptionErr := validateAgentSkillDescription("Antigravity", description); descriptionErr != nil {
		return descriptionErr
	}
	name, present, valid := metadata.LookupString(content, "name")
	if !valid {
		if present {
			return fmt.Errorf("invalid Antigravity skill: frontmatter name must be a string")
		}
		return fmt.Errorf("invalid Antigravity skill: invalid YAML frontmatter")
	}
	if present && strings.TrimSpace(name) == "" {
		return fmt.Errorf("invalid Antigravity skill: frontmatter name must be a non-empty string when present")
	}
	if present && name != artifactName {
		return fmt.Errorf("invalid Antigravity skill: frontmatter name %q differs from directory %q", name, artifactName)
	}
	return nil
}

func validateAgentSkillName(consumerName, name string) error {
	if !codexSkillNamePattern.MatchString(name) || strings.Contains(name, "--") {
		return fmt.Errorf("invalid %s skill: name must contain 1 to 64 characters from a-z, 0-9, and hyphens, with no leading, trailing, or consecutive hyphen", consumerName)
	}
	return nil
}

func validateAgentSkillDescription(consumerName, description string) error {
	descriptionLength := utf8.RuneCountInString(description)
	if descriptionLength > 1024 {
		return fmt.Errorf("invalid %s skill: description contains %d characters, maximum 1024", consumerName, descriptionLength)
	}
	return nil
}

func requiredSkillMetadataString(content []byte, consumerName, key string) (string, error) {
	value, present, valid := metadata.LookupString(content, key)
	if !valid {
		if present {
			return "", fmt.Errorf("invalid %s skill: frontmatter %s must be a string", consumerName, key)
		}
		return "", fmt.Errorf("invalid %s skill: invalid YAML frontmatter", consumerName)
	}
	if !present || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("invalid %s skill: frontmatter %s must be a non-empty string", consumerName, key)
	}
	return value, nil
}
