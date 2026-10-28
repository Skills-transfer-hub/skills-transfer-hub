package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
	"gopkg.in/yaml.v3"

	metadata "sth/internal/frontmatter"
	"sth/internal/scanner"
)

type globalTreeEntry struct {
	Path string `json:"path"`
	Type string `json:"type"`
	Mode string `json:"mode"`
	Size int64  `json:"size"`
}

var globalSkillName = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

type globalRepositorySnapshot struct {
	source   GlobalSource
	revision string
	mutex    sync.Mutex
	tree     []globalTreeEntry
}

func (c *Client) globalTree(ctx context.Context, resolution GlobalResolution) ([]globalTreeEntry, error) {
	if err := validateGlobalSource(resolution.Source); err != nil {
		return nil, err
	}
	if !globalFullRevision.MatchString(resolution.Revision) {
		return nil, fmt.Errorf("global source revision must be a full Git SHA")
	}
	if resolution.snapshot != nil && (resolution.snapshot.source != resolution.Source || resolution.snapshot.revision != resolution.Revision) {
		return nil, fmt.Errorf("resolved source identity or revision was changed")
	}
	if resolution.snapshot == nil {
		return c.loadGlobalTree(ctx, resolution)
	}
	resolution.snapshot.mutex.Lock()
	defer resolution.snapshot.mutex.Unlock()
	if resolution.snapshot.tree != nil {
		return resolution.snapshot.tree, nil
	}
	entries, err := c.loadGlobalTree(ctx, resolution)
	if err != nil {
		return nil, err
	}
	resolution.snapshot.tree = entries
	return entries, nil
}

func (c *Client) loadGlobalTree(ctx context.Context, resolution GlobalResolution) ([]globalTreeEntry, error) {
	if err := validateGlobalSource(resolution.Source); err != nil {
		return nil, err
	}
	if !globalFullRevision.MatchString(resolution.Revision) {
		return nil, fmt.Errorf("global source revision must be a full Git SHA")
	}
	profile := globalSourceProfile(resolution.Source, resolution.Revision)
	headers, err := resolveCredentialHeaders(ctx, profile)
	if err != nil {
		return nil, err
	}
	baseURL := strings.TrimRight(getEnvOrDefault("STH_GITHUB_API_BASE_URL", "https://api.github.com"), "/")
	data, err := c.downloadURL(ctx, baseURL+"/repos/"+resolution.Source.Repository+"/git/trees/"+url.PathEscape(resolution.Revision)+"?recursive=1", headers.headers)
	if err != nil {
		return nil, decorateAccessError(err, headers.source, "github")
	}
	var response struct {
		Tree      []globalTreeEntry `json:"tree"`
		Truncated bool              `json:"truncated"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("invalid GitHub tree response: %w", err)
	}
	if response.Truncated {
		return nil, fmt.Errorf("GitHub returned a truncated repository tree; global installation requires a complete tree")
	}
	seen := map[string]string{}
	entries := map[string]bool{}
	for _, entry := range response.Tree {
		if err := validateGlobalRelativePath(entry.Path); err != nil {
			return nil, err
		}
		if entries[entry.Path] {
			return nil, fmt.Errorf("duplicate repository tree path %q", entry.Path)
		}
		entries[entry.Path] = true
		for prefix := entry.Path; prefix != "."; prefix = path.Dir(prefix) {
			folded := strings.ToLower(norm.NFC.String(prefix))
			if previous, exists := seen[folded]; exists && previous != prefix {
				return nil, fmt.Errorf("case collision between %q and %q", previous, prefix)
			}
			seen[folded] = prefix
		}
	}
	sort.Slice(response.Tree, func(left, right int) bool { return response.Tree[left].Path < response.Tree[right].Path })
	return response.Tree, nil
}

func validateGlobalRelativePath(filePath string) error {
	if filePath == "" || filePath == "." || path.IsAbs(filePath) || path.Clean(filePath) != filePath || len(filePath) > 4096 || strings.ContainsAny(filePath, `\<>:"|?*`) {
		return &GlobalSelectionError{Message: fmt.Sprintf("unsafe repository path %q", filePath)}
	}
	for _, character := range filePath {
		if unicode.IsControl(character) {
			return &GlobalSelectionError{Message: "repository path contains control characters"}
		}
	}
	for _, segment := range strings.Split(filePath, "/") {
		if segment == ".." || segment == "." || segment == "" || len(segment) > 255 || strings.TrimRight(segment, ". ") != segment {
			return &GlobalSelectionError{Message: fmt.Sprintf("unsafe repository path %q", filePath)}
		}
		base := strings.ToUpper(strings.SplitN(segment, ".", 2)[0])
		reserved := base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || base == "CLOCK$"
		if len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9' {
			reserved = true
		}
		if strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT") {
			suffix := strings.TrimPrefix(strings.TrimPrefix(base, "COM"), "LPT")
			if suffix == "¹" || suffix == "²" || suffix == "³" {
				reserved = true
			}
		}
		if reserved {
			return &GlobalSelectionError{Message: fmt.Sprintf("Windows-reserved repository path %q", filePath)}
		}
	}
	return nil
}

