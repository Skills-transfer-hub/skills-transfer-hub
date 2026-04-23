// Package http centralizes LoreSkills network operations.
package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"loreskills/internal/fs"
	stdhttp "net/http"
	neturl "net/url"
	"os"
	"os/exec"
	"path"
	"sort"
	"strings"
	"time"
)

type Client struct {
	httpClient *stdhttp.Client
}

// AccessSource describes how LoreSkills authenticated the current request.
type AccessSource struct {
	Kind     string
	Hint     string
	Resolved bool
}

// ReleaseMetadata describes the build-injected stable release channel.
type ReleaseMetadata struct {
	CurrentVersion string
	ReleasesAPIURL string
	AssetName      string
	ChecksumName   string
}

// ReleaseAsset describes the stable release asset selected for self-update.
type ReleaseAsset struct {
	Version      string
	DownloadURL  string
	ChecksumURL  string
	ChecksumName string
	AssetName    string
}

type credentialHeaders struct {
	headers map[string]string
	source  AccessSource
}

type githubRelease struct {
	TagName    string               `json:"tag_name"`
	PreRelease bool                 `json:"prerelease"`
	Assets     []githubReleaseAsset `json:"assets"`
}

type githubReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// NewClient builds a network client for LoreSkills operations.
func NewClient(httpClient *stdhttp.Client) *Client {
	if httpClient == nil {
		httpClient = &stdhttp.Client{Timeout: 10 * time.Second}
	}

	return &Client{httpClient: httpClient}
}

// CheckProjectAccess verifies that the remote catalog is reachable.
func (c *Client) CheckProjectAccess(ctx context.Context, profile fs.ProjectProfile) error {
	_, err := c.fetchCatalogBytes(ctx, profile)
	return err
}

// ListCatalog returns the remote installable resources from SKILLS.md.
func (c *Client) ListCatalog(ctx context.Context, profile fs.ProjectProfile) ([]fs.CatalogEntry, error) {
	data, err := c.fetchCatalogBytes(ctx, profile)
	if err != nil {
		return nil, err
	}

	entries, err := parseCatalogEntries(string(data))
	if err != nil {
		return nil, err
	}

	return entries, nil
}

// DownloadCatalogEntry downloads the remote resource body for one catalog entry.
func (c *Client) DownloadCatalogEntry(ctx context.Context, profile fs.ProjectProfile, entry fs.CatalogEntry) ([]byte, error) {
	if err := validateProfile(profile); err != nil {
		return nil, err
	}
	if err := validateCatalogEntry(entry); err != nil {
		return nil, err
	}

	resourceURL, err := providerResourceURL(profile, entry.SourcePath)
	if err != nil {
		return nil, err
	}

	headers, err := resolveCredentialHeaders(ctx, profile)
	if err != nil {
		return nil, err
	}

	data, err := c.downloadURL(ctx, resourceURL, headers.headers)
	if err != nil {
		return nil, decorateAccessError(err, headers.source, profile.Provider)
	}

	return data, nil
}

// CheckForStableUpdate selects the latest stable release that matches the configured asset name.
func (c *Client) CheckForStableUpdate(ctx context.Context, metadata ReleaseMetadata) (*ReleaseAsset, error) {
	if strings.TrimSpace(metadata.ReleasesAPIURL) == "" {
		return nil, errors.New("métadonnées de release absentes pour la mise à jour automatique")
	}
	if strings.TrimSpace(metadata.AssetName) == "" {
		return nil, errors.New("nom d'artefact de release absent pour la plateforme courante")
	}
	if strings.TrimSpace(metadata.ChecksumName) == "" {
		metadata.ChecksumName = "SHA256SUMS"
	}

	data, err := c.downloadURL(ctx, strings.TrimSpace(metadata.ReleasesAPIURL), nil)
	if err != nil {
		return nil, err
	}

	var releases []githubRelease
	if err := json.Unmarshal(data, &releases); err != nil {
		return nil, fmt.Errorf("réponse des releases GitHub invalide : %w", err)
	}

	for _, release := range releases {
		if release.PreRelease {
			continue
		}
		if sameVersion(metadata.CurrentVersion, release.TagName) {
			return nil, nil
		}

		assetURL := ""
		checksumURL := ""
		for _, asset := range release.Assets {
			switch asset.Name {
			case metadata.AssetName:
				assetURL = asset.BrowserDownloadURL
			case metadata.ChecksumName:
				checksumURL = asset.BrowserDownloadURL
			}
		}

		if assetURL == "" || checksumURL == "" {
			continue
		}

		return &ReleaseAsset{
			Version:      strings.TrimSpace(release.TagName),
			DownloadURL:  assetURL,
			ChecksumURL:  checksumURL,
			ChecksumName: metadata.ChecksumName,
			AssetName:    metadata.AssetName,
		}, nil
	}

	return nil, nil
}

