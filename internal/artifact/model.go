package artifact

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
)

type Kind string

const (
	KindSkill       Kind = "skill"
	KindAgent       Kind = "agent"
	KindInstruction Kind = "instruction"
	KindRule        Kind = "rule"
	KindCommand     Kind = "command"
	KindWorkflow    Kind = "workflow"
)

type Dialect string

const (
	DialectSTHCatalog          Dialect = "sth.catalog.v1"
	DialectAgentSkills         Dialect = "agent-skills.v1"
	DialectSharedAgentsMD      Dialect = "agents-md.v1"
	DialectClaudeAgent         Dialect = "claude.agent.v1"
	DialectClaudeInstruction   Dialect = "claude.instruction.v1"
	DialectClaudeRule          Dialect = "claude.rule.v1"
	DialectClaudeCommand       Dialect = "claude.command.v1"
	DialectClaudeWorkflow      Dialect = "claude.workflow.v1"
	DialectCodexAgent          Dialect = "codex.agent.v1"
	DialectCodexExecutionRule  Dialect = "codex.execution-rule.v1"
	DialectGeminiInstruction   Dialect = "gemini.instruction.v1"
	DialectGeminiAgent         Dialect = "gemini.agent.v1"
	DialectGeminiCommand       Dialect = "gemini.command.v1"
	DialectAntigravityAgent    Dialect = "antigravity.agent.v1"
	DialectAntigravityTeam     Dialect = "antigravity.team.v1"
	DialectAntigravityRule     Dialect = "antigravity.rule.v1"
	DialectAntigravityWorkflow Dialect = "antigravity.workflow.v1"
	DialectCopilotInstruction  Dialect = "copilot.instruction.v1"
	DialectCopilotRule         Dialect = "copilot.rule.v1"
	DialectCopilotAgent        Dialect = "copilot.agent.v1"
	DialectCopilotPrompt       Dialect = "copilot.prompt.v1"
	DialectCursorRule          Dialect = "cursor.rule.v1"
	DialectCursorAgent         Dialect = "cursor.agent.v1"
)

type Scope struct {
	Level     string   `json:"level"`
	Path      string   `json:"path,omitempty"`
	AppliesTo []string `json:"applies_to,omitempty"`
}

type CommonFields struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Body        string   `json:"body,omitempty"`
	AppliesTo   []string `json:"applies_to,omitempty"`
	Tools       []string `json:"tools,omitempty"`
}

type NativePayload struct {
	Format     string         `json:"format"`
	Raw        []byte         `json:"-"`
	Fields     map[string]any `json:"fields,omitempty"`
	Unmapped   []string       `json:"unmapped,omitempty"`
	ParseError string         `json:"parse_error,omitempty"`
}

type File struct {
	Path    string `json:"path"`
	Content []byte `json:"-"`
	SHA256  string `json:"sha256"`
	Mode    uint32 `json:"mode,omitempty"`
}

type ArtifactDefinition struct {
	ID            string        `json:"id"`
	Kind          Kind          `json:"kind"`
	Name          string        `json:"name"`
	SourceDialect Dialect       `json:"source_dialect"`
	SourcePath    string        `json:"source_path"`
	SourceSHA256  string        `json:"source_sha256"`
	Scope         Scope         `json:"scope"`
	Common        CommonFields  `json:"common"`
	Native        NativePayload `json:"native"`
	Files         []File        `json:"files"`
}

type Definition = ArtifactDefinition

type Severity string

const (
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

type Diagnostic struct {
	Severity Severity `json:"severity"`
	Code     string   `json:"code"`
	Path     string   `json:"path,omitempty"`
	Field    string   `json:"field,omitempty"`
	Message  string   `json:"message"`
}

type CompatibilityLevel string

const (
	CompatibilityExact       CompatibilityLevel = "exact"
	CompatibilityLossy       CompatibilityLevel = "lossy"
	CompatibilityNativeOnly  CompatibilityLevel = "native-only"
	CompatibilityUnsupported CompatibilityLevel = "unsupported"
)

type CompatibilityReport struct {
	Preserved   []string `json:"preserved,omitempty"`
	Transformed []string `json:"transformed,omitempty"`
	Dropped     []string `json:"dropped,omitempty"`
	Warnings    []string `json:"warnings,omitempty"`
}

type PlannedOutput struct {
	Consumers []string `json:"consumers"`
	Path      string   `json:"path"`
	SHA256    string   `json:"sha256,omitempty"`
}

type CompatibilityPlan struct {
	SourceDialect      Dialect             `json:"source_dialect"`
	TargetDialect      Dialect             `json:"target_dialect"`
	Level              CompatibilityLevel  `json:"level"`
	Transformer        string              `json:"transformer"`
	TransformerVersion string              `json:"transformer_version"`
	Report             CompatibilityReport `json:"report"`
	Outputs            []PlannedOutput     `json:"outputs,omitempty"`
}

func Kinds() []Kind {
	return []Kind{KindSkill, KindAgent, KindInstruction, KindRule, KindCommand, KindWorkflow}
}

func HashFiles(files []File) string {
	orderedFiles := append([]File(nil), files...)
	sort.Slice(orderedFiles, func(leftIndex, rightIndex int) bool {
		return orderedFiles[leftIndex].Path < orderedFiles[rightIndex].Path
	})
	hash := sha256.New()
	for _, file := range orderedFiles {
		hash.Write([]byte(file.Path))
		hash.Write([]byte{0})
		hash.Write(file.Content)
		hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}
