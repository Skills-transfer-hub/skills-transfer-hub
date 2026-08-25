package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"sth/internal/fs"
	"sth/internal/tui"
)

const alphaProviderGateSkipValue = "0"

type initUserError struct {
	message string
	cause   error
}

func (userErr initUserError) Error() string {
	return userErr.message
}

func (userErr initUserError) Unwrap() error {
	return userErr.cause
}

func newInitUserError(message string, cause error) error {
	return initUserError{message: message, cause: cause}
}

func (a *app) runInit(args ...string) error {
	skipCloudPrompt := false
	for _, arg := range args {
		if arg == "--no-cloud-prompt" {
			skipCloudPrompt = true
		}
	}

	session := tui.NewSession(a.stdin, a.stdout)
	defer session.Close()

	existingCfg, err := fs.ReadProjectConfigV2()
	if err != nil {
		return newInitUserError("could not read the project configuration", err)
	}
	legacyConfig, err := fs.ReadConfigIfPresent()
	if err != nil {
		return newInitUserError("could not read the legacy configuration", err)
	}

	defaults := fs.ProjectProfile{
		Provider:    "github",
		Ref:         "main",
		CatalogPath: "SKILLS.md",
		TargetDir:   ".sth",
	}

	assignedAlias := "default"
	if len(existingCfg.Providers) > 0 {
		assignedAlias = ""
	}

	var legacyProfile *fs.ProjectProfile
	if len(existingCfg.Providers) == 0 && legacyConfig != nil {
		if legacyProfileValue, legacyErr := fs.LegacyProfileFromConfig(*legacyConfig); legacyErr == nil {
			legacyProfile = legacyProfileValue
		}
	}

	if session.Interactive {
		flow := newInitFlowModel(a, session.MaxWidth, skipCloudPrompt, existingCfg, defaults, assignedAlias, legacyProfile)
		finalModel, programErr := session.RunProgram(flow)
		if programErr != nil {
			return programErr
		}
		result := finalModel.(initFlowModel)
		if len(result.resultLines) > 0 {
			a.writeStdout(result.resultLines...)
		}
		if errors.Is(result.flowErr, tui.ErrAborted) {
			a.writeStdout("Setup cancelled.")
			return nil
		}
		return result.flowErr
	}

	if !fs.HasTelemetryDecision() {
		mode, err := a.promptTelemetryMode(session)
		if err != nil {
			return a.mapInitAbort(err)
		}
		if !skipCloudPrompt {
			a.promptCloudLoginIfNeeded(session, mode)
		}
	}

	if len(existingCfg.Providers) > 0 {
		a.writeStdout(fmt.Sprintf("Existing project configuration: %d provider(s):", len(existingCfg.Providers)))
		for _, provider := range existingCfg.Providers {
			a.writeStdout(fmt.Sprintf("- [%s] %s: %s @ %s", provider.ID, provider.Provider, provider.RepositoryID, provider.CatalogRef))
		}
		addNew, promptErr := tui.Confirm(session, tui.ConfirmConfig{Question: "Add a new provider?", DefaultYes: false})
		if promptErr != nil {
			return a.mapInitAbort(promptErr)
		}
		if !addNew {
			a.writeStdout("Project configuration kept.", "Next step: run `sth list`.")
			return nil
		}
	}

	if legacyProfile != nil {
		shouldImport, promptErr := tui.Confirm(session, tui.ConfirmConfig{Question: "Import legacy ~/.sthrc configuration?", DefaultYes: true})
		if promptErr != nil {
			return a.mapInitAbort(promptErr)
		}
		if shouldImport {
			defaults = *legacyProfile
		}
	}

	if err := a.runInitWizard(session, existingCfg, defaults, assignedAlias); err != nil {
		return a.mapInitAbort(err)
	}
	return nil
}

