package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"sth/internal/fs"
	sthhttp "sth/internal/http"
	"sth/internal/writer"
)

// wizardEmotion drives the buddy face/emoji shown above each prompt.
// It is intentionally a wizard-private enum so we never have to add
// narrator-specific entries to fs.BuddyState (which would break sth buddy).
type wizardEmotion int

const (
	wizardHello    wizardEmotion = iota // greeting / neutral question
	wizardThinking                      // contextual info / clarification
	wizardWorried                       // invalid input, re-prompt
	wizardSad                           // user aborted
	wizardHappy                         // success / validation OK
	wizardError                         // remote validation failed
)

type wizardView struct {
	Emoji string
	Face  string
}

func wizardViewFor(e wizardEmotion) wizardView {
	switch e {
	case wizardThinking:
		return wizardView{Emoji: "🤔", Face: "o_o"}
	case wizardWorried:
		return wizardView{Emoji: "😟", Face: ">_<"}
	case wizardSad:
		return wizardView{Emoji: "🙁", Face: ":("}
	case wizardHappy:
		return wizardView{Emoji: "✅", Face: "^.^"}
	case wizardError:
		return wizardView{Emoji: "⚠️", Face: "o_o"}
	default:
		return wizardView{Emoji: "😊", Face: "^_^"}
	}
}

// renderWizardBox draws a buddy-in-his-box block with narration on the right
// of the small face box. Same outer frame width as the launch banner so the UI
// stays visually consistent.
func renderWizardBox(v wizardView, lines ...string) string {
	const (
		faceCells       = 3
		leftFacePadding = 4 // spaces before the face box opening
	)
	face := padFace(v.Face, faceCells)              // " ^_^ "
	innerBoxWidth := displayCells(face) + 2         // " ^_^ " + two borders
	leftCol := strings.Repeat(" ", leftFacePadding) // "    "
	box := struct {
		Top, Mid, Bot string
	}{
		Top: leftCol + "╭" + strings.Repeat("─", innerBoxWidth-2) + "╮",
		Mid: leftCol + "│" + face + "│",
		Bot: leftCol + "╰" + strings.Repeat("─", innerBoxWidth-2) + "╯",
	}
	emojiRow := "      " + v.Emoji
	const narrSep = "   " // 3 spaces between face box and narration

	rows := []string{
		"",
		emojiRow,
		box.Top + narrSep + lineAt(lines, 0),
		box.Mid + narrSep + lineAt(lines, 1),
		box.Bot + narrSep + lineAt(lines, 2),
	}
	for i := 3; i < len(lines); i++ {
		// continuation lines align under the narration column (after box+sep).
		indent := strings.Repeat(" ", displayCells(box.Bot)+len(narrSep))
		rows = append(rows, indent+lines[i])
	}
	rows = append(rows, "")

	innerWidth := bannerOuterWidth - 2
	var b strings.Builder
	b.WriteString("╭" + strings.Repeat("─", innerWidth) + "╮\n")
	for _, row := range rows {
		padN := innerWidth - displayCells(row)
		if padN < 0 {
			padN = 0
		}
		b.WriteString("│")
		b.WriteString(row)
		b.WriteString(strings.Repeat(" ", padN))
		b.WriteString("│\n")
	}
	b.WriteString("╰" + strings.Repeat("─", innerWidth) + "╯")
	return b.String()
}

func lineAt(lines []string, i int) string {
	if i < len(lines) {
		return lines[i]
	}
	return ""
}

func (a *app) narrate(emotion wizardEmotion, lines ...string) {
	v := wizardViewFor(emotion)
	fmt.Fprintln(a.stdout, renderWizardBox(v, lines...))
}

// wizardProfile is the in-memory draft built up step by step.
// It is converted to fs.ProviderEntry + fs.ProjectProfile once confirmed.
type wizardProfile struct {
	Alias        string
	Provider     string
	RepositoryID string
	Ref          string
	CatalogPath  string
	TargetDir    string
	Targets      []string
	IsPrivate    bool
	Token        string // session-only, never persisted
	TokenSource  string // "env:GITHUB_TOKEN" / "manual" / "" (none)
}

// tokenLabel returns a redacted, user-facing description of the token state.
// We deliberately never show even a fragment of the token value.
func (p wizardProfile) tokenLabel() string {
	switch {
	case p.TokenSource == "":
		return "aucun"
	case strings.HasPrefix(p.TokenSource, "env:"):
		return fmt.Sprintf("depuis $%s", strings.TrimPrefix(p.TokenSource, "env:"))
	default:
		return "saisi pour cette session"
	}
}

