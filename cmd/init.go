package cmd

import (
	"bufio"
	"context"
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

	a.writeStdout(
		"Assistant de configuration LoreSkills",
		"Ce wizard vous guide pas a pas pour connecter le projet courant a un catalogue distant.",
	)
	a.writeStdout(
		"Etape 1/5 - Choisir le provider",
		"Providers pris en charge : github, gitlab, azure-devops, bitbucket.",
	)

	provider, err := promptProvider(reader, a.stdout, defaults.Provider)
	if err != nil {
		return err
	}

	a.writeStdout(
		"Etape 2/5 - Type d'acces au depot",
	)
	isPrivate, err := promptYesNo(reader, a.stdout, "Le depot distant est-il prive ?", true)
	if err != nil {
		return err
	}
	for {
		a.writeStdout(providerAccessGuidance(provider, isPrivate)...)
		reviewAgain, promptErr := promptYesNo(reader, a.stdout, "Relire la guidance provider avant de continuer ?", false)
		if promptErr != nil {
			return promptErr
		}
		if !reviewAgain {
			break
		}
	}

	var apiToken string
	if isPrivate {
		var tokenErr error
		apiToken, tokenErr = promptOptionalToken(reader, a.stdout)
		if tokenErr != nil {
			return tokenErr
		}
	}

	a.writeStdout(
		"Etape 3/5 - Identifier le depot",
		fmt.Sprintf("Format attendu pour repository_id : %s", providerRepositoryIDFormat(provider)),
		fmt.Sprintf("Exemple : %s", providerRepositoryIDExample(provider)),
	)

	repositoryID, err := promptRepositoryID(reader, a.stdout, provider, defaults.RepositoryID)
	if err != nil {
		return err
	}

	a.writeStdout(
		"Etape 4/5 - Reference Git et catalogue",
		"Choisissez la reference distante, le fichier SKILLS.md, puis le dossier cible local.",
	)
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
		APIToken:           apiToken,
	}

	a.writeStdout(
		"Etape 5/5 - Validation de l'acces distant",
		"Validation en cours du depot et du catalogue...",
	)

	if err := a.client.CheckProjectAccess(context.Background(), profile); err != nil {
		return err
	}

	path, err := fs.WriteProjectProfile(profile)
	if err != nil {
		return err
	}

	successLines := []string{
		fmt.Sprintf("Configuration projet enregistrée : %s", path),
		fmt.Sprintf("Provider: %s", profile.Provider),
		fmt.Sprintf("Dépôt: %s", profile.RepositoryID),
		fmt.Sprintf("Catalogue: %s @ %s", profile.CatalogPath, profile.Ref),
		fmt.Sprintf("Dossier cible: %s", profile.TargetDir),
		"Accès au dépôt validé.",
		"Prochaine étape : utilisez `loreskills list`.",
	}
	if profile.APIToken != "" {
		successLines = append(successLines,
			"",
			"Le token saisi a ete utilise pour la validation mais n'est pas sauvegarde.",
			"Pour les prochaines utilisations, definissez la variable d'environnement :",
			fmt.Sprintf("  export %s=<votre-token>", providerTokenEnvVar(profile.Provider)),
		)
	}
	a.writeStdout(successLines...)
	return nil
}


func providerAccessGuidance(provider string, isPrivate bool) []string {
	if !isPrivate {
		return providerPublicAccessGuidance(provider)
	}
	return providerPrivateAccessGuidance(provider)
}

func providerPublicAccessGuidance(provider string) []string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "github":
		return []string{"Depot public", "Authentification GitHub : aucune authentification n'est necessaire pour un depot public lisible anonymement."}
	case "gitlab":
		return []string{"Depot public", "Authentification GitLab : aucune authentification n'est necessaire pour un depot public lisible anonymement."}
	case "azure-devops":
		return []string{"Depot public", "Authentification Azure DevOps : un depot public peut rester accessible sans secret, sinon fournissez un PAT Azure DevOps."}
	case "bitbucket":
		return []string{"Depot public", "Authentification Bitbucket : aucune authentification n'est necessaire pour un depot public lisible anonymement."}
	default:
		return []string{"Depot public", "Si le depot est public, l'acces anonyme peut suffire; sinon configurez une authentification provider-specific."}
	}
}

