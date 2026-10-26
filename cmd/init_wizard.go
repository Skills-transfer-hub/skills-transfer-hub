package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"sth/internal/fs"
	sthhttp "sth/internal/http"
	"sth/internal/tui"
	"sth/internal/writer"
)

type wizardProfile struct {
	Alias        string
	Provider     string
	RepositoryID string
	Ref          string
	CatalogPath  string
	TargetDir    string
	Targets      []string
	IsPrivate    bool
	Token        string
	TokenSource  string
}

func (p wizardProfile) tokenLabel() string {
	switch {
	case p.TokenSource == "":
		return "none"
	case strings.HasPrefix(p.TokenSource, "env:"):
		return fmt.Sprintf("from $%s", strings.TrimPrefix(p.TokenSource, "env:"))
	default:
		return "entered for this session"
	}
}

func (a *app) narrateMood(session *tui.Session, mood tui.Mood, lines ...string) {
	maxWidth := 0
	if session != nil {
		maxWidth = session.MaxWidth
	}
	a.writeStdout(tui.RenderNarrator(mood, maxWidth, lines...))
}

var providerKeysOrdered = []string{"github", "gitlab", "azure-devops", "bitbucket", "sth-cloud"}

func providerSelectOptions() []tui.Option {
	labels := map[string]string{
		"github":       "GitHub",
		"gitlab":       "GitLab",
		"azure-devops": "Azure DevOps",
		"bitbucket":    "Bitbucket",
		"sth-cloud":    "STH Cloud (coming soon — Alpha)",
	}
	options := make([]tui.Option, 0, len(providerKeysOrdered))
	for _, key := range providerKeysOrdered {
		options = append(options, tui.Option{Label: labels[key], Aliases: []string{key}})
	}
	return options
}

func indexOfProvider(options []tui.Option, slug string) int {
	for index, option := range options {
		for _, alias := range option.Aliases {
			if strings.EqualFold(alias, slug) {
				return index
			}
		}
	}
	return -1
}

func (a *app) runInitWizard(session *tui.Session, existingCfg fs.ProjectConfigV2, defaults fs.ProjectProfile, assignedAlias string) error {
	a.narrateMood(session, tui.MoodHello,
		"Hi! We will connect your skills catalog",
		"in a few questions. You can fix everything",
		"before I write anything. (Esc to cancel.)",
	)

	draft := wizardProfile{
		Alias:        assignedAlias,
		Provider:     defaults.Provider,
		RepositoryID: defaults.RepositoryID,
		Ref:          defaults.Ref,
		CatalogPath:  defaults.CatalogPath,
		TargetDir:    defaults.TargetDir,
		IsPrivate:    true,
	}

	if assignedAlias == "" {
		if err := a.askAlias(session, &draft, existingCfg); err != nil {
			return err
		}
	}

	if err := a.askProvider(session, &draft); err != nil {
		return err
	}

	if err := a.askRepository(session, &draft); err != nil {
		return err
	}

	if err := a.askCatalogSettings(session, &draft); err != nil {
		return err
	}

	if err := a.askTargets(session, &draft, existingCfg); err != nil {
		return err
	}

	if err := a.askAccessAndToken(session, &draft, existingCfg); err != nil {
		return err
	}

	confirmed, err := a.recapAndConfirm(session, &draft, existingCfg)
	if err != nil {
		return err
	}
	if !confirmed {
		a.narrateMood(session, tui.MoodSad, "OK, setup cancelled.")
		return nil
	}

	return a.writeAndValidate(draft)
}

func aliasIsDuplicate(alias string, existingCfg fs.ProjectConfigV2) bool {
	for _, provider := range existingCfg.Providers {
		if provider.ID == alias {
			return true
		}
	}
	return false
}

func aliasValidationMessage(alias string) string {
	if strings.TrimSpace(alias) == "" {
		return "Alias cannot be empty."
	}
	return "Alias may contain only letters, numbers, hyphens, and underscores."
}