func (c *Client) globalDownload(ctx context.Context, resolution GlobalResolution, filePath string) ([]byte, error) {
	if err := validateGlobalRelativePath(filePath); err != nil {
		return nil, err
	}
	profile := globalSourceProfile(resolution.Source, resolution.Revision)
	headers, err := resolveCredentialHeaders(ctx, profile)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(filePath, "/")
	for index, segment := range parts {
		parts[index] = url.PathEscape(segment)
	}
	remoteURL, err := providerResourceURL(profile, strings.Join(parts, "/"))
	if err != nil {
		return nil, err
	}
	data, err := c.downloadURL(ctx, remoteURL, headers.headers)
	if err != nil {
		return nil, decorateAccessError(err, headers.source, "github")
	}
	return data, nil
}

func (c *Client) DiscoverGlobalSkills(ctx context.Context, resolution GlobalResolution, scope string) ([]GlobalCandidate, error) {
	if scope != "" {
		if err := validateGlobalRelativePath(scope); err != nil {
			return nil, err
		}
	}
	tree, err := c.globalTree(ctx, resolution)
	if err != nil {
		return nil, err
	}
	paths := map[string]globalTreeEntry{}
	scannerTree := make([]scanner.TreeEntry, 0, len(tree))
	for _, entry := range tree {
		paths[entry.Path] = entry
		scannerTree = append(scannerTree, scanner.TreeEntry{Path: entry.Path, Type: entry.Type})
	}
	entrypoints := []string{}
	if _, exists := paths["SKILLS.md"]; exists && scope == "" {
		content, err := c.globalDownload(ctx, resolution, "SKILLS.md")
		if err != nil {
			return nil, err
		}
		entries, err := parseCatalogEntries(string(content))
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.ResourceType == "bundle" || entry.ResourceType == "skill" {
				if err := validateGlobalRelativePath(entry.SourcePath); err != nil {
					return nil, err
				}
				if !strings.HasSuffix(entry.SourcePath, "/SKILL.md") {
					return nil, fmt.Errorf("catalog skill %q must reference a dedicated folder containing SKILL.md", entry.Name)
				}
				entrypoints = append(entrypoints, entry.SourcePath)
			}
		}
	} else {
		artifacts, _ := (scanner.MultiStackScanner{}).Scan(scannerTree)
		for _, artifact := range artifacts {
			if strings.HasSuffix(artifact.Entrypoint, "/SKILL.md") {
				entrypoints = append(entrypoints, artifact.Entrypoint)
			}
		}
		for _, entry := range tree {
			if strings.Count(entry.Path, "/") == 1 && strings.HasSuffix(entry.Path, "/SKILL.md") {
				entrypoints = append(entrypoints, entry.Path)
			}
		}
		if scope == "" {
			canonical := []string{}
			for _, entrypoint := range entrypoints {
				if strings.HasPrefix(entrypoint, "skills/") {
					canonical = append(canonical, entrypoint)
				}
			}
			if len(canonical) > 0 {
				entrypoints = canonical
			}
		}
	}
	sort.Strings(entrypoints)
	seen := map[string]bool{}
	candidates := []GlobalCandidate{}
	for _, entrypoint := range entrypoints {
		folder := path.Dir(entrypoint)
		if seen[folder] || (scope != "" && folder != scope && !strings.HasPrefix(folder, scope+"/")) {
			continue
		}
		seen[folder] = true
		entry, exists := paths[entrypoint]
		if !exists {
			return nil, fmt.Errorf("catalog references missing skill %q", entrypoint)
		}
		if entry.Type != "blob" || (entry.Mode != "100644" && entry.Mode != "100755") {
			return nil, fmt.Errorf("skill entrypoint %q must be a regular file, not a symlink or submodule", entrypoint)
		}
		content, err := c.globalDownload(ctx, resolution, entrypoint)
		if err != nil {
			return nil, err
		}
		candidate, _, err := validateGlobalSkillMetadata(folder, content)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate)
	}
	if len(candidates) == 0 {
		if _, exists := paths["SKILL.md"]; exists {
			return nil, &GlobalSelectionError{Message: "repository-root SKILL.md is unsupported in global V1; provide --path to a dedicated skill directory"}
		}
		return nil, &GlobalSelectionError{Message: "no skills found in the selected repository path"}
	}
	return candidates, nil
}