// runInitWizard drives the new narrated flow. It is called by runInit after
// the dispatcher has handled existing-config and legacy-import branches.
// `defaults` carries pre-filled values (e.g. from legacy import).
// `existingCfg` is the current project config; we append our new entry to it.
// `assignedAlias` is "default" for the first provider, or empty to ask.
func (a *app) runInitWizard(reader *bufio.Reader, existingCfg fs.ProjectConfigV2, defaults fs.ProjectProfile, assignedAlias string) error {
	a.narrate(wizardHello,
		"Salut ! On va connecter ton catalogue de skills",
		"en quelques questions. Tu pourras tout corriger",
		"avant que j'écrive quoi que ce soit. (Ctrl+C pour annuler.)",
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
		alias, err := a.askAlias(reader, existingCfg)
		if err != nil {
			return err
		}
		draft.Alias = alias
	}

	if err := a.askProvider(reader, &draft); err != nil {
		return err
	}

	if IsCloudProvider(draft.Provider) {
		a.writeStdout(
			"",
			"STH Cloud n'est pas encore disponible en Alpha.",
			"Inscris-toi à la liste d'attente : https://sth-dashboard.vercel.app/pricing",
			"",
			"En attendant, choisis un provider Git (github, gitlab, azure-devops, bitbucket).",
		)
		return fmt.Errorf("sth-cloud sera disponible après l'Alpha")
	}

	if err := a.askRepository(reader, &draft); err != nil {
		return err
	}

	if err := a.askCatalogSettings(reader, &draft); err != nil {
		return err
	}

	if err := a.askTargets(reader, &draft, existingCfg); err != nil {
		return err
	}

	if err := a.askAccessAndToken(reader, &draft, existingCfg); err != nil {
		return err
	}

	confirmed, err := a.recapAndConfirm(reader, &draft, existingCfg)
	if err != nil {
		return err
	}
	if !confirmed {
		a.narrate(wizardSad, "OK, j'abandonne. Aucun fichier modifié.")
		return nil
	}

	return a.writeAndValidate(existingCfg, draft)
}

// aliasIsDuplicate reports whether `alias` collides with an existing
// provider entry. Used by both the initial alias prompt and the recap-edit
// loop so a recap-time edit can't sneak a duplicate past validation.
func aliasIsDuplicate(alias string, existingCfg fs.ProjectConfigV2) bool {
	for _, p := range existingCfg.Providers {
		if p.ID == alias {
			return true
		}
	}
	return false
}

func (a *app) askAlias(reader *bufio.Reader, existingCfg fs.ProjectConfigV2) (string, error) {
	a.narrate(wizardThinking,
		fmt.Sprintf("Tu as déjà %d provider(s) configuré(s).", len(existingCfg.Providers)),
		"Donne un alias court à ce nouveau (ex : work, oss).",
	)
	for {
		alias, err := promptValue(reader, a.stdout, "Alias", "")
		if err != nil {
			return "", err
		}
		if validateErr := fs.ValidateProviderID(alias); validateErr != nil {
			a.narrate(wizardWorried, "Alias invalide : "+validateErr.Error())
			continue
		}
		if aliasIsDuplicate(alias, existingCfg) {
			a.narrate(wizardWorried, fmt.Sprintf("L'alias %q est déjà utilisé. Choisis-en un autre.", alias))
			continue
		}
		return alias, nil
	}
}

func (a *app) askProvider(reader *bufio.Reader, draft *wizardProfile) error {
	a.narrate(wizardHello, "Quel hébergeur héberge ton catalogue ?")
	provider, err := promptProvider(reader, a.stdout, draft.Provider)
	if err != nil {
		return err
	}
	draft.Provider = provider
	return nil
}

func (a *app) askRepository(reader *bufio.Reader, draft *wizardProfile) error {
	a.narrate(wizardHello,
		"Donne-moi l'identifiant du dépôt.",
		"Tu peux coller une URL (ex: https://github.com/owner/repo)",
		"ou juste owner/repo — je m'occupe du nettoyage.",
	)
	repo, err := promptRepositoryID(reader, a.stdout, draft.Provider, draft.RepositoryID)
	if err != nil {
		return err
	}
	draft.RepositoryID = repo
	return nil
}