func (a *app) askAlias(session *tui.Session, draft *wizardProfile, existingCfg fs.ProjectConfigV2) error {
	a.narrateMood(session, tui.MoodThinking,
		fmt.Sprintf("You already have %d configured provider(s).", len(existingCfg.Providers)),
		"Give it a short alias (e.g. work, oss).",
	)
	for {
		alias, err := tui.Input(session, tui.InputConfig{Label: "Alias", Required: true})
		if err != nil {
			return err
		}
		if validateErr := fs.ValidateProviderID(alias); validateErr != nil {
			a.narrateMood(session, tui.MoodWorried, aliasValidationMessage(alias))
			continue
		}
		if aliasIsDuplicate(alias, existingCfg) {
			a.narrateMood(session, tui.MoodWorried, fmt.Sprintf("Alias %q is already in use. Pick another one.", alias))
			continue
		}
		draft.Alias = alias
		return nil
	}
}

func (a *app) askProvider(session *tui.Session, draft *wizardProfile) error {
	options := providerSelectOptions()
	recommended := indexOfProvider(options, draft.Provider)
	for {
		a.narrateMood(session, tui.MoodHello, "Where is your catalog hosted?")
		index, err := tui.Select(session, tui.SelectConfig{
			Title:       "Provider",
			Options:     options,
			Recommended: max(recommended, 0),
		})
		if err != nil {
			return err
		}
		chosen := providerKeysOrdered[index]
		if IsCloudProvider(chosen) {
			a.narrateMood(session, tui.MoodWorried,
				"STH Cloud repositories are not available during Alpha yet.",
				"Join the waitlist: " + DashboardPricingURL(),
				"For now, choose a Git provider below.",
			)
			continue
		}
		draft.Provider = chosen
		return nil
	}
}

func (a *app) askRepository(session *tui.Session, draft *wizardProfile) error {
	a.narrateMood(session, tui.MoodHello,
		"Give me the repository identifier.",
		"Paste a URL (e.g. https://github.com/owner/repo)",
		"or just owner/repo — I will clean it up.",
	)
	for {
		value, err := tui.Input(session, tui.InputConfig{
			Label:    "Repository (URL or owner/repo)",
			Default:  draft.RepositoryID,
			Required: true,
		})
		if err != nil {
			return err
		}
		normalizedValue, normalizeErr := fs.NormalizeRepositoryID(draft.Provider, value)
		if normalizeErr != nil {
			a.writeStdout(
				"Invalid repository identifier.",
				fmt.Sprintf("Expected format: %s", providerRepositoryIDFormat(draft.Provider)),
				fmt.Sprintf("Example: %s", providerRepositoryIDExample(draft.Provider)),
			)
			if hostedDomain := providerHostedDomain(draft.Provider); hostedDomain != "" {
				a.writeStdout(fmt.Sprintf("Supported hosted domain: %s", hostedDomain))
			}
			continue
		}
		draft.RepositoryID = normalizedValue
		return nil
	}
}

func (a *app) askCatalogSettings(session *tui.Session, draft *wizardProfile) error {
	a.narrateMood(session, tui.MoodHello,
		"Two quick settings.",
		"Press Enter to accept defaults.",
	)
	ref, err := tui.Input(session, tui.InputConfig{Label: "Git reference", Default: draft.Ref, Required: true})
	if err != nil {
		return err
	}
	draft.Ref = ref
	catalog, err := tui.Input(session, tui.InputConfig{Label: "Catalog path", Default: draft.CatalogPath, Required: true})
	if err != nil {
		return err
	}
	draft.CatalogPath = catalog
	return nil
}

func accessSelectOptions() []tui.Option {
	return []tui.Option{
		{Label: "Private — requires an access token", Aliases: []string{"privé", "prive", "private", "p"}},
		{Label: "Public — anonymous read access", Aliases: []string{"public", "pub"}},
	}
}

