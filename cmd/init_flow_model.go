package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbletea"

	"sth/internal/auth"
	"sth/internal/fs"
	"sth/internal/tui"
	"sth/internal/writer"
)

type initStepKind int

type legacyImportState int

const (
	stepTelemetry initStepKind = iota
	stepExistingCloudKeyChecking
	stepCloudLoginAsk
	stepCloudKey
	stepCloudKeyChecking
	stepCloudKeySaving
	stepAddProviderConfirm
	stepLegacyImportConfirm
	stepAlias
	stepProvider
	stepRepo
	stepRef
	stepCatalogPath
	stepTargets
	stepAccess
	stepPrivateDetailsAsk
	stepToken
	stepRecap
	stepEditField
	stepValidating
)

const (
	legacyImportUnavailable legacyImportState = iota
	legacyImportPending
	legacyImportHandled
)

type validationDoneMsg struct{ validationErr error }

type cloudAccessCheckDoneMsg struct {
	key          string
	organization string
	plan         string
	expiresAt    string
	loginErr     error
}

type cloudLoginDoneMsg struct {
	organization string
	plan         string
	expiresAt    string
	sourceLines  []string
	warning      string
	loginErr     error
}

type existingCloudKeyDoneMsg struct {
	available bool
}

type initFlowModel struct {
	app             *app
	narratorWidth   int
	skipCloudPrompt bool

	existingCfg   fs.ProjectConfigV2
	defaults      fs.ProjectProfile
	assignedAlias string
	legacyProfile *fs.ProjectProfile
	legacyState   legacyImportState
	returnToRecap bool
	completed     bool

	draft         wizardProfile
	telemetryMode fs.TelemetryMode

	kind         initStepKind
	mood         tui.Mood
	narration    []string
	selectM      *tui.SelectModel
	inputM       *tui.InputField
	secretM      *tui.SecretField
	multiM       *tui.MultiField
	componentErr string
	resultLines  []string

	operationCancel context.CancelFunc
	flowErr         error
}

func newInitFlowModel(application *app, narratorWidth int, skipCloudPrompt bool, existingCfg fs.ProjectConfigV2, defaults fs.ProjectProfile, assignedAlias string, legacyProfile *fs.ProjectProfile) initFlowModel {
	model := initFlowModel{
		app:             application,
		narratorWidth:   narratorWidth,
		skipCloudPrompt: skipCloudPrompt,
		existingCfg:     existingCfg,
		defaults:        defaults,
		assignedAlias:   assignedAlias,
		legacyProfile:   legacyProfile,
		draft: wizardProfile{
			Alias:        assignedAlias,
			Provider:     defaults.Provider,
			RepositoryID: defaults.RepositoryID,
			Ref:          defaults.Ref,
			CatalogPath:  defaults.CatalogPath,
			TargetDir:    defaults.TargetDir,
			IsPrivate:    true,
		},
	}
	if legacyProfile != nil {
		model.legacyState = legacyImportPending
	}
	if fs.HasTelemetryDecision() {
		model.enterNextProviderSection()
	} else {
		model.enter(stepTelemetry)
	}
	return model
}

