package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	stdhttp "net/http"
	"strconv"

	"sth/internal/auth"
	"sth/internal/fs"
	sthhttp "sth/internal/http"
)

var (
	ErrForbiddenOrg    = errors.New("org-config: organisation non autorisée")
	ErrForbiddenRole   = errors.New("org-config: rôle owner/admin requis")
	ErrNotEntitled     = errors.New("org-config: capability fleet_sync requise")
	ErrVersionConflict = errors.New("org-config: version périmée")
)

type OrgConfig struct {
	Config        *fs.ProjectConfigV2
	ConfigVersion int
}

func GetOrgConfig(ctx context.Context, httpClient *stdhttp.Client, orgID string) (OrgConfig, error) {
	if httpClient == nil {
		httpClient = sharedClient
	}
	key, err := auth.LoadAPIKey()
	if err != nil {
		return OrgConfig{}, err
	}
	url := sthhttp.CloudBaseURL() + "/v1/org-config"
	request, err := stdhttp.NewRequestWithContext(ctx, stdhttp.MethodGet, url, nil)
	if err != nil {
		return OrgConfig{}, fmt.Errorf("org-config: préparation requête : %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+key)
	request.Header.Set("Accept", "application/json")
	if orgID != "" {
		request.Header.Set("X-STH-Org", orgID)
	}

	resp, err := httpClient.Do(request)
	if err != nil {
		return OrgConfig{}, fmt.Errorf("org-config: requête vers %s échouée : %w", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return OrgConfig{}, fmt.Errorf("org-config: lecture du corps : %w", err)
	}

	switch resp.StatusCode {
	case stdhttp.StatusOK:
	case stdhttp.StatusUnauthorized:
		return OrgConfig{}, ErrUnauthorized
	case stdhttp.StatusForbidden:
		return OrgConfig{}, classifyForbidden(body)
	default:
		return OrgConfig{}, fmt.Errorf("org-config: HTTP %d", resp.StatusCode)
	}

	var decoded struct {
		Config        *fs.ProjectConfigV2 `json:"config"`
		ConfigVersion int                 `json:"config_version"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return OrgConfig{}, fmt.Errorf("org-config: JSON invalide : %w", err)
	}
	return OrgConfig{Config: decoded.Config, ConfigVersion: decoded.ConfigVersion}, nil
}

func PutOrgConfig(ctx context.Context, httpClient *stdhttp.Client, orgID string, config fs.ProjectConfigV2, baseVersion int) (int, error) {
	if httpClient == nil {
		httpClient = sharedClient
	}
	key, err := auth.LoadAPIKey()
	if err != nil {
		return 0, err
	}
	payload, err := json.Marshal(struct {
		Config      fs.ProjectConfigV2 `json:"config"`
		BaseVersion int                `json:"base_version"`
	}{Config: config, BaseVersion: baseVersion})
	if err != nil {
		return 0, fmt.Errorf("org-config: encodage : %w", err)
	}
	url := sthhttp.CloudBaseURL() + "/v1/org-config"
	request, err := stdhttp.NewRequestWithContext(ctx, stdhttp.MethodPut, url, bytes.NewReader(payload))
	if err != nil {
		return 0, fmt.Errorf("org-config: préparation requête : %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+key)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("If-Match", strconv.Itoa(baseVersion))
	if orgID != "" {
		request.Header.Set("X-STH-Org", orgID)
	}

	resp, err := httpClient.Do(request)
	if err != nil {
		return 0, fmt.Errorf("org-config: requête vers %s échouée : %w", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return 0, fmt.Errorf("org-config: lecture du corps : %w", err)
	}

	switch resp.StatusCode {
	case stdhttp.StatusOK:
	case stdhttp.StatusUnauthorized:
		return 0, ErrUnauthorized
	case stdhttp.StatusForbidden:
		return 0, classifyForbidden(body)
	case stdhttp.StatusConflict:
		return 0, ErrVersionConflict
	default:
		return 0, fmt.Errorf("org-config: HTTP %d", resp.StatusCode)
	}

	var decoded struct {
		ConfigVersion int `json:"config_version"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return 0, fmt.Errorf("org-config: JSON invalide : %w", err)
	}
	return decoded.ConfigVersion, nil
}

func classifyForbidden(body []byte) error {
	var decoded struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(body, &decoded)
	switch decoded.Error {
	case "forbidden_org":
		return ErrForbiddenOrg
	case "forbidden_role":
		return ErrForbiddenRole
	case "not_entitled":
		return ErrNotEntitled
	default:
		return errors.New("org-config: accès refusé (HTTP 403)")
	}
}