func (a *app) askAccessAndToken(session *tui.Session, draft *wizardProfile, existingCfg fs.ProjectConfigV2) error {
	for {
		a.narrateMood(session, tui.MoodHello, "Is the repository public or private?")
		accessIndex, err := tui.Select(session, tui.SelectConfig{
			Title:       "Repository access type",
			Options:     accessSelectOptions(),
			Recommended: accessRecommendedIndex(draft.IsPrivate),
		})
		if err != nil {
			return err
		}
		draft.IsPrivate = accessIndex == 0

		if draft.IsPrivate && alphaProviderGateActive() && countPrivateGitProviders(existingCfg) >= 1 {
			a.narrateMood(session, tui.MoodWorried,
				"During Alpha, a project can reference only one private repository.",
				"Choose public for this repository, or bypass at your own risk with STH_ALPHA_GATE=0.",
				fmt.Sprintf("Details: %s", AlphaLimitsDocsURL()),
			)
			continue
		}
		break
	}

	if !draft.IsPrivate {
		draft.Token = ""
		draft.TokenSource = ""
		return nil
	}
	a.writeStdout(privateCreationGuide(draft.Provider)...)

	if envVar, value := detectProviderToken(draft.Provider); value != "" {
		a.narrateMood(session, tui.MoodHappy,
			fmt.Sprintf("I found a token in $%s.", envVar),
			"I will use it for the access check.",
		)
		draft.Token = ""
		draft.TokenSource = "env:" + envVar
		return nil
	}

	guide, _ := findProviderGuide(draft.Provider)
	a.narrateMood(session, tui.MoodThinking, shortPrivateGuidance(draft.Provider, guide)...)
	expand, err := tui.Confirm(session, tui.ConfirmConfig{Question: "Show full details?", DefaultYes: false})
	if err != nil {
		return err
	}
	if expand {
		a.writeStdout(providerPrivateAccessGuidance(draft.Provider)...)
	}

	secretSession, closeSecretSession := a.secureSecretSession(session)
	defer closeSecretSession()
	token, err := tui.Secret(secretSession, tui.SecretConfig{
		Label: "Token (optional)",
		Hint:  "⚠ Input is hidden. Leave empty to skip.",
	})
	if err != nil {
		return err
	}
	if token == "" {
		draft.Token = ""
		draft.TokenSource = ""
	} else {
		draft.Token = token
		draft.TokenSource = "manual"
	}
	return nil
}

func accessRecommendedIndex(isPrivate bool) int {
	if isPrivate {
		return 0
	}
	return 1
}

func shortPrivateGuidance(provider string, guide providerGuide) []string {
	if guide.ActionURL == "" {
		return []string{
			"A private repository requires an access token.",
			"Set $STH_TOKEN or enter a token below.",
		}
	}
	return []string{
		fmt.Sprintf("Generate a PAT for a private %s repository:", strings.ToUpper(provider)),
		"→ " + guide.ActionURL,
		"→ permission \"" + guide.MinimumPermission + "\"",
	}
}

func detectProviderToken(provider string) (string, string) {
	if tokenValue := strings.TrimSpace(os.Getenv("STH_TOKEN")); tokenValue != "" {
		return "STH_TOKEN", tokenValue
	}
	guide, ok := findProviderGuide(provider)
	if !ok {
		return "", ""
	}
	for _, name := range guide.EnvVars {
		if name == "STH_TOKEN" {
			continue
		}
		if envValue := strings.TrimSpace(os.Getenv(name)); envValue != "" {
			return name, envValue
		}
	}
	return "", ""
}

func (a *app) recapAndConfirm(session *tui.Session, draft *wizardProfile, existingCfg fs.ProjectConfigV2) (bool, error) {
	for {
		a.narrateMood(session, tui.MoodHello,
			"Here is what I am about to save.",
			"Review and confirm.",
		)
		a.writeStdout(
			fmt.Sprintf("  Alias         : %s", draft.Alias),
			fmt.Sprintf("  Provider      : %s", draft.Provider),
			fmt.Sprintf("  Repository    : %s", draft.RepositoryID),
			fmt.Sprintf("  Git ref       : %s", draft.Ref),
			fmt.Sprintf("  Catalog       : %s", draft.CatalogPath),
			fmt.Sprintf("  Targets       : %s", strings.Join(draft.Targets, ", ")),
			fmt.Sprintf("  Access        : %s", accessLabel(draft.IsPrivate)),
			fmt.Sprintf("  Token         : %s", draft.tokenLabel()),
			"",
		)
		action, err := tui.Select(session, tui.SelectConfig{
			Title: "Save this configuration?",
			Options: []tui.Option{
				{Label: "Save this configuration", Aliases: []string{"y", "yes", "o", "oui"}},
				{Label: "Edit a field", Aliases: []string{"edit", "e"}},
				{Label: "Abort", Aliases: []string{"n", "no", "non"}},
			},
		})
		if err != nil {
			return false, err
		}
		switch action {
		case 0:
			return true, nil
		case 1:
			if editErr := a.editDraftField(session, draft, existingCfg); editErr != nil {
				return false, editErr
			}
		default:
			return false, nil
		}
	}
}

