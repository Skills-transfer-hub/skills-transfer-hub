package http

import (
	"context"
	"encoding/json"
	"fmt"
	"sth/internal/fs"
	neturl "net/url"
	"path"
	"sort"
	"strings"
)

const (
	discoveryKindCatalog       = "skills-md"
	discoveryKindStructureScan = "structure-scan"
	discoveryStatusUsed        = "used"
)

type discoverySource struct {
	Kind        string
	CatalogPath string
	ScanRoot    string
	Precedence  int
	Status      string
	Detail      string
}

type repositoryTreeEntry struct {
	Path string
	Type string
}

type githubTreeResponse struct {
	Tree []struct {
		Path string `json:"path"`
		Type string `json:"type"`
	} `json:"tree"`
}

type gitLabTreeEntry struct {
	Path string `json:"path"`
	Type string `json:"type"`
}

type azureTreeResponse struct {
	Value []struct {
		Path          string `json:"path"`
		GitObjectType string `json:"gitObjectType"`
	} `json:"value"`
}

type bitbucketTreeResponse struct {
	Values []struct {
		Path string `json:"path"`
		Type string `json:"type"`
	} `json:"values"`
	Next string `json:"next"`
}

func newCatalogDiscoverySource(catalogPath, detail string) discoverySource {
	return discoverySource{
		Kind:        discoveryKindCatalog,
		CatalogPath: normalizeDiscoveryPath(catalogPath),
		Precedence:  1,
		Status:      discoveryStatusUsed,
		Detail:      strings.TrimSpace(detail),
	}
}

func newScanDiscoverySource(scanRoot, detail string) discoverySource {
	return discoverySource{
		Kind:       discoveryKindStructureScan,
		ScanRoot:   normalizeDiscoveryPath(scanRoot),
		Precedence: 2,
		Status:     discoveryStatusUsed,
		Detail:     strings.TrimSpace(detail),
	}
}

func normalizeDiscoveryPath(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}

	return path.Clean(strings.ReplaceAll(trimmed, "\\", "/"))
}

func isSkillEntrypoint(resourcePath string) bool {
	categoryName, folderName, err := splitSkillEntrypoint(resourcePath)
	return err == nil && categoryName != "" && folderName != ""
}

func splitSkillEntrypoint(resourcePath string) (string, string, error) {
	normalized := normalizeDiscoveryPath(resourcePath)
	if normalized == "" || path.IsAbs(normalized) || normalized == ".." || strings.HasPrefix(normalized, "../") {
		return "", "", fmt.Errorf("resource path invalide : %q", resourcePath)
	}

	parts := strings.Split(normalized, "/")
	if len(parts) != 3 || parts[2] != "SKILL.md" {
		return "", "", fmt.Errorf("resource path incompatible : %q", resourcePath)
	}
	if strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", fmt.Errorf("resource path incompatible : %q", resourcePath)
	}

	return parts[0], parts[1], nil
}

func catalogEntriesFromRepositoryTree(entries []repositoryTreeEntry) ([]fs.CatalogEntry, error) {
	discovered := make(map[string]fs.CatalogEntry)
	for _, entry := range entries {
		categoryName, folderName, err := splitSkillEntrypoint(entry.Path)
		if err != nil {
			continue
		}

		catalogID := fs.CatalogID("bundle", categoryName, folderName)
		if _, exists := discovered[catalogID]; exists {
			continue
		}

		discovered[catalogID] = fs.CatalogEntry{
			CatalogID:    catalogID,
			ResourceType: "bundle",
			Name:         folderName,
			FolderName:   categoryName,
			SourcePath:   path.Join(categoryName, folderName, "SKILL.md"),
			Description:  "Découvert via la structure du dépôt",
		}
	}

	if len(discovered) == 0 {
		return nil, fmt.Errorf("aucun bundle compatible détecté dans l'arborescence du dépôt")
	}

	results := make([]fs.CatalogEntry, 0, len(discovered))
	for _, entry := range discovered {
		results = append(results, entry)
	}

	sort.Slice(results, func(left, right int) bool {
		if results[left].FolderName == results[right].FolderName {
			return results[left].Name < results[right].Name
		}
		return results[left].FolderName < results[right].FolderName
	})

	return results, nil
}

func isRepositoryFileEntry(entryType string) bool {
	normalized := strings.ToLower(strings.TrimSpace(entryType))
	if normalized == "" {
		return true
	}

	return !strings.Contains(normalized, "tree") && !strings.Contains(normalized, "directory")
}

func (c *Client) ListRepositoryTree(ctx context.Context, profile fs.ProjectProfile) ([]repositoryTreeEntry, error) {
	if err := validateProfile(profile); err != nil {
		return nil, err
	}

	headers, err := resolveCredentialHeaders(ctx, profile)
	if err != nil {
		return nil, err
	}

	switch strings.ToLower(strings.TrimSpace(profile.Provider)) {
	case "loreskills-cloud":
		// Phase 1: bundle discovery is unsupported on the cloud provider.
		// Catalog is the single source of truth via /v1/catalog.
		_ = headers
		return []repositoryTreeEntry{}, nil
	case "github":
		return c.listGitHubTree(ctx, profile, headers)
	case "gitlab":
		return c.listGitLabTree(ctx, profile, headers)
	case "azure-devops":
		return c.listAzureDevOpsTree(ctx, profile, headers)
	case "bitbucket":
		return c.listBitbucketTree(ctx, profile, headers)
	default:
		return nil, fmt.Errorf("provider non pris en charge : %s", profile.Provider)
	}
}

