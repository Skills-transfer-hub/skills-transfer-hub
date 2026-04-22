package fs

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

const (
	configFileName        = ".loreskillsrc"
	projectConfigDirName  = ".loreskills"
	projectConfigFileName = "project.json"
	stateFileName         = "lore-state.json"
	defaultRef            = "main"
	defaultCatalogPath    = "SKILLS.md"
	defaultTargetDir      = ".claude/skills"

	// En-tête ajouté en début de chaque ressource Markdown gérée par LoreSkills.
	skillHeader = "> ⚠️ ATTENTION : Skill géré par LoreSkills. Ne pas modifier manuellement.\n\n"
)

// Config stores the legacy LoreSkills user configuration.
type Config struct {
	RepoRawURL string `json:"repo_raw_url"`
	TargetDir  string `json:"target_dir"`
}

// ProjectProfile stores the project-local LoreSkills configuration.
type ProjectProfile struct {
	Provider           string `json:"provider"`
	RepositoryID       string `json:"repository_id"`
	Ref                string `json:"ref"`
	CatalogPath        string `json:"catalog_path"`
	TargetDir          string `json:"target_dir"`
	MigratedFromLegacy bool   `json:"migrated_from_legacy,omitempty"`
}

// CatalogEntry describes one installable resource from SKILLS.md.
type CatalogEntry struct {
	CatalogID    string `json:"catalog_id"`
	ResourceType string `json:"resource_type"`
	Name         string `json:"name"`
	FolderName   string `json:"folder_name"`
	SourcePath   string `json:"source_path"`
	Description  string `json:"description,omitempty"`
}

// InstalledResourceRecord describes one installed resource in lore-state.json.
type InstalledResourceRecord struct {
	CatalogID    string `json:"catalog_id"`
	ResourceType string `json:"resource_type"`
	Name         string `json:"name"`
	FolderName   string `json:"folder_name"`
	InstalledPath string `json:"installed_path"`
	SourcePath   string `json:"source_path"`
	Status       string `json:"status"`
}

// InstalledResourceState stores the tracked installed resources.
type InstalledResourceState struct {
	Entries []InstalledResourceRecord `json:"entries"`
}

// ConfigPath returns the absolute path to ~/.loreskillsrc.
func ConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("impossible de localiser le répertoire personnel : %w", err)
	}
	return filepath.Join(home, configFileName), nil
}

// ReadConfigIfPresent loads the legacy config if it exists.
func ReadConfigIfPresent() (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lecture de %s échouée : %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("fichier de configuration malformé (%s) : %w", path, err)
	}
	if err := validateConfig(cfg); err != nil {
		return nil, fmt.Errorf("configuration invalide (%s) : %w", path, err)
	}

	cfg.RepoRawURL = strings.TrimRight(strings.TrimSpace(cfg.RepoRawURL), "/")
	cfg.TargetDir = filepath.Clean(strings.TrimSpace(cfg.TargetDir))

	return &cfg, nil
}

// WriteDefaultConfig creates ~/.loreskillsrc with LoreSkills starter values.
func WriteDefaultConfig() (string, error) {
	path, err := ConfigPath()
	if err != nil {
		return "", err
	}

	if _, err := os.Stat(path); err == nil {
		return path, fmt.Errorf("un fichier de configuration existe déjà : %s", path)
	} else if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("vérification de %s échouée : %w", path, err)
	}

	defaults := Config{
		RepoRawURL: "https://raw.githubusercontent.com/votre-organisation/votre-repo/main",
		TargetDir:  defaultTargetDir,
	}

	data, err := json.MarshalIndent(defaults, "", "  ")
	if err != nil {
		return "", fmt.Errorf("sérialisation de la configuration échouée : %w", err)
	}

	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return "", fmt.Errorf("écriture de %s échouée : %w", path, err)
	}

	return path, nil
}

// ReadConfig loads the legacy config and returns a user-facing error when absent.
func ReadConfig() (*Config, error) {
	cfg, err := ReadConfigIfPresent()
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		path, pathErr := ConfigPath()
		if pathErr != nil {
			return nil, pathErr
		}
		return nil, fmt.Errorf("configuration introuvable (%s) — exécutez `loreskills init`", path)
	}

	return cfg, nil
}