func (a *app) askCatalogSettings(reader *bufio.Reader, draft *wizardProfile) error {
	a.narrate(wizardHello,
		"Deux petits réglages.",
		"Appuie sur Entrée pour accepter les valeurs par défaut.",
	)
	ref, err := promptValue(reader, a.stdout, "Référence Git", draft.Ref)
	if err != nil {
		return err
	}
	draft.Ref = ref
	catalog, err := promptValue(reader, a.stdout, "Chemin du catalogue", draft.CatalogPath)
	if err != nil {
		return err
	}
	draft.CatalogPath = catalog
	return nil
}

func (a *app) askAccessAndToken(reader *bufio.Reader, draft *wizardProfile, existingCfg fs.ProjectConfigV2) error {
	a.narrate(wizardHello, "Le dépôt est public ou privé ?")
	access, err := promptAccessType(reader, a.stdout)
	if err != nil {
		return err
	}
	draft.IsPrivate = access

	if draft.IsPrivate && alphaProviderGateActive() && countPrivateGitProviders(existingCfg) >= 1 {
		a.writeStdout(
			"",
			"Pendant l'Alpha, un projet ne peut référencer qu'un seul dépôt privé.",
			"Choisis public pour ce dépôt, ou contourne (à tes risques) avec STH_ALPHA_GATE=0.",
			fmt.Sprintf("Détails : %s", AlphaLimitsDocsURL()),
		)
		return fmt.Errorf("limite Alpha : 1 dépôt privé max par projet")
	}

	if !draft.IsPrivate {
		draft.Token = ""
		draft.TokenSource = ""
		return nil
	}

	if envVar, value := detectProviderToken(draft.Provider); value != "" {
		a.narrate(wizardHappy,
			fmt.Sprintf("J'ai trouvé un token dans $%s.", envVar),
			"Je vais l'utiliser pour la vérification.",
		)
		draft.Token = ""
		draft.TokenSource = "env:" + envVar
		return nil
	}

	guide, _ := findProviderGuide(draft.Provider)
	a.narrate(wizardThinking, shortPrivateGuidance(draft.Provider, guide)...)
	expand, err := promptYesNo(reader, a.stdout, "Détails complets ?", false)
	if err != nil {
		return err
	}
	if expand {
		a.writeStdout(providerPrivateAccessGuidance(draft.Provider)...)
	}

	a.writeStdout("⚠ La saisie sera masquée (rien ne s'affichera). Entrée vide = ignorer.")
	token, err := promptSecret(a, reader, "Token (optionnel)")
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

// shortPrivateGuidance returns a 2-3 line teaser for the private-repo flow.
// The full multi-line guidance lives in providerPrivateAccessGuidance and is
// shown on demand via the "Détails complets ?" gate.
func shortPrivateGuidance(provider string, guide providerGuide) []string {
	if guide.ActionURL == "" {
		return []string{
			"Dépôt privé : il te faut un token d'accès.",
			"Définis $STH_TOKEN ou saisis-le ci-dessous.",
		}
	}
	return []string{
		fmt.Sprintf("Pour un dépôt %s privé, génère un PAT :", strings.ToUpper(provider)),
		"→ " + guide.ActionURL,
		"→ permission « " + guide.MinimumPermission + " »",
	}
}

// promptAccessType reads "public" or "privé" (default privé). For backward
// compat with the old y/n test scripts we also accept y/n/yes/no/oui/non.
// Returns true when the user picked "privé".
func promptAccessType(reader *bufio.Reader, stdout io.Writer) (bool, error) {
	for {
		raw, err := promptValue(reader, stdout, "Type d'accès [public/privé]", "privé")
		if err != nil {
			return false, err
		}
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "privé", "prive", "private", "p", "y", "yes", "o", "oui":
			return true, nil
		case "public", "pub", "n", "no", "non":
			return false, nil
		}
		fmt.Fprintln(stdout, "Réponse invalide. Tape `public` ou `privé`.")
	}
}

// promptChoice reads a value and re-prompts until the trimmed input matches
// (case-insensitively) one of the allowed values. Empty input returns the
// default (which must itself be in the allowed list, otherwise empty is rejected).
// Returns io.EOF immediately when stdin is closed without input, mirroring
// promptValue, so callers can unwind cleanly instead of looping forever.
func promptChoice(reader *bufio.Reader, stdout io.Writer, label, defaultValue string, allowed []string) (string, error) {
	allowedLower := make(map[string]string, len(allowed))
	for _, v := range allowed {
		allowedLower[strings.ToLower(v)] = v
	}
	for {
		if _, err := fmt.Fprintf(stdout, "%s : ", label); err != nil {
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
			trimmed = defaultValue
		}
		if canonical, ok := allowedLower[strings.ToLower(trimmed)]; ok {
			return canonical, nil
		}
		fmt.Fprintf(stdout, "Réponse invalide. Choix valides : %s.\n", strings.Join(allowed, ", "))
	}
}

