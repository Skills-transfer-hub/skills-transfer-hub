package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"sth/internal/fs"
)

func (a *app) runProvider(args []string) error {
	if len(args) == 0 || isHelpFlag(args[0]) {
		a.writeStdout(commandHelpText("provider"))
		return nil
	}
	switch args[0] {
	case "list", "ls":
		return a.runProviderList()
	case "add":
		return a.runProviderAdd(args[1:])
	case "remove", "rm":
		return a.runProviderRemove(args[1:])
	default:
		return fmt.Errorf("unknown provider command: %q (expected list, add or remove)", args[0])
	}
}

func (a *app) runProviderList() error {
	cfg, err := fs.ReadProjectConfigV2ReadOnly()
	if err != nil {
		return err
	}
	if len(cfg.Providers) == 0 {
		a.writeStdout("No provider configured — run `sth init`.")
		return nil
	}

	defaultProviderID := fs.DefaultProviderID(cfg)
	installedCounts := a.installedCountByProvider(cfg)

	lines := []string{fmt.Sprintf("%d provider(s):", len(cfg.Providers))}
	for _, provider := range cfg.Providers {
		providerID := provider.ID
		marker := " "
		if providerID == defaultProviderID {
			marker = "*"
		}
		descriptor := provider.Provider
		if strings.TrimSpace(provider.RepositoryID) != "" {
			descriptor = fmt.Sprintf("%s %s", descriptor, provider.RepositoryID)
		}
		if strings.TrimSpace(provider.CatalogRef) != "" {
			descriptor = fmt.Sprintf("%s@%s", descriptor, provider.CatalogRef)
		}
		health := "ok"
		if _, profileErr := fs.ProviderProfile(cfg, providerID); profileErr != nil {
			health = "misconfigured"
		}
		lines = append(lines, fmt.Sprintf("%s %-16s %-40s %-13s %d resource(s)", marker, providerID, descriptor, health, installedCounts[providerID]))
	}
	lines = append(lines, "", "* default source when a resource exists in a single catalog or `--provider` is omitted.")
	a.writeStdout(lines...)
	return nil
}

func (a *app) installedCountByProvider(cfg fs.ProjectConfigV2) map[string]int {
	counts := make(map[string]int)
	state, err := fs.ReadInstalledStateAcrossDirs(fs.StateTargetDirs(cfg))
	if err != nil {
		return counts
	}
	seen := make(map[string]struct{})
	for _, record := range state.Entries {
		providerID := fs.RecordProviderID(cfg, record)
		key := providerID + "\x00" + record.ResourceName
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		counts[providerID]++
	}
	return counts
}

type providerAddOptions struct {
	id           string
	providerType string
	repositoryID string
	catalogRef   string
	catalogPath  string
	targetDir    string
	private      bool
	skipCheck    bool
}

func parseProviderAddOptions(args []string) (providerAddOptions, error) {
	options := providerAddOptions{}
	remaining := args
	var err error
	if options.providerType, remaining, err = parseValueFlag(remaining, "--type"); err != nil {
		return options, err
	}
	if options.repositoryID, remaining, err = parseValueFlag(remaining, "--repo"); err != nil {
		return options, err
	}
	if options.catalogRef, remaining, err = parseValueFlag(remaining, "--ref"); err != nil {
		return options, err
	}
	if options.catalogPath, remaining, err = parseValueFlag(remaining, "--catalog"); err != nil {
		return options, err
	}
	if options.targetDir, remaining, err = parseValueFlag(remaining, "--target-dir"); err != nil {
		return options, err
	}
	flags, positional := parseFlags(remaining, "--private", "--skip-check")
	options.private = flags["--private"]
	options.skipCheck = flags["--skip-check"]
	if len(positional) != 1 {
		return options, fmt.Errorf("usage: sth provider add <id> --type <provider> [--repo <repository>] [--ref <ref>] [--catalog <path>] [--target-dir <dir>] [--private] [--skip-check]")
	}
	options.id = strings.TrimSpace(positional[0])
	return options, nil
}

