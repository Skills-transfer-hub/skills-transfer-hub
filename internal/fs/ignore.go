package fs

import (
	"bufio"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// ReadLoreSkillsIgnore reads .loreskillsignore from projectDir.
// Returns nil slice (no error) when the file is absent.
func ReadLoreSkillsIgnore(projectDir string) ([]string, error) {
	p := filepath.Join(projectDir, ".loreskillsignore")
	f, err := os.Open(p)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var patterns []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns, sc.Err()
}

// MatchesIgnorePattern reports whether relPath matches any gitignore-style pattern.
// Supports *, ?, ** (double-star matches across path segments).
func MatchesIgnorePattern(relPath string, patterns []string) bool {
	normalized := filepath.ToSlash(relPath)
	for _, pat := range patterns {
		if matchGlob(pat, normalized) {
			return true
		}
	}
	return false
}

// matchGlob matches a gitignore-style pattern against a normalized (forward-slash) path.
func matchGlob(pattern, target string) bool {
	if !strings.Contains(pattern, "**") {
		ok, _ := path.Match(pattern, target)
		if ok {
			return true
		}
		// Also try matching basename only (gitignore behaviour).
		base := path.Base(target)
		ok, _ = path.Match(pattern, base)
		return ok
	}
	// Handle ** by splitting on ** and matching segments.
	parts := strings.SplitN(pattern, "**", 2)
	prefix := strings.TrimRight(parts[0], "/")
	suffix := strings.TrimLeft(parts[1], "/")

	if prefix != "" {
		if !strings.HasPrefix(target, prefix) {
			return false
		}
		target = target[len(prefix):]
		target = strings.TrimPrefix(target, "/")
	}
	if suffix == "" {
		return true
	}
	if strings.HasSuffix(target, suffix) {
		return true
	}
	segments := strings.Split(target, "/")
	for i := range segments {
		sub := strings.Join(segments[i:], "/")
		ok, _ := path.Match(suffix, sub)
		if ok {
			return true
		}
	}
	return false
}