// ProjectProfilePath returns the absolute path to the project-local profile.
func ProjectProfilePath() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("impossible de localiser le projet courant : %w", err)
	}

	return filepath.Join(wd, projectConfigDirName, projectConfigFileName), nil
}

// ReadProjectProfileIfPresent loads the project-local profile if it exists.
func ReadProjectProfileIfPresent() (*ProjectProfile, error) {
	path, err := ProjectProfilePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lecture de %s échouée : %w", path, err)
	}

	var profile ProjectProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return nil, fmt.Errorf("profil projet malformé (%s) : %w", path, err)
	}
	if err := validateProjectProfile(profile); err != nil {
		return nil, fmt.Errorf("profil projet invalide (%s) : %w", path, err)
	}

	profile.Provider = normalizeProvider(profile.Provider)
	profile.RepositoryID = strings.TrimSpace(profile.RepositoryID)
	profile.Ref = normalizeRef(profile.Ref)
	profile.CatalogPath = normalizeCatalogPath(profile.CatalogPath)
	profile.TargetDir = filepath.Clean(strings.TrimSpace(profile.TargetDir))

	return &profile, nil
}

// ReadProjectProfile loads the current project profile or returns a user-facing error when absent.
func ReadProjectProfile() (*ProjectProfile, error) {
	profile, err := ReadProjectProfileIfPresent()
	if err != nil {
		return nil, err
	}
	if profile == nil {
		path, pathErr := ProjectProfilePath()
		if pathErr != nil {
			return nil, pathErr
		}
		return nil, fmt.Errorf("configuration projet introuvable (%s) — exécutez `loreskills init` dans ce projet", path)
	}

	return profile, nil
}

// WriteProjectProfile persists the project-local profile.
func WriteProjectProfile(profile ProjectProfile) (string, error) {
	if err := validateProjectProfile(profile); err != nil {
		return "", err
	}

	path, err := ProjectProfilePath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("création du dossier de configuration projet échouée : %w", err)
	}

	profile.Provider = normalizeProvider(profile.Provider)
	profile.RepositoryID = strings.TrimSpace(profile.RepositoryID)
	profile.Ref = normalizeRef(profile.Ref)
	profile.CatalogPath = normalizeCatalogPath(profile.CatalogPath)
	profile.TargetDir = filepath.Clean(strings.TrimSpace(profile.TargetDir))

	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return "", fmt.Errorf("sérialisation du profil projet échouée : %w", err)
	}

	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return "", fmt.Errorf("écriture de %s échouée : %w", path, err)
	}

	return path, nil
}

// LegacyProfileFromConfig converts the legacy GitHub raw config into a project profile.
func LegacyProfileFromConfig(cfg Config) (*ProjectProfile, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	parsed, err := url.Parse(cfg.RepoRawURL)
	if err != nil {
		return nil, fmt.Errorf("repo_raw_url invalide : %w", err)
	}
	if !strings.EqualFold(parsed.Host, "raw.githubusercontent.com") {
		return nil, fmt.Errorf("import legacy non pris en charge pour %s", parsed.Host)
	}

	parts := splitNonEmpty(parsed.Path)
	if len(parts) < 3 {
		return nil, fmt.Errorf("repo_raw_url GitHub invalide : %s", cfg.RepoRawURL)
	}

	catalogPath := defaultCatalogPath
	if len(parts) > 3 {
		catalogPath = path.Join(strings.Join(parts[3:], "/"), defaultCatalogPath)
	}

	profile := ProjectProfile{
		Provider:           "github",
		RepositoryID:       parts[0] + "/" + parts[1],
		Ref:                parts[2],
		CatalogPath:        catalogPath,
		TargetDir:          cfg.TargetDir,
		MigratedFromLegacy: true,
	}

	return &profile, validateProjectProfile(profile)
}