func (model *initFlowModel) enter(kind initStepKind) {
	model.kind = kind
	model.componentErr = ""
	switch kind {
	case stepTelemetry:
		model.mood = tui.MoodHello
		model.narration = []string{"Welcome to STH (Alpha).", "Pick how much you want to share."}
		model.selectM = tui.NewSelectModel(tui.SelectConfig{Title: "How do you want to contribute?", Options: telemetryOptions(), MaxAttempts: 3})
	case stepExistingCloudKeyChecking:
		model.mood = tui.MoodThinking
		model.narration = []string{"Checking for an existing STH Cloud key…"}
	case stepCloudLoginAsk:
		model.mood = tui.MoodHello
		model.narration = []string{
			"Connected mode feeds your STH Cloud dashboard.",
			fmt.Sprintf("Get your `lsk_live_…` key from %s", DashboardLicensesURL()),
			"You can decline and run `sth login` later.",
		}
		model.selectM = tui.NewSelectModel(tui.SelectConfig{
			Title:       "Connect to STH Cloud now?",
			Options:     yesNoSelectOptions(),
			Recommended: 1,
		})
	case stepCloudKey:
		model.mood = tui.MoodThinking
		model.narration = []string{"Paste your key, I will verify it with the server."}
		model.secretM = tui.NewSecretField(tui.SecretConfig{Label: "Paste your STH Cloud key (lsk_live_…)"})
	case stepCloudKeyChecking:
		model.mood = tui.MoodThinking
		model.narration = []string{"Checking your STH Cloud key…"}
	case stepCloudKeySaving:
		model.mood = tui.MoodThinking
		model.narration = []string{"Saving your STH Cloud connection…"}
	case stepAddProviderConfirm:
		model.mood = tui.MoodHello
		model.narration = existingProvidersSummary(model.existingCfg)
		model.selectM = tui.NewSelectModel(tui.SelectConfig{
			Title:       "Add a new provider?",
			Options:     yesNoSelectOptions(),
			Recommended: 1,
		})
	case stepLegacyImportConfirm:
		model.mood = tui.MoodThinking
		model.narration = []string{"A legacy ~/.sthrc configuration was detected."}
		model.selectM = tui.NewSelectModel(tui.SelectConfig{
			Title:       "Import legacy ~/.sthrc configuration?",
			Options:     yesNoSelectOptions(),
			Recommended: 0,
		})
	case stepAlias:
		model.mood = tui.MoodThinking
		model.narration = []string{
			fmt.Sprintf("You already have %d configured provider(s).", len(model.existingCfg.Providers)),
			"Give it a short alias (e.g. work, oss).",
		}
		model.inputM = tui.NewInputField(tui.InputConfig{Label: "Alias", Required: true})
	case stepProvider:
		model.mood = tui.MoodHello
		model.narration = []string{"Where is your catalog hosted?"}
		options := providerSelectOptions()
		model.selectM = tui.NewSelectModel(tui.SelectConfig{
			Title:       "Provider",
			Options:     options,
			Recommended: max(indexOfProvider(options, model.draft.Provider), 0),
		})
	case stepRepo:
		model.mood = tui.MoodHello
		model.narration = []string{
			"Give me the repository identifier.",
			"Full URL or owner/repo — I will clean it up.",
		}
		model.inputM = tui.NewInputField(tui.InputConfig{
			Label:    "Repository (URL or owner/repo)",
			Default:  model.draft.RepositoryID,
			Required: true,
		})
	case stepRef:
		model.mood = tui.MoodHello
		model.narration = []string{"Two quick settings.", "Enter keeps the default value."}
		model.inputM = tui.NewInputField(tui.InputConfig{Label: "Git reference", Default: model.draft.Ref, Required: true})
	case stepCatalogPath:
		model.mood = tui.MoodHello
		model.narration = nil
		model.inputM = tui.NewInputField(tui.InputConfig{Label: "Catalog path", Default: model.draft.CatalogPath, Required: true})
	case stepTargets:
		model.mood = tui.MoodHello
		model.narration = []string{
			"Which assistants do you target?",
			"Ones detected in this project are pre-checked.",
		}
		specs := writer.AllTargetSpecs()
		preselected := preselectedKinds(&model.draft, model.existingCfg)
		options := make([]tui.Option, 0, len(specs))
		for _, spec := range specs {
			options = append(options, tui.Option{Label: spec.DisplayName, Aliases: []string{spec.Kind}})
		}
		selectedIndices := make([]int, 0, len(preselected))
		for _, kindName := range preselected {
			for index, spec := range specs {
				if spec.Kind == kindName {
					selectedIndices = append(selectedIndices, index)
					break
				}
			}
		}
		model.multiM = tui.NewMultiField(tui.MultiSelectConfig{
			Title:    "Targets to generate",
			Options:  options,
			Selected: selectedIndices,
		})
	case stepAccess:
		model.mood = tui.MoodHello
		model.narration = []string{"Is the repository public or private?"}
		model.selectM = tui.NewSelectModel(tui.SelectConfig{
			Title:       "Repository access type",
			Options:     accessSelectOptions(),
			Recommended: accessRecommendedIndex(model.draft.IsPrivate),
		})
	case stepPrivateDetailsAsk:
		model.mood = tui.MoodThinking
		guide, _ := findProviderGuide(model.draft.Provider)
		model.narration = shortPrivateGuidance(model.draft.Provider, guide)
		model.selectM = tui.NewSelectModel(tui.SelectConfig{
			Title:       "Show full details?",
			Options:     yesNoSelectOptions(),
			Recommended: 1,
		})
	case stepToken:
		model.mood = tui.MoodThinking
		model.narration = []string{"The token will not be displayed or saved."}
		model.secretM = tui.NewSecretField(tui.SecretConfig{
			Label: "Token (optional)",
			Hint:  "⚠ Input is hidden. Leave empty to skip.",
		})
	case stepRecap:
		model.mood = tui.MoodHello
		model.narration = []string{"Here is what I am about to save.", "Review and confirm."}
		model.selectM = tui.NewSelectModel(tui.SelectConfig{
			Title: "Save this configuration?",
			Options: []tui.Option{
				{Label: "Save this configuration", Aliases: []string{"y", "yes", "o", "oui"}},
				{Label: "Edit a field", Aliases: []string{"edit", "e"}},
				{Label: "Abort", Aliases: []string{"n", "no", "non"}},
			},
		})
	case stepEditField:
		model.mood = tui.MoodThinking
		model.narration = []string{"Which field do you want to edit?"}
		model.selectM = tui.NewSelectModel(tui.SelectConfig{Title: "Field to edit", Options: recapFieldOptions()})
	case stepValidating:
		model.mood = tui.MoodThinking
		model.narration = []string{"Checking remote access…"}
	}
}