func recapFieldOptions() []tui.Option {
	return []tui.Option{
		{Label: "Alias", Aliases: []string{"alias"}},
		{Label: "Provider", Aliases: []string{"provider"}},
		{Label: "Repository", Aliases: []string{"dépôt", "depot", "repository"}},
		{Label: "Git reference", Aliases: []string{"ref"}},
		{Label: "Catalog path", Aliases: []string{"catalogue", "catalog"}},
		{Label: "Targets", Aliases: []string{"cibles", "targets"}},
		{Label: "Access and token", Aliases: []string{"accès", "acces", "access", "token"}},
	}
}

func (a *app) editDraftField(session *tui.Session, draft *wizardProfile, existingCfg fs.ProjectConfigV2) error {
	a.narrateMood(session, tui.MoodThinking, "Which field do you want to edit?")
	fieldIndex, err := tui.Select(session, tui.SelectConfig{
		Title:   "Field to edit",
		Options: recapFieldOptions(),
	})
	if err != nil {
		return err
	}
	switch fieldIndex {
	case 0:
		newAlias, err := tui.Input(session, tui.InputConfig{Label: "Alias", Default: draft.Alias, Required: true})
		if err != nil {
			return err
		}
		if validateErr := fs.ValidateProviderID(newAlias); validateErr != nil {
			a.narrateMood(session, tui.MoodWorried, aliasValidationMessage(newAlias))
			return nil
		}
		if newAlias != draft.Alias && aliasIsDuplicate(newAlias, existingCfg) {
			a.narrateMood(session, tui.MoodWorried, fmt.Sprintf("Alias %q is already in use. Alias unchanged.", newAlias))
			return nil
		}
		draft.Alias = newAlias
	case 1:
		return a.askProvider(session, draft)
	case 2:
		return a.askRepository(session, draft)
	case 3:
		ref, err := tui.Input(session, tui.InputConfig{Label: "Git reference", Default: draft.Ref, Required: true})
		if err != nil {
			return err
		}
		draft.Ref = ref
	case 4:
		catalog, err := tui.Input(session, tui.InputConfig{Label: "Catalog path", Default: draft.CatalogPath, Required: true})
		if err != nil {
			return err
		}
		draft.CatalogPath = catalog
	case 5:
		return a.askTargets(session, draft, existingCfg)
	default:
		return a.askAccessAndToken(session, draft, existingCfg)
	}
	return nil
}

func accessLabel(private bool) string {
	if private {
		return "private"
	}
	return "public"
}

func (a *app) askTargets(session *tui.Session, draft *wizardProfile, existingCfg fs.ProjectConfigV2) error {
	specs := writer.AllTargetSpecs()
	preselected := preselectedKinds(draft, existingCfg)

	options := make([]tui.Option, 0, len(specs))
	for _, spec := range specs {
		options = append(options, tui.Option{Label: spec.DisplayName, Aliases: []string{spec.Kind}})
	}
	selectedIndices := make([]int, 0, len(preselected))
	for _, kind := range preselected {
		for index, spec := range specs {
			if spec.Kind == kind {
				selectedIndices = append(selectedIndices, index)
				break
			}
		}
	}

	a.narrateMood(session, tui.MoodHello,
		"Which assistants do you target?",
		"Ones detected in this project are pre-checked.",
		"Enter keeps selection; typing \"1,3\", names or \"all\" also works.",
	)

	indices, err := tui.MultiSelect(session, tui.MultiSelectConfig{
		Title:    "Targets to generate",
		Options:  options,
		Selected: selectedIndices,
	})
	if err != nil {
		return err
	}
	selected := make([]string, 0, len(indices))
	for _, index := range indices {
		selected = append(selected, specs[index].Kind)
	}
	if len(selected) == 0 {
		selected = []string{writer.KindClaude}
	}
	draft.Targets = selected
	return nil
}