// CatalogID returns the stable identity for a catalog entry.
func CatalogID(resourceType, folderName, name string) string {
	return strings.ToLower(strings.Join([]string{
		strings.TrimSpace(resourceType),
		strings.TrimSpace(folderName),
		strings.TrimSpace(name),
	}, "::"))
}

// ResolveCatalogEntry resolves a user-facing resource reference from the catalog.
func ResolveCatalogEntry(entries []CatalogEntry, resourceRef string) (CatalogEntry, error) {
	trimmed := strings.TrimSpace(resourceRef)
	if trimmed == "" {
		return CatalogEntry{}, errors.New("la ressource à installer doit être renseignée")
	}

	if strings.Contains(trimmed, "/") || strings.Contains(trimmed, "\\") {
		normalized := strings.ReplaceAll(trimmed, "\\", "/")
		parts := strings.Split(normalized, "/")
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return CatalogEntry{}, fmt.Errorf("référence de ressource invalide : %q", resourceRef)
		}

		for _, entry := range entries {
			if strings.EqualFold(entry.FolderName, parts[0]) && strings.EqualFold(entry.Name, parts[1]) {
				return entry, nil
			}
		}

		return CatalogEntry{}, fmt.Errorf("ressource introuvable : %q", resourceRef)
	}

	matches := make([]CatalogEntry, 0, 2)
	for _, entry := range entries {
		if strings.EqualFold(entry.Name, trimmed) {
			matches = append(matches, entry)
		}
	}

	switch len(matches) {
	case 0:
		return CatalogEntry{}, fmt.Errorf("ressource introuvable : %q", resourceRef)
	case 1:
		return matches[0], nil
	default:
		folders := make([]string, 0, len(matches))
		for _, entry := range matches {
			folders = append(folders, entry.FolderName)
		}
		sort.Strings(folders)
		return CatalogEntry{}, fmt.Errorf("ressource ambiguë : %q (précisez folder/name parmi %s)", resourceRef, strings.Join(folders, ", "))
	}
}

// EnsureTargetDir creates the configured target directory in the current project.
func EnsureTargetDir(targetDir string) error {
	if err := validateTargetDir(targetDir); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Clean(targetDir), 0o755); err != nil {
		return fmt.Errorf("création du dossier %s échouée : %w", targetDir, err)
	}
	return nil
}

// BuildInstalledPath computes the destination path for a catalog entry.
func BuildInstalledPath(targetDir string, entry CatalogEntry) (string, error) {
	if err := validateTargetDir(targetDir); err != nil {
		return "", err
	}
	if err := validateCatalogEntry(entry); err != nil {
		return "", err
	}

	fileName := path.Base(strings.TrimSpace(entry.SourcePath))
	if fileName == "." || fileName == "/" || fileName == "" {
		return "", fmt.Errorf("source_path invalide : %q", entry.SourcePath)
	}

	relativePath := filepath.Join(filepath.Clean(entry.FolderName), filepath.FromSlash(fileName))
	if entry.FolderName == "." {
		relativePath = filepath.FromSlash(fileName)
	}

	return filepath.Join(filepath.Clean(targetDir), relativePath), nil
}

// InstallCatalogEntry writes the catalog entry and updates lore-state.json.
func InstallCatalogEntry(targetDir string, entry CatalogEntry, content []byte) (InstalledResourceRecord, error) {
	record, err := SaveCatalogEntry(targetDir, entry, content)
	if err != nil {
		return InstalledResourceRecord{}, err
	}

	if err := UpsertInstalledResource(targetDir, record); err != nil {
		return InstalledResourceRecord{}, err
	}

	return record, nil
}

