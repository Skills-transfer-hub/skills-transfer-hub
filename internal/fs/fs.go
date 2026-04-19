// Package fs gère toutes les opérations d'I/O fichier et dossier de LoreSkills.
// Il est volontairement isolé de toute logique réseau.
package fs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	configFileName = ".loreskillsrc"
	stateFileName  = "lore-state.json"

	// En-tête ajouté en début de chaque skill pour signaler qu'il est géré par LoreSkills.
	skillHeader = "> ⚠️ ATTENTION : Skill géré par LoreSkills. Ne pas modifier manuellement.\n\n"
)

// Config représente la configuration utilisateur lue depuis ~/.loreskillsrc.
type Config struct {
	RepoRawURL string `json:"repo_raw_url"`
	TargetDir  string `json:"target_dir"`
}

// ConfigPath retourne le chemin absolu du fichier de configuration (~/.loreskillsrc).
func ConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("impossible de localiser le répertoire personnel : %w", err)
	}
	return filepath.Join(home, configFileName), nil
}

// WriteDefaultConfig crée ~/.loreskillsrc avec les valeurs par défaut.
// Retourne le chemin du fichier créé. Échoue si un fichier existe déjà.
func WriteDefaultConfig() (string, error) {
	path, err := ConfigPath()
	if err != nil {
		return "", err
	}

	// Protéger contre l'écrasement involontaire d'une config existante
	if _, err := os.Stat(path); err == nil {
		return path, fmt.Errorf("un fichier de configuration existe déjà : %s", path)
	}

	defaults := Config{
		RepoRawURL: "https://raw.githubusercontent.com/VOTRE_ORGA/skills-repo/main",
		TargetDir:  ".claude/skills",
	}

	data, err := json.MarshalIndent(defaults, "", "  ")
	if err != nil {
		return "", fmt.Errorf("sérialisation de la configuration échouée : %w", err)
	}

	// 0600 : seul le propriétaire peut lire/écrire (bonne pratique pour les configs)
	if err := os.WriteFile(path, data, 0600); err != nil {
		return "", fmt.Errorf("écriture de %s échouée : %w", path, err)
	}

	return path, nil
}

// ReadConfig lit et désérialise le fichier de configuration utilisateur.
func ReadConfig() (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("impossible de lire %s — avez-vous exécuté `loreskills init` ? : %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("fichier de configuration malformé (%s) : %w", path, err)
	}

	return &cfg, nil
}

// EnsureTargetDir crée le dossier targetDir (et ses parents) dans le CWD s'il n'existe pas déjà.
func EnsureTargetDir(targetDir string) error {
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("création du dossier %s échouée : %w", targetDir, err)
	}
	return nil
}

// SaveSkill écrit le contenu d'un skill dans targetDir/<skillName>.md,
// précédé de l'en-tête d'avertissement LoreSkills.
func SaveSkill(targetDir, skillName string, content []byte) error {
	full := append([]byte(skillHeader), content...)
	dest := filepath.Join(targetDir, skillName+".md")

	if err := os.WriteFile(dest, full, 0644); err != nil {
		return fmt.Errorf("écriture du skill %s échouée : %w", dest, err)
	}

	return nil
}

// UpdateState ajoute ou met à jour la clé skillName dans lore-state.json.
func UpdateState(targetDir, skillName string) error {
	statePath := filepath.Join(targetDir, stateFileName)
	state := map[string]string{}

	// Charger l'état existant ; en cas de corruption, on repart d'une map vide
	if data, err := os.ReadFile(statePath); err == nil {
		_ = json.Unmarshal(data, &state)
	}

	state[skillName] = "installed"

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("sérialisation de l'état échouée : %w", err)
	}

	if err := os.WriteFile(statePath, data, 0644); err != nil {
		return fmt.Errorf("écriture de %s échouée : %w", statePath, err)
	}

	return nil
}

// ReadState lit lore-state.json et retourne la map des skills installés.
// Retourne (nil, nil) si le fichier est absent (aucun skill installé).
func ReadState(targetDir string) (map[string]string, error) {
	statePath := filepath.Join(targetDir, stateFileName)

	data, err := os.ReadFile(statePath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lecture de %s échouée : %w", statePath, err)
	}

	var state map[string]string
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("fichier d'état malformé (%s) : %w", statePath, err)
	}

	return state, nil
}