func detectProviderToken(provider string) (string, string) {
	if v := strings.TrimSpace(os.Getenv("STH_TOKEN")); v != "" {
		return "STH_TOKEN", v
	}
	guide, ok := findProviderGuide(provider)
	if !ok {
		return "", ""
	}
	for _, name := range guide.EnvVars {
		if name == "STH_TOKEN" {
			continue
		}
		if v := strings.TrimSpace(os.Getenv(name)); v != "" {
			return name, v
		}
	}
	return "", ""
}

func (a *app) recapAndConfirm(reader *bufio.Reader, draft *wizardProfile, existingCfg fs.ProjectConfigV2) (bool, error) {
	for {
		a.narrate(wizardHello,
			"Voici ce que je vais enregistrer.",
			"Vérifie et confirme.",
		)
		a.writeStdout(
			fmt.Sprintf("  Alias         : %s", draft.Alias),
			fmt.Sprintf("  Provider      : %s", draft.Provider),
			fmt.Sprintf("  Dépôt         : %s", draft.RepositoryID),
			fmt.Sprintf("  Référence     : %s", draft.Ref),
			fmt.Sprintf("  Catalogue     : %s", draft.CatalogPath),
			fmt.Sprintf("  Cibles        : %s", strings.Join(draft.Targets, ", ")),
			fmt.Sprintf("  Accès         : %s", accessLabel(draft.IsPrivate)),
			fmt.Sprintf("  Token         : %s", draft.tokenLabel()),
			"",
		)
		choice, err := promptChoice(reader, a.stdout, "Confirmer et enregistrer ? [Y/n/edit]", "Y", []string{"Y", "y", "yes", "o", "oui", "n", "no", "non", "edit", "e"})
		if err != nil {
			return false, err
		}
		switch strings.ToLower(choice) {
		case "n", "no", "non":
			return false, nil
		case "edit", "e":
			if err := a.editDraftField(reader, draft, existingCfg); err != nil {
				return false, err
			}
			continue
		default:
			return true, nil
		}
	}
}

func (a *app) editDraftField(reader *bufio.Reader, draft *wizardProfile, existingCfg fs.ProjectConfigV2) error {
	a.narrate(wizardThinking, "Quelle entrée veux-tu corriger ?")
	field, err := promptChoice(reader, a.stdout,
		"Champ [alias|provider|dépôt|ref|catalogue|cibles|accès|token]",
		"dépôt",
		[]string{"alias", "provider", "dépôt", "depot", "ref", "catalogue", "cibles", "targets", "accès", "acces", "token"},
	)
	if err != nil {
		return err
	}
	switch strings.ToLower(field) {
	case "alias":
		newAlias, err := promptValue(reader, a.stdout, "Alias", draft.Alias)
		if err != nil {
			return err
		}
		if v := fs.ValidateProviderID(newAlias); v != nil {
			a.narrate(wizardWorried, "Alias invalide : "+v.Error())
			return nil
		}
		// Mirror the duplicate-check from askAlias so a recap-edit can't smuggle
		// in a colliding alias and corrupt project.json.
		if newAlias != draft.Alias && aliasIsDuplicate(newAlias, existingCfg) {
			a.narrate(wizardWorried, fmt.Sprintf("L'alias %q est déjà utilisé. Alias inchangé.", newAlias))
			return nil
		}
		draft.Alias = newAlias
	case "provider":
		return a.askProvider(reader, draft)
	case "dépôt", "depot":
		return a.askRepository(reader, draft)
	case "ref":
		v, err := promptValue(reader, a.stdout, "Référence Git", draft.Ref)
		if err != nil {
			return err
		}
		draft.Ref = v
	case "catalogue":
		v, err := promptValue(reader, a.stdout, "Chemin du catalogue", draft.CatalogPath)
		if err != nil {
			return err
		}
		draft.CatalogPath = v
	case "cibles", "targets":
		return a.askTargets(reader, draft, existingCfg)
	case "accès", "acces", "token":
		return a.askAccessAndToken(reader, draft, existingCfg)
	}
	return nil
}

