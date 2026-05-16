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

// TelemetryMode is the granular telemetry choice persisted in user settings.
// Three values:
//
//   - TelemetryOff:        nothing is sent.
//   - TelemetryAnonymous:  events are sent to /v1/usage/anon WITHOUT any
//                          authentication or identifier. Contributes to
//                          product-level stats only — the user has no
//                          dashboard view for these events.
//   - TelemetryConnected:  events are sent to /v1/usage WITH a Bearer
//                          lsk_live_* key from the keychain. Visible in the
//                          user's STH Cloud dashboard.
type TelemetryMode string

const (
	TelemetryOff       TelemetryMode = "off"
	TelemetryAnonymous TelemetryMode = "anonymous"
	TelemetryConnected TelemetryMode = "connected"
)

// TelemetrySettings records the user's telemetry choice at first init.
// Absent file or absent block both mean "not decided yet" (fail-closed):
// the CLI must never send telemetry until an explicit affirmative is
// recorded here.
//
// Mode is the source of truth since the 3-tier rework. The Consented bool
// is kept for backwards compatibility with settings files written before
// the rework — TelemetryModeActive migrates them silently on read.
type TelemetrySettings struct {
	Mode                TelemetryMode `json:"mode,omitempty"`
	Consented           bool          `json:"consented"`
	ConsentedAt         string        `json:"consented_at,omitempty"`
	CLIVersionAtConsent string        `json:"cli_version_at_consent,omitempty"`
}

// UserSettings is the durable preferences blob stored at
// os.UserConfigDir()/sth/settings.json. It is user-scoped (not project-scoped)
// because consent travels with the human, not the repo.
//
// ProActive / ProCheckedAt cache the result of the Pro entitlement check
// driven by `cmd/pro_gate.go`. Both are omitempty so legacy settings files
// (telemetry-only) round-trip unchanged.
type UserSettings struct {
	Telemetry    TelemetrySettings `json:"telemetry"`
	ProActive    bool              `json:"pro_active,omitempty"`
	ProCheckedAt time.Time         `json:"pro_checked_at,omitempty"`
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

// ReadUserSettings returns nil (without error) for absent, empty or malformed
// files — every one of those states is treated as "no decision yet" by the
// callers (HasTelemetryDecision, TelemetryConsented) so they can fall back to
// the safe default (no telemetry, re-prompt at next sth init). Without this
// tolerance a corrupt settings file would block sth init entirely.
//
// Real I/O errors (permission denied, etc.) still surface — those signal a
// configuration problem the user needs to see, not a content issue we can
// safely ignore.
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
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, nil
	}
	var s UserSettings
	if err := json.Unmarshal(data, &s); err != nil {
		// Treat a malformed file as "no decision yet" — the wizard will simply
		// re-prompt at next init and overwrite the file with a fresh, valid one.
		return nil, nil
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

// SetTelemetryConsent records a legacy boolean choice and stamps the moment.
// Deprecated: prefer SetTelemetryMode. Kept so external callers / tests can
// still toggle the simple consent, which maps to anonymous (true) / off (false).
func SetTelemetryConsent(consented bool, cliVersion string) (string, error) {
	mode := TelemetryOff
	if consented {
		mode = TelemetryAnonymous
	}
	return SetTelemetryMode(mode, cliVersion)
}

// SetTelemetryMode records the user's explicit choice and stamps the moment.
// The CLI version is captured so future audits know which prompt copy the
// user agreed to — important if the consent wording ever changes materially.
//
// Mode and the legacy Consented bool are kept in sync on write so older code
// paths (or rolled-back binaries) keep working without a re-prompt loop.
func SetTelemetryMode(mode TelemetryMode, cliVersion string) (string, error) {
	existing, err := ReadUserSettings()
	if err != nil {
		return "", err
	}
	if existing == nil {
		existing = &UserSettings{}
	}
	existing.Telemetry = TelemetrySettings{
		Mode:                mode,
		Consented:           mode != TelemetryOff,
		ConsentedAt:         time.Now().UTC().Format(time.RFC3339),
		CLIVersionAtConsent: strings.TrimSpace(cliVersion),
	}
	return WriteUserSettings(*existing)
}

// TelemetryConsented is fail-closed: missing file, missing block, or any
// read error all return false. Callers gating network calls on this can
// trust a true return; a false might be "not yet asked" or "explicitly
// refused" — neither should produce traffic.
//
// Alias of (TelemetryModeActive() != TelemetryOff) kept for callers that
// don't care about the granular mode.
func TelemetryConsented() bool {
	return TelemetryModeActive() != TelemetryOff
}

// TelemetryModeActive returns the user's current telemetry mode. Reads the
// persisted Mode when present; otherwise migrates silently from the legacy
// Consented bool:
//
//   - Consented:false (or absent settings) → TelemetryOff.
//   - Consented:true                       → TelemetryAnonymous by default.
//
// The init wizard upgrades anonymous → connected when the user actively
// completes the cloud-login sub-prompt; this function is intentionally
// keychain-agnostic so it does not require the auth package and can't
// trigger a keyring prompt on platforms where reading is interactive.
func TelemetryModeActive() TelemetryMode {
	s, err := ReadUserSettings()
	if err != nil || s == nil {
		return TelemetryOff
	}
	if s.Telemetry.Mode != "" {
		return s.Telemetry.Mode
	}
	if s.Telemetry.Consented {
		return TelemetryAnonymous
	}
	return TelemetryOff
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