// SaveCatalogEntry writes the resource content to disk and returns the tracked record.
func SaveCatalogEntry(targetDir string, entry CatalogEntry, content []byte) (InstalledResourceRecord, error) {
	installedPath, err := BuildInstalledPath(targetDir, entry)
	if err != nil {
		return InstalledResourceRecord{}, err
	}

	if err := os.MkdirAll(filepath.Dir(installedPath), 0o755); err != nil {
		return InstalledResourceRecord{}, fmt.Errorf("création du dossier %s échouée : %w", filepath.Dir(installedPath), err)
	}

	fileContent := addManagedHeaderIfMarkdown(installedPath, content)
	if err := os.WriteFile(installedPath, fileContent, 0o644); err != nil {
		return InstalledResourceRecord{}, fmt.Errorf("écriture de %s échouée : %w", installedPath, err)
	}

	record := InstalledResourceRecord{
		CatalogID:     entry.CatalogID,
		ResourceType:  entry.ResourceType,
		Name:          entry.Name,
		FolderName:    entry.FolderName,
		InstalledPath: filepath.ToSlash(filepath.Join(targetDir, entry.FolderName, filepath.Base(filepath.FromSlash(entry.SourcePath)))),
		SourcePath:    path.Clean(strings.TrimSpace(entry.SourcePath)),
		Status:        "installed",
	}
	if entry.FolderName == "." {
		record.InstalledPath = filepath.ToSlash(filepath.Join(filepath.Clean(targetDir), filepath.Base(filepath.FromSlash(entry.SourcePath))))
	}

	return record, nil
}

// SaveInstalledResource writes an existing tracked resource back to disk.
func SaveInstalledResource(record InstalledResourceRecord, content []byte) error {
	if err := validateInstalledRecord(record); err != nil {
		return err
	}

	installedPath := filepath.Clean(filepath.FromSlash(record.InstalledPath))
	if err := os.MkdirAll(filepath.Dir(installedPath), 0o755); err != nil {
		return fmt.Errorf("création du dossier %s échouée : %w", filepath.Dir(installedPath), err)
	}

	if err := os.WriteFile(installedPath, addManagedHeaderIfMarkdown(installedPath, content), 0o644); err != nil {
		return fmt.Errorf("écriture de %s échouée : %w", installedPath, err)
	}

	return nil
}

// UpsertInstalledResource stores or replaces the given record in lore-state.json.
func UpsertInstalledResource(targetDir string, record InstalledResourceRecord) error {
	if err := validateInstalledRecord(record); err != nil {
		return err
	}
	if err := EnsureTargetDir(targetDir); err != nil {
		return err
	}

	state, err := ReadInstalledState(targetDir)
	if err != nil {
		return err
	}

	entries := make([]InstalledResourceRecord, 0, len(state.Entries)+1)
	replaced := false
	for _, existing := range state.Entries {
		if existing.CatalogID == record.CatalogID {
			entries = append(entries, record)
			replaced = true
			continue
		}
		entries = append(entries, existing)
	}
	if !replaced {
		entries = append(entries, record)
	}

	sort.Slice(entries, func(left, right int) bool {
		return entries[left].CatalogID < entries[right].CatalogID
	})

	state.Entries = entries
	return writeInstalledState(targetDir, state)
}