func (model initFlowModel) isFinished() bool { return false }

func (model initFlowModel) Init() tea.Cmd { return nil }

type forwardedStep struct {
	cmd     tea.Cmd
	index   int
	value   string
	values  []int
	done    bool
	aborted bool
}

func (model *initFlowModel) forwardSelect(message tea.Msg) forwardedStep {
	forwardCmd := model.selectM.Forward(message)
	index, done, aborted := model.selectM.Result()
	return forwardedStep{cmd: forwardCmd, index: index, done: done, aborted: aborted}
}

func (model *initFlowModel) forwardInput(message tea.Msg) forwardedStep {
	forwardCmd := model.inputM.Forward(message)
	value, submitted, aborted := model.inputM.Result()
	return forwardedStep{cmd: forwardCmd, value: value, done: submitted, aborted: aborted}
}

func (model *initFlowModel) forwardSecret(message tea.Msg) forwardedStep {
	forwardCmd := model.secretM.Forward(message)
	value, submitted, aborted := model.secretM.Result()
	return forwardedStep{cmd: forwardCmd, value: value, done: submitted, aborted: aborted}
}

func (model *initFlowModel) forwardMulti(message tea.Msg) forwardedStep {
	forwardCmd := model.multiM.Forward(message)
	values, done, aborted := model.multiM.Result()
	return forwardedStep{cmd: forwardCmd, values: values, done: done, aborted: aborted}
}