func accessLabel(private bool) string {
	if private {
		return "privé"
	}
	return "public"
}

func (a *app) askTargets(reader *bufio.Reader, draft *wizardProfile, existingCfg fs.ProjectConfigV2) error {
	specs := writer.AllTargetSpecs()
	preselected := preselectedKinds(draft, existingCfg)

	a.narrate(wizardHello,
		"Quels modèles/agents veux-tu cibler ?",
		"Je pré-coche ceux détectés dans le projet.",
		"Entrée = garder ; ex : 1,3 ou claude,copilot ou all.",
	)
	for index, spec := range specs {
		mark := " "
		if containsKind(preselected, spec.Kind) {
			mark = "x"
		}
		a.writeStdout(fmt.Sprintf("  %d. [%s] %s", index+1, mark, spec.DisplayName))
	}

	selected, err := promptMultiChoice(reader, a.stdout, "Cibles", specs, preselected)
	if err != nil {
		return err
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

func promptMultiChoice(reader *bufio.Reader, stdout io.Writer, label string, specs []writer.TargetSpec, preselected []string) ([]string, error) {
	for {
		if _, err := fmt.Fprintf(stdout, "%s [%s]: ", label, strings.Join(preselected, ",")); err != nil {
			return nil, err
		}
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF && strings.TrimSpace(line) == "" {
				return nil, io.EOF
			}
			if err != io.EOF {
				return nil, fmt.Errorf("lecture de l'entrée utilisateur échouée : %w", err)
			}
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			return preselected, nil
		}
		selected, parseErr := parseTargetSelection(trimmed, specs)
		if parseErr != nil {
			fmt.Fprintln(stdout, parseErr.Error())
			continue
		}
		return selected, nil
	}
}

func parseTargetSelection(input string, specs []writer.TargetSpec) ([]string, error) {
	if strings.EqualFold(strings.TrimSpace(input), "all") {
		all := make([]string, 0, len(specs))
		for _, spec := range specs {
			all = append(all, spec.Kind)
		}
		return all, nil
	}
	fields := strings.FieldsFunc(input, func(separator rune) bool {
		return separator == ',' || separator == ' ' || separator == '\t'
	})
	selected := make([]string, 0, len(fields))
	seen := make(map[string]bool)
	for _, field := range fields {
		kind, ok := resolveTargetToken(field, specs)
		if !ok {
			return nil, fmt.Errorf("cible invalide : %q (choix : numéros, %s, ou all)", field, strings.Join(specKinds(specs), ", "))
		}
		if !seen[kind] {
			seen[kind] = true
			selected = append(selected, kind)
		}
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("aucune cible valide")
	}
	return selected, nil
}

func resolveTargetToken(token string, specs []writer.TargetSpec) (string, bool) {
	trimmed := strings.TrimSpace(token)
	if index, err := strconv.Atoi(trimmed); err == nil {
		if index >= 1 && index <= len(specs) {
			return specs[index-1].Kind, true
		}
		return "", false
	}
	lower := strings.ToLower(trimmed)
	for _, spec := range specs {
		if strings.ToLower(spec.Kind) == lower {
			return spec.Kind, true
		}
	}
	return "", false
}

func specKinds(specs []writer.TargetSpec) []string {
	kinds := make([]string, 0, len(specs))
	for _, spec := range specs {
		kinds = append(kinds, spec.Kind)
	}
	return kinds
}

func containsKind(kinds []string, kind string) bool {
	for _, candidate := range kinds {
		if candidate == kind {
			return true
		}
	}
	return false
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

func (a *app) writeAndValidate(existingCfg fs.ProjectConfigV2, draft wizardProfile) error {
	entry := fs.ProviderEntry{
		ID:           draft.Alias,
		Provider:     draft.Provider,
		RepositoryID: draft.RepositoryID,
		CatalogRef:   draft.Ref,
		CatalogPath:  draft.CatalogPath,
		TargetDir:    draft.TargetDir,
		Private:      draft.IsPrivate,
	}
	existingCfg.Providers = append(existingCfg.Providers, entry)
	existingCfg.Targets = targetEntriesFromKinds(draft.Targets)

	path, err := fs.WriteProjectConfigV2(existingCfg)
	if err != nil {
		return err
	}
	a.writeStdout(fmt.Sprintf("✓ Configuration enregistrée : %s", path))
	if workingDir, wdErr := os.Getwd(); wdErr == nil {
		if changed, gitErr := fs.EnsureManagedGitignore(workingDir); gitErr == nil && changed {
			a.writeStdout("✓ .gitignore mis à jour (fichiers d'état STH ignorés)")
		}
	}

	profile := fs.ProjectProfile{
		Provider:     draft.Provider,
		RepositoryID: draft.RepositoryID,
		Ref:          draft.Ref,
		CatalogPath:  draft.CatalogPath,
		TargetDir:    draft.TargetDir,
		APIToken:     draft.Token,
	}
	if err := a.client.CheckProjectAccess(context.Background(), profile); err != nil {
		a.renderValidationFailure(draft, err)
		// Return the error so CI/scripting wrappers see a non-zero exit. The
		// config file is still on disk (written above) — the user can iterate
		// without losing input. Diagnostic + remediation are already printed.
		return err
	}
	a.renderValidationSuccess(draft)
	return nil
}

func (a *app) renderValidationSuccess(draft wizardProfile) {
	a.narrate(wizardHappy,
		"Tout est bon ! Le catalogue est accessible.",
		"Prochaine étape : exécute `sth list` pour voir les skills.",
	)
	if draft.TokenSource == "manual" {
		envVar := providerTokenEnvVar(draft.Provider)
		a.writeStdout(
			"",
			"Le token saisi a servi à valider mais n'est pas sauvegardé.",
			"Pour les prochaines fois, définis-le en variable d'environnement :",
			fmt.Sprintf("  export %s=<votre-token>", envVar),
		)
	}
}

func (a *app) renderValidationFailure(draft wizardProfile, err error) {
	diag := classifyValidationError(err, draft)
	a.narrate(wizardError,
		"J'ai écrit la config mais je n'arrive pas à valider",
		"l'accès au dépôt distant.",
	)
	a.writeStdout(
		"⚠ Configuration écrite mais non validée.",
		"",
		fmt.Sprintf("  Cause détectée : %s", diag.Kind),
		fmt.Sprintf("  Détail         : %s", diag.Summary),
		"",
		"  Pour corriger :",
	)
	for i, step := range diag.Remediation {
		a.writeStdout(fmt.Sprintf("    %d. %s", i+1, step))
	}
	a.writeStdout(
		"",
		"  → Relance la vérification : `sth doctor`",
	)
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
			Summary: "Le serveur a refusé le token (HTTP 401/403).",
			Remediation: []string{
				"Vérifie que le token n'est pas expiré.",
				fmt.Sprintf("Confirme la permission « %s ».", guide.MinimumPermission),
				fmt.Sprintf("Régénère via %s", guide.ActionURL),
				fmt.Sprintf("export %s=<nouveau-token>", envVar),
			},
		}
	}

	msg := err.Error()
	switch {
	case strings.Contains(msg, "catalogue introuvable et aucun bundle"):
		return validationDiagnosis{
			Kind:    "catalog_empty",
			Summary: "Le dépôt est accessible mais ne contient ni catalogue ni bundle reconnaissable.",
			Remediation: []string{
				fmt.Sprintf("Vérifie que `%s` existe au ref `%s`, ou qu'un dossier suit la convention category/folder/SKILL.md.", draft.CatalogPath, draft.Ref),
				"Astuce : si le catalogue est ailleurs, modifie le champ `catalog_path` dans `.sth/project.json`.",
			},
		}
	case strings.Contains(msg, "HTTP 404"):
		return validationDiagnosis{
			Kind:    "not_found",
			Summary: "Le dépôt, la branche ou le fichier catalogue n'existe pas (HTTP 404).",
			Remediation: []string{
				fmt.Sprintf("Vérifie l'orthographe de `%s` et que la branche `%s` existe.", draft.RepositoryID, draft.Ref),
				fmt.Sprintf("Vérifie que `%s` se trouve à cet emplacement dans le dépôt.", draft.CatalogPath),
				"Pour un dépôt privé sans token : ça ressemble aussi à un 404 — fournis un token et réessaie.",
			},
		}
	default:
		return validationDiagnosis{
			Kind:    "network",
			Summary: "Impossible de joindre le serveur distant.",
			Remediation: []string{
				"Vérifie ta connexion réseau.",
				"Si tu passes par un proxy, exporte HTTPS_PROXY.",
				fmt.Sprintf("Détail : %s", msg),
			},
		}
	}
}