// DownloadURL downloads a raw URL without provider-specific resolution.
func (c *Client) DownloadURL(ctx context.Context, rawURL string, headers map[string]string) ([]byte, error) {
	return c.downloadURL(ctx, rawURL, headers)
}

func (c *Client) fetchCatalogBytes(ctx context.Context, profile fs.ProjectProfile) ([]byte, error) {
	if err := validateProfile(profile); err != nil {
		return nil, err
	}

	catalogURL, err := providerResourceURL(profile, profile.CatalogPath)
	if err != nil {
		return nil, err
	}

	headers, err := resolveCredentialHeaders(ctx, profile)
	if err != nil {
		return nil, err
	}

	data, err := c.downloadURL(ctx, catalogURL, headers.headers)
	if err != nil {
		return nil, decorateAccessError(err, headers.source, profile.Provider)
	}

	return data, nil
}

func (c *Client) downloadURL(ctx context.Context, rawURL string, headers map[string]string) ([]byte, error) {
	req, err := stdhttp.NewRequestWithContext(ctx, stdhttp.MethodGet, strings.TrimSpace(rawURL), nil)
	if err != nil {
		return nil, fmt.Errorf("préparation de la requête pour %s échouée : %w", rawURL, err)
	}
	for key, value := range headers {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		req.Header.Set(key, value)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("échec de la requête vers %s : %w", rawURL, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case stdhttp.StatusOK:
	case stdhttp.StatusUnauthorized, stdhttp.StatusForbidden:
		return nil, fmt.Errorf("accès refusé (HTTP %d) pour %s", resp.StatusCode, rawURL)
	case stdhttp.StatusNotFound:
		return nil, fmt.Errorf("ressource distante introuvable (HTTP 404) : %s", rawURL)
	default:
		return nil, fmt.Errorf("réponse inattendue du serveur : HTTP %d pour %s", resp.StatusCode, rawURL)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("lecture du corps de la réponse échouée : %w", err)
	}

	return data, nil
}

func providerResourceURL(profile fs.ProjectProfile, resourcePath string) (string, error) {
	rawPath := strings.TrimSpace(resourcePath)
	trimmedPath := path.Clean(rawPath)
	if rawPath == "" || trimmedPath == "." || trimmedPath == "/" {
		return "", fmt.Errorf("chemin distant invalide : %q", resourcePath)
	}
	if path.IsAbs(rawPath) || path.IsAbs(trimmedPath) || trimmedPath == ".." || strings.HasPrefix(trimmedPath, "../") {
		return "", fmt.Errorf("chemin distant invalide : %q", resourcePath)
	}

	switch strings.ToLower(strings.TrimSpace(profile.Provider)) {
	case "github":
		baseURL := strings.TrimRight(getEnvOrDefault("LORESKILLS_GITHUB_RAW_BASE_URL", "https://raw.githubusercontent.com"), "/")
		return fmt.Sprintf("%s/%s/%s/%s", baseURL, strings.TrimSpace(profile.RepositoryID), strings.TrimSpace(profile.Ref), strings.TrimPrefix(trimmedPath, "/")), nil
	case "gitlab":
		baseURL := strings.TrimRight(getEnvOrDefault("LORESKILLS_GITLAB_API_BASE_URL", "https://gitlab.com/api/v4"), "/")
		return fmt.Sprintf("%s/projects/%s/repository/files/%s/raw?ref=%s", baseURL, neturl.PathEscape(strings.TrimSpace(profile.RepositoryID)), neturl.PathEscape(strings.TrimPrefix(trimmedPath, "/")), neturl.QueryEscape(strings.TrimSpace(profile.Ref))), nil
	case "azure-devops":
		parts := strings.Split(strings.TrimSpace(profile.RepositoryID), "/")
		if len(parts) != 3 {
			return "", fmt.Errorf("repository_id Azure DevOps invalide : %q (attendu : organisation/projet/repo)", profile.RepositoryID)
		}
		baseURL := strings.TrimRight(getEnvOrDefault("LORESKILLS_AZURE_DEVOPS_BASE_URL", "https://dev.azure.com"), "/")
		return fmt.Sprintf("%s/%s/%s/_apis/git/repositories/%s/items?path=/%s&versionDescriptor.version=%s&includeContent=true&resolveLfs=true&api-version=7.1", baseURL, neturl.PathEscape(parts[0]), neturl.PathEscape(parts[1]), neturl.PathEscape(parts[2]), neturl.QueryEscape(strings.TrimPrefix(trimmedPath, "/")), neturl.QueryEscape(strings.TrimSpace(profile.Ref))), nil
	case "bitbucket":
		baseURL := strings.TrimRight(getEnvOrDefault("LORESKILLS_BITBUCKET_API_BASE_URL", "https://api.bitbucket.org/2.0"), "/")
		return fmt.Sprintf("%s/repositories/%s/src/%s/%s", baseURL, strings.TrimSpace(profile.RepositoryID), neturl.PathEscape(strings.TrimSpace(profile.Ref)), strings.TrimPrefix(trimmedPath, "/")), nil
	default:
		return "", fmt.Errorf("provider non pris en charge : %s", profile.Provider)
	}
}

func resolveCredentialHeaders(ctx context.Context, profile fs.ProjectProfile) (credentialHeaders, error) {
	// Session-only token entered during init (not persisted to disk).
	if profile.APIToken != "" {
		return providerAPITokenHeaders(profile.APIToken, profile.Provider), nil
	}

	if headers, ok := resolveEnvironmentHeaders(profile); ok {
		return headers, nil
	}

	host, credentialPath, err := providerCredentialTarget(profile)
	if err != nil {
		return credentialHeaders{}, err
	}

	username, password, err := gitCredentialFill(ctx, host, credentialPath)
	if err == nil && (strings.TrimSpace(username) != "" || strings.TrimSpace(password) != "") {
		return credentialHeaders{
			headers: map[string]string{
				"Authorization": basicAuthorization(username, password),
			},
			source: AccessSource{Kind: "git-credential", Hint: host, Resolved: true},
		}, nil
	}

	return credentialHeaders{source: AccessSource{Kind: "none", Hint: host, Resolved: false}}, nil
}

// providerAPITokenHeaders builds provider-appropriate credential headers for a session token.
func providerAPITokenHeaders(token, provider string) credentialHeaders {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "gitlab":
		return credentialHeaders{
			headers: map[string]string{"PRIVATE-TOKEN": token},
			source:  AccessSource{Kind: "session", Hint: "APIToken", Resolved: true},
		}
	case "azure-devops":
		return basicTokenHeaders(token, "APIToken")
	default: // github, bitbucket, unknown
		return bearerHeaders(token, "APIToken")
	}
}