func (model initFlowModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if keyPress, ok := message.(tea.KeyMsg); ok && keyPress.Type == tea.KeyCtrlC {
		if model.kind == stepCloudKeySaving {
			return model, nil
		}
		if model.completed {
			return model, tea.Quit
		}
		if model.operationCancel != nil {
			model.operationCancel()
			model.operationCancel = nil
		}
		model.flowErr = tui.ErrAborted
		return model, tea.Quit
	}
	if doneMsg, ok := message.(validationDoneMsg); ok {
		if errors.Is(model.flowErr, tui.ErrAborted) || model.kind != stepValidating {
			return model, tea.Quit
		}
		if model.operationCancel != nil {
			model.operationCancel()
			model.operationCancel = nil
		}
		resultLines, writeErr := model.app.collectInitDraftOutput(model.draft, doneMsg.validationErr)
		model.resultLines = append(model.resultLines, resultLines...)
		model.flowErr = writeErr
		model.completed = true
		return model, tea.Quit
	}
	if accessDone, ok := message.(cloudAccessCheckDoneMsg); ok {
		if errors.Is(model.flowErr, tui.ErrAborted) || model.kind != stepCloudKeyChecking {
			return model, nil
		}
		if model.operationCancel != nil {
			model.operationCancel()
			model.operationCancel = nil
		}
		if accessDone.loginErr != nil {
			model.enter(stepCloudKey)
			model.secretM.SetError(accessDone.loginErr.Error())
			return model, nil
		}
		model.enter(stepCloudKeySaving)
		return model, saveCloudLoginCmd(accessDone)
	}
	if loginDone, ok := message.(cloudLoginDoneMsg); ok {
		if model.kind != stepCloudKeySaving {
			return model, nil
		}
		if loginDone.loginErr != nil {
			model.enter(stepCloudKey)
			model.secretM.SetError(loginDone.loginErr.Error())
			return model, nil
		}
		lines := []string{
			fmt.Sprintf("Connected to %s (plan %s, expires %s).", loginDone.organization, loginDone.plan, loginDone.expiresAt),
			"Key stored in the OS keychain.",
		}
		lines = append(lines, loginDone.sourceLines...)
		lines = append(lines, "")
		if loginDone.warning != "" {
			lines = append(lines, loginDone.warning)
		}
		model.enterNextProviderSection()
		return model, printInitLinesCmd(lines...)
	}
	if keyDone, ok := message.(existingCloudKeyDoneMsg); ok {
		if errors.Is(model.flowErr, tui.ErrAborted) || model.kind != stepExistingCloudKeyChecking {
			return model, nil
		}
		hasKey, downgraded, lines := model.existingCloudKeyState(keyDone.available)
		if hasKey || downgraded {
			model.enterNextProviderSection()
			return model, printInitLinesCmd(lines...)
		}
		model.enter(stepCloudLoginAsk)
		return model, nil
	}

	switch model.kind {
	case stepTelemetry:
		step := model.forwardSelect(message)
		if step.aborted {
			model.flowErr = tui.ErrAborted
			return model, tea.Quit
		}
		if !step.done {
			return model, step.cmd
		}
		mode, persistErr := model.app.saveTelemetryMode(telemetryModeForIndex(step.index))
		if persistErr != nil {
			model.flowErr = persistErr
			return model, tea.Quit
		}
		model.telemetryMode = mode
		lines := telemetryModeConfirmation(mode)
		if mode == fs.TelemetryConnected && !model.skipCloudPrompt {
			model.enter(stepExistingCloudKeyChecking)
			return model, tea.Batch(printInitLinesCmd(lines...), loadExistingCloudKeyCmd())
		}
		model.enterNextProviderSection()
		return model, printInitLinesCmd(lines...)

	case stepCloudLoginAsk:
		step := model.forwardSelect(message)
		if step.aborted {
			lines := model.switchToAnonymous("cancelled")
			model.enterNextProviderSection()
			return model, printInitLinesCmd(lines...)
		}
		if !step.done {
			return model, step.cmd
		}
		if step.index == 0 {
			model.enter(stepCloudKey)
			return model, nil
		}
		lines := model.switchToAnonymous("declined")
		model.enterNextProviderSection()
		return model, printInitLinesCmd(lines...)

	case stepCloudKey:
		step := model.forwardSecret(message)
		if step.aborted {
			lines := model.switchToAnonymous("cancelled")
			model.enterNextProviderSection()
			return model, printInitLinesCmd(lines...)
		}
		if !step.done {
			return model, step.cmd
		}
		key := strings.TrimSpace(step.value)
		if key == "" || !strings.HasPrefix(key, "lsk_live_") {
			model.secretM.SetError("invalid key: it must start with lsk_live_")
			return model, nil
		}
		model.enter(stepCloudKeyChecking)
		loginContext, loginCancel := context.WithTimeout(context.Background(), accessCheckTimeout)
		model.operationCancel = loginCancel
		return model, verifyCloudLoginCmd(model.app, loginContext, key)

	case stepAddProviderConfirm:
		step := model.forwardSelect(message)
		if step.aborted {
			model.flowErr = tui.ErrAborted
			return model, tea.Quit
		}
		if !step.done {
			return model, step.cmd
		}
		if step.index != 0 {
			model.resultLines = append(model.resultLines, "Project configuration kept.", "Next step: run `sth list`.")
			model.completed = true
			return model, tea.Quit
		}
		model.enterFirstAsk()
		return model, nil

	case stepLegacyImportConfirm:
		step := model.forwardSelect(message)
		if step.aborted {
			model.flowErr = tui.ErrAborted
			return model, tea.Quit
		}
		if !step.done {
			return model, step.cmd
		}
		model.legacyState = legacyImportHandled
		if step.index == 0 && model.legacyProfile != nil {
			model.defaults = *model.legacyProfile
			model.draft.Alias = model.assignedAlias
			model.draft.Provider = model.defaults.Provider
			model.draft.RepositoryID = model.defaults.RepositoryID
			model.draft.Ref = model.defaults.Ref
			model.draft.CatalogPath = model.defaults.CatalogPath
			model.draft.TargetDir = model.defaults.TargetDir
		}
		model.enterFirstAsk()
		return model, nil

	case stepAlias:
		step := model.forwardInput(message)
		if step.aborted {
			model.flowErr = tui.ErrAborted
			return model, tea.Quit
		}
		if !step.done {
			return model, step.cmd
		}
		if validateErr := fs.ValidateProviderID(step.value); validateErr != nil {
			model.inputM.SetValue(step.value)
			model.inputM.SetError(aliasValidationMessage(step.value))
			return model, nil
		}
		if aliasIsDuplicate(step.value, model.existingCfg) {
			model.inputM.SetValue(step.value)
			model.inputM.SetError(fmt.Sprintf("Alias %q is already in use. Pick another one.", step.value))
			return model, nil
		}
		model.draft.Alias = step.value
		model.enterAfterField(stepProvider)
		return model, nil

	case stepProvider:
		step := model.forwardSelect(message)
		if step.aborted {
			model.flowErr = tui.ErrAborted
			return model, tea.Quit
		}
		if !step.done {
			return model, step.cmd
		}
		chosen := providerKeysOrdered[step.index]
		if IsCloudProvider(chosen) {
			model.mood = tui.MoodWorried
			model.narration = []string{
				"STH Cloud repositories are not available during Alpha yet.",
				"Join the waitlist: " + DashboardPricingURL(),
				"For now, choose a Git provider below.",
			}
			model.selectM.Reset(tui.SelectConfig{
				Title:       "Provider",
				Options:     providerSelectOptions(),
				Recommended: max(indexOfProvider(providerSelectOptions(), model.draft.Provider), 0),
			})
			return model, nil
		}
		model.draft.Provider = chosen
		model.enterAfterField(stepRepo)
		return model, nil

	case stepRepo:
		step := model.forwardInput(message)
		if step.aborted {
			model.flowErr = tui.ErrAborted
			return model, tea.Quit
		}
		if !step.done {
			return model, step.cmd
		}
		normalizedValue, normalizeErr := fs.NormalizeRepositoryID(model.draft.Provider, step.value)
		if normalizeErr != nil {
			model.inputM.SetValue(step.value)
			model.inputM.SetError("Invalid repository identifier — expected format: " + providerRepositoryIDFormat(model.draft.Provider))
			return model, nil
		}
		model.draft.RepositoryID = normalizedValue
		model.enterAfterField(stepRef)
		return model, nil

	case stepRef:
		step := model.forwardInput(message)
		if step.aborted {
			model.flowErr = tui.ErrAborted
			return model, tea.Quit
		}
		if !step.done {
			return model, step.cmd
		}
		model.draft.Ref = step.value
		model.enterAfterField(stepCatalogPath)
		return model, nil

	case stepCatalogPath:
		step := model.forwardInput(message)
		if step.aborted {
			model.flowErr = tui.ErrAborted
			return model, tea.Quit
		}
		if !step.done {
			return model, step.cmd
		}
		model.draft.CatalogPath = step.value
		model.enterAfterField(stepTargets)
		return model, nil

	case stepTargets:
		step := model.forwardMulti(message)
		if step.aborted {
			model.flowErr = tui.ErrAborted
			return model, tea.Quit
		}
		if !step.done {
			return model, step.cmd
		}
		specs := writer.AllTargetSpecs()
		selected := make([]string, 0, len(step.values))
		for _, index := range step.values {
			selected = append(selected, specs[index].Kind)
		}
		if len(selected) == 0 {
			selected = []string{writer.KindClaude}
		}
		model.draft.Targets = selected
		model.enterAfterField(stepAccess)
		return model, nil

	case stepAccess:
		step := model.forwardSelect(message)
		if step.aborted {
			model.flowErr = tui.ErrAborted
			return model, tea.Quit
		}
		if !step.done {
			return model, step.cmd
		}
		model.draft.IsPrivate = step.index == 0
		if model.draft.IsPrivate && alphaProviderGateActive() && countPrivateGitProviders(model.existingCfg) >= 1 {
			model.mood = tui.MoodWorried
			model.narration = []string{
				"During Alpha a project can reference only one private repository.",
				"Choose public for this one, or bypass at your own risk with STH_ALPHA_GATE=0.",
				fmt.Sprintf("Details: %s", AlphaLimitsDocsURL()),
			}
			model.selectM.Reset(tui.SelectConfig{
				Title:       "Repository access type",
				Options:     accessSelectOptions(),
				Recommended: accessRecommendedIndex(false),
			})
			return model, nil
		}
		model.completeAccessAndToken()
		if model.draft.IsPrivate {
			return model, printInitLinesCmd(privateCreationGuide(model.draft.Provider)...)
		}
		return model, nil

	case stepPrivateDetailsAsk:
		step := model.forwardSelect(message)
		if step.aborted {
			model.flowErr = tui.ErrAborted
			return model, tea.Quit
		}
		if !step.done {
			return model, step.cmd
		}
		if step.index == 0 {
			lines := providerPrivateAccessGuidance(model.draft.Provider)
			model.enter(stepToken)
			return model, printInitLinesCmd(lines...)
		}
		model.enter(stepToken)
		return model, nil

	case stepToken:
		step := model.forwardSecret(message)
		if step.aborted {
			model.flowErr = tui.ErrAborted
			return model, tea.Quit
		}
		if !step.done {
			return model, step.cmd
		}
		if step.value == "" {
			model.draft.Token = ""
			model.draft.TokenSource = ""
		} else {
			model.draft.Token = step.value
			model.draft.TokenSource = "manual"
		}
		model.enterAfterField(stepRecap)
		return model, nil

	case stepRecap:
		step := model.forwardSelect(message)
		if step.aborted {
			model.flowErr = tui.ErrAborted
			return model, tea.Quit
		}
		if !step.done {
			return model, step.cmd
		}
		switch step.index {
		case 0:
			model.enter(stepValidating)
			validationContext, validationCancel := context.WithTimeout(context.Background(), accessCheckTimeout)
			model.operationCancel = validationCancel
			return model, validateInitDraftCmd(model.app, validationContext, model.draft)
		case 1:
			model.returnToRecap = true
			model.enter(stepEditField)
			return model, nil
		default:
			model.resultLines = append(model.resultLines, tui.RenderNarrator(tui.MoodSad, 0, "OK, setup cancelled."))
			model.completed = true
			return model, tea.Quit
		}

	case stepEditField:
		step := model.forwardSelect(message)
		if step.aborted {
			model.flowErr = tui.ErrAborted
			return model, tea.Quit
		}
		if !step.done {
			return model, step.cmd
		}
		model.jumpToRecapField(step.index)
		return model, nil
	}
	return model, nil
}

