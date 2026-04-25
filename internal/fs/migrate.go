package fs

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
)

var providerIDRegexp = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// backupFile copies the file at path to path+".bak", overwriting any existing backup.
func backupFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("lecture de %s échouée : %w", path, err)
	}
	if err := os.WriteFile(path+".bak", data, 0o644); err != nil {
		return fmt.Errorf("écriture du backup %s.bak échouée : %w", path, err)
	}
	return nil
}

// ValidateProviderID returns an error if id is empty or contains invalid characters.
// Valid IDs match ^[a-zA-Z0-9_-]+$.
func ValidateProviderID(id string) error {
	if id == "" {
		return fmt.Errorf("provider ID ne peut pas être vide")
	}
	if !providerIDRegexp.MatchString(id) {
		return fmt.Errorf("provider ID invalide %q : seuls les caractères alphanumérique, tiret et underscore sont autorisés", id)
	}
	return nil
}

// MigrateProjectConfig reads the project config at path and migrates it from v1 to v2 if needed.
// If the file already has schema_version 2 it is returned as-is without rewriting.
// On v1→v2 migration, a progress message is written to w.
func MigrateProjectConfig(path string, w io.Writer) (ProjectConfigV2, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ProjectConfigV2{}, fmt.Errorf("lecture de %s échouée : %w", path, err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return ProjectConfigV2{}, fmt.Errorf("fichier de configuration malformé (%s) : %w", path, err)
	}

	if schemaVersion, _ := raw["schema_version"].(float64); int(schemaVersion) == 2 {
		var cfg ProjectConfigV2
		if err := json.Unmarshal(data, &cfg); err != nil {
			return ProjectConfigV2{}, fmt.Errorf("fichier de configuration v2 malformé (%s) : %w", path, err)
		}
		return cfg, nil
	}

	if err := backupFile(path); err != nil {
		return ProjectConfigV2{}, err
	}

	provider, _ := raw["provider"].(string)
	repositoryID, _ := raw["repository_id"].(string)
	ref, _ := raw["ref"].(string)
	catalogPath, _ := raw["catalog_path"].(string)
	targetDir, _ := raw["target_dir"].(string)

	cfg := ProjectConfigV2{
		SchemaVersion: 2,
		Providers: []ProviderEntry{
			{
				ID:           "default",
				Provider:     provider,
				RepositoryID: repositoryID,
				CatalogRef:   ref,
				CatalogPath:  catalogPath,
				TargetDir:    targetDir,
			},
		},
		Resources: []ResourceEntry{},
	}

	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return ProjectConfigV2{}, fmt.Errorf("sérialisation v2 échouée : %w", err)
	}
	if err := atomicWriteFile(path, append(out, '\n'), 0o644); err != nil {
		return ProjectConfigV2{}, fmt.Errorf("écriture de %s échouée : %w", path, err)
	}
	fmt.Fprintf(w, "[loreskills] migrating project.json v1 → v2 … done\n")

	return cfg, nil
}

// MigrateInstalledState reads the installed state at path and migrates it from v1 to v2 if needed.
// If the file already has schema_version 2 it is returned as-is without rewriting.
// On v1→v2 migration, a progress message is written to w.
func MigrateInstalledState(path string, w io.Writer) (InstalledResourceStateV2, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return InstalledResourceStateV2{}, fmt.Errorf("lecture de %s échouée : %w", path, err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return InstalledResourceStateV2{}, fmt.Errorf("fichier d'état malformé (%s) : %w", path, err)
	}

	if schemaVersion, _ := raw["schema_version"].(float64); int(schemaVersion) == 2 {
		var state InstalledResourceStateV2
		if err := json.Unmarshal(data, &state); err != nil {
			return InstalledResourceStateV2{}, fmt.Errorf("fichier d'état v2 malformé (%s) : %w", path, err)
		}
		return state, nil
	}

	if err := backupFile(path); err != nil {
		return InstalledResourceStateV2{}, err
	}

	var v1State struct {
		Entries []struct {
			CatalogID string `json:"catalog_id"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(data, &v1State); err != nil {
		return InstalledResourceStateV2{}, fmt.Errorf("fichier d'état v1 malformé (%s) : %w", path, err)
	}

	entries := make([]InstalledResourceRecordV2, 0, len(v1State.Entries))
	for _, e := range v1State.Entries {
		entries = append(entries, InstalledResourceRecordV2{
			ResourceName: e.CatalogID,
			ProviderID:   "default",
			ManagedFiles: []ManagedFile{},
		})
	}

	state := InstalledResourceStateV2{
		SchemaVersion: 2,
		Entries:       entries,
	}

	out, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return InstalledResourceStateV2{}, fmt.Errorf("sérialisation v2 échouée : %w", err)
	}
	if err := atomicWriteFile(path, append(out, '\n'), 0o644); err != nil {
		return InstalledResourceStateV2{}, fmt.Errorf("écriture de %s échouée : %w", path, err)
	}
	fmt.Fprintf(w, "[loreskills] migrating lore-state.json v1 → v2 … done\n")

	return state, nil
}
