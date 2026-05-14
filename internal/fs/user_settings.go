package fs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	userSettingsDirName  = "sth"
	userSettingsFileName = "settings.json"
)

// TelemetrySettings records whether the user has consented to anonymous
// usage reporting at first init. Absent file or absent block both mean
// "not consented" (fail-closed) — the CLI must never send telemetry until
// an explicit affirmative is recorded here.
type TelemetrySettings struct {
	Consented           bool   `json:"consented"`
	ConsentedAt         string `json:"consented_at,omitempty"`
	CLIVersionAtConsent string `json:"cli_version_at_consent,omitempty"`
}

// UserSettings is the durable preferences blob stored at
// os.UserConfigDir()/sth/settings.json. It is user-scoped (not project-scoped)
// because consent travels with the human, not the repo.
type UserSettings struct {
	Telemetry TelemetrySettings `json:"telemetry"`
}

// UserSettingsPath returns the absolute path of the settings file, applying
// the STH_USER_CONFIG_DIR override when set so tests and CI can sandbox.
func UserSettingsPath() (string, error) {
	baseDir := strings.TrimSpace(os.Getenv("STH_USER_CONFIG_DIR"))
	if baseDir == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("impossible de localiser le dossier de configuration utilisateur : %w", err)
		}
		baseDir = dir
	}
	return filepath.Join(filepath.Clean(baseDir), userSettingsDirName, userSettingsFileName), nil
}

// ReadUserSettings returns nil (without error) when the file does not exist —
// the absence of settings is a valid state on a fresh install.
func ReadUserSettings() (*UserSettings, error) {
	settingsPath, err := UserSettingsPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(settingsPath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lecture de %s échouée : %w", settingsPath, err)
	}
	var s UserSettings
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("fichier de settings malformé (%s) : %w", settingsPath, err)
	}
	return &s, nil
}

// WriteUserSettings persists the settings atomically. Creates the parent
// directory if missing. Returns the written path for caller logging.
func WriteUserSettings(s UserSettings) (string, error) {
	settingsPath, err := UserSettingsPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		return "", fmt.Errorf("création du dossier de settings échouée : %w", err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return "", fmt.Errorf("sérialisation des settings échouée : %w", err)
	}
	if err := atomicWriteFile(settingsPath, append(data, '\n'), 0o644); err != nil {
		return "", err
	}
	return settingsPath, nil
}

// SetTelemetryConsent records the user's choice and stamps the moment. The
// CLI version is captured so future audits know which prompt copy the user
// agreed to — important if the consent wording ever changes materially.
func SetTelemetryConsent(consented bool, cliVersion string) (string, error) {
	existing, err := ReadUserSettings()
	if err != nil {
		return "", err
	}
	if existing == nil {
		existing = &UserSettings{}
	}
	existing.Telemetry = TelemetrySettings{
		Consented:           consented,
		ConsentedAt:         time.Now().UTC().Format(time.RFC3339),
		CLIVersionAtConsent: strings.TrimSpace(cliVersion),
	}
	return WriteUserSettings(*existing)
}

// TelemetryConsented is fail-closed: missing file, missing block, or any
// read error all return false. Callers gating network calls on this can
// trust a true return; a false might be "not yet asked" or "explicitly
// refused" — neither should produce traffic.
func TelemetryConsented() bool {
	s, err := ReadUserSettings()
	if err != nil || s == nil {
		return false
	}
	return s.Telemetry.Consented
}

// HasTelemetryDecision distinguishes "user has been asked and answered"
// from "user has never been asked". The init wizard uses this to decide
// whether to prompt — re-prompting on every init would be annoying.
func HasTelemetryDecision() bool {
	s, err := ReadUserSettings()
	if err != nil || s == nil {
		return false
	}
	return strings.TrimSpace(s.Telemetry.ConsentedAt) != ""
}