func (model *initFlowModel) completeAccessAndToken() {
	if !model.draft.IsPrivate {
		model.draft.Token = ""
		model.draft.TokenSource = ""
		model.enterAfterField(stepRecap)
		return
	}
	if envVar, envValue := detectProviderToken(model.draft.Provider); envValue != "" {
		model.draft.Token = ""
		model.draft.TokenSource = "env:" + envVar
		model.mood = tui.MoodHappy
		model.narration = []string{
			fmt.Sprintf("I found a token in $%s.", envVar),
			"I will use it for the access check.",
		}
		model.enterRecap()
		return
	}
	model.enter(stepPrivateDetailsAsk)
}

func (model *initFlowModel) jumpToRecapField(index int) {
	switch index {
	case 0:
		model.mood = tui.MoodThinking
		model.narration = nil
		model.inputM = tui.NewInputField(tui.InputConfig{Label: "Alias", Default: model.draft.Alias, Required: true})
		model.kind = stepAlias
	case 1:
		model.enter(stepProvider)
	case 2:
		model.enter(stepRepo)
	case 3:
		model.mood = tui.MoodHello
		model.narration = nil
		model.inputM = tui.NewInputField(tui.InputConfig{Label: "Git reference", Default: model.draft.Ref, Required: true})
		model.kind = stepRef
	case 4:
		model.mood = tui.MoodHello
		model.narration = nil
		model.inputM = tui.NewInputField(tui.InputConfig{Label: "Catalog path", Default: model.draft.CatalogPath, Required: true})
		model.kind = stepCatalogPath
	case 5:
		model.enter(stepTargets)
	default:
		model.enter(stepAccess)
	}
}

