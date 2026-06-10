package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	stdhttp "net/http"
	"strings"
	"time"

	"sth/internal/fs"
)

func nowRFC3339() string { return time.Now().UTC().Format(time.RFC3339) }

// defaultCloudBaseURL is the production base URL for the STH Cloud API,
// deployed as the Supabase Edge Function `cli` in the sth-back project.
// STH_CLOUD_BASE_URL overrides it (dev/staging backends, integration tests).
const defaultCloudBaseURL = "https://jcdadtdkgnwobejkmpgv.supabase.co/functions/v1/cli"

// CloudBaseURL returns the STH Cloud base URL with the env override applied
// and any trailing slash stripped. Exported so cmd/* can reuse it.
func CloudBaseURL() string {
	return strings.TrimRight(getEnvOrDefault("STH_CLOUD_BASE_URL", defaultCloudBaseURL), "/")
}

// AccessCheckResult is what `sth login` gets back after validating a key.
type AccessCheckResult struct {
	OK        bool           `json:"ok"`
	Org       AccessCheckOrg `json:"org"`
	Plan      string         `json:"plan"`
	ExpiresAt string         `json:"expires_at"`
}

// AccessCheckOrg is the org block embedded in AccessCheckResult.
type AccessCheckOrg struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// AccessCheck validates an API key against POST /v1/access-check. Used by
// `sth login` before persisting the key to the keychain — the key isn't yet
// stored, so we set Authorization manually instead of going through
// resolveCredentialHeaders. Reuses the shared httpClient for connection pooling.
func (c *Client) AccessCheck(ctx context.Context, key string) (AccessCheckResult, error) {
	req, err := stdhttp.NewRequestWithContext(ctx, stdhttp.MethodPost, CloudBaseURL()+"/v1/access-check", nil)
	if err != nil {
		return AccessCheckResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.emitHTTP(stdhttp.MethodPost, CloudBaseURL()+"/v1/access-check", 0)
		return AccessCheckResult{}, err
	}
	defer resp.Body.Close()
	c.emitHTTP(stdhttp.MethodPost, CloudBaseURL()+"/v1/access-check", resp.StatusCode)

	if resp.StatusCode == stdhttp.StatusUnauthorized {
		return AccessCheckResult{}, fmt.Errorf("clé refusée (HTTP 401)")
	}
	if resp.StatusCode != stdhttp.StatusOK {
		return AccessCheckResult{}, fmt.Errorf("réponse inattendue (HTTP %d)", resp.StatusCode)
	}

	var out AccessCheckResult
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return AccessCheckResult{}, fmt.Errorf("réponse JSON invalide : %w", err)
	}
	return out, nil
}

// STH Cloud-specific request paths and decoding live here so the rest of
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

type usageReportRecord struct {
	Ts         string `json:"ts"`
	CLIVersion string `json:"cli_version"`
	// CommandName carries only the top-level command (e.g. "init", "install").
	// Never the args, never a path. Omitted when empty so older back versions
	// that don't know the field still accept the payload.
	CommandName string `json:"command_name,omitempty"`
}