func resolveEnvironmentHeaders(profile fs.ProjectProfile) (credentialHeaders, bool) {
	provider := strings.ToLower(strings.TrimSpace(profile.Provider))
	if token := strings.TrimSpace(os.Getenv("LORESKILLS_TOKEN")); token != "" {
		return bearerHeaders(token, "LORESKILLS_TOKEN"), true
	}

	switch provider {
	case "github":
		if token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); token != "" {
			return bearerHeaders(token, "GITHUB_TOKEN"), true
		}
	case "gitlab":
		if token := strings.TrimSpace(os.Getenv("GITLAB_TOKEN")); token != "" {
			return credentialHeaders{
				headers: map[string]string{"PRIVATE-TOKEN": token},
				source:  AccessSource{Kind: "env", Hint: "GITLAB_TOKEN", Resolved: true},
			}, true
		}
	case "azure-devops":
		if token := strings.TrimSpace(os.Getenv("AZURE_DEVOPS_EXT_PAT")); token != "" {
			return basicTokenHeaders(token, "AZURE_DEVOPS_EXT_PAT"), true
		}
		if token := strings.TrimSpace(os.Getenv("AZURE_DEVOPS_TOKEN")); token != "" {
			return basicTokenHeaders(token, "AZURE_DEVOPS_TOKEN"), true
		}
	case "bitbucket":
		if token := strings.TrimSpace(os.Getenv("BITBUCKET_TOKEN")); token != "" {
			return bearerHeaders(token, "BITBUCKET_TOKEN"), true
		}
		username := strings.TrimSpace(os.Getenv("BITBUCKET_USERNAME"))
		appPassword := strings.TrimSpace(os.Getenv("BITBUCKET_APP_PASSWORD"))
		if username != "" && appPassword != "" {
			return credentialHeaders{
				headers: map[string]string{"Authorization": basicAuthorization(username, appPassword)},
				source:  AccessSource{Kind: "env", Hint: "BITBUCKET_USERNAME/BITBUCKET_APP_PASSWORD", Resolved: true},
			}, true
		}
	}

	return credentialHeaders{}, false
}

