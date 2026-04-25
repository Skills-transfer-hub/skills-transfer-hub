package cmd

import (
	"sort"
	"strings"
)

type providerGuide struct {
	Key                 string
	DisplayName         string
	HostedDomain        string
	DocumentationURL    string
	ActionURL           string
	CredentialKind      string
	MinimumPermission   string
	RepositoryIDFormat  string
	RepositoryIDExample string
	EnvVars             []string
	Notes               []string
}

var providerGuides = map[string]providerGuide{
	"github": {
		Key:                 "github",
		DisplayName:         "GitHub",
		HostedDomain:        "github.com",
		DocumentationURL:    "https://docs.github.com/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens",
		ActionURL:           "https://github.com/settings/personal-access-tokens/new",
		CredentialKind:      "fine-grained-pat",
		MinimumPermission:   "Contents: Read",
		RepositoryIDFormat:  "owner/repo",
		RepositoryIDExample: "octo-org/skills-catalog",
		EnvVars:             []string{"GITHUB_TOKEN", "STH_TOKEN"},
	},
	"gitlab": {
		Key:                 "gitlab",
		DisplayName:         "GitLab",
		HostedDomain:        "gitlab.com",
		DocumentationURL:    "https://docs.gitlab.com/user/profile/personal_access_tokens/",
		ActionURL:           "https://gitlab.com/-/user_settings/personal_access_tokens",
		CredentialKind:      "pat",
		MinimumPermission:   "read_repository",
		RepositoryIDFormat:  "group/project ou group/subgroup/project",
		RepositoryIDExample: "plateforme/ia/skills-catalog",
		EnvVars:             []string{"GITLAB_TOKEN", "STH_TOKEN"},
	},
	"azure-devops": {
		Key:                 "azure-devops",
		DisplayName:         "Azure DevOps",
		HostedDomain:        "dev.azure.com",
		DocumentationURL:    "https://learn.microsoft.com/azure/devops/organizations/accounts/use-personal-access-tokens-to-authenticate",
		ActionURL:           "https://dev.azure.com/<organisation>/_usersSettings/tokens",
		CredentialKind:      "pat",
		MinimumPermission:   "Code (Read)",
		RepositoryIDFormat:  "organisation/projet/repo",
		RepositoryIDExample: "mon-org/mon-projet/skills-catalog",
		EnvVars:             []string{"AZURE_DEVOPS_EXT_PAT", "AZURE_DEVOPS_TOKEN", "STH_TOKEN"},
	},
	"bitbucket": {
		Key:                 "bitbucket",
		DisplayName:         "Bitbucket Cloud",
		HostedDomain:        "bitbucket.org",
		DocumentationURL:    "https://support.atlassian.com/bitbucket-cloud/docs/create-an-api-token/",
		ActionURL:           "https://id.atlassian.com/manage-profile/security/api-tokens",
		CredentialKind:      "api-token",
		MinimumPermission:   "Repositories: Read",
		RepositoryIDFormat:  "workspace/repo",
		RepositoryIDExample: "mon-workspace/skills-catalog",
		EnvVars:             []string{"BITBUCKET_TOKEN", "STH_TOKEN"},
		Notes:               []string{"Utiliser un API token Atlassian pour Bitbucket Cloud."},
	},
}

func supportedProviders() []providerGuide {
	providers := make([]providerGuide, 0, len(providerGuides))
	for _, guide := range providerGuides {
		providers = append(providers, guide)
	}

	sort.Slice(providers, func(i, j int) bool {
		return providers[i].Key < providers[j].Key
	})

	return providers
}

func supportedProviderKeys() []string {
	providers := supportedProviders()
	keys := make([]string, 0, len(providers))
	for _, guide := range providers {
		keys = append(keys, guide.Key)
	}

	return keys
}

func findProviderGuide(provider string) (providerGuide, bool) {
	guide, ok := providerGuides[strings.ToLower(strings.TrimSpace(provider))]
	return guide, ok
}

func providerHostedDomain(provider string) string {
	guide, ok := findProviderGuide(provider)
	if !ok {
		return ""
	}

	return guide.HostedDomain
}

func isSupportedHostedDomain(provider, host string) bool {
	guide, ok := findProviderGuide(provider)
	if !ok {
		return false
	}

	return strings.EqualFold(strings.TrimSpace(host), guide.HostedDomain)
}