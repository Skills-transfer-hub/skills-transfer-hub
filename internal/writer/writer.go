// Package writer installs AI artifacts into the project tree for a given
// target tool (Claude Code, GitHub Copilot, OpenAI Codex CLI, Cursor, ...).
// The package is intentionally decoupled from the storage and discovery
// layers: callers pass already-resolved paths and content; writers
// produce on-disk files and report what they wrote.
package writer

// WriteRequest is everything a target writer needs to materialize one file.
type WriteRequest struct {
	// InstalledPath is the on-disk path the writer must produce — either an
	// absolute path or a path resolved relative to the caller's working
	// directory. STH callers (internal/fs) currently pass relative paths
	// rooted in the project; external callers may pass either.
	InstalledPath string
	// Content is the raw file content as fetched from the source repository.
	Content []byte
	// AddManagedHeader requests that the writer prepend its managed-file
	// marker when the destination is a Markdown file. Writers are free to
	// honor this in a tool-specific way (full banner for Claude, HTML
	// comment for Copilot/Cursor/Codex, none for non-Markdown).
	AddManagedHeader bool
}

// WriteResult describes one file that was actually written.
type WriteResult struct {
	Path   string // slash-normalized installed path
	SHA256 string // hex digest of the bytes as they sit on disk
}

// PlannedFile is one file a writer wants to materialize for a given artifact.
// SourcePath is the upstream path the orchestrator must fetch the content from;
// RelativePath is where the file will live under projectRoot once written.
type PlannedFile struct {
	SourcePath   string
	RelativePath string
}

// PlanResult is the outcome of TargetPlanner.Plan for one artifact.
//
// When Skipped is true the writer signals that the artifact's concept does
// not exist for this target (e.g. a slash-command for Copilot, an agent for
// Cursor). The orchestrator should NOT install anything, NOT record state,
// and log Reason verbatim. Files is empty when Skipped is true.
type PlanResult struct {
	Skipped bool
	Reason  string
	Files   []PlannedFile
}

// TargetWriter writes artifacts for one target tool.
type TargetWriter interface {
	Kind() string
	Write(req WriteRequest) (WriteResult, error)
}

// TargetPlanner is the extension every concrete writer implements so the
// orchestrator (lands in PR6) can fan out one artifact onto N targets without
// hardcoding tool-specific conventions.
type TargetPlanner interface {
	Plan(artifactKind, artifactName, lang string, sources []string) PlanResult
}