func providerCredentialTarget(profile fs.ProjectProfile) (string, string, error) {
	switch strings.ToLower(strings.TrimSpace(profile.Provider)) {
	case "github":
		return "github.com", strings.TrimSpace(profile.RepositoryID), nil
	case "gitlab":
		return "gitlab.com", strings.TrimSpace(profile.RepositoryID), nil
	case "azure-devops":
		parts := strings.Split(strings.TrimSpace(profile.RepositoryID), "/")
		if len(parts) != 3 {
			return "", "", fmt.Errorf("repository_id Azure DevOps invalide : %q", profile.RepositoryID)
		}
		return "dev.azure.com", strings.Join(parts[:2], "/") + "/_git/" + parts[2], nil
	case "bitbucket":
		return "bitbucket.org", strings.TrimSpace(profile.RepositoryID), nil
	default:
		return "", "", fmt.Errorf("provider non pris en charge : %s", profile.Provider)
	}
}

func gitCredentialFill(ctx context.Context, host, credentialPath string) (string, string, error) {
	cmd := exec.CommandContext(ctx, "git", "credential", "fill")
	cmd.Stdin = strings.NewReader(fmt.Sprintf("protocol=https\nhost=%s\npath=%s\n\n", host, credentialPath))
	output, err := cmd.Output()
	if err != nil {
		return "", "", err
	}

	username := ""
	password := ""
	for _, line := range strings.Split(string(output), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch key {
		case "username":
			username = value
		case "password":
			password = value
		}
	}

	return username, password, nil
}

func parseCatalogEntries(markdown string) ([]fs.CatalogEntry, error) {
	lines := strings.Split(markdown, "\n")
	for index := 0; index < len(lines)-1; index++ {
		headers := parseTableRow(lines[index])
		if len(headers) == 0 || !isTableSeparator(lines[index+1]) {
			continue
		}

		headerIndex := map[string]int{}
		for columnIndex, header := range headers {
			headerIndex[normalizeTableHeader(header)] = columnIndex
		}

		typeIndex, okType := headerIndex["type"]
		nameIndex, okName := headerIndex["name"]
		folderIndex, okFolder := headerIndex["folder"]
		sourceIndex, okSource := headerIndex["source"]
		if !okSource {
			sourceIndex, okSource = headerIndex["source_path"]
		}
		if !okSource {
			sourceIndex, okSource = headerIndex["sourcepath"]
		}
		descriptionIndex, hasDescription := headerIndex["description"]

		if !okType || !okName || !okFolder || !okSource {
			continue
		}

		entries := make([]fs.CatalogEntry, 0)
		for rowIndex := index + 2; rowIndex < len(lines); rowIndex++ {
			if strings.TrimSpace(lines[rowIndex]) == "" {
				break
			}
			values := parseTableRow(lines[rowIndex])
			if len(values) == 0 {
				break
			}

			resourceType := tableValue(values, typeIndex)
			name := tableValue(values, nameIndex)
			folderName := tableValue(values, folderIndex)
			sourcePath := tableValue(values, sourceIndex)
			if resourceType == "" || name == "" || folderName == "" || sourcePath == "" {
				return nil, fmt.Errorf("entrée catalogue invalide dans SKILLS.md : type, name, folder et source sont requis")
			}

			entry := fs.CatalogEntry{
				CatalogID:    fs.CatalogID(resourceType, folderName, name),
				ResourceType: strings.ToLower(strings.TrimSpace(resourceType)),
				Name:         strings.TrimSpace(name),
				FolderName:   strings.TrimSpace(folderName),
				SourcePath:   strings.TrimSpace(sourcePath),
			}
			if hasDescription {
				entry.Description = strings.TrimSpace(tableValue(values, descriptionIndex))
			}
			entries = append(entries, entry)
		}

		if len(entries) == 0 {
			return nil, errors.New("aucune ressource installable trouvée dans SKILLS.md")
		}

		if err := validateCatalogUniqueness(entries); err != nil {
			return nil, err
		}

		sort.Slice(entries, func(left, right int) bool {
			if entries[left].FolderName == entries[right].FolderName {
				return entries[left].Name < entries[right].Name
			}
			return entries[left].FolderName < entries[right].FolderName
		})

		return entries, nil
	}

	return nil, errors.New("table catalogue introuvable dans SKILLS.md")
}