func validateGlobalSkillMetadata(folder string, content []byte) (GlobalCandidate, []string, error) {
	values, valid := scanner.ExtractFrontmatter(content)
	if !valid || !globalSkillName.MatchString(values.Name) || len(values.Name) > 64 || values.Name != path.Base(folder) {
		return GlobalCandidate{}, nil, fmt.Errorf("invalid skill %q: frontmatter name must match its directory and contain 1 to 64 lowercase letters, digits, and single hyphens", folder)
	}
	if strings.TrimSpace(values.Description) == "" || utf8.RuneCountInString(values.Description) > 1024 {
		return GlobalCandidate{}, nil, fmt.Errorf("invalid skill %q: description must contain 1 to 1024 characters", folder)
	}
	if _, present, _ := metadata.LookupString(content, "hooks"); present {
		return GlobalCandidate{}, nil, fmt.Errorf("skill %q declares hooks; plugin activation is outside global skills V1", folder)
	}
	warnings := globalFrontmatterWarnings(folder, content)
	return GlobalCandidate{Name: values.Name, Path: folder, Description: values.Description}, warnings, nil
}

func SelectGlobalSkills(candidates []GlobalCandidate, names []string) ([]GlobalCandidate, error) {
	if len(candidates) == 0 {
		return nil, &GlobalSelectionError{Message: "no skills available for selection"}
	}
	if len(names) == 0 {
		if len(candidates) == 1 {
			return candidates, nil
		}
		descriptions := []string{}
		for _, candidate := range candidates {
			descriptions = append(descriptions, candidate.Name+" ("+candidate.Path+")")
		}
		return nil, &GlobalSelectionError{Message: "select --skill from: " + strings.Join(descriptions, ", ")}
	}
	wildcard := len(names) == 1 && names[0] == "*"
	requested := map[string]bool{}
	for _, name := range names {
		if name == "*" && !wildcard {
			return nil, &GlobalSelectionError{Message: "--skill '*' cannot be combined with named skills"}
		}
		requested[name] = true
	}
	matches := map[string][]GlobalCandidate{}
	for _, candidate := range candidates {
		if wildcard || requested[candidate.Name] {
			matches[candidate.Name] = append(matches[candidate.Name], candidate)
		}
	}
	for name := range requested {
		if !wildcard && len(matches[name]) == 0 {
			return nil, &GlobalSelectionError{Message: fmt.Sprintf("unknown skill %q", name)}
		}
	}
	selected := []GlobalCandidate{}
	for name, found := range matches {
		if len(found) > 1 {
			paths := []string{}
			for _, candidate := range found {
				paths = append(paths, candidate.Path)
			}
			sort.Strings(paths)
			return nil, &GlobalSelectionError{Message: fmt.Sprintf("ambiguous skill %q: %s; select a directory with --path", name, strings.Join(paths, ", "))}
		}
		selected = append(selected, found[0])
	}
	sort.Slice(selected, func(left, right int) bool { return selected[left].Path < selected[right].Path })
	return selected, nil
}

func globalFrontmatterWarnings(folder string, content []byte) []string {
	root := yaml.Node{}
	if err := yaml.NewDecoder(bytes.NewReader(content)).Decode(&root); err != nil || len(root.Content) != 1 {
		return nil
	}
	mapping := root.Content[0]
	warnings := []string{}
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		field := mapping.Content[index].Value
		switch field {
		case "name", "description", "license":
			continue
		}
		warnings = append(warnings, fmt.Sprintf("%s: field %q is preserved; its behavior depends on the consuming agent", folder, field))
	}
	sort.Strings(warnings)
	return warnings
}