func (model *initFlowModel) enterAfterField(next initStepKind) {
	if model.returnToRecap {
		model.enterRecap()
		return
	}
	model.enter(next)
}

func (model *initFlowModel) enterRecap() {
	model.returnToRecap = false
	model.enter(stepRecap)
}

func (model *initFlowModel) enterNextProviderSection() {
	if len(model.existingCfg.Providers) > 0 {
		model.enter(stepAddProviderConfirm)
		return
	}
	model.enterFirstAsk()
}

func (model *initFlowModel) enterFirstAsk() {
	if model.legacyState == legacyImportPending && len(model.existingCfg.Providers) == 0 {
		model.enter(stepLegacyImportConfirm)
		return
	}
	if model.assignedAlias == "" {
		model.enter(stepAlias)
		return
	}
	model.enter(stepProvider)
}

func (model *initFlowModel) existingCloudKeyState(available bool) (bool, bool, []string) {
	if !available {
		return false, false, nil
	}
	cfg, _ := fs.ReadProjectConfigV2()
	_, attached := findCloudProviderProfile(cfg)
	if !attached {
		attachment, ensureErr := ensureCloudProviderEntry()
		if ensureErr != nil {
			lines := []string{"Warning: could not attach the sth-cloud provider."}
			lines = append(lines, model.switchToAnonymous("STH Cloud unavailable")...)
			return false, true, lines
		}
		return true, false, append(attachment.announcement(), "")
	}
	return true, false, nil
}