func providerPrivateAccessGuidance(provider string) []string {
	guide, ok := findProviderGuide(provider)
	if !ok {
		return []string{
			"Depot prive - Authentification requise",
			"",
			"Generez un token d'acces depuis l'interface de votre provider et definissez :",
			"  export LORESKILLS_TOKEN=<votre-token>",
		}
	}

	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "github":
		return []string{
			"Depot prive - Authentification requise",
			"",
			"Pour acceder a ce depot GitHub prive, vous avez besoin d'un Personal Access Token (PAT).",
			"",
			"Comment generer votre token :",
			"  1. Rendez-vous sur : " + guide.ActionURL,
			"  2. Selectionnez \"Fine-grained token\" (recommande) ou \"Classic token\"",
			"     Fine-grained : accordez la permission \"" + guide.MinimumPermission + "\" sur le depot cible",
			"     Classic      : cochez \"repo\" pour l'acces complet aux depots prives",
			"  3. Cliquez \"Generate token\" et copiez la valeur affichee",
			"",
			"Variable d'environnement a definir pour les prochaines utilisations :",
			"  export GITHUB_TOKEN=<votre-token>",
			"  ou LORESKILLS_TOKEN=<votre-token>   (compatible toutes plateformes)",
		}
	case "gitlab":
		return []string{
			"Depot prive - Authentification requise",
			"",
			"Pour acceder a ce depot GitLab prive, vous avez besoin d'un Personal Access Token.",
			"",
			"Comment generer votre token :",
			"  1. Rendez-vous sur : " + guide.ActionURL,
			"  2. Donnez un nom au token et choisissez une date d'expiration",
			"  3. Cochez la permission : \"" + guide.MinimumPermission + "\"",
			"  4. Cliquez \"Create personal access token\" et copiez la valeur affichee",
			"",
			"Variable d'environnement a definir pour les prochaines utilisations :",
			"  export GITLAB_TOKEN=<votre-token>",
			"  ou LORESKILLS_TOKEN=<votre-token>   (compatible toutes plateformes)",
		}
	case "azure-devops":
		return []string{
			"Depot prive - Authentification requise",
			"",
			"Pour acceder a ce depot Azure DevOps prive, vous avez besoin d'un Personal Access Token (PAT).",
			"",
			"Comment generer votre token :",
			"  1. Connectez-vous sur : https://dev.azure.com/<votre-organisation>",
			"  2. Cliquez sur votre avatar en haut a droite > \"Personal access tokens\"",
			"     Acces direct : " + guide.ActionURL,
			"  3. Cliquez \"New Token\", selectionnez la permission : \"" + guide.MinimumPermission + "\"",
			"  4. Cliquez \"Create\" et copiez la valeur affichee",
			"",
			"Variable d'environnement a definir pour les prochaines utilisations :",
			"  export AZURE_DEVOPS_EXT_PAT=<votre-token>",
			"  ou LORESKILLS_TOKEN=<votre-token>   (compatible toutes plateformes)",
		}
	case "bitbucket":
		return []string{
			"Depot prive - Authentification requise",
			"",
			"Pour acceder a ce depot Bitbucket Cloud prive, vous avez besoin d'un API token Atlassian.",
			"",
			"Documentation officielle : " + guide.DocumentationURL,
			"Point d'entree compte Atlassian : " + guide.ActionURL,
			"Permission minimale attendue : \"" + guide.MinimumPermission + "\"",
			"",
			"Variables d'environnement a definir pour les prochaines utilisations :",
			"  export BITBUCKET_TOKEN=<votre-token>",
			"  ou LORESKILLS_TOKEN=<votre-token>   (compatible toutes plateformes)",
		}
	default:
		return []string{
			"Depot prive - Authentification requise",
			"",
			"Generez un token d'acces depuis l'interface de votre provider et definissez :",
			"  export LORESKILLS_TOKEN=<votre-token>",
		}
	}
}

// providerTokenEnvVar returns the primary environment variable name for the given provider's token.
func providerTokenEnvVar(provider string) string {
	guide, ok := findProviderGuide(provider)
	if !ok || len(guide.EnvVars) == 0 {
		return "LORESKILLS_TOKEN"
	}

	return guide.EnvVars[0]
}

// promptOptionalToken prompts the user to optionally enter an API token for immediate access validation.
// Returns the entered token or an empty string if the user skips.
func promptOptionalToken(reader *bufio.Reader, stdout io.Writer) (string, error) {
	if _, err := fmt.Fprintf(stdout, "Token d'acces pour valider maintenant (optionnel, Entree pour ignorer): "); err != nil {
		return "", err
	}
	line, err := reader.ReadString('\n')
	if err != nil && err.Error() != "EOF" {
		return "", fmt.Errorf("lecture de l'entree utilisateur echouee : %w", err)
	}
	return strings.TrimSpace(line), nil
}

func promptRepositoryID(reader *bufio.Reader, stdout io.Writer, provider, defaultValue string) (string, error) {
	for {
		value, err := promptValue(reader, stdout, "Repository ID", defaultValue)
		if err != nil {
			return "", err
		}
		normalizedValue, err := fs.NormalizeRepositoryID(provider, value)
		if err != nil {
			fmt.Fprintln(stdout, "Repository ID invalide.")
			fmt.Fprintf(stdout, "Format attendu pour repository_id : %s\n", providerRepositoryIDFormat(provider))
			fmt.Fprintf(stdout, "Exemple : %s\n", providerRepositoryIDExample(provider))
			if hostedDomain := providerHostedDomain(provider); hostedDomain != "" {
				fmt.Fprintf(stdout, "Host hébergé supporté : %s\n", hostedDomain)
			}
			continue
		}

		return normalizedValue, nil
	}
}

func validateRepositoryID(provider, repositoryID string) error {
	_, err := fs.NormalizeRepositoryID(provider, repositoryID)
	return err
}

func providerRepositoryIDFormat(provider string) string {
	guide, ok := findProviderGuide(provider)
	if !ok {
		return "provider/repository"
	}

	return guide.RepositoryIDFormat
}

func providerRepositoryIDExample(provider string) string {
	guide, ok := findProviderGuide(provider)
	if !ok {
		return "provider/repository"
	}

	return guide.RepositoryIDExample
}

func splitRepositoryID(repositoryID string) []string {
	rawParts := strings.Split(strings.TrimSpace(repositoryID), "/")
	parts := make([]string, 0, len(rawParts))
	for _, part := range rawParts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		parts = append(parts, trimmed)
	}

	return parts
}

func promptProvider(reader *bufio.Reader, stdout io.Writer, defaultValue string) (string, error) {
	allowed := supportedProviderKeys()
	for {
		value, err := promptValue(reader, stdout, "Provider [github|gitlab|azure-devops|bitbucket]", defaultValue)
		if err != nil {
			return "", err
		}
		normalized := strings.ToLower(strings.TrimSpace(value))
		if _, ok := findProviderGuide(normalized); ok {
			return normalized, nil
		}
		fmt.Fprintf(stdout, "Provider invalide. Choisissez %s.\n", strings.Join(allowed, ", "))
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
