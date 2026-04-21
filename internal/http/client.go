// Package http centralizes LoreSkills network operations.
package http

import (
	"context"
	"fmt"
	"io"
	stdhttp "net/http"
	neturl "net/url"
	"strings"
	"time"
)

type Client struct {
	httpClient *stdhttp.Client
}

// NewClient builds a network client for LoreSkills downloads.
func NewClient(httpClient *stdhttp.Client) *Client {
	if httpClient == nil {
		httpClient = &stdhttp.Client{Timeout: 10 * time.Second}
	}

	return &Client{httpClient: httpClient}
}

// DownloadSkill retrieves the remote markdown body for a skill.
func (c *Client) DownloadSkill(ctx context.Context, baseURL, skillName string) ([]byte, error) {
	skillURL, err := buildSkillURL(baseURL, skillName)
	if err != nil {
		return nil, err
	}

	req, err := stdhttp.NewRequestWithContext(ctx, stdhttp.MethodGet, skillURL, nil)
	if err != nil {
		return nil, fmt.Errorf("préparation de la requête pour %s échouée : %w", skillURL, err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("échec de la requête vers %s : %w", skillURL, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case stdhttp.StatusOK:
	case stdhttp.StatusNotFound:
		return nil, fmt.Errorf("skill %q introuvable sur le dépôt distant (HTTP 404) : %s", skillName, skillURL)
	default:
		return nil, fmt.Errorf("réponse inattendue du serveur : HTTP %d pour %s", resp.StatusCode, skillURL)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("lecture du corps de la réponse échouée : %w", err)
	}

	return data, nil
}

func buildSkillURL(baseURL, skillName string) (string, error) {
	trimmedBaseURL := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	trimmedSkillName := strings.TrimSpace(skillName)
	if trimmedBaseURL == "" {
		return "", fmt.Errorf("repo_raw_url doit être renseigné")
	}
	if _, err := neturl.ParseRequestURI(trimmedBaseURL); err != nil {
		return "", fmt.Errorf("repo_raw_url invalide : %w", err)
	}
	if trimmedSkillName == "" {
		return "", fmt.Errorf("le nom du skill doit être renseigné")
	}

	return trimmedBaseURL + "/" + neturl.PathEscape(trimmedSkillName) + ".md", nil
}
