package artifact

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

var portableNamePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9_-]*[a-z0-9])?$`)

func Validate(definition Definition) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)
	if definition.Native.ParseError != "" {
		diagnostics = append(diagnostics, Diagnostic{Severity: SeverityError, Code: "parse_error", Path: definition.SourcePath, Message: definition.Native.ParseError})
	}
	if !portableNamePattern.MatchString(definition.Name) {
		diagnostics = append(diagnostics, Diagnostic{Severity: SeverityError, Code: "invalid_name", Path: definition.SourcePath, Field: "name", Message: "name must use lowercase letters, digits, hyphens, or underscores"})
	}
	if len(definition.Files) == 0 {
		diagnostics = append(diagnostics, Diagnostic{Severity: SeverityError, Code: "missing_files", Path: definition.SourcePath, Message: "artifact has no files"})
	}
	for _, file := range definition.Files {
		if filepath.IsAbs(file.Path) || strings.HasPrefix(filepath.ToSlash(filepath.Clean(file.Path)), "../") {
			diagnostics = append(diagnostics, Diagnostic{Severity: SeverityError, Code: "unsafe_path", Path: file.Path, Message: "artifact file escapes the project"})
		}
		if !utf8.Valid(file.Content) {
			diagnostics = append(diagnostics, Diagnostic{Severity: SeverityError, Code: "invalid_utf8", Path: file.Path, Message: "artifact file is not valid UTF-8"})
		}
	}
	switch definition.Kind {
	case KindSkill:
		diagnostics = append(diagnostics, validateSkill(definition)...)
	case KindAgent:
		if definition.SourceDialect != DialectAntigravityTeam {
			diagnostics = append(diagnostics, requireDescription(definition)...)
		}
	case KindCommand:
		if definition.SourceDialect == DialectGeminiCommand && strings.TrimSpace(definition.Common.Body) == "" {
			diagnostics = append(diagnostics, Diagnostic{Severity: SeverityError, Code: "missing_prompt", Path: definition.SourcePath, Field: "prompt", Message: "Gemini commands require prompt"})
		}
	case KindWorkflow:
		if definition.SourceDialect == DialectClaudeWorkflow {
			raw := string(definition.Native.Raw)
			if !strings.Contains(raw, "export const meta") {
				diagnostics = append(diagnostics, Diagnostic{Severity: SeverityError, Code: "missing_workflow_meta", Path: definition.SourcePath, Field: "meta", Message: "Claude workflows require export const meta"})
			}
		}
	}
	if len(definition.Native.Unmapped) > 0 {
		diagnostics = append(diagnostics, Diagnostic{Severity: SeverityWarning, Code: "native_fields", Path: definition.SourcePath, Field: strings.Join(definition.Native.Unmapped, ","), Message: "native fields require an explicit compatibility decision"})
	}
	return diagnostics
}

func validateSkill(definition Definition) []Diagnostic {
	diagnostics := requireDescription(definition)
	directoryName := filepath.Base(filepath.Dir(definition.SourcePath))
	if directoryName != definition.Name {
		diagnostics = append(diagnostics, Diagnostic{Severity: SeverityError, Code: "skill_folder_mismatch", Path: definition.SourcePath, Field: "name", Message: fmt.Sprintf("skill name %q differs from folder %q", definition.Name, directoryName)})
	}
	if len(definition.Common.Description) > 1024 {
		diagnostics = append(diagnostics, Diagnostic{Severity: SeverityError, Code: "description_too_long", Path: definition.SourcePath, Field: "description", Message: "skill description exceeds 1024 characters"})
	}
	return diagnostics
}

func requireDescription(definition Definition) []Diagnostic {
	if strings.TrimSpace(definition.Common.Description) != "" {
		return nil
	}
	return []Diagnostic{{Severity: SeverityError, Code: "missing_description", Path: definition.SourcePath, Field: "description", Message: "description is required"}}
}

func HasErrors(diagnostics []Diagnostic, strict bool) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == SeverityError || strict && diagnostic.Severity == SeverityWarning {
			return true
		}
	}
	return false
}