type usageReportPayload struct {
	Commands []usageReportRecord `json:"commands"`
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
func (c *Client) reportCloudUsage(ctx context.Context, profile fs.ProjectProfile, cliVersion, commandName string) error {
	if !strings.EqualFold(strings.TrimSpace(profile.Provider), "sth-cloud") {
		return nil
	}
	headers, err := resolveCredentialHeaders(ctx, profile)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(usageReportPayload{
		Commands: []usageReportRecord{{
			Ts:          nowRFC3339(),
			CLIVersion:  cliVersion,
			CommandName: strings.TrimSpace(commandName),
		}},
	})
	if err != nil {
		return fmt.Errorf("encodage usage : %w", err)
	}
	req, err := stdhttp.NewRequestWithContext(ctx, stdhttp.MethodPost, CloudBaseURL()+"/v1/usage", strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	for k, v := range headers.headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	usageURL := CloudBaseURL() + "/v1/usage"
	if err != nil {
		c.emitHTTP(stdhttp.MethodPost, usageURL, 0)
		return err
	}
	defer resp.Body.Close()
	c.emitHTTP(stdhttp.MethodPost, usageURL, resp.StatusCode)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("usage ingest failed: HTTP %d", resp.StatusCode)
	}
	return nil
}

// ReportCloudUsage is the public entry-point used by cmd to fire-and-forget the
// usage record after a successful CLI command.
func (c *Client) ReportCloudUsage(ctx context.Context, profile fs.ProjectProfile, cliVersion, commandName string) error {
	return c.reportCloudUsage(ctx, profile, cliVersion, commandName)
}

// ReportCloudUsageAnonymous POSTs the same usage payload as ReportCloudUsage
// but to /v1/usage/anon, without any Authorization header. Used when the
// user opted into telemetry but did NOT log in — they contribute to
// product-level stats only, and the backend never associates the event
// with a customer identity.
//
// The payload is identical (ts, cli_version, command_name) — no machine_id,
// no fingerprint. The backend may still see the IP + User-Agent at the
// transport layer; documenting how those are handled is a backend concern.
//
// Errors are returned but the caller (cmd/root.go reportCloudUsageBestEffort)
// swallows them — best-effort. HTTP 404 / 501 is treated as success-equivalent
// here (returns nil) so a CLI shipping ahead of the backend rollout does not
// surface noise.
func (c *Client) ReportCloudUsageAnonymous(ctx context.Context, cliVersion, commandName string) error {
	payload, err := json.Marshal(usageReportPayload{
		Commands: []usageReportRecord{{
			Ts:          nowRFC3339(),
			CLIVersion:  cliVersion,
			CommandName: strings.TrimSpace(commandName),
		}},
	})
	if err != nil {
		return fmt.Errorf("encodage usage anonyme : %w", err)
	}

	anonURL := CloudBaseURL() + "/v1/usage/anon"
	req, err := stdhttp.NewRequestWithContext(ctx, stdhttp.MethodPost, anonURL, strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.emitHTTP(stdhttp.MethodPost, anonURL, 0)
		return err
	}
	defer resp.Body.Close()
	c.emitHTTP(stdhttp.MethodPost, anonURL, resp.StatusCode)

	switch {
	case resp.StatusCode < 400:
		return nil
	case resp.StatusCode == stdhttp.StatusNotFound, resp.StatusCode == stdhttp.StatusNotImplemented:
		// Backend not yet rolled out — degrade silently so CLI versions that
		// ship ahead of the endpoint do not generate user-visible noise.
		return nil
	default:
		return fmt.Errorf("anonymous usage ingest failed: HTTP %d", resp.StatusCode)
	}
}

// UsageMineRecord mirrors the payload returned by GET /v1/usage/mine. The
// CLI's `sth telemetry output` command renders these to the terminal.
type UsageMineRecord struct {
	Ts          string `json:"ts"`
	CLIVersion  string `json:"cli_version"`
	CommandName string `json:"command_name"`
}

type usageMineResponse struct {
	Records []UsageMineRecord `json:"records"`
}

// FetchOwnUsage calls GET /v1/usage/mine with the active sth-cloud credentials
// and returns the records the server has on file for the caller's key. Used
// by `sth telemetry output` for transparency — the user can see exactly what
// the back has stored about their CLI activity.
func (c *Client) FetchOwnUsage(ctx context.Context, profile fs.ProjectProfile, limit int) ([]UsageMineRecord, error) {
	if !strings.EqualFold(strings.TrimSpace(profile.Provider), "sth-cloud") {
		return nil, fmt.Errorf("`sth telemetry output` nécessite un provider sth-cloud")
	}
	headers, err := resolveCredentialHeaders(ctx, profile)
	if err != nil {
		return nil, err
	}
	url := CloudBaseURL() + "/v1/usage/mine"
	if limit > 0 {
		url += fmt.Sprintf("?limit=%d", limit)
	}
	req, err := stdhttp.NewRequestWithContext(ctx, stdhttp.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers.headers {
		req.Header.Set(k, v)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.emitHTTP(stdhttp.MethodGet, url, 0)
		return nil, err
	}
	defer resp.Body.Close()
	c.emitHTTP(stdhttp.MethodGet, url, resp.StatusCode)
	if resp.StatusCode == stdhttp.StatusUnauthorized {
		return nil, fmt.Errorf("clé refusée (HTTP 401) — vérifie `sth login`")
	}
	if resp.StatusCode != stdhttp.StatusOK {
		return nil, fmt.Errorf("réponse inattendue (HTTP %d)", resp.StatusCode)
	}
	var out usageMineResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return nil, fmt.Errorf("réponse usage/mine invalide : %w", err)
	}
	return out.Records, nil
}

type AlphaAccessResult struct {
	Status    string `json:"status"`
	Plan      string `json:"plan,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

func (c *Client) RequestAlphaAccess(ctx context.Context, profile fs.ProjectProfile) (AlphaAccessResult, error) {
	headers, err := resolveCredentialHeaders(ctx, profile)
	if err != nil {
		return AlphaAccessResult{}, err
	}
	url := CloudBaseURL() + "/v1/access-request"
	req, err := stdhttp.NewRequestWithContext(ctx, stdhttp.MethodPost, url, nil)
	if err != nil {
		return AlphaAccessResult{}, err
	}
	for name, value := range headers.headers {
		req.Header.Set(name, value)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.emitHTTP(stdhttp.MethodPost, url, 0)
		return AlphaAccessResult{}, err
	}
	defer resp.Body.Close()
	c.emitHTTP(stdhttp.MethodPost, url, resp.StatusCode)
	if resp.StatusCode == stdhttp.StatusUnauthorized {
		return AlphaAccessResult{}, fmt.Errorf("clé refusée (HTTP 401) — vérifie `sth login`")
	}
	if resp.StatusCode != stdhttp.StatusOK {
		return AlphaAccessResult{}, fmt.Errorf("réponse inattendue (HTTP %d)", resp.StatusCode)
	}
	var out AlphaAccessResult
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return AlphaAccessResult{}, fmt.Errorf("réponse access-request invalide : %w", err)
	}
	return out, nil
}