// ReadInstalledState loads the tracked resource state from lore-state.json.
func ReadInstalledState(targetDir string) (InstalledResourceState, error) {
	if err := validateTargetDir(targetDir); err != nil {
		return InstalledResourceState{}, err
	}

	statePath := filepath.Join(filepath.Clean(targetDir), stateFileName)
	data, err := os.ReadFile(statePath)
	if os.IsNotExist(err) {
		return InstalledResourceState{}, nil
	}
	if err != nil {
		return InstalledResourceState{}, fmt.Errorf("lecture de %s échouée : %w", statePath, err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return InstalledResourceState{}, nil
	}

	var state InstalledResourceState
	if err := json.Unmarshal(data, &state); err == nil && len(state.Entries) > 0 {
		for _, entry := range state.Entries {
			if err := validateInstalledRecord(entry); err != nil {
				return InstalledResourceState{}, fmt.Errorf("fichier d'état invalide (%s) : %w", statePath, err)
			}
		}
		return state, nil
	}

	var legacy map[string]string
	if err := json.Unmarshal(data, &legacy); err != nil {
		return InstalledResourceState{}, fmt.Errorf("fichier d'état malformé (%s) : %w", statePath, err)
	}

	entries := make([]InstalledResourceRecord, 0, len(legacy))
	for skillName, status := range legacy {
		if status != "installed" || strings.TrimSpace(skillName) == "" {
			continue
		}
		entries = append(entries, InstalledResourceRecord{
			CatalogID:     CatalogID("skill", "skills", skillName),
			ResourceType:  "skill",
			Name:          skillName,
			FolderName:    "skills",
			InstalledPath: filepath.ToSlash(filepath.Join(filepath.Clean(targetDir), "skills", skillName+".md")),
			SourcePath:    path.Join("skills", skillName+".md"),
			Status:        "installed",
		})
	}

	sort.Slice(entries, func(left, right int) bool {
		return entries[left].CatalogID < entries[right].CatalogID
	})

	return InstalledResourceState{Entries: entries}, nil
}

// ListInstalledResources returns the tracked installed resources sorted by catalog ID.
func ListInstalledResources(targetDir string) ([]InstalledResourceRecord, error) {
	state, err := ReadInstalledState(targetDir)
	if err != nil {
		return nil, err
	}
	if len(state.Entries) == 0 {
		return nil, nil
	}

	entries := make([]InstalledResourceRecord, 0, len(state.Entries))
	for _, entry := range state.Entries {
		if entry.Status != "installed" {
			continue
		}
		entries = append(entries, entry)
	}

	sort.Slice(entries, func(left, right int) bool {
		return entries[left].CatalogID < entries[right].CatalogID
	})

	if len(entries) == 0 {
		return nil, nil
	}

	return entries, nil
}

// ReplaceExecutableWithAsset verifies the downloaded archive and replaces the target executable.
func ReplaceExecutableWithAsset(executablePath, assetName string, assetData, checksumManifest []byte) error {
	trimmedExecutablePath := strings.TrimSpace(executablePath)
	if trimmedExecutablePath == "" {
		return errors.New("chemin du binaire courant introuvable")
	}
	if strings.TrimSpace(assetName) == "" {
		return errors.New("nom d'artefact de release introuvable")
	}

	expectedChecksum, err := checksumForAsset(checksumManifest, assetName)
	if err != nil {
		return err
	}
	actualChecksum := sha256.Sum256(assetData)
	if strings.ToLower(expectedChecksum) != hex.EncodeToString(actualChecksum[:]) {
		return fmt.Errorf("checksum invalide pour %s", assetName)
	}

	binaryData, fileMode, err := extractBinaryFromArchive(assetName, assetData)
	if err != nil {
		return err
	}

	targetPath := filepath.Clean(trimmedExecutablePath)
	tempPath := targetPath + ".update"
	if err := os.WriteFile(tempPath, binaryData, fileMode); err != nil {
		return fmt.Errorf("écriture du binaire temporaire échouée : %w", err)
	}
	defer func() {
		_ = os.Remove(tempPath)
	}()

	if runtime.GOOS != "windows" {
		if err := os.Rename(tempPath, targetPath); err != nil {
			return fmt.Errorf("remplacement du binaire échoué : %w", err)
		}
		return nil
	}

	backupPath := targetPath + ".bak"
	_ = os.Remove(backupPath)
	if err := os.Rename(targetPath, backupPath); err != nil {
		return fmt.Errorf("remplacement du binaire échoué : %w", err)
	}
	if err := os.Rename(tempPath, targetPath); err != nil {
		_ = os.Rename(backupPath, targetPath)
		return fmt.Errorf("activation de la nouvelle version échouée : %w", err)
	}
	_ = os.Remove(backupPath)

	return nil
}

func writeInstalledState(targetDir string, state InstalledResourceState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("sérialisation de l'état échouée : %w", err)
	}

	statePath := filepath.Join(filepath.Clean(targetDir), stateFileName)
	if err := os.WriteFile(statePath, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("écriture de %s échouée : %w", statePath, err)
	}

	return nil
}

func checksumForAsset(checksumManifest []byte, assetName string) (string, error) {
	lines := strings.Split(string(checksumManifest), "\n")
	for _, line := range lines {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}
		candidateName := strings.TrimPrefix(fields[len(fields)-1], "*")
		if candidateName == assetName {
			return strings.ToLower(fields[0]), nil
		}
	}

	return "", fmt.Errorf("checksum introuvable pour %s", assetName)
}