func (c *Client) listGitHubTree(ctx context.Context, profile fs.ProjectProfile, headers credentialHeaders) ([]repositoryTreeEntry, error) {
	baseURL := strings.TrimRight(getEnvOrDefault("STH_GITHUB_API_BASE_URL", "https://api.github.com"), "/")
	requestURL := fmt.Sprintf("%s/repos/%s/git/trees/%s?recursive=1", baseURL, strings.TrimSpace(profile.RepositoryID), neturl.PathEscape(strings.TrimSpace(profile.Ref)))
	data, err := c.downloadURL(ctx, requestURL, headers.headers)
	if err != nil {
		return nil, decorateAccessError(err, headers.source, profile.Provider)
	}

	var response githubTreeResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("réponse GitHub tree invalide : %w", err)
	}

	entries := make([]repositoryTreeEntry, 0, len(response.Tree))
	for _, item := range response.Tree {
		entries = append(entries, repositoryTreeEntry{Path: normalizeDiscoveryPath(item.Path), Type: item.Type})
	}

	return entries, nil
}

func (c *Client) listGitLabTree(ctx context.Context, profile fs.ProjectProfile, headers credentialHeaders) ([]repositoryTreeEntry, error) {
	const maxPages = 100
	baseURL := strings.TrimRight(getEnvOrDefault("STH_GITLAB_API_BASE_URL", "https://gitlab.com/api/v4"), "/")
	entries := make([]repositoryTreeEntry, 0)

	for page := 1; page <= maxPages; page++ {
		requestURL := fmt.Sprintf("%s/projects/%s/repository/tree?ref=%s&recursive=true&per_page=100&page=%d",
			baseURL, neturl.PathEscape(strings.TrimSpace(profile.RepositoryID)),
			neturl.QueryEscape(strings.TrimSpace(profile.Ref)), page)
		data, err := c.downloadURL(ctx, requestURL, headers.headers)
		if err != nil {
			return nil, decorateAccessError(err, headers.source, profile.Provider)
		}

		var response []gitLabTreeEntry
		if err := json.Unmarshal(data, &response); err != nil {
			return nil, fmt.Errorf("réponse GitLab tree invalide : %w", err)
		}

		for _, item := range response {
			entries = append(entries, repositoryTreeEntry{Path: normalizeDiscoveryPath(item.Path), Type: item.Type})
		}

		if len(response) < 100 {
			break
		}
	}

	return entries, nil
}

func (c *Client) listAzureDevOpsTree(ctx context.Context, profile fs.ProjectProfile, headers credentialHeaders) ([]repositoryTreeEntry, error) {
	parts := strings.Split(strings.TrimSpace(profile.RepositoryID), "/")
	if len(parts) != 3 {
		return nil, fmt.Errorf("repository_id Azure DevOps invalide : %q", profile.RepositoryID)
	}

	baseURL := strings.TrimRight(getEnvOrDefault("STH_AZURE_DEVOPS_BASE_URL", "https://dev.azure.com"), "/")
	requestURL := fmt.Sprintf("%s/%s/%s/_apis/git/repositories/%s/items?scopePath=/&recursionLevel=Full&versionDescriptor.version=%s&includeContentMetadata=true&api-version=7.1", baseURL, neturl.PathEscape(parts[0]), neturl.PathEscape(parts[1]), neturl.PathEscape(parts[2]), neturl.QueryEscape(strings.TrimSpace(profile.Ref)))
	data, err := c.downloadURL(ctx, requestURL, headers.headers)
	if err != nil {
		return nil, decorateAccessError(err, headers.source, profile.Provider)
	}

	var response azureTreeResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("réponse Azure DevOps tree invalide : %w", err)
	}

	entries := make([]repositoryTreeEntry, 0, len(response.Value))
	for _, item := range response.Value {
		entries = append(entries, repositoryTreeEntry{Path: normalizeDiscoveryPath(strings.TrimPrefix(item.Path, "/")), Type: item.GitObjectType})
	}

	return entries, nil
}

func (c *Client) listBitbucketTree(ctx context.Context, profile fs.ProjectProfile, headers credentialHeaders) ([]repositoryTreeEntry, error) {
	baseURL := strings.TrimRight(getEnvOrDefault("STH_BITBUCKET_API_BASE_URL", "https://api.bitbucket.org/2.0"), "/")
	requestURL := fmt.Sprintf("%s/repositories/%s/src/%s/?pagelen=100", baseURL, strings.TrimSpace(profile.RepositoryID), neturl.PathEscape(strings.TrimSpace(profile.Ref)))
	entries := make([]repositoryTreeEntry, 0)

	for requestURL != "" {
		data, err := c.downloadURL(ctx, requestURL, headers.headers)
		if err != nil {
			return nil, decorateAccessError(err, headers.source, profile.Provider)
		}

		var response bitbucketTreeResponse
		if err := json.Unmarshal(data, &response); err != nil {
			return nil, fmt.Errorf("réponse Bitbucket tree invalide : %w", err)
		}

		for _, item := range response.Values {
			entries = append(entries, repositoryTreeEntry{Path: normalizeDiscoveryPath(item.Path), Type: item.Type})
		}

		requestURL = strings.TrimSpace(response.Next)
	}

	return entries, nil
}
