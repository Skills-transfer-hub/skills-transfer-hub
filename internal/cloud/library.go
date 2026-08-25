package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	stdhttp "net/http"
	"net/url"

	"sth/internal/auth"
	sthhttp "sth/internal/http"
)

const mcpClientHeader = "X-STH-Client"
const mcpClientValue = "mcp"

type LibraryMatch struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Title       string `json:"title"`
	SourceTool  string `json:"source_tool"`
	SourceModel string `json:"source_model"`
}

type LibraryItem struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	SourceTool  string `json:"source_tool"`
	SourceModel string `json:"source_model"`
}

type LibrarySaveInput struct {
	Kind        string `json:"kind"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	SourceTool  string `json:"source_tool,omitempty"`
	SourceModel string `json:"source_model,omitempty"`
}

func authorizedRequest(
	ctx context.Context,
	method string,
	requestURL string,
	body io.Reader,
) (*stdhttp.Request, error) {
	key, err := auth.LoadAPIKey()
	if err != nil {
		return nil, err
	}
	request, err := stdhttp.NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		return nil, fmt.Errorf("library: failed to create request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+key)
	request.Header.Set("Accept", "application/json")
	request.Header.Set(mcpClientHeader, mcpClientValue)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	return request, nil
}

func libraryError(operation string, resp *stdhttp.Response) error {
	if resp.StatusCode == stdhttp.StatusUnauthorized {
		return ErrUnauthorized
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	var decoded struct {
		Error  string `json:"error"`
		Detail string `json:"detail"`
	}
	_ = json.Unmarshal(body, &decoded)
	if decoded.Detail != "" {
		return fmt.Errorf("%s: %s", operation, decoded.Detail)
	}
	if decoded.Error != "" {
		return fmt.Errorf("%s: %s", operation, decoded.Error)
	}
	return fmt.Errorf("%s: HTTP %d", operation, resp.StatusCode)
}

func SearchLibrary(
	ctx context.Context,
	httpClient *stdhttp.Client,
	query string,
	kind string,
) ([]LibraryMatch, error) {
	if httpClient == nil {
		httpClient = sharedClient
	}
	params := url.Values{}
	if query != "" {
		params.Set("q", query)
	}
	if kind != "" {
		params.Set("kind", kind)
	}
	requestURL := sthhttp.CloudBaseURL() + "/v1/library"
	if encoded := params.Encode(); encoded != "" {
		requestURL += "?" + encoded
	}
	request, err := authorizedRequest(ctx, stdhttp.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("library: request to %s failed: %w", requestURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != stdhttp.StatusOK {
		return nil, libraryError("library search", resp)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("library: failed to read response body: %w", err)
	}
	var decoded struct {
		Items []LibraryMatch `json:"items"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("library: invalid JSON: %w", err)
	}
	return decoded.Items, nil
}

func GetLibrary(
	ctx context.Context,
	httpClient *stdhttp.Client,
	itemID string,
) (LibraryItem, error) {
	if httpClient == nil {
		httpClient = sharedClient
	}
	requestURL := sthhttp.CloudBaseURL() + "/v1/library/" + url.PathEscape(itemID)
	request, err := authorizedRequest(ctx, stdhttp.MethodGet, requestURL, nil)
	if err != nil {
		return LibraryItem{}, err
	}
	resp, err := httpClient.Do(request)
	if err != nil {
		return LibraryItem{}, fmt.Errorf("library: request to %s failed: %w", requestURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != stdhttp.StatusOK {
		return LibraryItem{}, libraryError("opening an item", resp)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return LibraryItem{}, fmt.Errorf("library: failed to read response body: %w", err)
	}
	var decoded struct {
		Item LibraryItem `json:"item"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return LibraryItem{}, fmt.Errorf("library: invalid JSON: %w", err)
	}
	return decoded.Item, nil
}

func RecordReuse(ctx context.Context, httpClient *stdhttp.Client, itemID string) error {
	if httpClient == nil {
		httpClient = sharedClient
	}
	requestURL := sthhttp.CloudBaseURL() + "/v1/library/" + url.PathEscape(itemID) + "/events"
	payload, err := json.Marshal(map[string]string{"event_type": "reuse"})
	if err != nil {
		return err
	}
	request, err := authorizedRequest(ctx, stdhttp.MethodPost, requestURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	resp, err := httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("library: request to %s failed: %w", requestURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != stdhttp.StatusAccepted && resp.StatusCode != stdhttp.StatusOK {
		return libraryError("recording reuse", resp)
	}
	return nil
}

func SaveLibrary(
	ctx context.Context,
	httpClient *stdhttp.Client,
	input LibrarySaveInput,
) (string, error) {
	if httpClient == nil {
		httpClient = sharedClient
	}
	requestURL := sthhttp.CloudBaseURL() + "/v1/library"
	payload, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	request, err := authorizedRequest(ctx, stdhttp.MethodPost, requestURL, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	resp, err := httpClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("library: request to %s failed: %w", requestURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != stdhttp.StatusCreated && resp.StatusCode != stdhttp.StatusOK {
		return "", libraryError("saving to the library", resp)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return "", fmt.Errorf("library: failed to read response body: %w", err)
	}
	var decoded struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return "", fmt.Errorf("library: invalid JSON: %w", err)
	}
	return decoded.ID, nil
}