func preselectedKinds(draft *wizardProfile, existingCfg fs.ProjectConfigV2) []string {
	if len(draft.Targets) > 0 {
		return draft.Targets
	}
	if len(existingCfg.Targets) > 0 {
		kinds := make([]string, 0, len(existingCfg.Targets))
		for _, target := range existingCfg.Targets {
			kinds = append(kinds, target.Kind)
		}
		return kinds
	}
	workingDir, err := os.Getwd()
	if err != nil {
		return []string{writer.KindClaude}
	}
	detected := writer.DetectAvailableTargets(workingDir)
	kinds := make([]string, 0, len(detected))
	for _, target := range detected {
		kinds = append(kinds, target.Kind)
	}
	if len(kinds) == 0 {
		return []string{writer.KindClaude}
	}
	return kinds
}

func targetEntriesFromKinds(kinds []string) []fs.TargetEntry {
	entries := make([]fs.TargetEntry, 0, len(kinds))
	for _, kind := range kinds {
		rootDir := kind
		if spec, ok := writer.SpecForKind(kind); ok {
			rootDir = spec.RootDir
		}
		entries = append(entries, fs.TargetEntry{ID: kind, Kind: kind, RootDir: rootDir, Enabled: true})
	}
	return entries
}

func (a *app) validateInitDraft(validationContext context.Context, draft wizardProfile) error {
	profile := fs.ProjectProfile{
		Provider:     draft.Provider,
		RepositoryID: draft.RepositoryID,
		Ref:          draft.Ref,
		CatalogPath:  draft.CatalogPath,
		TargetDir:    draft.TargetDir,
		APIToken:     draft.Token,
	}
	return a.client.CheckProjectAccess(validationContext, profile)
}

func (a *app) writeInitDraft(draft wizardProfile, validationErr error) error {
	lines, err := a.collectInitDraftOutput(draft, validationErr)
	if len(lines) > 0 {
		a.writeStdout(lines...)
	}
	return err
}

func (a *app) collectInitDraftOutput(draft wizardProfile, validationErr error) ([]string, error) {
	currentCfg, err := fs.ReadProjectConfigV2()
	if err != nil {
		return nil, newInitUserError("could not reload the project configuration", err)
	}
	if aliasIsDuplicate(draft.Alias, currentCfg) {
		return nil, fmt.Errorf("alias %q was configured while setup was running; restart setup", draft.Alias)
	}
	entry := fs.ProviderEntry{
		ID:           draft.Alias,
		Provider:     draft.Provider,
		RepositoryID: draft.RepositoryID,
		CatalogRef:   draft.Ref,
		CatalogPath:  draft.CatalogPath,
		TargetDir:    draft.TargetDir,
		Private:      draft.IsPrivate,
	}
	currentCfg.Providers = append(currentCfg.Providers, entry)
	currentCfg.Targets = targetEntriesFromKinds(draft.Targets)

	path, err := fs.WriteProjectConfigV2(currentCfg)
	if err != nil {
		return nil, newInitUserError("could not save the project configuration", err)
	}
	a.recordOperationResult("init", draft.Alias, validationErr)
	lines := []string{fmt.Sprintf("✓ Configuration saved: %s", path)}
	lines = append(lines, providerAddedLines(currentCfg, entry)...)
	if workingDir, wdErr := os.Getwd(); wdErr == nil {
		if changed, gitErr := fs.EnsureManagedGitignore(workingDir); gitErr == nil && changed {
			lines = append(lines, "✓ .gitignore updated (STH state files ignored)")
		}
	}

	if validationErr != nil {
		diagnosis, failureLines := validationFailureOutput(draft, validationErr)
		lines = append(lines, failureLines...)
		message := fmt.Sprintf("remote access validation failed (%s): %s", diagnosis.Kind, diagnosis.Summary)
		return lines, newInitUserError(message, validationErr)
	}
	lines = append(lines, validationSuccessOutput(draft)...)
	return lines, nil
}