func (a *app) runProviderAdd(args []string) error {
	options, err := parseProviderAddOptions(args)
	if err != nil {
		return err
	}
	if options.id == "" {
		return fmt.Errorf("the provider identifier is required")
	}

	cfg, err := fs.ReadProjectConfigV2()
	if err != nil {
		return err
	}
	if aliasIsDuplicate(options.id, cfg) {
		return fmt.Errorf("provider %q already exists", options.id)
	}

	providerType := strings.ToLower(strings.TrimSpace(options.providerType))
	if providerType == "" {
		return fmt.Errorf("--type is required (supported: %s)", strings.Join(supportedProviderKeys(), ", "))
	}
	if _, known := findProviderGuide(providerType); !known {
		return fmt.Errorf("unsupported provider type: %q (supported: %s)", options.providerType, strings.Join(supportedProviderKeys(), ", "))
	}

	entry := fs.ProviderEntry{
		ID:          options.id,
		Provider:    providerType,
		CatalogRef:  strings.TrimSpace(options.catalogRef),
		CatalogPath: strings.TrimSpace(options.catalogPath),
		TargetDir:   strings.TrimSpace(options.targetDir),
		Private:     options.private,
	}
	if entry.TargetDir == "" {
		entry.TargetDir = fs.ProviderStateTargetDir(cfg, fs.DefaultProviderID(cfg))
	}

	if !fs.IsCloudProviderName(providerType) {
		if strings.TrimSpace(options.repositoryID) == "" {
			return fmt.Errorf("--repo is required for provider type %q", providerType)
		}
		repositoryID, normalizeErr := fs.NormalizeRepositoryID(providerType, options.repositoryID)
		if normalizeErr != nil {
			return normalizeErr
		}
		entry.RepositoryID = repositoryID
		if entry.CatalogRef == "" {
			entry.CatalogRef = "main"
		}
		if entry.CatalogPath == "" {
			entry.CatalogPath = "SKILLS.md"
		}
	}

	candidate := cfg
	candidate.Providers = append(append([]fs.ProviderEntry{}, cfg.Providers...), entry)
	profile, profileErr := fs.ProviderProfile(candidate, entry.ID)
	if profileErr != nil {
		return profileErr
	}

	if !options.skipCheck {
		if checkErr := a.withAuthRecovery(context.Background(), func() error {
			return a.client.CheckProjectAccess(context.Background(), profile)
		}); checkErr != nil {
			return fmt.Errorf("provider %q is not reachable: %w — re-run with --skip-check to add it anyway", entry.ID, checkErr)
		}
	}

	cfg.Providers = append(cfg.Providers, entry)
	path, writeErr := fs.WriteProjectConfigV2(cfg)
	if writeErr != nil {
		return writeErr
	}
	a.writeStdout(
		fmt.Sprintf("Provider '%s' added (%s).", entry.ID, path),
		fmt.Sprintf("Next step: `sth list` to browse its catalog, `sth install %s:<resource>` to install from it.", entry.ID),
	)
	return nil
}

func (a *app) runProviderRemove(args []string) error {
	flags, positional := parseFlags(args, "--force")
	if len(positional) != 1 {
		return fmt.Errorf("usage: sth provider remove <id> [--force]")
	}
	providerID := strings.TrimSpace(positional[0])

	cfg, err := fs.ReadProjectConfigV2()
	if err != nil {
		return err
	}
	if _, found := fs.ProviderByID(cfg, providerID); !found {
		return fmt.Errorf("unknown provider: %q (configured: %s)", providerID, strings.Join(fs.ProviderIDs(cfg), ", "))
	}

	installed := a.resourcesFromProvider(cfg, providerID)
	if len(installed) > 0 && !flags["--force"] {
		return fmt.Errorf("provider %q still owns %d installed resource(s): %s — remove them first or pass --force", providerID, len(installed), strings.Join(installed, ", "))
	}

	remaining := make([]fs.ProviderEntry, 0, len(cfg.Providers))
	for _, provider := range cfg.Providers {
		if provider.ID == providerID {
			continue
		}
		remaining = append(remaining, provider)
	}
	cfg.Providers = remaining

	path, writeErr := fs.WriteProjectConfigV2(cfg)
	if writeErr != nil {
		return writeErr
	}
	lines := []string{fmt.Sprintf("Provider '%s' removed (%s).", providerID, path)}
	if len(installed) > 0 {
		lines = append(lines, fmt.Sprintf("warning: %d resource(s) remain installed without a source: %s", len(installed), strings.Join(installed, ", ")))
	}
	a.writeStdout(lines...)
	return nil
}

func (a *app) resourcesFromProvider(cfg fs.ProjectConfigV2, providerID string) []string {
	state, err := fs.ReadInstalledStateAcrossDirs(fs.StateTargetDirs(cfg))
	if err != nil {
		return nil
	}
	names := make([]string, 0)
	seen := make(map[string]struct{})
	for _, record := range state.Entries {
		if fs.RecordProviderID(cfg, record) != providerID {
			continue
		}
		if _, exists := seen[record.ResourceName]; exists {
			continue
		}
		seen[record.ResourceName] = struct{}{}
		names = append(names, record.ResourceName)
	}
	sort.Strings(names)
	return names
}
