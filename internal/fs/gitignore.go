package fs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	gitignoreManagedHeader = "# >>> sth (généré, ne pas éditer)"
	gitignoreManagedFooter = "# <<< sth"
)

func managedGitignoreBlock() string {
	lines := []string{
		gitignoreManagedHeader,
		"**/sth-state-v2.json",
		"**/sth-state.json",
		"**/.sth-state.lock",
		"**/.sth-tmp-*",
		".sth/last-operation.json",
		gitignoreManagedFooter,
	}
	return strings.Join(lines, "\n") + "\n"
}

func EnsureManagedGitignore(projectRoot string) (bool, error) {
	if _, statErr := os.Stat(filepath.Join(projectRoot, ".git")); statErr != nil {
		return false, nil
	}

	gitignorePath := filepath.Join(projectRoot, ".gitignore")
	existing, err := os.ReadFile(gitignorePath)
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("lecture de %s échouée : %w", gitignorePath, err)
	}

	updated, changed := upsertManagedBlock(string(existing), managedGitignoreBlock())
	if !changed {
		return false, nil
	}
	if err := atomicWriteFile(gitignorePath, []byte(updated), 0o644); err != nil {
		return false, fmt.Errorf("écriture de %s échouée : %w", gitignorePath, err)
	}
	return true, nil
}

func upsertManagedBlock(existing, block string) (string, bool) {
	start := strings.Index(existing, gitignoreManagedHeader)
	if start == -1 {
		prefix := existing
		if prefix != "" && !strings.HasSuffix(prefix, "\n") {
			prefix += "\n"
		}
		if prefix != "" {
			prefix += "\n"
		}
		return prefix + block, true
	}

	footerOffset := strings.Index(existing[start:], gitignoreManagedFooter)
	if footerOffset == -1 {
		return existing[:start] + block, true
	}
	end := start + footerOffset + len(gitignoreManagedFooter)
	if end < len(existing) && existing[end] == '\n' {
		end++
	}
	if existing[start:end] == block {
		return existing, false
	}
	return existing[:start] + block + existing[end:], true
}
