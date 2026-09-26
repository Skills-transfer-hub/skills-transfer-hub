package artifact

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	iofs "io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type sourceCandidate struct {
	kind       Kind
	dialect    Dialect
	path       string
	bundleRoot string
	name       string
}

func ScanProject(projectRoot string) ([]Definition, error) {
	absoluteRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve project root: %w", err)
	}
	paths := make([]string, 0)
	walkErr := filepath.WalkDir(absoluteRoot, func(path string, entry iofs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && entry.Name() == ".git" {
			return filepath.SkipDir
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		relativePath, relativeErr := filepath.Rel(absoluteRoot, path)
		if relativeErr != nil {
			return relativeErr
		}
		paths = append(paths, filepath.ToSlash(relativePath))
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("scan project: %w", walkErr)
	}
	sort.Strings(paths)
	candidates := make([]sourceCandidate, 0)
	for _, path := range paths {
		candidate, matched := classifyPath(path)
		if matched {
			candidates = append(candidates, candidate)
		}
	}
	definitions := make([]Definition, 0, len(candidates))
	for _, candidate := range candidates {
		definition, definitionErr := readDefinition(absoluteRoot, paths, candidate)
		if definitionErr != nil {
			return nil, definitionErr
		}
		definitions = append(definitions, definition)
	}
	sort.Slice(definitions, func(leftIndex, rightIndex int) bool {
		if definitions[leftIndex].Kind != definitions[rightIndex].Kind {
			return definitions[leftIndex].Kind < definitions[rightIndex].Kind
		}
		return definitions[leftIndex].SourcePath < definitions[rightIndex].SourcePath
	})
	return definitions, nil
}

func DefinitionFromContent(kind Kind, name, sourcePath string, content []byte) Definition {
	candidate, matched := classifyPath(filepath.ToSlash(sourcePath))
	if !matched || candidate.kind != kind {
		candidate = sourceCandidate{kind: kind, dialect: DialectSTHCatalog, path: filepath.ToSlash(sourcePath), name: name}
	}
	if candidate.name == "" {
		candidate.name = name
	}
	common, native := parseContent(candidate, content)
	if common.Name == "" || candidate.dialect == DialectSTHCatalog {
		common.Name = name
	}
	contentHash := sha256.Sum256(content)
	file := File{Path: filepath.ToSlash(sourcePath), Content: append([]byte(nil), content...), SHA256: hex.EncodeToString(contentHash[:]), Mode: 0o644}
	definition := Definition{ID: string(kind) + "::" + common.Name, Kind: kind, Name: common.Name, SourceDialect: candidate.dialect, SourcePath: filepath.ToSlash(sourcePath), Scope: Scope{Level: "project"}, Common: common, Native: native, Files: []File{file}}
	definition.SourceSHA256 = HashFiles(definition.Files)
	return definition
}

func classifyPath(path string) (sourceCandidate, bool) {
	lowerPath := strings.ToLower(path)
	baseName := filepath.Base(path)
	if baseName == "SKILL.md" && isSkillPath(lowerPath) {
		bundleRoot := filepath.ToSlash(filepath.Dir(path))
		return sourceCandidate{kind: KindSkill, dialect: DialectAgentSkills, path: path, bundleRoot: bundleRoot, name: filepath.Base(bundleRoot)}, true
	}
	if strings.HasPrefix(lowerPath, ".claude/agents/") && strings.HasSuffix(lowerPath, ".md") {
		return fileCandidate(KindAgent, DialectClaudeAgent, path, ".md"), true
	}
	if strings.HasPrefix(lowerPath, ".codex/agents/") && strings.HasSuffix(lowerPath, ".toml") {
		return fileCandidate(KindAgent, DialectCodexAgent, path, ".toml"), true
	}
	if strings.HasPrefix(lowerPath, ".gemini/agents/") && strings.HasSuffix(lowerPath, ".md") {
		return fileCandidate(KindAgent, DialectGeminiAgent, path, ".md"), true
	}
	if lowerPath == ".agents/agents.md" {
		return sourceCandidate{kind: KindAgent, dialect: DialectAntigravityTeam, path: path, name: "team"}, true
	}
	if strings.HasPrefix(lowerPath, ".agents/agents/") && strings.HasSuffix(lowerPath, "/agent.md") {
		return sourceCandidate{kind: KindAgent, dialect: DialectAntigravityAgent, path: path, name: filepath.Base(filepath.Dir(path))}, true
	}
	if strings.HasPrefix(lowerPath, ".github/agents/") && strings.HasSuffix(lowerPath, ".agent.md") {
		return fileCandidate(KindAgent, DialectCopilotAgent, path, ".agent.md"), true
	}
	if strings.HasPrefix(lowerPath, ".cursor/agents/") && strings.HasSuffix(lowerPath, ".md") {
		return fileCandidate(KindAgent, DialectCursorAgent, path, ".md"), true
	}
	if baseName == "CLAUDE.md" {
		return scopedCandidate(KindInstruction, DialectClaudeInstruction, path), true
	}
	if baseName == "AGENTS.md" {
		return scopedCandidate(KindInstruction, DialectSharedAgentsMD, path), true
	}
	if baseName == "GEMINI.md" {
		return scopedCandidate(KindInstruction, DialectGeminiInstruction, path), true
	}
	if lowerPath == ".github/copilot-instructions.md" {
		return sourceCandidate{kind: KindInstruction, dialect: DialectCopilotInstruction, path: path, name: "project"}, true
	}
	if strings.HasPrefix(lowerPath, ".github/instructions/") && strings.HasSuffix(lowerPath, ".instructions.md") {
		return fileCandidate(KindRule, DialectCopilotRule, path, ".instructions.md"), true
	}
	if strings.HasPrefix(lowerPath, ".claude/rules/") && strings.HasSuffix(lowerPath, ".md") {
		return fileCandidate(KindRule, DialectClaudeRule, path, ".md"), true
	}
	if strings.HasPrefix(lowerPath, ".codex/rules/") && strings.HasSuffix(lowerPath, ".rules") {
		return fileCandidate(KindRule, DialectCodexExecutionRule, path, ".rules"), true
	}
	if strings.HasPrefix(lowerPath, ".agents/rules/") && strings.HasSuffix(lowerPath, ".md") {
		return fileCandidate(KindRule, DialectAntigravityRule, path, ".md"), true
	}
	if strings.HasPrefix(lowerPath, ".cursor/rules/") && strings.HasSuffix(lowerPath, ".mdc") {
		return fileCandidate(KindRule, DialectCursorRule, path, ".mdc"), true
	}
	if strings.HasPrefix(lowerPath, ".claude/commands/") && strings.HasSuffix(lowerPath, ".md") {
		return fileCandidate(KindCommand, DialectClaudeCommand, path, ".md"), true
	}
	if strings.HasPrefix(lowerPath, ".gemini/commands/") && strings.HasSuffix(lowerPath, ".toml") {
		return fileCandidate(KindCommand, DialectGeminiCommand, path, ".toml"), true
	}
	if strings.HasPrefix(lowerPath, ".github/prompts/") && strings.HasSuffix(lowerPath, ".prompt.md") {
		return fileCandidate(KindCommand, DialectCopilotPrompt, path, ".prompt.md"), true
	}
	if strings.HasPrefix(lowerPath, ".claude/workflows/") && strings.HasSuffix(lowerPath, ".js") {
		return fileCandidate(KindWorkflow, DialectClaudeWorkflow, path, ".js"), true
	}
	if strings.HasPrefix(lowerPath, ".agents/workflows/") && strings.HasSuffix(lowerPath, ".md") {
		return fileCandidate(KindWorkflow, DialectAntigravityWorkflow, path, ".md"), true
	}
	return sourceCandidate{}, false
}

func isSkillPath(lowerPath string) bool {
	for _, prefix := range []string{".claude/skills/", ".agents/skills/", ".gemini/skills/", ".github/skills/", ".cursor/skills/", "skills/"} {
		if strings.HasPrefix(lowerPath, prefix) {
			return true
		}
	}
	return false
}

func fileCandidate(kind Kind, dialect Dialect, path, suffix string) sourceCandidate {
	return sourceCandidate{kind: kind, dialect: dialect, path: path, name: strings.TrimSuffix(filepath.Base(path), suffix)}
}

func scopedCandidate(kind Kind, dialect Dialect, path string) sourceCandidate {
	scopePath := filepath.ToSlash(filepath.Dir(path))
	if scopePath == "." {
		scopePath = ""
	}
	name := "project"
	if scopePath != "" {
		name = strings.ReplaceAll(scopePath, "/", "-")
	}
	return sourceCandidate{kind: kind, dialect: dialect, path: path, name: name}
}

func readDefinition(projectRoot string, allPaths []string, candidate sourceCandidate) (Definition, error) {
	definitionPaths := []string{candidate.path}
	if candidate.bundleRoot != "" {
		definitionPaths = definitionPaths[:0]
		prefix := candidate.bundleRoot + "/"
		for _, path := range allPaths {
			if strings.HasPrefix(path, prefix) {
				definitionPaths = append(definitionPaths, path)
			}
		}
	}
	files := make([]File, 0, len(definitionPaths))
	var entrypointContent []byte
	for _, path := range definitionPaths {
		content, err := os.ReadFile(filepath.Join(projectRoot, filepath.FromSlash(path)))
		if err != nil {
			return Definition{}, fmt.Errorf("read artifact %s: %w", path, err)
		}
		contentHash := sha256.Sum256(content)
		files = append(files, File{Path: path, Content: content, SHA256: hex.EncodeToString(contentHash[:]), Mode: 0o644})
		if path == candidate.path {
			entrypointContent = content
		}
	}
	common, native := parseContent(candidate, entrypointContent)
	if common.Name == "" {
		common.Name = candidate.name
	}
	scopePath := ""
	scopeLevel := "project"
	baseName := filepath.Base(candidate.path)
	if baseName == "CLAUDE.md" || baseName == "AGENTS.md" || baseName == "GEMINI.md" {
		scopePath = filepath.ToSlash(filepath.Dir(candidate.path))
		if scopePath == "." {
			scopePath = ""
		} else {
			scopeLevel = "nested"
		}
	}
	definition := Definition{
		ID:            string(candidate.kind) + "::" + common.Name,
		Kind:          candidate.kind,
		Name:          common.Name,
		SourceDialect: candidate.dialect,
		SourcePath:    candidate.path,
		Scope:         Scope{Level: scopeLevel, Path: scopePath, AppliesTo: common.AppliesTo},
		Common:        common,
		Native:        native,
		Files:         files,
	}
	definition.SourceSHA256 = HashFiles(files)
	return definition, nil
}

func parseContent(candidate sourceCandidate, content []byte) (CommonFields, NativePayload) {
	format := "markdown"
	if strings.HasSuffix(strings.ToLower(candidate.path), ".toml") {
		format = "toml"
	}
	if strings.HasSuffix(strings.ToLower(candidate.path), ".js") {
		format = "javascript"
	}
	native := NativePayload{Format: format, Raw: append([]byte(nil), content...), Fields: map[string]any{}}
	common := CommonFields{Name: candidate.name, Body: strings.TrimSpace(string(content))}
	if format == "toml" {
		fields, parseErr := parseTOMLFields(string(content))
		native.Fields = fields
		if parseErr != nil {
			native.ParseError = parseErr.Error()
		}
		common.Name = stringField(fields, "name", candidate.name)
		common.Description = stringField(fields, "description", "")
		common.Body = stringField(fields, "developer_instructions", stringField(fields, "prompt", common.Body))
		native.Unmapped = unmappedKeys(fields, []string{"name", "description", "developer_instructions", "prompt"})
		return common, native
	}
	metadata, body, parseErr := parseFrontmatter(content)
	native.Fields = metadata
	if parseErr != nil {
		native.ParseError = parseErr.Error()
	}
	common.Name = stringField(metadata, "name", candidate.name)
	common.Description = stringField(metadata, "description", "")
	common.Body = strings.TrimSpace(body)
	common.AppliesTo = stringSliceField(metadata, "paths", "globs", "applyTo")
	common.Tools = stringSliceField(metadata, "tools")
	native.Unmapped = unmappedKeys(metadata, []string{"name", "description", "paths", "globs", "applyTo", "tools"})
	return common, native
}

func parseFrontmatter(content []byte) (map[string]any, string, error) {
	textContent := string(content)
	if !strings.HasPrefix(textContent, "---\n") && !strings.HasPrefix(textContent, "---\r\n") {
		return map[string]any{}, textContent, nil
	}
	lineEnding := "\n"
	if strings.HasPrefix(textContent, "---\r\n") {
		lineEnding = "\r\n"
	}
	separator := lineEnding + "---" + lineEnding
	endIndex := strings.Index(textContent[len("---"+lineEnding):], separator)
	if endIndex < 0 {
		return map[string]any{}, textContent, fmt.Errorf("frontmatter closing delimiter is missing")
	}
	metadataStart := len("---" + lineEnding)
	metadataEnd := metadataStart + endIndex
	metadata := map[string]any{}
	if err := yaml.Unmarshal([]byte(textContent[metadataStart:metadataEnd]), &metadata); err != nil {
		return map[string]any{}, textContent, fmt.Errorf("invalid YAML frontmatter: %w", err)
	}
	bodyStart := metadataEnd + len(separator)
	return metadata, textContent[bodyStart:], nil
}

func parseTOMLFields(content string) (map[string]any, error) {
	fields := map[string]any{}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	for lineIndex := 0; lineIndex < len(lines); lineIndex++ {
		line := strings.TrimSpace(lines[lineIndex])
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
				continue
			}
			return fields, fmt.Errorf("invalid TOML line %d", lineIndex+1)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if strings.HasPrefix(value, "\"\"\"") {
			value = strings.TrimPrefix(value, "\"\"\"")
			parts := []string{}
			if endIndex := strings.Index(value, "\"\"\""); endIndex >= 0 {
				parts = append(parts, value[:endIndex])
			} else {
				parts = append(parts, value)
				closed := false
				for lineIndex++; lineIndex < len(lines); lineIndex++ {
					if endIndex := strings.Index(lines[lineIndex], "\"\"\""); endIndex >= 0 {
						parts = append(parts, lines[lineIndex][:endIndex])
						closed = true
						break
					}
					parts = append(parts, lines[lineIndex])
				}
				if !closed {
					return fields, fmt.Errorf("unterminated TOML multiline string for %s", key)
				}
			}
			fields[key] = strings.Join(parts, "\n")
			continue
		}
		fields[key] = strings.Trim(strings.TrimSpace(value), "\"'")
	}
	return fields, nil
}

func stringField(fields map[string]any, key, fallback string) string {
	value, exists := fields[key]
	if !exists {
		return fallback
	}
	text, valid := value.(string)
	if !valid {
		return fallback
	}
	return strings.TrimSpace(text)
}

func stringSliceField(fields map[string]any, keys ...string) []string {
	for _, key := range keys {
		value, exists := fields[key]
		if !exists {
			continue
		}
		switch typedValue := value.(type) {
		case string:
			if strings.TrimSpace(typedValue) != "" {
				return []string{strings.TrimSpace(typedValue)}
			}
		case []any:
			values := make([]string, 0, len(typedValue))
			for _, item := range typedValue {
				text, valid := item.(string)
				if valid && strings.TrimSpace(text) != "" {
					values = append(values, strings.TrimSpace(text))
				}
			}
			return values
		}
	}
	return nil
}

func unmappedKeys(fields map[string]any, knownKeys []string) []string {
	known := make(map[string]struct{}, len(knownKeys))
	for _, key := range knownKeys {
		known[key] = struct{}{}
	}
	keys := make([]string, 0)
	for key := range fields {
		if _, exists := known[key]; !exists {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}
