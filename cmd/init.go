package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"sth/internal/fs"
)

// alphaProviderGateSkipValue must match cmd/alpha_gate.go — the same env var
// disables every Alpha-period restriction (version check + provider cap).
const alphaProviderGateSkipValue = "0"

// runInit is the dispatcher: handles the existing-config branch and the
// optional legacy ~/.sthrc import, then hands off to runInitWizard for the
// narrated step-by-step flow. The bulk of the UX lives in cmd/init_wizard.go.
//
// Recognized flags:
//
//	--no-cloud-prompt   skip the connected-mode cloud-login sub-prompt even
//	                    when the user picks telemetry mode = connected. Used
//	                    by power users / CI wrappers that don't want the
//	                    extra interactive step.
func (a *app) runInit(args ...string) error {
	skipCloudPrompt := false
	for _, arg := range args {
		if arg == "--no-cloud-prompt" {
			skipCloudPrompt = true
		}
	}

	reader := bufio.NewReader(a.stdin)

	// First init asks for telemetry mode before anything else. Re-running
	// init never re-prompts: the decision is user-scoped and durable.
	if !fs.HasTelemetryDecision() {
		mode, err := a.promptTelemetryMode(reader)
		if err != nil {
			return err
		}
		if !skipCloudPrompt {
			a.promptCloudLoginIfNeeded(reader, mode)
		}
	}

	existingCfg, err := fs.ReadProjectConfigV2()
	if err != nil {
		return err
	}
	legacyConfig, err := fs.ReadConfigIfPresent()
	if err != nil {
		return err
	}

	if len(existingCfg.Providers) > 0 {
		a.writeStdout(fmt.Sprintf("Configuration projet existante : %d provider(s) configuré(s) :", len(existingCfg.Providers)))
		for _, p := range existingCfg.Providers {
			a.writeStdout(fmt.Sprintf("- [%s] %s: %s @ %s", p.ID, p.Provider, p.RepositoryID, p.CatalogRef))
		}
		addNew, promptErr := promptYesNo(reader, a.stdout, "Ajouter un nouveau provider ?", false)
		if promptErr != nil {
			return promptErr
		}
		if !addNew {
			a.writeStdout("Configuration projet conservée.", "Prochaine étape : utilisez `sth list`.")
			return nil
		}
	}

	defaults := fs.ProjectProfile{
		Provider:    "github",
		Ref:         "main",
		CatalogPath: "SKILLS.md",
		TargetDir:   ".sth",
	}
	if len(existingCfg.Providers) == 0 && legacyConfig != nil {
		legacyProfile, legacyErr := fs.LegacyProfileFromConfig(*legacyConfig)
		if legacyErr == nil {
			shouldImport, promptErr := promptYesNo(reader, a.stdout, "Importer la configuration legacy ~/.sthrc ?", true)
			if promptErr != nil {
				return promptErr
			}
			if shouldImport {
				defaults = *legacyProfile
			}
		}
	}

	// First provider gets "default" silently. For the 2nd+ provider we'll ask
	// inside the wizard (passing "" signals "ask").
	assignedAlias := "default"
	if len(existingCfg.Providers) > 0 {
		assignedAlias = ""
	}

	return a.runInitWizard(reader, existingCfg, defaults, assignedAlias)
}

// providerAccessGuidance returns the public-or-private access guidance for a
// provider. Kept here (not in init_wizard.go) because the per-provider wall
// of text is shared with the "Détails complets ?" gate and any future
// command that needs to show provider help.
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
			"  export STH_TOKEN=<votre-token>",
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
			"  ou STH_TOKEN=<votre-token>   (compatible toutes plateformes)",
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
			"  ou STH_TOKEN=<votre-token>   (compatible toutes plateformes)",
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
			"  ou STH_TOKEN=<votre-token>   (compatible toutes plateformes)",
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
			"  ou STH_TOKEN=<votre-token>   (compatible toutes plateformes)",
		}
	default:
		return []string{
			"Depot prive - Authentification requise",
			"",
			"Generez un token d'acces depuis l'interface de votre provider et definissez :",
			"  export STH_TOKEN=<votre-token>",
		}
	}
}

// providerTokenEnvVar returns the primary environment variable name for the given provider's token.
func providerTokenEnvVar(provider string) string {
	guide, ok := findProviderGuide(provider)
	if !ok || len(guide.EnvVars) == 0 {
		return "STH_TOKEN"
	}

	return guide.EnvVars[0]
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
	if err != nil {
		if err == io.EOF && strings.TrimSpace(line) == "" {
			return "", io.EOF
		}
		if err != io.EOF {
			return "", fmt.Errorf("lecture de l'entrée utilisateur échouée : %w", err)
		}
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

// (promptFirstInitWelcome was the legacy y/N consent prompt. It has been
// replaced by promptTelemetryMode + promptCloudLoginIfNeeded which expose
// the 3-tier choice — see cmd/init_telemetry_mode.go and
// cmd/init_cloud_login.go.)

func countPrivateGitProviders(cfg fs.ProjectConfigV2) int {
	count := 0
	for _, provider := range cfg.Providers {
		if IsCloudProvider(provider.Provider) {
			continue
		}
		if provider.Private {
			count++
		}
	}
	return count
}

// alphaProviderGateActive reports whether the Alpha-period provider cap is
// currently enforced. The same escape hatch as the version gate (STH_ALPHA_GATE=0)
// disables it — keeping the two gates aligned avoids users having to remember
// two env vars.
func alphaProviderGateActive() bool {
	return strings.TrimSpace(os.Getenv(alphaGateSkipEnvVar)) != alphaProviderGateSkipValue
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
	if err != nil && err != io.EOF {
		return false, fmt.Errorf("lecture de l'entrée utilisateur échouée : %w", err)
	}
	trimmed := strings.ToLower(strings.TrimSpace(line))
	if trimmed == "" {
		return defaultYes, nil
	}

	return trimmed == "y" || trimmed == "yes" || trimmed == "o" || trimmed == "oui", nil
}