func (a *app) mapInitAbort(err error) error {
	if errors.Is(err, tui.ErrAborted) {
		a.writeStdout("Setup cancelled.")
		return nil
	}
	return err
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
		return []string{"Public repository", "GitHub authentication: none is required for a public repository with anonymous read access."}
	case "gitlab":
		return []string{"Public repository", "GitLab authentication: none is required for a public repository with anonymous read access."}
	case "azure-devops":
		return []string{"Public repository", "Azure DevOps authentication: a public repository may be readable without a secret; otherwise provide an Azure DevOps PAT."}
	case "bitbucket":
		return []string{"Public repository", "Bitbucket authentication: none is required for a public repository with anonymous read access."}
	default:
		return []string{"Public repository", "Anonymous access may be enough for a public repository; otherwise configure authentication for this provider."}
	}
}

func providerPrivateAccessGuidance(provider string) []string {
	guide, ok := findProviderGuide(provider)
	if !ok {
		return []string{
			"Private repository — authentication required",
			"",
			"Generate an access token in your provider, then set:",
			"  export STH_TOKEN=<your-token>",
		}
	}

	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "github":
		return []string{
			"Private repository — authentication required",
			"",
			"You need a Personal Access Token (PAT) to access this private GitHub repository.",
			"",
			"Generate your token:",
			"  1. Open: " + guide.ActionURL,
			"  2. Select \"Fine-grained token\" (recommended) or \"Classic token\"",
			"     Fine-grained: grant \"" + guide.MinimumPermission + "\" on the target repository",
			"     Classic: enable \"repo\" for full access to private repositories",
			"  3. Click \"Generate token\" and copy the displayed value",
			"",
			"Set an environment variable for future runs:",
			"  export GITHUB_TOKEN=<your-token>",
			"  or STH_TOKEN=<your-token>   (works with every provider)",
		}
	case "gitlab":
		return []string{
			"Private repository — authentication required",
			"",
			"You need a Personal Access Token to access this private GitLab repository.",
			"",
			"Generate your token:",
			"  1. Open: " + guide.ActionURL,
			"  2. Name the token and choose an expiration date",
			"  3. Enable the permission: \"" + guide.MinimumPermission + "\"",
			"  4. Click \"Create personal access token\" and copy the displayed value",
			"",
			"Set an environment variable for future runs:",
			"  export GITLAB_TOKEN=<your-token>",
			"  or STH_TOKEN=<your-token>   (works with every provider)",
		}
	case "azure-devops":
		return []string{
			"Private repository — authentication required",
			"",
			"You need a Personal Access Token (PAT) to access this private Azure DevOps repository.",
			"",
			"Generate your token:",
			"  1. Sign in at: https://dev.azure.com/<your-organization>",
			"  2. Click your avatar in the upper-right corner, then \"Personal access tokens\"",
			"     Direct link: " + guide.ActionURL,
			"  3. Click \"New Token\" and select the permission: \"" + guide.MinimumPermission + "\"",
			"  4. Click \"Create\" and copy the displayed value",
			"",
			"Set an environment variable for future runs:",
			"  export AZURE_DEVOPS_EXT_PAT=<your-token>",
			"  or STH_TOKEN=<your-token>   (works with every provider)",
		}
	case "bitbucket":
		return []string{
			"Private repository — authentication required",
			"",
			"You need an Atlassian API token to access this private Bitbucket Cloud repository.",
			"",
			"Official documentation: " + guide.DocumentationURL,
			"Atlassian account page: " + guide.ActionURL,
			"Minimum required permission: \"" + guide.MinimumPermission + "\"",
			"",
			"Set an environment variable for future runs:",
			"  export BITBUCKET_TOKEN=<your-token>",
			"  or STH_TOKEN=<your-token>   (works with every provider)",
		}
	default:
		return []string{
			"Private repository — authentication required",
			"",
			"Generate an access token in your provider, then set:",
			"  export STH_TOKEN=<your-token>",
		}
	}
}

func providerTokenEnvVar(provider string) string {
	guide, ok := findProviderGuide(provider)
	if !ok || len(guide.EnvVars) == 0 {
		return "STH_TOKEN"
	}

	return guide.EnvVars[0]
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

func alphaProviderGateActive() bool {
	return strings.TrimSpace(os.Getenv(alphaGateSkipEnvVar)) != alphaProviderGateSkipValue
}
