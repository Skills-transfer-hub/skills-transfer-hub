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
// Supports * (single segment), ** (any number of segments), and basename-only matching.
func matchGlob(pattern, target string) bool {
	// Handle trailing-slash directory patterns: "skills/" matches any path inside "skills/".
	if strings.HasSuffix(pattern, "/") {
		return strings.Contains("/"+target, "/"+pattern)
	}
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
	// Find the first ** and split there; recurse for multiple ** occurrences.
	idx := strings.Index(pattern, "**")
	prefix := strings.TrimRight(pattern[:idx], "/")
	rest := strings.TrimLeft(pattern[idx+2:], "/")

	if prefix != "" {
		if !strings.HasPrefix(target, prefix) {
			return false
		}
		target = strings.TrimPrefix(target[len(prefix):], "/")
	}
	if rest == "" {
		return true
	}
	// ** can match zero or more path segments; try each suffix of target.
	segments := strings.Split(target, "/")
	for i := 0; i <= len(segments); i++ {
		sub := strings.Join(segments[i:], "/")
		if matchGlob(rest, sub) {
			return true
		}
	}
	return false
}

// ParseIgnoreFile reads .loreskillsignore from projectRoot and returns active patterns.
// Lines starting with '#' and blank lines are ignored.
// Returns nil patterns (not an error) if the file does not exist.
func ParseIgnoreFile(projectRoot string) ([]string, error) {
	return ReadLoreSkillsIgnore(projectRoot)
}

// MatchIgnore reports whether bundleRelativePath matches any of the given gitignore patterns.
// Patterns support *, **, and ? glob syntax. bundleRelativePath must use forward slashes.
func MatchIgnore(patterns []string, bundleRelativePath string) bool {
	return MatchesIgnorePattern(bundleRelativePath, patterns)
}
