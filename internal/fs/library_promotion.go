package fs

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

const promotionSlugMaxBytes = 64

// LibraryPromotionFile describes one deterministic file created by a local
// Library -> Package promotion. Content is deliberately omitted from JSON so
// bridge previews never echo captured material to the renderer.
type LibraryPromotionFile struct {
	Path    string `json:"path"`
	SHA256  string `json:"sha256"`
	Size    int    `json:"size"`
	Content []byte `json:"-"`
}

// LibraryPromotionPlan is a local-only, Git-backed promotion plan.
type LibraryPromotionPlan struct {
	LibraryID    string                 `json:"library_id"`
	Output       string                 `json:"output"`
	WorktreeRoot string                 `json:"-"`
	Files        []LibraryPromotionFile `json:"files"`
}

// PlanLocalLibraryPromotion reads only .sth/local/code/library.json from the
// enclosing Git worktree and produces a deterministic, non-mutating plan.
func PlanLocalLibraryPromotion(startDir, libraryID, requestedOutput string) (LibraryPromotionPlan, error) {
	root, err := gitWorktreeRoot(startDir)
	if err != nil {
		return LibraryPromotionPlan{}, err
	}
	items, err := ReadLocalLibrary(root)
	if err != nil {
		return LibraryPromotionPlan{}, fmt.Errorf("lecture de la bibliothèque locale échouée : %w", err)
	}
	id := strings.TrimSpace(libraryID)
	var selected *LocalLibraryItem
	for i := range items {
		if items[i].ID == id {
			selected = &items[i]
			break
		}
	}
	if selected == nil {
		return LibraryPromotionPlan{}, fmt.Errorf("élément local %q introuvable", id)
	}
	if err := validatePromotionItem(*selected); err != nil {
		return LibraryPromotionPlan{}, err
	}

	slug := promotionSlug(selected.Title)
	if slug == "" {
		return LibraryPromotionPlan{}, errors.New("le titre ne permet pas de produire un nom de skill sûr")
	}
	output, err := normalizePromotionOutput(requestedOutput, slug)
	if err != nil {
		return LibraryPromotionPlan{}, err
	}
	if err := ensurePromotionDestinationAvailable(root, output); err != nil {
		return LibraryPromotionPlan{}, err
	}

	title := strings.Join(strings.Fields(selected.Title), " ")
	quotedTitle, err := json.Marshal(title)
	if err != nil {
		return LibraryPromotionPlan{}, err
	}
	skillContent := []byte(fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n\n# %s\n\nCette skill provient volontairement de la bibliothèque locale STH.\nLis `references/content.md` puis applique son contenu au contexte de la demande.\n", slug, quotedTitle, title))
	referenceContent := []byte(strings.ReplaceAll(strings.ReplaceAll(selected.Body, "\r\n", "\n"), "\r", "\n"))
	if len(referenceContent) == 0 || referenceContent[len(referenceContent)-1] != '\n' {
		referenceContent = append(referenceContent, '\n')
	}

	files := []LibraryPromotionFile{
		promotionFile(path.Join(output, "SKILL.md"), skillContent),
		promotionFile(path.Join(output, "references", "content.md"), referenceContent),
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return LibraryPromotionPlan{LibraryID: id, Output: output, WorktreeRoot: root, Files: files}, nil
}

// ApplyLocalLibraryPromotion materializes a previously previewed plan. It
// reserves a fresh destination and uses O_EXCL for every file: existing files
// are never overwritten. It never invokes git commit, push, or a provider API.
func ApplyLocalLibraryPromotion(plan LibraryPromotionPlan) (err error) {
	root, rootErr := gitWorktreeRoot(plan.WorktreeRoot)
	if rootErr != nil {
		return rootErr
	}
	if root != plan.WorktreeRoot {
		return errors.New("le worktree Git a changé depuis le preview")
	}
	output, normalizeErr := normalizePromotionOutput(plan.Output, "")
	if normalizeErr != nil {
		return normalizeErr
	}
	if output != plan.Output {
		return errors.New("le chemin de promotion a changé depuis le preview")
	}
	if err := validatePromotionPlanFiles(plan); err != nil {
		return err
	}
	if err := ensurePromotionDestinationAvailable(root, output); err != nil {
		return err
	}

	skillsDir := filepath.Join(root, "skills")
	createdSkillsDir := false
	if _, statErr := os.Lstat(skillsDir); errors.Is(statErr, os.ErrNotExist) {
		if err := os.Mkdir(skillsDir, 0o755); err != nil {
			return fmt.Errorf("création de skills échouée : %w", err)
		}
		createdSkillsDir = true
	} else if statErr != nil {
		return statErr
	}
	if info, statErr := os.Lstat(skillsDir); statErr != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		if createdSkillsDir {
			_ = os.Remove(skillsDir)
		}
		return errors.New("skills n'est pas un dossier local sûr")
	}

	targetDir := filepath.Join(root, filepath.FromSlash(output))
	if err := os.Mkdir(targetDir, 0o755); err != nil {
		if createdSkillsDir {
			_ = os.Remove(skillsDir)
		}
		return fmt.Errorf("réservation de %s échouée : %w", output, err)
	}
	createdFiles := make([]string, 0, len(plan.Files))
	createdDirs := []string{}
	defer func() {
		if err == nil {
			return
		}
		for i := len(createdFiles) - 1; i >= 0; i-- {
			_ = os.Remove(createdFiles[i])
		}
		for i := len(createdDirs) - 1; i >= 0; i-- {
			_ = os.Remove(createdDirs[i])
		}
		_ = os.Remove(targetDir)
		if createdSkillsDir {
			_ = os.Remove(skillsDir)
		}
	}()

	referencesDir := filepath.Join(targetDir, "references")
	if err = os.Mkdir(referencesDir, 0o755); err != nil {
		return fmt.Errorf("création du dossier references échouée : %w", err)
	}
	createdDirs = append(createdDirs, referencesDir)
	for _, file := range plan.Files {
		absolutePath := filepath.Join(root, filepath.FromSlash(file.Path))
		var handle *os.File
		handle, err = os.OpenFile(absolutePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return fmt.Errorf("création de %s échouée : %w", file.Path, err)
		}
		createdFiles = append(createdFiles, absolutePath)
		if _, err = handle.Write(file.Content); err != nil {
			_ = handle.Close()
			return fmt.Errorf("écriture de %s échouée : %w", file.Path, err)
		}
		if err = handle.Close(); err != nil {
			return fmt.Errorf("fermeture de %s échouée : %w", file.Path, err)
		}
	}
	return nil
}

func gitWorktreeRoot(startDir string) (string, error) {
	directory, err := filepath.Abs(startDir)
	if err != nil {
		return "", err
	}
	check := exec.Command("git", "rev-parse", "--is-inside-work-tree", "--is-bare-repository")
	check.Dir = directory
	check.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	output, err := check.Output()
	if err != nil {
		return "", errors.New("la promotion exige un vrai worktree Git")
	}
	lines := strings.Fields(string(output))
	if len(lines) != 2 || lines[0] != "true" || lines[1] != "false" {
		return "", errors.New("la promotion exige un vrai worktree Git non bare")
	}
	show := exec.Command("git", "rev-parse", "--show-toplevel")
	show.Dir = directory
	show.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	rawRoot, err := show.Output()
	if err != nil {
		return "", errors.New("la racine du worktree Git est introuvable")
	}
	root, err := filepath.EvalSymlinks(strings.TrimSpace(string(rawRoot)))
	if err != nil {
		return "", fmt.Errorf("résolution du worktree Git échouée : %w", err)
	}
	return filepath.Clean(root), nil
}

func promotionSlug(title string) string {
	var builder strings.Builder
	lastDash := false
	for _, value := range strings.ToLower(strings.TrimSpace(title)) {
		if value >= 'a' && value <= 'z' || value >= '0' && value <= '9' {
			if builder.Len() >= promotionSlugMaxBytes {
				break
			}
			builder.WriteRune(value)
			lastDash = false
			continue
		}
		if unicode.IsSpace(value) || value == '-' || value == '_' || unicode.IsLetter(value) {
			if builder.Len() > 0 && !lastDash && builder.Len() < promotionSlugMaxBytes {
				builder.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(builder.String(), "-")
}

func normalizePromotionOutput(requested, fallbackSlug string) (string, error) {
	value := strings.TrimSpace(requested)
	if value == "" {
		if fallbackSlug == "" {
			return "", errors.New("le dossier de sortie est requis")
		}
		value = path.Join("skills", fallbackSlug)
	}
	if strings.Contains(value, "\\") || strings.ContainsRune(value, 0) || path.IsAbs(value) || filepath.IsAbs(value) || filepath.VolumeName(value) != "" {
		return "", fmt.Errorf("chemin de sortie invalide : %q", requested)
	}
	cleaned := path.Clean(value)
	parts := strings.Split(cleaned, "/")
	if cleaned != value || len(parts) != 2 || parts[0] != "skills" || !isValidPromotionSlug(parts[1]) {
		return "", errors.New("--output doit être un chemin sûr de la forme skills/<slug>")
	}
	return cleaned, nil
}

func isValidPromotionSlug(value string) bool {
	if value == "" || len(value) > promotionSlugMaxBytes || value[0] == '.' || value[len(value)-1] == '-' {
		return false
	}
	for _, char := range value {
		if !(char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '-' || char == '_') {
			return false
		}
	}
	return true
}

func ensurePromotionDestinationAvailable(root, output string) error {
	if _, err := normalizePromotionOutput(output, ""); err != nil {
		return err
	}
	for _, relative := range []string{"skills", output} {
		candidate := filepath.Join(root, filepath.FromSlash(relative))
		info, err := os.Lstat(candidate)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("le chemin de promotion %s contient un symlink", relative)
		}
		if relative == output {
			return fmt.Errorf("la destination %s existe déjà ; aucun écrasement n'est autorisé", output)
		}
		if !info.IsDir() {
			return errors.New("skills existe mais n'est pas un dossier")
		}
	}
	return nil
}

func validatePromotionItem(item LocalLibraryItem) error {
	title := strings.TrimSpace(item.Title)
	body := strings.TrimSpace(item.Body)
	if title == "" || len([]byte(title)) > 200 || strings.ContainsRune(title, 0) {
		return errors.New("le titre de l'élément local est invalide")
	}
	if body == "" || len([]byte(item.Body)) > localLibraryMaxBody || strings.ContainsRune(item.Body, 0) {
		return errors.New("le contenu local est vide, invalide ou dépasse 1 Mio")
	}
	return nil
}

func promotionFile(filePath string, content []byte) LibraryPromotionFile {
	return LibraryPromotionFile{Path: filePath, SHA256: ComputeSHA256(content), Size: len(content), Content: content}
}

func validatePromotionPlanFiles(plan LibraryPromotionPlan) error {
	expected := map[string]bool{
		path.Join(plan.Output, "SKILL.md"):                 false,
		path.Join(plan.Output, "references", "content.md"): false,
	}
	for _, file := range plan.Files {
		seen, ok := expected[file.Path]
		if !ok || seen || file.SHA256 != ComputeSHA256(file.Content) || file.Size != len(file.Content) {
			return errors.New("le plan de promotion est invalide")
		}
		expected[file.Path] = true
	}
	for _, seen := range expected {
		if !seen {
			return errors.New("le plan de promotion est incomplet")
		}
	}
	return nil
}
