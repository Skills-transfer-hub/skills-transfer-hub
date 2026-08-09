package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	stdhttp "net/http"
	"net/url"
	"os"
	"strings"

	"sth/internal/auth"
	"sth/internal/cloud/sthv2"
	sthhttp "sth/internal/http"
)

// EnvCloudAccessToken carries the short-lived desktop OAuth token into the
// privileged bridge child. It deliberately takes precedence over the legacy
// API key and is never persisted by the Go process.
const EnvCloudAccessToken = "STH_CLOUD_ACCESS_TOKEN"

type generatedV2Transport struct {
	httpClient *stdhttp.Client
}

func newGeneratedV2Client(httpClient *stdhttp.Client) sthv2.Client {
	if httpClient == nil {
		httpClient = sharedClient
	}
	return sthv2.Client{Transport: generatedV2Transport{httpClient: httpClient}}
}

func (transport generatedV2Transport) Do(
	ctx context.Context,
	input sthv2.Request,
	output any,
) error {
	baseURL, err := validatedV2CloudBaseURL()
	if err != nil {
		return err
	}
	requestURL := baseURL + input.Path
	if encoded := input.Query.Encode(); encoded != "" {
		requestURL += "?" + encoded
	}

	var body io.Reader
	if input.Body != nil {
		encoded, err := json.Marshal(input.Body)
		if err != nil {
			return fmt.Errorf("v2: encodage du corps : %w", err)
		}
		body = bytes.NewReader(encoded)
	}
	request, err := v2AuthorizedRequest(ctx, input.Method, requestURL, body)
	if err != nil {
		return err
	}
	for name, value := range input.Headers {
		request.Header.Set(name, value)
	}

	response, err := noRedirectClient(transport.httpClient).Do(request)
	if err != nil {
		return fmt.Errorf("v2: requête vers %s échouée : %w", requestURL, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return v2ResponseError(response)
	}
	if output == nil || response.StatusCode == stdhttp.StatusNoContent {
		return nil
	}
	encoded, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("v2: lecture du corps : %w", err)
	}
	if len(encoded) > maxResponseBytes {
		return fmt.Errorf("v2: réponse trop volumineuse")
	}
	if err := json.Unmarshal(encoded, output); err != nil {
		return fmt.Errorf("v2: JSON invalide : %w", err)
	}
	return nil
}

func v2AuthorizedRequest(ctx context.Context, method, requestURL string, body io.Reader) (*stdhttp.Request, error) {
	token, desktopTokenPresent := os.LookupEnv(EnvCloudAccessToken)
	if desktopTokenPresent {
		if !validShortAccessToken(token) {
			return nil, fmt.Errorf("v2: jeton d’accès desktop invalide")
		}
	} else {
		var err error
		token, err = auth.LoadAPIKey()
		if err != nil {
			return nil, err
		}
	}
	request, err := stdhttp.NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		return nil, fmt.Errorf("v2: préparation requête : %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	return request, nil
}

func validShortAccessToken(value string) bool {
	const prefix = "sth_at_"
	if !strings.HasPrefix(value, prefix) || len(value) > 100 {
		return false
	}
	suffix := value[len(prefix):]
	if len(suffix) < 8 || len(suffix) > 94 {
		return false
	}
	for _, character := range suffix {
		if (character < 'A' || character > 'Z') && (character < 'a' || character > 'z') &&
			(character < '0' || character > '9') && character != '_' && character != '-' {
			return false
		}
	}
	return true
}

func validatedV2CloudBaseURL() (string, error) {
	value := sthhttp.CloudBaseURL()
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("v2: URL Cloud invalide")
	}
	if parsed.Scheme == "https" {
		return strings.TrimRight(value, "/"), nil
	}
	host := strings.ToLower(parsed.Hostname())
	if parsed.Scheme == "http" && (host == "localhost" || host == "127.0.0.1" || host == "::1") {
		return strings.TrimRight(value, "/"), nil
	}
	return "", fmt.Errorf("v2: HTTPS requis hors environnement loopback")
}

func noRedirectClient(client *stdhttp.Client) *stdhttp.Client {
	if client == nil {
		client = sharedClient
	}
	clone := *client
	clone.CheckRedirect = func(_ *stdhttp.Request, _ []*stdhttp.Request) error {
		return stdhttp.ErrUseLastResponse
	}
	return &clone
}

func v2ResponseError(response *stdhttp.Response) error {
	if response.StatusCode == stdhttp.StatusUnauthorized {
		return ErrUnauthorized
	}
	body, _ := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	var decoded struct {
		Error  string `json:"error"`
		Detail string `json:"detail"`
	}
	_ = json.Unmarshal(body, &decoded)
	if decoded.Detail != "" {
		return fmt.Errorf("v2: %s", decoded.Detail)
	}
	if decoded.Error != "" {
		return fmt.Errorf("v2: %s", decoded.Error)
	}
	return fmt.Errorf("v2: HTTP %d", response.StatusCode)
}
