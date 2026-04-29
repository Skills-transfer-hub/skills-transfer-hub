package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	stdhttp "net/http"
	"os"
	"strings"
	"time"

	"sth/internal/auth"
	"sth/internal/fs"
)

const (
	loginProviderName  = "loreskills-cloud"
	loginDefaultProvID = "loreskills"
)

type accessCheckResponse struct {
	OK   bool `json:"ok"`
	Org  struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"org"`
	Plan      string `json:"plan"`
	ExpiresAt string `json:"expires_at"`
}

// runLogin prompts for an API key, validates it via /v1/access-check, persists
// it in the OS keychain, and ensures a `loreskills-cloud` provider entry exists
// in .sth/project.json.
func (a *app) runLogin(args []string) error {
	if len(args) > 0 && isHelpFlag(args[0]) {
		a.writeStdout(loginHelpText())
		return nil
	}
	if len(args) > 1 {
		return fmt.Errorf("usage : sth login [<api-key>]")
	}

	var key string
	if len(args) == 1 {
		key = strings.TrimSpace(args[0])
	} else {
		k, err := a.promptAPIKey()
		if err != nil {
			return err
		}
		key = k
	}
	if key == "" {
		return fmt.Errorf("clé vide")
	}
	if !strings.HasPrefix(key, "lsk_live_") {
		return fmt.Errorf("clé invalide : doit commencer par lsk_live_")
	}

	resp, err := a.callAccessCheck(key)
	if err != nil {
		return fmt.Errorf("vérification de la clé échouée : %w", err)
	}
	if !resp.OK {
		return fmt.Errorf("clé refusée par le serveur")
	}

	if err := auth.SaveAPIKey(key); err != nil {
		return err
	}

	if err := ensureCloudProviderEntry(); err != nil {
		// Non-fatal: keychain is set, user can still call commands. Warn only.
		a.writeStderr(fmt.Sprintf("Avertissement : configuration projet non écrite : %v", err))
	}

	a.writeStdout(
		fmt.Sprintf("Connecté à %s (plan %s, expire le %s).", resp.Org.Name, resp.Plan, resp.ExpiresAt),
		"Clé stockée dans le keychain de l'OS.",
	)
	return nil
}

// runLogout deletes the keychain entry. Idempotent.
func (a *app) runLogout(args []string) error {
	if len(args) > 0 && isHelpFlag(args[0]) {
		a.writeStdout("Usage : sth logout — supprime la clé LoreSkills du keychain.")
		return nil
	}
	if len(args) > 0 {
		return fmt.Errorf("usage : sth logout")
	}
	if err := auth.DeleteAPIKey(); err != nil {
		return err
	}
	a.writeStdout("Déconnecté. Clé supprimée du keychain.")
	return nil
}

func (a *app) promptAPIKey() (string, error) {
	a.writeStdout("Colle ta clé LoreSkills (lsk_live_…) puis Entrée :")
	reader := bufio.NewReader(a.stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("lecture clé échouée : %w", err)
	}
	return strings.TrimSpace(line), nil
}

func (a *app) callAccessCheck(key string) (accessCheckResponse, error) {
	baseURL := strings.TrimRight(loreskillsBaseURL(), "/")
	req, err := stdhttp.NewRequestWithContext(
		context.Background(),
		stdhttp.MethodPost,
		baseURL+"/v1/access-check",
		nil,
	)
	if err != nil {
		return accessCheckResponse{}, err
	}
	req.Header.Set("Authorization", "Bearer "+key)

	httpClient := &stdhttp.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return accessCheckResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == stdhttp.StatusUnauthorized {
		return accessCheckResponse{}, fmt.Errorf("clé refusée (HTTP 401)")
	}
	if resp.StatusCode != stdhttp.StatusOK {
		return accessCheckResponse{}, fmt.Errorf("réponse inattendue (HTTP %d)", resp.StatusCode)
	}

	var out accessCheckResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return accessCheckResponse{}, fmt.Errorf("réponse JSON invalide : %w", err)
	}
	return out, nil
}

func loreskillsBaseURL() string {
	if v := strings.TrimSpace(os.Getenv("STH_LORESKILLS_BASE_URL")); v != "" {
		return v
	}
	return "https://api.loreskills.dev/cli"
}

// ensureCloudProviderEntry guarantees .sth/project.json has a loreskills-cloud
// provider entry. Idempotent: if one already exists, we leave it alone.
func ensureCloudProviderEntry() error {
	cfg, err := fs.ReadProjectConfigV2()
	if err != nil {
		return err
	}
	for _, p := range cfg.Providers {
		if strings.EqualFold(p.Provider, loginProviderName) {
			return nil
		}
	}
	cfg.Providers = append(cfg.Providers, fs.ProviderEntry{
		ID:        loginDefaultProvID,
		Provider:  loginProviderName,
		TargetDir: ".claude/skills",
	})
	_, err = fs.WriteProjectConfigV2(cfg)
	return err
}

func loginHelpText() string {
	return strings.Join([]string{
		"Usage : sth login [<api-key>]",
		"",
		"Authentifie le CLI auprès de LoreSkills Cloud.",
		"Si aucune clé n'est passée en argument, elle est demandée interactivement.",
		"La clé est validée puis stockée dans le keychain de l'OS.",
		"",
		"La variable LORESKILLS_API_KEY peut servir de fallback en l'absence de keychain (CI).",
		"Pour basculer le CLI sur un backend de dev, exporte STH_LORESKILLS_BASE_URL.",
	}, "\n")
}
