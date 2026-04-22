package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"loreskills/internal/fs"
	"strings"
)

func (a *app) runInit() error {
	reader := bufio.NewReader(a.stdin)

	existingProfile, err := fs.ReadProjectProfileIfPresent()
	if err != nil {
		return err
	}
	legacyConfig, err := fs.ReadConfigIfPresent()
	if err != nil {
		return err
	}

	defaults := fs.ProjectProfile{
		Provider:    "github",
		RepositoryID: "",
		Ref:         "main",
		CatalogPath: "SKILLS.md",
		TargetDir:   ".claude/skills",
	}
	if existingProfile != nil {
		defaults = *existingProfile
		a.writeStdout(
			"Configuration projet existante détectée :",
			fmt.Sprintf("- provider: %s", existingProfile.Provider),
			fmt.Sprintf("- repository_id: %s", existingProfile.RepositoryID),
			fmt.Sprintf("- ref: %s", existingProfile.Ref),
			fmt.Sprintf("- catalog_path: %s", existingProfile.CatalogPath),
			fmt.Sprintf("- target_dir: %s", existingProfile.TargetDir),
		)
		shouldReplace, promptErr := promptYesNo(reader, a.stdout, "Remplacer cette configuration ?", false)
		if promptErr != nil {
			return promptErr
		}
		if !shouldReplace {
			a.writeStdout("Configuration projet conservée.", "Prochaine étape : utilisez `loreskills list`.")
			return nil
		}
	} else if legacyConfig != nil {
		legacyProfile, legacyErr := fs.LegacyProfileFromConfig(*legacyConfig)
		if legacyErr == nil {
			shouldImport, promptErr := promptYesNo(reader, a.stdout, "Importer la configuration legacy ~/.loreskillsrc ?", true)
			if promptErr != nil {
				return promptErr
			}
			if shouldImport {
				defaults = *legacyProfile
			}
		}
	}

	provider, err := promptProvider(reader, a.stdout, defaults.Provider)
	if err != nil {
		return err
	}
	a.writeStdout(providerSetupGuidance(provider))

	repositoryID, err := promptValue(reader, a.stdout, "Repository ID", defaults.RepositoryID)
	if err != nil {
		return err
	}
	refValue, err := promptValue(reader, a.stdout, "Référence Git", defaults.Ref)
	if err != nil {
		return err
	}
	catalogPath, err := promptValue(reader, a.stdout, "Chemin du catalogue", defaults.CatalogPath)
	if err != nil {
		return err
	}
	targetDir, err := promptValue(reader, a.stdout, "Dossier cible du projet", defaults.TargetDir)
	if err != nil {
		return err
	}

	profile := fs.ProjectProfile{
		Provider:           provider,
		RepositoryID:       repositoryID,
		Ref:                refValue,
		CatalogPath:        catalogPath,
		TargetDir:          targetDir,
		MigratedFromLegacy: defaults.MigratedFromLegacy,
	}

	if err := a.client.CheckProjectAccess(context.Background(), profile); err != nil {
		return err
	}

	path, err := fs.WriteProjectProfile(profile)
	if err != nil {
		return err
	}

	a.writeStdout(
		fmt.Sprintf("Configuration projet enregistrée : %s", path),
		fmt.Sprintf("Provider: %s", profile.Provider),
		fmt.Sprintf("Dépôt: %s", profile.RepositoryID),
		fmt.Sprintf("Catalogue: %s @ %s", profile.CatalogPath, profile.Ref),
		fmt.Sprintf("Dossier cible: %s", profile.TargetDir),
		"Accès au dépôt validé.",
		"Prochaine étape : utilisez `loreskills list`.",
	)
	return nil
}

func providerSetupGuidance(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "github":
		return "Authentification GitHub : utilisez GITHUB_TOKEN, LORESKILLS_TOKEN, ou vos identifiants Git pour github.com."
	case "gitlab":
		return "Authentification GitLab : utilisez GITLAB_TOKEN, LORESKILLS_TOKEN, ou vos identifiants Git pour gitlab.com."
	case "azure-devops":
		return "Authentification Azure DevOps : utilisez AZURE_DEVOPS_EXT_PAT, AZURE_DEVOPS_TOKEN, ou vos identifiants Git pour dev.azure.com."
	case "bitbucket":
		return "Authentification Bitbucket : utilisez BITBUCKET_TOKEN ou BITBUCKET_USERNAME/BITBUCKET_APP_PASSWORD, ou vos identifiants Git pour bitbucket.org."
	default:
		return "Authentifiez-vous via une variable d'environnement provider-specific ou vos identifiants Git existants."
	}
}

func promptProvider(reader *bufio.Reader, stdout io.Writer, defaultValue string) (string, error) {
	for {
		value, err := promptValue(reader, stdout, "Provider [github|gitlab|azure-devops|bitbucket]", defaultValue)
		if err != nil {
			return "", err
		}
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "github", "gitlab", "azure-devops", "bitbucket":
			return strings.ToLower(strings.TrimSpace(value)), nil
		default:
			fmt.Fprintln(stdout, "Provider invalide. Choisissez github, gitlab, azure-devops ou bitbucket.")
		}
	}
}

func promptValue(reader *bufio.Reader, stdout io.Writer, label, defaultValue string) (string, error) {
	prompt := label
	if strings.TrimSpace(defaultValue) != "" {
		prompt = fmt.Sprintf("%s [%s]", label, defaultValue)
	}
	if _, err := fmt.Fprintf(stdout, "%s: ", prompt); err != nil {
		return "", err
	}
	line, err := reader.ReadString('\n')
	if err != nil && err.Error() != "EOF" {
		return "", fmt.Errorf("lecture de l'entrée utilisateur échouée : %w", err)
	}
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		trimmed = strings.TrimSpace(defaultValue)
	}
	if trimmed == "" {
		return "", fmt.Errorf("%s doit être renseigné", strings.ToLower(label))
	}

	return trimmed, nil
}

func promptYesNo(reader *bufio.Reader, stdout io.Writer, label string, defaultYes bool) (bool, error) {
	suffix := "[y/N]"
	if defaultYes {
		suffix = "[Y/n]"
	}
	if _, err := fmt.Fprintf(stdout, "%s %s: ", label, suffix); err != nil {
		return false, err
	}
	line, err := reader.ReadString('\n')
	if err != nil && err.Error() != "EOF" {
		return false, fmt.Errorf("lecture de l'entrée utilisateur échouée : %w", err)
	}
	trimmed := strings.ToLower(strings.TrimSpace(line))
	if trimmed == "" {
		return defaultYes, nil
	}

	return trimmed == "y" || trimmed == "yes" || trimmed == "o" || trimmed == "oui", nil
}