func (model *initFlowModel) switchToAnonymous(reason string) []string {
	_, err := fs.SetTelemetryMode(fs.TelemetryAnonymous, valueOrDefault(version, "dev"))
	model.telemetryMode = fs.TelemetryAnonymous
	lines := make([]string, 0, 3)
	if err != nil {
		lines = append(lines, fmt.Sprintf("Warning: could not save anonymous mode: %v", err))
	}
	lines = append(lines,
		fmt.Sprintf("Switched to anonymous mode (%s).", reason),
		"You can try again later with `sth login`.",
	)
	return lines
}

func (model initFlowModel) View() string {
	var builder strings.Builder
	hasNarration := len(model.narration) > 0 || model.kind == stepValidating
	if hasNarration {
		builder.WriteString(tui.RenderNarrator(model.mood, model.narratorWidth, model.narration...))
		builder.WriteString("\n\n")
	}
	switch model.kind {
	case stepTelemetry, stepCloudLoginAsk, stepAddProviderConfirm, stepLegacyImportConfirm,
		stepProvider, stepAccess, stepPrivateDetailsAsk, stepEditField:
		builder.WriteString(model.selectM.View())
	case stepRecap:
		builder.WriteString(renderInitRecap(model.draft))
		builder.WriteString(model.selectM.View())
	case stepAlias, stepRepo, stepRef, stepCatalogPath:
		builder.WriteString(model.inputM.View())
		if model.componentErr != "" {
			builder.WriteString(tui.ErrorStyle.Render(model.componentErr) + "\n")
		}
	case stepCloudKey, stepToken:
		builder.WriteString(model.secretM.View())
		if model.componentErr != "" {
			builder.WriteString(tui.ErrorStyle.Render(model.componentErr) + "\n")
		}
	case stepTargets:
		builder.WriteString(model.multiM.View())
	case stepExistingCloudKeyChecking, stepCloudKeyChecking, stepCloudKeySaving, stepValidating:
		builder.WriteString(tui.DimStyle.Render("Operation in progress…") + "\n")
	}
	return builder.String()
}

