package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"sth/internal/fs"
)

type GlobalSource struct {
	Provider   string `json:"provider"`
	Host       string `json:"host"`
	Repository string `json:"repository"`
}

type GlobalSelectionError struct{ Message string }

func (e *GlobalSelectionError) Error() string { return e.Message }

func (s GlobalSource) CanonicalID(subdir string) string {
	return s.Provider + ":" + s.Host + "/" + strings.ToLower(s.Repository) + "#" + subdir
}

var globalRepositorySegment = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func ParseGlobalSource(input string) (GlobalSource, error) {
	repository := ""
	if strings.HasPrefix(input, "github:") {
		repository = strings.TrimPrefix(input, "github:")
	} else {
		parsed, err := url.Parse(input)
		if err != nil || parsed.Scheme != "https" || !strings.EqualFold(parsed.Host, "github.com") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.RawPath != "" {
			return GlobalSource{}, &GlobalSelectionError{Message: "provide an HTTPS GitHub repository URL or github:owner/repo; use --ref and --path for a branch or subdirectory"}
		}
		repository = strings.TrimSuffix(strings.TrimPrefix(parsed.Path, "/"), "/")
	}
	parts := strings.Split(repository, "/")
	if len(parts) != 2 {
		return GlobalSource{}, &GlobalSelectionError{Message: "provide a GitHub repository as owner/repo; use --ref and --path instead of tree/blob URLs"}
	}
	parts[1] = strings.TrimSuffix(parts[1], ".git")
	for _, segment := range parts {
		if !globalRepositorySegment.MatchString(segment) || segment == "." || segment == ".." {
			return GlobalSource{}, &GlobalSelectionError{Message: "invalid GitHub owner or repository name"}
		}
	}
	return GlobalSource{Provider: "github", Host: "github.com", Repository: strings.ToLower(strings.Join(parts, "/"))}, nil
}

func validateGlobalSource(source GlobalSource) error {
	parsed, err := ParseGlobalSource("github:" + source.Repository)
	if err != nil || source.Provider != parsed.Provider || source.Host != parsed.Host || source.Repository != parsed.Repository {
		return fmt.Errorf("invalid global GitHub source")
	}
	return nil
}

type GlobalResolution struct {
	Source       GlobalSource
	RequestedRef string
	Revision     string
	Candidates   []GlobalCandidate
	Warnings     []string
	snapshot     *globalRepositorySnapshot
}

type GlobalCandidate struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Description string `json:"description"`
}

var globalFullRevision = regexp.MustCompile(`^[a-fA-F0-9]{40}$`)

func (c *Client) ResolveGlobalSource(ctx context.Context, source GlobalSource, reference string) (GlobalResolution, error) {
	if err := validateGlobalSource(source); err != nil {
		return GlobalResolution{}, err
	}
	if reference != "" && !validGlobalReference(reference) {
		return GlobalResolution{}, &GlobalSelectionError{Message: "invalid Git reference"}
	}
	profile := globalSourceProfile(source, "")
	headers, err := resolveCredentialHeaders(ctx, profile)
	if err != nil {
		return GlobalResolution{}, err
	}
	baseURL := strings.TrimRight(getEnvOrDefault("STH_GITHUB_API_BASE_URL", "https://api.github.com"), "/") + "/repos/" + source.Repository
	if reference == "" {
		data, err := c.downloadURL(ctx, baseURL, headers.headers)
		if err != nil {
			return GlobalResolution{}, decorateAccessError(err, headers.source, "github")
		}
		var repository struct {
			DefaultBranch string `json:"default_branch"`
		}
		if err := json.Unmarshal(data, &repository); err != nil {
			return GlobalResolution{}, fmt.Errorf("invalid GitHub repository response: %w", err)
		}
		reference = repository.DefaultBranch
		if !validGlobalReference(reference) {
			return GlobalResolution{}, fmt.Errorf("GitHub repository has no default branch")
		}
	}
	data, err := c.downloadURL(ctx, baseURL+"/commits/"+url.PathEscape(reference), headers.headers)
	if err != nil {
		return GlobalResolution{}, decorateAccessError(err, headers.source, "github")
	}
	var commit struct {
		SHA string `json:"sha"`
	}
	if err := json.Unmarshal(data, &commit); err != nil {
		return GlobalResolution{}, fmt.Errorf("invalid GitHub commit response: %w", err)
	}
	if !globalFullRevision.MatchString(commit.SHA) {
		return GlobalResolution{}, fmt.Errorf("invalid GitHub commit SHA for %s", source.Repository)
	}
	if globalFullRevision.MatchString(reference) && !strings.EqualFold(reference, commit.SHA) {
		return GlobalResolution{}, fmt.Errorf("GitHub returned a different commit for the explicitly requested SHA")
	}
	return GlobalResolution{Source: source, RequestedRef: reference, Revision: strings.ToLower(commit.SHA), snapshot: &globalRepositorySnapshot{source: source, revision: strings.ToLower(commit.SHA)}}, nil
}

func globalSourceProfile(source GlobalSource, revision string) fs.ProjectProfile {
	return fs.ProjectProfile{Provider: source.Provider, RepositoryID: source.Repository, Ref: revision, CatalogPath: "SKILLS.md"}
}

func validGlobalReference(reference string) bool {
	if reference == "" || reference == "@" || strings.Contains(reference, "..") || strings.Contains(reference, "@{") || strings.Contains(reference, "//") || strings.ContainsAny(reference, " ~^:?*[\\") || strings.HasPrefix(reference, "/") || strings.HasSuffix(reference, "/") || strings.HasSuffix(reference, ".") {
		return false
	}
	for _, character := range reference {
		if character < 33 || character == 127 {
			return false
		}
	}
	for _, segment := range strings.Split(reference, "/") {
		if strings.HasPrefix(segment, ".") || strings.HasSuffix(segment, ".lock") {
			return false
		}
	}
	return true
}
