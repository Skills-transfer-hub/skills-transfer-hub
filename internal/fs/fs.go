package fs

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	configFileName = ".loreskillsrc"
	stateFileName  = "lore-state.json"

	// En-tête ajouté en début de chaque skill pour signaler qu'il est géré par LoreSkills.
	skillHeader = "> ⚠️ ATTENTION : Skill géré par LoreSkills. Ne pas modifier manuellement.\n\n"
)

// Config stores the LoreSkills user configuration.
type Config struct {
	RepoRawURL string `json:"repo_raw_url"`
	TargetDir  string `json:"target_dir"`
}

// ConfigPath returns the absolute path to ~/.loreskillsrc.
func ConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("impossible de localiser le répertoire personnel : %w", err)
	}
	return filepath.Join(home, configFileName), nil
}

// WriteDefaultConfig creates ~/.loreskillsrc with LoreSkills starter values.
func WriteDefaultConfig() (string, error) {
	path, err := ConfigPath()
	if err != nil {
		return "", err
	}

	if _, err := os.Stat(path); err == nil {
		return path, fmt.Errorf("un fichier de configuration existe déjà : %s", path)
	} else if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("vérification de %s échouée : %w", path, err)
	}

	defaults := Config{
		RepoRawURL: "https://raw.githubusercontent.com/votre-organisation/votre-repo/main",
		TargetDir:  ".claude/skills",
	}

	data, err := json.MarshalIndent(defaults, "", "  ")
	if err != nil {
		return "", fmt.Errorf("sérialisation de la configuration échouée : %w", err)
	}

	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return "", fmt.Errorf("écriture de %s échouée : %w", path, err)
	}

	return path, nil
}

// ReadConfig loads and validates the LoreSkills user configuration.
func ReadConfig() (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("configuration introuvable (%s) — exécutez `loreskills init`", path)
		}
		return nil, fmt.Errorf("lecture de %s échouée : %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("fichier de configuration malformé (%s) : %w", path, err)
	}
	if err := validateConfig(cfg); err != nil {
		return nil, fmt.Errorf("configuration invalide (%s) : %w", path, err)
	}

	cfg.RepoRawURL = strings.TrimRight(strings.TrimSpace(cfg.RepoRawURL), "/")
	cfg.TargetDir = filepath.Clean(strings.TrimSpace(cfg.TargetDir))

	return &cfg, nil
}

// EnsureTargetDir creates the configured target directory in the current project.
func EnsureTargetDir(targetDir string) error {
	if err := validateTargetDir(targetDir); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Clean(targetDir), 0o755); err != nil {
		return fmt.Errorf("création du dossier %s échouée : %w", targetDir, err)
	}
	return nil
}

// SaveSkill writes the managed Markdown file for a skill.
func SaveSkill(targetDir, skillName string, content []byte) error {
	if err := validateSkillName(skillName); err != nil {
		return err
	}
	if err := EnsureTargetDir(targetDir); err != nil {
		return err
	}

	full := append([]byte(skillHeader), content...)
	dest := filepath.Join(filepath.Clean(targetDir), skillName+".md")

	if err := os.WriteFile(dest, full, 0o644); err != nil {
		return fmt.Errorf("écriture du skill %s échouée : %w", dest, err)
	}

	return nil
}

// InstallSkill performs the filesystem side of a successful install.
func InstallSkill(targetDir, skillName string, content []byte) error {
	if err := SaveSkill(targetDir, skillName, content); err != nil {
		return err
	}

	return UpdateState(targetDir, skillName)
}

// UpdateState stores the installed marker for the given skill.
func UpdateState(targetDir, skillName string) error {
	if err := validateSkillName(skillName); err != nil {
		return err
	}
	if err := EnsureTargetDir(targetDir); err != nil {
		return err
	}

	statePath := filepath.Join(filepath.Clean(targetDir), stateFileName)
	state, err := ReadState(targetDir)
	if err != nil {
		return err
	}
	if state == nil {
		state = map[string]string{}
	}

	state[skillName] = "installed"

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("sérialisation de l'état échouée : %w", err)
	}

	if err := os.WriteFile(statePath, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("écriture de %s échouée : %w", statePath, err)
	}

	return nil
}

// ReadState loads the tracked skill state from lore-state.json.
func ReadState(targetDir string) (map[string]string, error) {
	if err := validateTargetDir(targetDir); err != nil {
		return nil, err
	}

	statePath := filepath.Join(filepath.Clean(targetDir), stateFileName)

	data, err := os.ReadFile(statePath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lecture de %s échouée : %w", statePath, err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return nil, nil
	}

	var state map[string]string
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("fichier d'état malformé (%s) : %w", statePath, err)
	}

	return state, nil
}

// ListInstalledSkills returns the tracked skills sorted by name.
func ListInstalledSkills(targetDir string) ([]string, error) {
	state, err := ReadState(targetDir)
	if err != nil {
		return nil, err
	}
	if len(state) == 0 {
		return nil, nil
	}

	skills := make([]string, 0, len(state))
	for skill, status := range state {
		if skill == "" || status != "installed" {
			continue
		}
		skills = append(skills, skill)
	}
	sort.Strings(skills)
	if len(skills) == 0 {
		return nil, nil
	}

	return skills, nil
}

func validateConfig(cfg Config) error {
	if strings.TrimSpace(cfg.RepoRawURL) == "" {
		return errors.New("repo_raw_url doit être renseigné")
	}
	if strings.TrimSpace(cfg.TargetDir) == "" {
		return errors.New("target_dir doit être renseigné")
	}

	return validateTargetDir(cfg.TargetDir)
}

func validateTargetDir(targetDir string) error {
	trimmed := strings.TrimSpace(targetDir)
	if trimmed == "" {
		return errors.New("target_dir doit être renseigné")
	}
	if filepath.IsAbs(trimmed) {
		return errors.New("target_dir doit être un chemin relatif au projet courant")
	}

	return nil
}

func validateSkillName(skillName string) error {
	trimmed := strings.TrimSpace(skillName)
	if trimmed == "" {
		return errors.New("le nom du skill doit être renseigné")
	}
	if trimmed == "." || trimmed == ".." || strings.Contains(trimmed, "/") || strings.Contains(trimmed, "\\") {
		return fmt.Errorf("nom de skill invalide : %q", skillName)
	}

	return nil
}
