// Package scanner discovers AI artifacts (skills, agents, commands, instructions)
// inside a remote repository tree. It is intentionally decoupled from any
// transport (internal/http) or persistence (internal/fs) layer so the same
// scanners can run against any tree provider.
package scanner

import "sth/internal/artifact"

type ArtifactKind = artifact.Kind

const (
	KindSkill       = artifact.KindSkill
	KindAgent       = artifact.KindAgent
	KindCommand     = artifact.KindCommand
	KindInstruction = artifact.KindInstruction
	KindRule        = artifact.KindRule
	KindWorkflow    = artifact.KindWorkflow
)

// TreeEntry is a single path returned by a repository tree listing.
// Type is the upstream object type ("blob", "tree", "directory", ...).
type TreeEntry struct {
	Path string
	Type string
}

// Artifact is a discovered installable artifact, decoupled from any catalog format.
type Artifact struct {
	Kind       ArtifactKind
	Name       string
	Category   string
	Lang       string
	Files      []string
	Entrypoint string
	AppliesTo  []string
}

// RepoScanner discovers Artifacts inside a repository tree.
// Implementations must be safe to call repeatedly and must not mutate the input.
type RepoScanner interface {
	Name() string
	Detect(tree []TreeEntry) bool
	Scan(tree []TreeEntry) ([]Artifact, error)
}