func validateCatalogUniqueness(entries []fs.CatalogEntry) error {
	seen := map[string]struct{}{}
	for _, entry := range entries {
		if err := validateCatalogEntry(entry); err != nil {
			return err
		}
		if _, ok := seen[entry.CatalogID]; ok {
			return fmt.Errorf("entrée catalogue dupliquée : %s", entry.CatalogID)
		}
		seen[entry.CatalogID] = struct{}{}
	}

	return nil
}

func validateProfile(profile fs.ProjectProfile) error {
	if strings.TrimSpace(profile.Provider) == "" {
		return errors.New("provider doit être renseigné")
	}
	if strings.TrimSpace(profile.RepositoryID) == "" {
		return errors.New("repository_id doit être renseigné")
	}
	if strings.TrimSpace(profile.Ref) == "" {
		return errors.New("ref doit être renseigné")
	}
	if strings.TrimSpace(profile.CatalogPath) == "" {
		return errors.New("catalog_path doit être renseigné")
	}

	return nil
}

func validateCatalogEntry(entry fs.CatalogEntry) error {
	if strings.TrimSpace(entry.Name) == "" {
		return errors.New("name doit être renseigné")
	}
	if strings.TrimSpace(entry.FolderName) == "" {
		return errors.New("folder_name doit être renseigné")
	}
	if strings.TrimSpace(entry.SourcePath) == "" {
		return errors.New("source_path doit être renseigné")
	}

	return nil
}

func bearerHeaders(token, hint string) credentialHeaders {
	return credentialHeaders{
		headers: map[string]string{"Authorization": "Bearer " + token},
		source:  AccessSource{Kind: "env", Hint: hint, Resolved: true},
	}
}

func basicTokenHeaders(token, hint string) credentialHeaders {
	return credentialHeaders{
		headers: map[string]string{"Authorization": basicAuthorization("", token)},
		source:  AccessSource{Kind: "env", Hint: hint, Resolved: true},
	}
}

func basicAuthorization(username, password string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password))
}

func decorateAccessError(err error, source AccessSource, provider string) error {
	if source.Resolved {
		return err
	}

	return fmt.Errorf("%w — authentifiez-vous via les variables d'environnement du provider ou vos identifiants Git pour %s", err, provider)
}

func sameVersion(current, candidate string) bool {
	trimmedCurrent := strings.TrimPrefix(strings.TrimSpace(current), "v")
	trimmedCandidate := strings.TrimPrefix(strings.TrimSpace(candidate), "v")
	if trimmedCurrent == "" || trimmedCandidate == "" {
		return false
	}

	return trimmedCurrent == trimmedCandidate
}

func parseTableRow(line string) []string {
	trimmed := strings.TrimSpace(line)
	if !strings.Contains(trimmed, "|") {
		return nil
	}

	trimmed = strings.Trim(trimmed, "|")
	parts := strings.Split(trimmed, "|")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		values = append(values, strings.TrimSpace(part))
	}

	return values
}

func isTableSeparator(line string) bool {
	trimmed := strings.TrimSpace(strings.Trim(line, "|"))
	if trimmed == "" {
		return false
	}

	for _, part := range strings.Split(trimmed, "|") {
		candidate := strings.TrimSpace(part)
		if candidate == "" {
			return false
		}
		if strings.Trim(candidate, "-:") != "" {
			return false
		}
	}

	return true
}

func normalizeTableHeader(header string) string {
	replacer := strings.NewReplacer(" ", "", "-", "", "_", "")
	normalized := replacer.Replace(strings.ToLower(strings.TrimSpace(header)))
	switch normalized {
	case "source", "sourcepath", "sourcefile":
		return "source"
	case "folder", "foldername":
		return "folder"
	default:
		return normalized
	}
}

func tableValue(values []string, index int) string {
	if index < 0 || index >= len(values) {
		return ""
	}

	return strings.TrimSpace(values[index])
}

func getEnvOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}

	return fallback
}