func (a *app) writeAndValidate(draft wizardProfile) error {
	validationErr := a.validateInitDraft(context.Background(), draft)
	return a.writeInitDraft(draft, validationErr)
}

func (a *app) renderValidationSuccess(draft wizardProfile) {
	a.writeStdout(validationSuccessOutput(draft)...)
}

func validationSuccessOutput(draft wizardProfile) []string {
	lines := []string{tui.RenderNarrator(tui.MoodHappy, 0,
		"All good! The catalog is reachable.",
		"Next step: run `sth list` to browse skills.",
	)}
	if draft.TokenSource == "manual" {
		envVar := providerTokenEnvVar(draft.Provider)
		lines = append(lines,
			"",
			"The token was used for validation but was not saved.",
			"Set it as an environment variable for future runs:",
			fmt.Sprintf("  export %s=<your-token>", envVar),
		)
	}
	return lines
}

func (a *app) renderValidationFailure(draft wizardProfile, err error) validationDiagnosis {
	diagnosis, lines := validationFailureOutput(draft, err)
	a.writeStdout(lines...)
	return diagnosis
}

func validationFailureOutput(draft wizardProfile, err error) (validationDiagnosis, []string) {
	diagnosis := classifyValidationError(err, draft)
	lines := []string{tui.RenderNarrator(tui.MoodError, 0,
		"Config written, but I could not validate",
		"remote repository access.",
	),
		"⚠ Config written but not validated.",
		"",
		fmt.Sprintf("  Detected cause : %s", diagnosis.Kind),
		fmt.Sprintf("  Details        : %s", diagnosis.Summary),
		"",
		"  How to fix it:",
	}
	for index, step := range diagnosis.Remediation {
		lines = append(lines, fmt.Sprintf("    %d. %s", index+1, step))
	}
	lines = append(lines,
		"",
		"  → Run the check again: `sth doctor`",
	)
	return diagnosis, lines
}

type validationDiagnosis struct {
	Kind        string
	Summary     string
	Remediation []string
}

func classifyValidationError(err error, draft wizardProfile) validationDiagnosis {
	guide, _ := findProviderGuide(draft.Provider)
	envVar := providerTokenEnvVar(draft.Provider)

	if _, ok := sthhttp.IsAuthError(err); ok {
		return validationDiagnosis{
			Kind:    "auth_failed",
			Summary: "The server rejected the token (HTTP 401/403).",
			Remediation: []string{
				"Check that the token has not expired.",
				fmt.Sprintf("Confirm the \"%s\" permission.", guide.MinimumPermission),
				fmt.Sprintf("Generate a new token at %s", guide.ActionURL),
				fmt.Sprintf("export %s=<new-token>", envVar),
			},
		}
	}

	msg := err.Error()
	switch {
	case strings.Contains(strings.ToLower(msg), "bundle"):
		return validationDiagnosis{
			Kind:    "catalog_empty",
			Summary: "The repository is reachable but contains no recognizable catalog or bundle.",
			Remediation: []string{
				fmt.Sprintf("Check that `%s` exists at ref `%s`, or that a directory follows the category/folder/SKILL.md convention.", draft.CatalogPath, draft.Ref),
				"Tip: if the catalog is elsewhere, edit `catalog_path` in `.sth/project.json`.",
			},
		}
	case strings.Contains(msg, "HTTP 404"):
		return validationDiagnosis{
			Kind:    "not_found",
			Summary: "The repository, branch, or catalog file does not exist (HTTP 404).",
			Remediation: []string{
				fmt.Sprintf("Check the spelling of `%s` and that branch `%s` exists.", draft.RepositoryID, draft.Ref),
				fmt.Sprintf("Check that `%s` exists at this location in the repository.", draft.CatalogPath),
				"A private repository without a token can also look like a 404. Provide a token and try again.",
			},
		}
	default:
		return validationDiagnosis{
			Kind:    "network",
			Summary: "Could not reach the remote server.",
			Remediation: []string{
				"Check your network connection.",
				"If you use a proxy, export HTTPS_PROXY.",
			},
		}
	}
}