func extractBinaryFromArchive(assetName string, assetData []byte) ([]byte, os.FileMode, error) {
	if strings.HasSuffix(assetName, ".zip") {
		reader, err := zip.NewReader(bytes.NewReader(assetData), int64(len(assetData)))
		if err != nil {
			return nil, 0, fmt.Errorf("archive zip invalide : %w", err)
		}
		for _, file := range reader.File {
			baseName := path.Base(file.Name)
			if baseName != "loreskills" && baseName != "loreskills.exe" {
				continue
			}
			opened, err := file.Open()
			if err != nil {
				return nil, 0, fmt.Errorf("lecture de %s échouée : %w", file.Name, err)
			}
			defer opened.Close()

			data, err := io.ReadAll(opened)
			if err != nil {
				return nil, 0, fmt.Errorf("lecture de %s échouée : %w", file.Name, err)
			}

			mode := file.Mode()
			if mode == 0 {
				mode = 0o755
			}
			return data, mode, nil
		}

		return nil, 0, fmt.Errorf("binaire LoreSkills introuvable dans %s", assetName)
	}

	if strings.HasSuffix(assetName, ".tar.gz") {
		gzipReader, err := gzip.NewReader(bytes.NewReader(assetData))
		if err != nil {
			return nil, 0, fmt.Errorf("archive tar.gz invalide : %w", err)
		}
		defer gzipReader.Close()

		tarReader := tar.NewReader(gzipReader)
		for {
			header, err := tarReader.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, 0, fmt.Errorf("lecture de l'archive tar échouée : %w", err)
			}

			baseName := path.Base(header.Name)
			if baseName != "loreskills" && baseName != "loreskills.exe" {
				continue
			}

			data, err := io.ReadAll(tarReader)
			if err != nil {
				return nil, 0, fmt.Errorf("lecture de %s échouée : %w", header.Name, err)
			}

			mode := os.FileMode(header.Mode)
			if mode == 0 {
				mode = 0o755
			}
			return data, mode, nil
		}

		return nil, 0, fmt.Errorf("binaire LoreSkills introuvable dans %s", assetName)
	}

	return nil, 0, fmt.Errorf("format d'archive non pris en charge : %s", assetName)
}

func addManagedHeaderIfMarkdown(installedPath string, content []byte) []byte {
	if !strings.EqualFold(filepath.Ext(installedPath), ".md") {
		return content
	}
	if bytes.HasPrefix(content, []byte(skillHeader)) {
		return content
	}

	return append([]byte(skillHeader), content...)
}

func validateConfig(cfg Config) error {
	if strings.TrimSpace(cfg.RepoRawURL) == "" {
		return errors.New("repo_raw_url doit être renseigné")
	}
	if _, err := url.ParseRequestURI(strings.TrimSpace(cfg.RepoRawURL)); err != nil {
		return fmt.Errorf("repo_raw_url invalide : %w", err)
	}
	if strings.TrimSpace(cfg.TargetDir) == "" {
		return errors.New("target_dir doit être renseigné")
	}

	return validateTargetDir(cfg.TargetDir)
}

func validateProjectProfile(profile ProjectProfile) error {
	provider := normalizeProvider(profile.Provider)
	if provider == "" {
		return errors.New("provider doit être renseigné")
	}
	switch provider {
	case "github", "gitlab", "azure-devops", "bitbucket":
	default:
		return fmt.Errorf("provider non pris en charge : %s", profile.Provider)
	}
	if strings.TrimSpace(profile.RepositoryID) == "" {
		return errors.New("repository_id doit être renseigné")
	}
	if normalizeRef(profile.Ref) == "" {
		return errors.New("ref doit être renseigné")
	}
	if err := validateCatalogPath(profile.CatalogPath); err != nil {
		return err
	}
	if err := validateTargetDir(profile.TargetDir); err != nil {
		return err
	}

	return nil
}

