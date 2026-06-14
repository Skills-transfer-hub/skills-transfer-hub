package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	stdhttp "net/http"
	"net/url"
	"strings"

	"sth/internal/auth"
	sthhttp "sth/internal/http"
)

var ErrLibraryNotFound = errors.New("library: élément introuvable")

type LibraryItem struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	SourceTool  string `json:"source_tool"`
	SourceModel string `json:"source_model"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
}

type LibraryItemDetail struct {
	Item        LibraryItem
	DownloadURL string
}

type LibrarySaveInput struct {
	Kind        string `json:"kind"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	SourceTool  string `json:"source_tool,omitempty"`
	SourceModel string `json:"source_model,omitempty"`
}

func authorizedLibraryRequest(ctx context.Context, method, endpoint, orgID string, body io.Reader) (*stdhttp.Request, error) {
	key, err := auth.LoadAPIKey()
	if err != nil {
		return nil, err
	}
	request, err := stdhttp.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("library: préparation requête : %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+key)
	request.Header.Set("Accept", "application/json")
	if orgID != "" {
		request.Header.Set("X-STH-Org", orgID)
	}
	return request, nil
}

func SearchLibrary(ctx context.Context, httpClient *stdhttp.Client, orgID, query, kind string) ([]LibraryItem, error) {
	if httpClient == nil {
		httpClient = sharedClient
	}
	endpoint := sthhttp.CloudBaseURL() + "/v1/library"
	values := url.Values{}
	if trimmedQuery := strings.TrimSpace(query); trimmedQuery != "" {
		values.Set("q", trimmedQuery)
	}
	if trimmedKind := strings.TrimSpace(kind); trimmedKind != "" {
		values.Set("kind", trimmedKind)
	}
	if encoded := values.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	request, err := authorizedLibraryRequest(ctx, stdhttp.MethodGet, endpoint, orgID, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("library: requête vers %s échouée : %w", endpoint, err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case stdhttp.StatusOK:
	case stdhttp.StatusUnauthorized:
		return nil, ErrUnauthorized
	default:
		return nil, fmt.Errorf("library: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("library: lecture du corps : %w", err)
	}
	var decoded struct {
		Items []LibraryItem `json:"items"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("library: JSON invalide : %w", err)
	}
	return decoded.Items, nil
}

func GetLibraryItem(ctx context.Context, httpClient *stdhttp.Client, orgID, identifier string) (LibraryItemDetail, error) {
	if httpClient == nil {
		httpClient = sharedClient
	}
	endpoint := sthhttp.CloudBaseURL() + "/v1/library/" + url.PathEscape(identifier)
	request, err := authorizedLibraryRequest(ctx, stdhttp.MethodGet, endpoint, orgID, nil)
	if err != nil {
		return LibraryItemDetail{}, err
	}
	resp, err := httpClient.Do(request)
	if err != nil {
		return LibraryItemDetail{}, fmt.Errorf("library: requête vers %s échouée : %w", endpoint, err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case stdhttp.StatusOK:
	case stdhttp.StatusUnauthorized:
		return LibraryItemDetail{}, ErrUnauthorized
	case stdhttp.StatusNotFound:
		return LibraryItemDetail{}, ErrLibraryNotFound
	default:
		return LibraryItemDetail{}, fmt.Errorf("library: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return LibraryItemDetail{}, fmt.Errorf("library: lecture du corps : %w", err)
	}
	var decoded struct {
		Item        LibraryItem `json:"item"`
		DownloadURL string      `json:"download_url"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return LibraryItemDetail{}, fmt.Errorf("library: JSON invalide : %w", err)
	}
	return LibraryItemDetail{Item: decoded.Item, DownloadURL: decoded.DownloadURL}, nil
}

func SaveLibraryItem(ctx context.Context, httpClient *stdhttp.Client, orgID string, input LibrarySaveInput) (string, error) {
	if httpClient == nil {
		httpClient = sharedClient
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("library: encodage : %w", err)
	}
	endpoint := sthhttp.CloudBaseURL() + "/v1/library"
	request, err := authorizedLibraryRequest(ctx, stdhttp.MethodPost, endpoint, orgID, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("library: requête vers %s échouée : %w", endpoint, err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case stdhttp.StatusOK, stdhttp.StatusCreated:
	case stdhttp.StatusUnauthorized:
		return "", ErrUnauthorized
	default:
		return "", fmt.Errorf("library: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return "", fmt.Errorf("library: lecture du corps : %w", err)
	}
	var decoded struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return "", fmt.Errorf("library: JSON invalide : %w", err)
	}
	return decoded.ID, nil
}

func RecordLibraryEvent(ctx context.Context, httpClient *stdhttp.Client, orgID, identifier, eventType string) error {
	if httpClient == nil {
		httpClient = sharedClient
	}
	payload, err := json.Marshal(struct {
		EventType string `json:"event_type"`
	}{EventType: eventType})
	if err != nil {
		return fmt.Errorf("library: encodage : %w", err)
	}
	endpoint := sthhttp.CloudBaseURL() + "/v1/library/" + url.PathEscape(identifier) + "/events"
	request, err := authorizedLibraryRequest(ctx, stdhttp.MethodPost, endpoint, orgID, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("library: requête vers %s échouée : %w", endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("library: HTTP %d", resp.StatusCode)
	}
	return nil
}
