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

// TargetWriter writes artifacts for one target tool.
type TargetWriter interface {
	Kind() string
	Write(req WriteRequest) (WriteResult, error)
}