func renderInitRecap(draft wizardProfile) string {
	lines := []string{
		fmt.Sprintf("Alias: %s", draft.Alias),
		fmt.Sprintf("Provider: %s", draft.Provider),
		fmt.Sprintf("Repository: %s", draft.RepositoryID),
		fmt.Sprintf("Git reference: %s", draft.Ref),
		fmt.Sprintf("Catalog: %s", draft.CatalogPath),
		fmt.Sprintf("Targets: %s", strings.Join(draft.Targets, ", ")),
		fmt.Sprintf("Access: %s", accessLabel(draft.IsPrivate)),
		fmt.Sprintf("Token: %s", draft.tokenLabel()),
	}
	return strings.Join(lines, "\n") + "\n\n"
}

func yesNoSelectOptions() []tui.Option {
	return []tui.Option{
		{Label: "Yes", Aliases: []string{"o", "oui", "y", "yes"}},
		{Label: "No", Aliases: []string{"n", "non", "no"}},
	}
}

func existingProvidersSummary(cfg fs.ProjectConfigV2) []string {
	lines := []string{fmt.Sprintf("Existing configuration: %d provider(s).", len(cfg.Providers))}
	for _, provider := range cfg.Providers {
		lines = append(lines, fmt.Sprintf("- [%s] %s : %s", provider.ID, provider.Provider, provider.RepositoryID))
	}
	return lines
}

func validateInitDraftCmd(application *app, validationContext context.Context, draft wizardProfile) tea.Cmd {
	return func() tea.Msg {
		validationErr := application.validateInitDraft(validationContext, draft)
		return validationDoneMsg{validationErr: validationErr}
	}
}

func verifyCloudLoginCmd(application *app, loginContext context.Context, key string) tea.Cmd {
	return func() tea.Msg {
		response, err := application.client.AccessCheck(loginContext, key)
		if err != nil {
			return cloudAccessCheckDoneMsg{loginErr: newInitUserError("key verification failed", err)}
		}
		if !response.OK {
			return cloudAccessCheckDoneMsg{loginErr: errors.New("the server rejected the key")}
		}
		if err := loginContext.Err(); err != nil {
			return cloudAccessCheckDoneMsg{loginErr: err}
		}
		return cloudAccessCheckDoneMsg{
			key:          key,
			organization: response.Org.Name,
			plan:         response.Plan,
			expiresAt:    response.ExpiresAt,
		}
	}
}

func saveCloudLoginCmd(accessDone cloudAccessCheckDoneMsg) tea.Cmd {
	return func() tea.Msg {
		if err := auth.SaveAPIKey(accessDone.key); err != nil {
			return cloudLoginDoneMsg{loginErr: newInitUserError("could not store the key in the OS keychain", err)}
		}
		warning := ""
		attachment, attachErr := ensureCloudProviderEntry()
		if attachErr != nil {
			warning = "Warning: could not write the project configuration."
		}
		return cloudLoginDoneMsg{
			organization: accessDone.organization,
			plan:         accessDone.plan,
			expiresAt:    accessDone.expiresAt,
			sourceLines:  attachment.announcement(),
			warning:      warning,
		}
	}
}

func loadExistingCloudKeyCmd() tea.Cmd {
	return func() tea.Msg {
		key, err := auth.LoadAPIKey()
		return existingCloudKeyDoneMsg{available: err == nil && strings.TrimSpace(key) != ""}
	}
}

func printInitLinesCmd(lines ...string) tea.Cmd {
	if len(lines) == 0 {
		return nil
	}
	return tea.Printf("%s", strings.TrimSuffix(renderMessage(lines...), "\n"))
}
