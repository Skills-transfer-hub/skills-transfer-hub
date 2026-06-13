package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	stdhttp "net/http"

	"sth/internal/auth"
	sthhttp "sth/internal/http"
)

type Org struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Role   string `json:"role"`
	Plan   string `json:"plan"`
	Active bool   `json:"active"`
}

func ListOrgs(ctx context.Context, httpClient *stdhttp.Client) ([]Org, error) {
	if httpClient == nil {
		httpClient = sharedClient
	}
	key, err := auth.LoadAPIKey()
	if err != nil {
		return nil, err
	}
	url := sthhttp.CloudBaseURL() + "/v1/orgs"
	request, err := stdhttp.NewRequestWithContext(ctx, stdhttp.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("orgs: préparation requête : %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+key)
	request.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("orgs: requête vers %s échouée : %w", url, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case stdhttp.StatusOK:
	case stdhttp.StatusUnauthorized:
		return nil, ErrUnauthorized
	default:
		return nil, fmt.Errorf("orgs: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("orgs: lecture du corps : %w", err)
	}
	var decoded struct {
		Orgs []Org `json:"orgs"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("orgs: JSON invalide : %w", err)
	}
	return decoded.Orgs, nil
}
