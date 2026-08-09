package fs

import (
	"encoding/json"
	"fmt"
	"os"
)

func ReadProjectConfigV2ReadOnly() (ProjectConfigV2, error) {
	configPath, err := ProjectProfilePath()
	if err != nil {
		return ProjectConfigV2{}, err
	}
	data, err := os.ReadFile(configPath)
	if os.IsNotExist(err) {
		return ProjectConfigV2{SchemaVersion: 2, Providers: []ProviderEntry{}, Resources: []ResourceEntry{}}, nil
	}
	if err != nil {
		return ProjectConfigV2{}, fmt.Errorf("lecture de %s échouée : %w", configPath, err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return ProjectConfigV2{}, fmt.Errorf("fichier de configuration malformé (%s) : %w", configPath, err)
	}
	if schemaVersion, _ := raw["schema_version"].(float64); int(schemaVersion) == 2 {
		var config ProjectConfigV2
		if err := json.Unmarshal(data, &config); err != nil {
			return ProjectConfigV2{}, fmt.Errorf("fichier de configuration v2 malformé (%s) : %w", configPath, err)
		}
		if config.Providers == nil {
			config.Providers = []ProviderEntry{}
		}
		if config.Resources == nil {
			config.Resources = []ResourceEntry{}
		}
		return config, nil
	}

	provider, _ := raw["provider"].(string)
	repositoryID, _ := raw["repository_id"].(string)
	catalogRef, _ := raw["ref"].(string)
	catalogPath, _ := raw["catalog_path"].(string)
	targetDir, _ := raw["target_dir"].(string)
	return ProjectConfigV2{
		SchemaVersion: 2,
		Providers: []ProviderEntry{{
			ID:           "default",
			Provider:     provider,
			RepositoryID: repositoryID,
			CatalogRef:   catalogRef,
			CatalogPath:  catalogPath,
			TargetDir:    targetDir,
		}},
		Resources: []ResourceEntry{},
	}, nil
}
