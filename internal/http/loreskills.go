package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	stdhttp "net/http"
	"strings"
	"time"

	"sth/internal/fs"
)

func nowRFC3339() string { return time.Now().UTC().Format(time.RFC3339) }

// LoreSkills Cloud-specific request paths and decoding live here so the rest of
// client.go stays focused on Git providers.

type cloudCatalogResponse struct {
	Entries []cloudCatalogEntry `json:"entries"`
}

type cloudCatalogEntry struct {
	CatalogID    string `json:"catalog_id"`
	ResourceType string `json:"resource_type"`
	Name         string `json:"name"`
	FolderName   string `json:"folder_name"`
	SourcePath   string `json:"source_path"`
	Description  string `json:"description,omitempty"`
}

type cloudSkillResponse struct {
	DownloadURL string `json:"download_url"`
	SHA256      string `json:"sha256"`
	Version     string `json:"version"`
}

// listCloudCatalog calls GET /v1/catalog and maps the response onto
// []fs.CatalogEntry — the same shape the rest of the CLI expects.
func (c *Client) listCloudCatalog(ctx context.Context, profile fs.ProjectProfile) ([]fs.CatalogEntry, error) {
	headers, err := resolveCredentialHeaders(ctx, profile)
	if err != nil {
		return nil, err
	}
	url, err := providerResourceURL(profile, "__catalog__")
	if err != nil {
		return nil, err
	}
	body, err := c.downloadURL(ctx, url, headers.headers)
	if err != nil {
		return nil, decorateAccessError(err, headers.source, profile.Provider)
	}

	var resp cloudCatalogResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("réponse catalog invalide : %w", err)
	}

	entries := make([]fs.CatalogEntry, 0, len(resp.Entries))
	for _, e := range resp.Entries {
		entry := fs.CatalogEntry{
			CatalogID:    e.CatalogID,
			ResourceType: strings.ToLower(strings.TrimSpace(e.ResourceType)),
			Name:         strings.TrimSpace(e.Name),
			FolderName:   strings.TrimSpace(e.FolderName),
			SourcePath:   strings.TrimSpace(e.SourcePath),
			Description:  strings.TrimSpace(e.Description),
		}
		// Recompute CatalogID locally to keep parity with the Git-provider path
		// in case the server sends a slightly different normalization.
		if entry.CatalogID == "" {
			entry.CatalogID = fs.CatalogID(entry.ResourceType, entry.FolderName, entry.Name)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// downloadCloudResource calls GET /v1/skills/<catalog_id>, follows the signed
// URL it returns, and returns the raw bytes of the skill content.
func (c *Client) downloadCloudResource(ctx context.Context, profile fs.ProjectProfile, entry fs.CatalogEntry) ([]byte, error) {
	headers, err := resolveCredentialHeaders(ctx, profile)
	if err != nil {
		return nil, err
	}
	url, err := providerResourceURL(profile, entry.CatalogID)
	if err != nil {
		return nil, err
	}

	body, err := c.downloadURL(ctx, url, headers.headers)
	if err != nil {
		return nil, decorateAccessError(err, headers.source, profile.Provider)
	}

	var resp cloudSkillResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("réponse skill invalide : %w", err)
	}
	if strings.TrimSpace(resp.DownloadURL) == "" {
		return nil, errors.New("download_url manquant dans la réponse")
	}

	// Fetch the signed-URL contents without auth headers (signed URL embeds
	// its own auth — passing Bearer would be ignored at best, rejected at worst).
	return c.downloadURL(ctx, resp.DownloadURL, nil)
}

// reportCloudUsage POSTs /v1/usage in fire-and-forget mode. Errors are returned
// but callers typically ignore them (best-effort telemetry).
func (c *Client) reportCloudUsage(ctx context.Context, profile fs.ProjectProfile, cliVersion string) error {
	if !strings.EqualFold(strings.TrimSpace(profile.Provider), "loreskills-cloud") {
		return nil
	}
	headers, err := resolveCredentialHeaders(ctx, profile)
	if err != nil {
		return err
	}
	baseURL := strings.TrimRight(getEnvOrDefault("STH_LORESKILLS_BASE_URL", "https://api.loreskills.dev/cli"), "/")
	payload, _ := json.Marshal(map[string]interface{}{
		"commands": []map[string]string{
			{"ts": nowRFC3339(), "cli_version": cliVersion},
		},
	})
	req, err := stdhttp.NewRequestWithContext(ctx, stdhttp.MethodPost, baseURL+"/v1/usage", strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	for k, v := range headers.headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("usage ingest failed: HTTP %d", resp.StatusCode)
	}
	return nil
}

// ReportCloudUsage is the public entry-point used by cmd to fire-and-forget the
// usage record after a successful CLI command.
func (c *Client) ReportCloudUsage(ctx context.Context, profile fs.ProjectProfile, cliVersion string) error {
	return c.reportCloudUsage(ctx, profile, cliVersion)
}