func validateCatalogEntry(entry CatalogEntry) error {
	if strings.TrimSpace(entry.ResourceType) == "" {
		return errors.New("resource_type doit être renseigné")
	}
	if strings.TrimSpace(entry.Name) == "" {
		return errors.New("name doit être renseigné")
	}
	if err := validateFolderName(entry.FolderName); err != nil {
		return err
	}
	if err := validateSourcePath(entry.SourcePath); err != nil {
		return err
	}
	if strings.TrimSpace(entry.CatalogID) == "" {
		entry.CatalogID = CatalogID(entry.ResourceType, entry.FolderName, entry.Name)
	}

	return nil
}

func validateInstalledRecord(record InstalledResourceRecord) error {
	if strings.TrimSpace(record.CatalogID) == "" {
		return errors.New("catalog_id doit être renseigné")
	}
	if strings.TrimSpace(record.ResourceType) == "" {
		return errors.New("resource_type doit être renseigné")
	}
	if strings.TrimSpace(record.Name) == "" {
		return errors.New("name doit être renseigné")
	}
	if err := validateFolderName(record.FolderName); err != nil {
		return err
	}
	if strings.TrimSpace(record.InstalledPath) == "" {
		return errors.New("installed_path doit être renseigné")
	}
	if filepath.IsAbs(filepath.FromSlash(record.InstalledPath)) {
		return errors.New("installed_path doit rester relatif au projet courant")
	}
	if err := validateSourcePath(record.SourcePath); err != nil {
		return err
	}
	if strings.TrimSpace(record.Status) == "" {
		return errors.New("status doit être renseigné")
	}

	return nil
}

func validateTargetDir(targetDir string) error {
	trimmed := strings.TrimSpace(targetDir)
	if trimmed == "" {
		return errors.New("target_dir doit être renseigné")
	}
	if filepath.IsAbs(trimmed) {
		return errors.New("target_dir doit être un chemin relatif au projet courant")
	}
	normalized := filepath.Clean(trimmed)
	if normalized == ".." || strings.HasPrefix(normalized, ".."+string(filepath.Separator)) {
		return errors.New("target_dir doit rester sous le projet courant")
	}

	return nil
}

func validateCatalogPath(catalogPath string) error {
	normalized := normalizeCatalogPath(catalogPath)
	if normalized == "" {
		return errors.New("catalog_path doit être renseigné")
	}
	if path.IsAbs(normalized) {
		return errors.New("catalog_path doit rester relatif au dépôt distant")
	}
	if normalized != defaultCatalogPath && path.Base(normalized) != defaultCatalogPath {
		return errors.New("catalog_path doit pointer vers SKILLS.md")
	}

	return nil
}

func validateFolderName(folderName string) error {
	trimmed := strings.TrimSpace(folderName)
	if trimmed == "" {
		return errors.New("folder_name doit être renseigné")
	}
	normalized := path.Clean(strings.ReplaceAll(trimmed, "\\", "/"))
	if path.IsAbs(normalized) || normalized == ".." || strings.HasPrefix(normalized, "../") {
		return fmt.Errorf("folder_name invalide : %q", folderName)
	}

	return nil
}

func validateSourcePath(sourcePath string) error {
	trimmed := strings.TrimSpace(sourcePath)
	if trimmed == "" {
		return errors.New("source_path doit être renseigné")
	}
	normalized := path.Clean(strings.ReplaceAll(trimmed, "\\", "/"))
	if path.IsAbs(normalized) || normalized == ".." || strings.HasPrefix(normalized, "../") {
		return fmt.Errorf("source_path invalide : %q", sourcePath)
	}

	return nil
}

func normalizeProvider(provider string) string {
	return strings.ToLower(strings.TrimSpace(provider))
}

func normalizeRef(ref string) string {
	trimmed := strings.TrimSpace(ref)
	if trimmed == "" {
		return defaultRef
	}

	return trimmed
}

func normalizeCatalogPath(catalogPath string) string {
	trimmed := strings.TrimSpace(catalogPath)
	if trimmed == "" {
		return defaultCatalogPath
	}

	return path.Clean(strings.ReplaceAll(trimmed, "\\", "/"))
}

func splitNonEmpty(value string) []string {
	parts := strings.Split(value, "/")
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		filtered = append(filtered, trimmed)
	}

	return filtered
}
