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
	"sth/internal/writer"
	"strings"
	"time"
)

const (
	configFileName        = ".sthrc"
	projectConfigDirName  = ".sth"
	projectConfigFileName = "project.json"
	stateFileName         = "sth-state.json"
	stateFileNameV2       = "sth-state-v2.json"
	defaultRef            = "main"
	defaultCatalogPath    = "SKILLS.md"
	defaultTargetDir      = ".claude/skills"
)

// skillHeader is the visible STH banner prepended to Markdown files installed
// for Claude Code. Single source of truth lives in the writer package so the
// fs legacy paths (SaveInstalledResource, SyncManagedBundle, RefreshManagedFileHashes)
// can not drift from what ClaudeWriter actually writes.
const skillHeader = writer.ClaudeManagedHeader
const legacySkillHeader = "> ⚠️ ATTENTION : Skill géré par STH. Ne pas modifier manuellement.\n\n"

// Config stores the legacy STH user configuration.
type Config struct {
	RepoRawURL string `json:"repo_raw_url"`
	TargetDir  string `json:"target_dir"`
}

// ProjectProfile stores the project-local STH configuration.
type ProjectProfile struct {
	Provider           string `json:"provider"`
	RepositoryID       string `json:"repository_id"`
	Ref                string `json:"ref"`
	CatalogPath        string `json:"catalog_path"`
	TargetDir          string `json:"target_dir"`
	MigratedFromLegacy bool   `json:"migrated_from_legacy,omitempty"`
	// APIToken is a session-only token entered during init for immediate access validation.
	// It is never persisted to disk.
	APIToken string `json:"-"`
}

// CatalogEntry describes one installable resource from SKILLS.md.
//
// ArtifactKind, SourceKind, AppliesTo and Lang are PR3+ extensions for
// multi-source / multi-target discovery. They are omitempty so legacy
// catalog files stay untouched on round-trip; empty ArtifactKind is
// interpreted as "skill" by downstream consumers.
type CatalogEntry struct {
	CatalogID    string   `json:"catalog_id"`
	ResourceType string   `json:"resource_type"`
	Name         string   `json:"name"`
	FolderName   string   `json:"folder_name"`
	SourcePath   string   `json:"source_path"`
	Description  string   `json:"description,omitempty"`
	ArtifactKind string   `json:"artifact_kind,omitempty"`
	SourceKind   string   `json:"source_kind,omitempty"`
	AppliesTo    []string `json:"applies_to,omitempty"`
	Lang         string   `json:"lang,omitempty"`
}

// ManagedFileRecord describes one file owned by a managed bundle installation.
type ManagedFileRecord struct {
	RelativePath  string `json:"relative_path"`
	InstalledPath string `json:"installed_path"`
	SourcePath    string `json:"source_path"`
}

func NormalizeRepositoryID(provider, repositoryID string) (string, error) {
	normalizedProvider := normalizeProvider(provider)
	trimmed := strings.TrimSpace(repositoryID)
	if trimmed == "" {
		return "", errors.New("repository_id is required")
	}

	normalized := strings.ReplaceAll(trimmed, "\\", "/")
	expectedHost := supportedRepositoryHost(normalizedProvider)
	hasScheme := strings.Contains(normalized, "://")
	if !hasScheme {
		if strings.HasPrefix(normalized, "/") || strings.HasSuffix(normalized, "/") || strings.Contains(normalized, "//") {
			return "", fmt.Errorf("invalid repository_id for %s", provider)
		}
	}

	if hasScheme {
		parsed, err := url.Parse(normalized)
		if err != nil {
			return "", fmt.Errorf("invalid repository_id: %w", err)
		}
		if expectedHost != "" && !strings.EqualFold(strings.TrimSpace(parsed.Host), expectedHost) {
			return "", fmt.Errorf("repository_id must reference hosted provider %s", expectedHost)
		}
		normalized = strings.Trim(parsed.Path, "/")
	}

	if expectedHost != "" && strings.HasPrefix(strings.ToLower(normalized), expectedHost+"/") {
		normalized = normalized[len(expectedHost)+1:]
	}

	parts := splitNonEmpty(normalized)
	if len(parts) > 0 && strings.Contains(parts[0], ".") && expectedHost != "" && !strings.EqualFold(parts[0], expectedHost) {
		return "", fmt.Errorf("repository_id must reference hosted provider %s", expectedHost)
	}
	if len(parts) > 0 && expectedHost != "" && strings.EqualFold(parts[0], expectedHost) {
		parts = parts[1:]
	}

	if normalizedProvider == "azure-devops" && len(parts) == 4 && parts[2] == "_git" {
		parts = []string{parts[0], parts[1], parts[3]}
	}

	switch normalizedProvider {
	case "github", "bitbucket":
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid repository_id for %s", provider)
		}
	case "gitlab":
		if len(parts) < 2 {
			return "", fmt.Errorf("invalid repository_id for %s", provider)
		}
	case "azure-devops":
		if len(parts) != 3 {
			return "", fmt.Errorf("invalid repository_id for %s", provider)
		}
	default:
		return "", fmt.Errorf("unsupported provider: %s", provider)
	}

	return strings.Join(parts, "/"), nil
}

// InstalledResourceRecord describes one installed resource in sth-state.json.
type InstalledResourceRecord struct {
	CatalogID     string              `json:"catalog_id"`
	ResourceType  string              `json:"resource_type"`
	Name          string              `json:"name"`
	FolderName    string              `json:"folder_name"`
	InstalledPath string              `json:"installed_path"`
	SourcePath    string              `json:"source_path"`
	ManagedRoot   string              `json:"managed_root,omitempty"`
	SourceRoot    string              `json:"source_root,omitempty"`
	ManagedFiles  []ManagedFileRecord `json:"managed_files,omitempty"`
	Status        string              `json:"status"`
}

// InstalledResourceState stores the tracked installed resources.
type InstalledResourceState struct {
	Entries []InstalledResourceRecord `json:"entries"`
}

// ConfigPath returns the absolute path to ~/.sthrc.
func ConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("unable to locate home directory: %w", err)
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
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("malformed configuration file (%s): %w", path, err)
	}
	if err := validateConfig(cfg); err != nil {
		return nil, fmt.Errorf("invalid configuration (%s): %w", path, err)
	}
	cfg.RepoRawURL = strings.TrimRight(strings.TrimSpace(cfg.RepoRawURL), "/")
	cfg.TargetDir = filepath.Clean(strings.TrimSpace(cfg.TargetDir))

	return &cfg, nil
}

// WriteDefaultConfig creates ~/.sthrc with STH starter values.
func WriteDefaultConfig() (string, error) {
	path, err := ConfigPath()
	if err != nil {
		return "", err
	}

	if _, err := os.Stat(path); err == nil {
		return path, fmt.Errorf("a configuration file already exists: %s", path)
	} else if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("failed to check %s: %w", path, err)
	}

	defaults := Config{
		RepoRawURL: "https://raw.githubusercontent.com/your-organization/your-repository/main",
		TargetDir:  defaultTargetDir,
	}

	data, err := json.MarshalIndent(defaults, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to serialize configuration: %w", err)
	}

	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return "", fmt.Errorf("failed to write %s: %w", path, err)
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
		return nil, fmt.Errorf("configuration not found (%s); run `sth init`", path)
	}

	return cfg, nil
}

// ProjectProfilePath returns the absolute path to the project-local profile.
func ProjectProfilePath() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("unable to locate the current project: %w", err)
	}

	return filepath.Join(wd, projectConfigDirName, projectConfigFileName), nil
}

// ReadProjectProfileIfPresent loads the project-local profile if it exists.
// For v2 format files it returns the first provider's data as a ProjectProfile
// so that callers written against the v1 API continue to work.
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
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}

	// Try v1 format first.
	var profile ProjectProfile
	if jsonErr := json.Unmarshal(data, &profile); jsonErr == nil {
		if validateErr := validateProjectProfile(profile); validateErr == nil {
			profile.Provider = normalizeProvider(profile.Provider)
			profile.RepositoryID = strings.TrimSpace(profile.RepositoryID)
			profile.Ref = normalizeRef(profile.Ref)
			profile.CatalogPath = normalizeCatalogPath(profile.CatalogPath)
			profile.TargetDir = filepath.Clean(strings.TrimSpace(profile.TargetDir))
			return &profile, nil
		}
	}

	// Try v2 fallback — return first provider as ProjectProfile so that
	// list/install/update continue to work after init writes v2 format.
	var v2cfg ProjectConfigV2
	if jsonErr := json.Unmarshal(data, &v2cfg); jsonErr == nil && len(v2cfg.Providers) > 0 {
		p := v2cfg.Providers[0]
		profile = ProjectProfile{
			Provider:     p.Provider,
			RepositoryID: p.RepositoryID,
			Ref:          p.CatalogRef,
			CatalogPath:  p.CatalogPath,
			TargetDir:    p.TargetDir,
		}
		if validateErr := validateProjectProfile(profile); validateErr == nil {
			profile.Provider = normalizeProvider(profile.Provider)
			profile.RepositoryID = strings.TrimSpace(profile.RepositoryID)
			profile.Ref = normalizeRef(profile.Ref)
			profile.CatalogPath = normalizeCatalogPath(profile.CatalogPath)
			profile.TargetDir = filepath.Clean(strings.TrimSpace(profile.TargetDir))
			return &profile, nil
		}
	}

	return nil, fmt.Errorf("malformed project profile (%s)", path)
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
		return nil, fmt.Errorf("project configuration not found (%s); run `sth init` in this project", path)
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
		return "", fmt.Errorf("failed to create project configuration directory: %w", err)
	}

	profile.Provider = normalizeProvider(profile.Provider)
	normalizedRepositoryID, err := NormalizeRepositoryID(profile.Provider, profile.RepositoryID)
	if err != nil {
		return "", err
	}
	profile.RepositoryID = normalizedRepositoryID
	profile.Ref = normalizeRef(profile.Ref)
	profile.CatalogPath = normalizeCatalogPath(profile.CatalogPath)
	profile.TargetDir = filepath.Clean(strings.TrimSpace(profile.TargetDir))

	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to serialize project profile: %w", err)
	}

	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return "", fmt.Errorf("failed to write %s: %w", path, err)
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
		return nil, fmt.Errorf("invalid repo_raw_url: %w", err)
	}
	if !strings.EqualFold(parsed.Host, "raw.githubusercontent.com") {
		return nil, fmt.Errorf("legacy import is not supported for %s", parsed.Host)
	}

	parts := splitNonEmpty(parsed.Path)
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid GitHub repo_raw_url: %s", cfg.RepoRawURL)
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

// ParseCatalogID splits a catalog ID (type::folder::name) into its components.
// Returns false if the ID does not have exactly three non-empty parts.
func ParseCatalogID(id string) (resourceType, folderName, name string, ok bool) {
	parts := strings.SplitN(id, "::", 3)
	if len(parts) != 3 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" || strings.TrimSpace(parts[2]) == "" {
		return "", "", "", false
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), strings.TrimSpace(parts[2]), true
}

// ProjectProfileFromProvider builds a ProjectProfile from a ProviderEntry.
// If the entry's TargetDir is empty, fallbackTargetDir is used instead.
func ProjectProfileFromProvider(p ProviderEntry, fallbackTargetDir string) ProjectProfile {
	ref := p.CatalogRef
	if ref == "" {
		ref = defaultRef
	}
	targetDir := p.TargetDir
	if targetDir == "" {
		targetDir = fallbackTargetDir
	}
	return ProjectProfile{
		Provider:     p.Provider,
		RepositoryID: p.RepositoryID,
		Ref:          ref,
		CatalogPath:  p.CatalogPath,
		TargetDir:    targetDir,
	}
}

// ResolveCatalogEntry resolves a user-facing resource reference from the catalog.
func ResolveCatalogEntry(entries []CatalogEntry, resourceRef string) (CatalogEntry, error) {
	trimmed := strings.TrimSpace(resourceRef)
	if trimmed == "" {
		return CatalogEntry{}, errors.New("the resource to install is required")
	}

	if strings.Contains(trimmed, "/") || strings.Contains(trimmed, "\\") {
		normalized := strings.ReplaceAll(trimmed, "\\", "/")
		parts := strings.Split(normalized, "/")
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return CatalogEntry{}, fmt.Errorf("invalid resource reference: %q", resourceRef)
		}

		for _, entry := range entries {
			if strings.EqualFold(entry.FolderName, parts[0]) && strings.EqualFold(entry.Name, parts[1]) {
				return entry, nil
			}
		}

		return CatalogEntry{}, fmt.Errorf("resource not found: %q", resourceRef)
	}

	matches := make([]CatalogEntry, 0, 2)
	for _, entry := range entries {
		if strings.EqualFold(entry.Name, trimmed) {
			matches = append(matches, entry)
		}
	}

	switch len(matches) {
	case 0:
		return CatalogEntry{}, fmt.Errorf("resource not found: %q", resourceRef)
	case 1:
		return matches[0], nil
	default:
		folders := make([]string, 0, len(matches))
		for _, entry := range matches {
			folders = append(folders, entry.FolderName)
		}
		sort.Strings(folders)
		return CatalogEntry{}, fmt.Errorf("ambiguous resource: %q (specify folder/name from %s)", resourceRef, strings.Join(folders, ", "))
	}
}

// EnsureTargetDir creates the configured target directory in the current project.
func EnsureTargetDir(targetDir string) error {
	if err := validateTargetDir(targetDir); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Clean(targetDir), 0o755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", targetDir, err)
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
		return "", fmt.Errorf("invalid source_path: %q", entry.SourcePath)
	}

	relativePath := filepath.Join(filepath.Clean(entry.FolderName), filepath.FromSlash(fileName))
	if entry.FolderName == "." {
		relativePath = filepath.FromSlash(fileName)
	}

	return filepath.Join(filepath.Clean(targetDir), relativePath), nil
}

func BundleID(categoryName, folderName string) string {
	return strings.ToLower(strings.TrimSpace(categoryName) + "::" + strings.TrimSpace(folderName))
}

func BuildManagedRoot(targetDir, categoryName, folderName string) (string, error) {
	if err := validateTargetDir(targetDir); err != nil {
		return "", err
	}
	if err := validateManagedPathSegment(categoryName); err != nil {
		return "", err
	}
	if err := validateManagedPathSegment(folderName); err != nil {
		return "", err
	}

	return filepath.Join(filepath.Clean(targetDir), filepath.FromSlash(path.Join(categoryName, folderName))), nil
}

func ResolveManagedPath(managedRoot, relativePath string) (string, error) {
	trimmedRoot := filepath.Clean(strings.TrimSpace(managedRoot))
	if trimmedRoot == "" || trimmedRoot == "." {
		return "", errors.New("managed_root is required")
	}

	normalized := path.Clean(strings.ReplaceAll(strings.TrimSpace(relativePath), "\\", "/"))
	if normalized == "." || normalized == "" {
		return trimmedRoot, nil
	}
	if path.IsAbs(normalized) || normalized == ".." || strings.HasPrefix(normalized, "../") {
		return "", fmt.Errorf("invalid managed path: %q", relativePath)
	}

	candidate := filepath.Join(trimmedRoot, filepath.FromSlash(normalized))
	relative, err := filepath.Rel(trimmedRoot, candidate)
	if err != nil {
		return "", fmt.Errorf("unable to resolve managed path: %w", err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid managed path: %q", relativePath)
	}

	return candidate, nil
}

// InstallCatalogEntry writes the catalog entry and updates sth-state.json.
func InstallCatalogEntry(targetDir string, entry CatalogEntry, content []byte) (InstalledResourceRecord, error) {
	record, _, err := SaveCatalogEntry(targetDir, entry, content)
	if err != nil {
		return InstalledResourceRecord{}, err
	}

	if err := UpsertInstalledResource(targetDir, record); err != nil {
		return InstalledResourceRecord{}, err
	}

	return record, nil
}

// SaveCatalogEntry writes the resource content to disk and returns the tracked record and managed file.
// The on-disk write is delegated to the Claude target writer; other targets are wired in PR5.
func SaveCatalogEntry(targetDir string, entry CatalogEntry, content []byte) (InstalledResourceRecord, ManagedFile, error) {
	installedPath, err := BuildInstalledPath(targetDir, entry)
	if err != nil {
		return InstalledResourceRecord{}, ManagedFile{}, err
	}

	result, err := writer.NewClaudeWriter().Write(writer.WriteRequest{
		InstalledPath:    installedPath,
		Content:          content,
		AddManagedHeader: true,
	})
	if err != nil {
		return InstalledResourceRecord{}, ManagedFile{}, err
	}

	managedFile := ManagedFile{
		Path:   result.Path,
		SHA256: result.SHA256,
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

	return record, managedFile, nil
}

// SaveInstalledResource writes an existing tracked resource back to disk.
func SaveInstalledResource(record InstalledResourceRecord, content []byte) error {
	if err := validateInstalledRecord(record); err != nil {
		return err
	}

	installedPath := filepath.Clean(filepath.FromSlash(record.InstalledPath))
	if err := os.MkdirAll(filepath.Dir(installedPath), 0o755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", filepath.Dir(installedPath), err)
	}

	if err := os.WriteFile(installedPath, addManagedHeaderIfMarkdown(installedPath, content), 0o644); err != nil {
		return fmt.Errorf("failed to write %s: %w", installedPath, err)
	}

	return nil
}

// UpsertInstalledResource stores or replaces the given record in sth-state.json.
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

// ReadInstalledState loads the tracked resource state from sth-state.json.
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
		return InstalledResourceState{}, fmt.Errorf("failed to read %s: %w", statePath, err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return InstalledResourceState{}, nil
	}

	var state InstalledResourceState
	if err := json.Unmarshal(data, &state); err == nil && len(state.Entries) > 0 {
		for _, entry := range state.Entries {
			if err := validateInstalledRecord(entry); err != nil {
				return InstalledResourceState{}, fmt.Errorf("invalid state file (%s): %w", statePath, err)
			}
		}
		return state, nil
	}

	var legacy map[string]string
	if err := json.Unmarshal(data, &legacy); err != nil {
		return InstalledResourceState{}, fmt.Errorf("malformed state file (%s): %w", statePath, err)
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
func ReplaceExecutableWithAsset(executablePath, assetName string, assetData, checksumManifest, checksumSignature []byte) error {
	trimmedExecutablePath := strings.TrimSpace(executablePath)
	if trimmedExecutablePath == "" {
		return errors.New("current binary path not found")
	}
	if strings.TrimSpace(assetName) == "" {
		return errors.New("release asset name not found")
	}

	if err := verifyChecksumSignature(checksumManifest, checksumSignature); err != nil {
		return err
	}

	expectedChecksum, err := checksumForAsset(checksumManifest, assetName)
	if err != nil {
		return err
	}
	actualChecksum := sha256.Sum256(assetData)
	if strings.ToLower(expectedChecksum) != hex.EncodeToString(actualChecksum[:]) {
		return fmt.Errorf("invalid checksum for %s", assetName)
	}

	binaryData, fileMode, err := extractBinaryFromArchive(assetName, assetData)
	if err != nil {
		return err
	}

	targetPath := filepath.Clean(trimmedExecutablePath)
	tempPath := targetPath + ".update"
	if err := os.WriteFile(tempPath, binaryData, fileMode); err != nil {
		return fmt.Errorf("failed to write temporary binary: %w", err)
	}
	defer func() {
		_ = os.Remove(tempPath)
	}()

	if runtime.GOOS != "windows" {
		if err := os.Rename(tempPath, targetPath); err != nil {
			return fmt.Errorf("failed to replace binary: %w", err)
		}
		return nil
	}

	backupPath := targetPath + ".bak"
	_ = os.Remove(backupPath)
	if err := os.Rename(targetPath, backupPath); err != nil {
		return fmt.Errorf("failed to replace binary: %w", err)
	}
	if err := os.Rename(tempPath, targetPath); err != nil {
		_ = os.Rename(backupPath, targetPath)
		return fmt.Errorf("failed to activate new version: %w", err)
	}
	_ = os.Remove(backupPath)

	return nil
}

func writeInstalledState(targetDir string, state InstalledResourceState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize state: %w", err)
	}

	statePath := filepath.Join(filepath.Clean(targetDir), stateFileName)
	if err := os.WriteFile(statePath, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("failed to write %s: %w", statePath, err)
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

	return "", fmt.Errorf("checksum not found for %s", assetName)
}

func extractBinaryFromArchive(assetName string, assetData []byte) ([]byte, os.FileMode, error) {
	if strings.HasSuffix(assetName, ".zip") {
		reader, err := zip.NewReader(bytes.NewReader(assetData), int64(len(assetData)))
		if err != nil {
			return nil, 0, fmt.Errorf("invalid zip archive: %w", err)
		}
		for _, file := range reader.File {
			baseName := path.Base(file.Name)
			if baseName != "sth" && baseName != "sth.exe" {
				continue
			}
			opened, err := file.Open()
			if err != nil {
				return nil, 0, fmt.Errorf("failed to read %s: %w", file.Name, err)
			}
			defer opened.Close()

			data, err := io.ReadAll(opened)
			if err != nil {
				return nil, 0, fmt.Errorf("failed to read %s: %w", file.Name, err)
			}

			mode := file.Mode()
			if mode == 0 {
				mode = 0o755
			}
			return data, mode, nil
		}

		return nil, 0, fmt.Errorf("STH binary not found in %s", assetName)
	}

	if strings.HasSuffix(assetName, ".tar.gz") {
		gzipReader, err := gzip.NewReader(bytes.NewReader(assetData))
		if err != nil {
			return nil, 0, fmt.Errorf("invalid tar.gz archive: %w", err)
		}
		defer gzipReader.Close()

		tarReader := tar.NewReader(gzipReader)
		for {
			header, err := tarReader.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, 0, fmt.Errorf("failed to read tar archive: %w", err)
			}

			baseName := path.Base(header.Name)
			if baseName != "sth" && baseName != "sth.exe" {
				continue
			}

			data, err := io.ReadAll(tarReader)
			if err != nil {
				return nil, 0, fmt.Errorf("failed to read %s: %w", header.Name, err)
			}

			mode := os.FileMode(header.Mode)
			if mode == 0 {
				mode = 0o755
			}
			return data, mode, nil
		}

		return nil, 0, fmt.Errorf("STH binary not found in %s", assetName)
	}

	return nil, 0, fmt.Errorf("unsupported archive format: %s", assetName)
}

func addManagedHeaderIfMarkdown(installedPath string, content []byte) []byte {
	if !strings.EqualFold(filepath.Ext(installedPath), ".md") {
		return content
	}

	headerBytes := []byte(skillHeader)
	legacyHeaderBytes := []byte(legacySkillHeader)

	if bodyStart := writer.FrontmatterBodyOffset(content); bodyStart >= 0 {
		// File has YAML frontmatter (e.g. SKILL.md). Claude requires the
		// frontmatter on line 1, so we inject the STH warning *after* the
		// closing `---` instead of prepending it.
		if bytes.HasPrefix(content[bodyStart:], headerBytes) {
			return content
		}
		if bytes.HasPrefix(content[bodyStart:], legacyHeaderBytes) {
			updatedContent := make([]byte, 0, len(content)-len(legacyHeaderBytes)+len(headerBytes))
			updatedContent = append(updatedContent, content[:bodyStart]...)
			updatedContent = append(updatedContent, headerBytes...)
			updatedContent = append(updatedContent, content[bodyStart+len(legacyHeaderBytes):]...)
			return updatedContent
		}
		out := make([]byte, 0, len(content)+len(headerBytes))
		out = append(out, content[:bodyStart]...)
		out = append(out, headerBytes...)
		out = append(out, content[bodyStart:]...)
		return out
	}

	if bytes.HasPrefix(content, headerBytes) {
		return content
	}
	if bytes.HasPrefix(content, legacyHeaderBytes) {
		updatedContent := make([]byte, 0, len(content)-len(legacyHeaderBytes)+len(headerBytes))
		updatedContent = append(updatedContent, headerBytes...)
		updatedContent = append(updatedContent, content[len(legacyHeaderBytes):]...)
		return updatedContent
	}
	return append(headerBytes, content...)
}

// (frontmatterBodyOffset moved to internal/writer.FrontmatterBodyOffset
// so the writer package owns the single source of truth.)

func validateConfig(cfg Config) error {
	if strings.TrimSpace(cfg.RepoRawURL) == "" {
		return errors.New("repo_raw_url is required")
	}
	if _, err := url.ParseRequestURI(strings.TrimSpace(cfg.RepoRawURL)); err != nil {
		return fmt.Errorf("invalid repo_raw_url: %w", err)
	}
	if strings.TrimSpace(cfg.TargetDir) == "" {
		return errors.New("target_dir is required")
	}

	return validateTargetDir(cfg.TargetDir)
}

func validateProjectProfile(profile ProjectProfile) error {
	provider := normalizeProvider(profile.Provider)
	if provider == "" {
		return errors.New("provider is required")
	}
	switch provider {
	case "github", "gitlab", "azure-devops", "bitbucket":
	default:
		return fmt.Errorf("unsupported provider: %s", profile.Provider)
	}
	if strings.TrimSpace(profile.RepositoryID) == "" {
		return errors.New("repository_id is required")
	}
	if _, err := NormalizeRepositoryID(provider, profile.RepositoryID); err != nil {
		return err
	}
	if normalizeRef(profile.Ref) == "" {
		return errors.New("ref is required")
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
		return errors.New("resource_type is required")
	}
	if strings.TrimSpace(entry.Name) == "" {
		return errors.New("name is required")
	}
	if err := validateFolderName(entry.FolderName); err != nil {
		return err
	}
	if err := validateSourcePath(entry.SourcePath); err != nil {
		return err
	}
	if strings.TrimSpace(entry.CatalogID) == "" {
		return errors.New("catalog_id is required")
	}

	return nil
}

func validateInstalledRecord(record InstalledResourceRecord) error {
	if strings.TrimSpace(record.CatalogID) == "" {
		return errors.New("catalog_id is required")
	}
	if strings.TrimSpace(record.ResourceType) == "" {
		return errors.New("resource_type is required")
	}
	if strings.TrimSpace(record.Name) == "" {
		return errors.New("name is required")
	}
	if err := validateFolderName(record.FolderName); err != nil {
		return err
	}
	if strings.TrimSpace(record.InstalledPath) == "" {
		return errors.New("installed_path is required")
	}
	if filepath.IsAbs(filepath.FromSlash(record.InstalledPath)) {
		return errors.New("installed_path must remain relative to the current project")
	}
	if err := validateSourcePath(record.SourcePath); err != nil {
		return err
	}
	managedRoot := strings.TrimSpace(record.ManagedRoot)
	if managedRoot != "" {
		if filepath.IsAbs(filepath.FromSlash(managedRoot)) {
			return errors.New("managed_root must remain relative to the current project")
		}
		if filepath.Clean(filepath.FromSlash(managedRoot)) != filepath.Clean(filepath.FromSlash(record.InstalledPath)) {
			return errors.New("managed_root must match installed_path for a managed bundle")
		}
		if strings.TrimSpace(record.SourceRoot) == "" {
			return errors.New("source_root is required for a managed bundle")
		}
		if len(record.ManagedFiles) == 0 {
			return errors.New("managed_files must contain at least one file for a managed bundle")
		}
		for _, managedFile := range record.ManagedFiles {
			if err := validateManagedFileRecord(managedFile, record.ManagedRoot, record.SourceRoot); err != nil {
				return err
			}
		}
	}
	if strings.TrimSpace(record.Status) == "" {
		return errors.New("status is required")
	}

	return nil
}

func validateTargetDir(targetDir string) error {
	trimmed := strings.TrimSpace(targetDir)
	if trimmed == "" {
		return errors.New("target_dir is required")
	}
	if filepath.IsAbs(trimmed) {
		return errors.New("target_dir must be a path relative to the current project")
	}
	normalized := filepath.Clean(trimmed)
	if normalized == ".." || strings.HasPrefix(normalized, ".."+string(filepath.Separator)) {
		return errors.New("target_dir must remain within the current project")
	}

	return nil
}

func validateManagedPathSegment(segment string) error {
	trimmed := strings.TrimSpace(segment)
	if trimmed == "" {
		return errors.New("managed segment is required")
	}
	if strings.Contains(trimmed, "/") || strings.Contains(trimmed, "\\") {
		return fmt.Errorf("invalid managed segment: %q", segment)
	}
	if trimmed == "." || trimmed == ".." {
		return fmt.Errorf("invalid managed segment: %q", segment)
	}

	return nil
}

func validateManagedFileRecord(record ManagedFileRecord, managedRoot, sourceRoot string) error {
	trimmedRelativePath := strings.TrimSpace(record.RelativePath)
	if trimmedRelativePath == "" {
		return errors.New("relative_path is required")
	}
	normalizedRelativePath := path.Clean(strings.ReplaceAll(trimmedRelativePath, "\\", "/"))
	if normalizedRelativePath == "." || path.IsAbs(normalizedRelativePath) || normalizedRelativePath == ".." || strings.HasPrefix(normalizedRelativePath, "../") {
		return fmt.Errorf("invalid relative_path: %q", record.RelativePath)
	}
	if strings.TrimSpace(record.InstalledPath) == "" {
		return errors.New("installed_path is required for a managed file")
	}
	if filepath.IsAbs(filepath.FromSlash(record.InstalledPath)) {
		return errors.New("installed_path must remain relative to the current project")
	}
	if err := validateSourcePath(record.SourcePath); err != nil {
		return err
	}

	resolvedInstalledPath, err := ResolveManagedPath(managedRoot, normalizedRelativePath)
	if err != nil {
		return err
	}
	if filepath.Clean(filepath.FromSlash(record.InstalledPath)) != filepath.Clean(resolvedInstalledPath) {
		return errors.New("inconsistent installed_path for a managed file")
	}

	relativeSourcePath, err := relativeRepositoryPath(sourceRoot, record.SourcePath)
	if err != nil {
		return fmt.Errorf("inconsistent source_path for a managed file: %w", err)
	}
	if relativeSourcePath != normalizedRelativePath {
		return errors.New("inconsistent source_path for a managed file")
	}

	return nil
}

func validateCatalogPath(catalogPath string) error {
	normalized := normalizeCatalogPath(catalogPath)
	if normalized == "" {
		return errors.New("catalog_path is required")
	}
	if path.IsAbs(normalized) {
		return errors.New("catalog_path must remain relative to the remote repository")
	}
	if normalized != defaultCatalogPath && path.Base(normalized) != defaultCatalogPath {
		return errors.New("catalog_path must point to SKILLS.md")
	}

	return nil
}

func relativeRepositoryPath(root, fullPath string) (string, error) {
	normalizedRoot := path.Clean(strings.ReplaceAll(strings.TrimSpace(root), "\\", "/"))
	normalizedFullPath := path.Clean(strings.ReplaceAll(strings.TrimSpace(fullPath), "\\", "/"))
	if normalizedRoot == "" || normalizedRoot == "." {
		return "", errors.New("remote root is required")
	}
	if normalizedFullPath == "" || normalizedFullPath == "." {
		return "", errors.New("remote path is required")
	}
	if normalizedFullPath == normalizedRoot {
		return ".", nil
	}

	prefix := normalizedRoot + "/"
	if !strings.HasPrefix(normalizedFullPath, prefix) {
		return "", fmt.Errorf("%q is not contained in %q", normalizedFullPath, normalizedRoot)
	}

	relativePath := path.Clean(strings.TrimPrefix(normalizedFullPath, prefix))
	if relativePath == "" || relativePath == "." || relativePath == ".." || strings.HasPrefix(relativePath, "../") || path.IsAbs(relativePath) {
		return "", fmt.Errorf("invalid relative repository path: %q", relativePath)
	}

	return relativePath, nil
}

func validateFolderName(folderName string) error {
	trimmed := strings.TrimSpace(folderName)
	if trimmed == "" {
		return errors.New("folder_name is required")
	}
	normalized := path.Clean(strings.ReplaceAll(trimmed, "\\", "/"))
	if path.IsAbs(normalized) || normalized == ".." || strings.HasPrefix(normalized, "../") {
		return fmt.Errorf("invalid folder_name: %q", folderName)
	}

	return nil
}

func validateSourcePath(sourcePath string) error {
	trimmed := strings.TrimSpace(sourcePath)
	if trimmed == "" {
		return errors.New("source_path is required")
	}
	normalized := path.Clean(strings.ReplaceAll(trimmed, "\\", "/"))
	if path.IsAbs(normalized) || normalized == ".." || strings.HasPrefix(normalized, "../") {
		return fmt.Errorf("invalid source_path: %q", sourcePath)
	}

	return nil
}

func normalizeProvider(provider string) string {
	return strings.ToLower(strings.TrimSpace(provider))
}

func supportedRepositoryHost(provider string) string {
	switch normalizeProvider(provider) {
	case "github":
		return "github.com"
	case "gitlab":
		return "gitlab.com"
	case "azure-devops":
		return "dev.azure.com"
	case "bitbucket":
		return "bitbucket.org"
	default:
		return ""
	}
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

// ComputeSHA256 returns the lowercase hex-encoded SHA-256 digest of data.
func ComputeSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// ReadProjectConfigV2 loads the v2 project config, migrating from v1 if needed.
// Returns an empty v2 config if the file does not exist.
func ReadProjectConfigV2() (ProjectConfigV2, error) {
	path, err := ProjectProfilePath()
	if err != nil {
		return ProjectConfigV2{}, err
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return ProjectConfigV2{SchemaVersion: 2, Providers: []ProviderEntry{}, Resources: []ResourceEntry{}}, nil
	}
	return MigrateProjectConfig(path, io.Discard)
}

// ReadInstalledStateV2 loads the v2 installed state for targetDir.
// If sth-state-v2.json does not exist, it falls back to sth-state.json,
// converting v1 format in memory without writing to disk.
// Returns an empty v2 state if neither file exists.
func ReadInstalledStateV2(targetDir string) (InstalledResourceStateV2, error) {
	if err := validateTargetDir(targetDir); err != nil {
		return InstalledResourceStateV2{}, err
	}
	statePath := filepath.Join(filepath.Clean(targetDir), stateFileNameV2)
	if _, err := os.Stat(statePath); err == nil {
		data, err := os.ReadFile(statePath)
		if err != nil {
			return InstalledResourceStateV2{}, fmt.Errorf("failed to read %s: %w", statePath, err)
		}
		var state InstalledResourceStateV2
		if err := json.Unmarshal(data, &state); err != nil {
			return InstalledResourceStateV2{}, fmt.Errorf("failed to deserialize v2 state: %w", err)
		}
		return state, nil
	}

	legacyPath := filepath.Join(filepath.Clean(targetDir), stateFileName)
	data, err := os.ReadFile(legacyPath)
	if os.IsNotExist(err) {
		return InstalledResourceStateV2{SchemaVersion: 2, Entries: []InstalledResourceRecordV2{}}, nil
	}
	if err != nil {
		return InstalledResourceStateV2{}, fmt.Errorf("failed to read %s: %w", legacyPath, err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return InstalledResourceStateV2{SchemaVersion: 2, Entries: []InstalledResourceRecordV2{}}, nil
	}
	return parseInstalledStateV2FromBytes(data, legacyPath)
}

// parseInstalledStateV2FromBytes parses a state file that may be v1 or v2 format.
// For v1 format it converts entries in memory without writing to disk.
func parseInstalledStateV2FromBytes(data []byte, path string) (InstalledResourceStateV2, error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return InstalledResourceStateV2{}, fmt.Errorf("malformed state file (%s): %w", path, err)
	}
	if schemaVersion, _ := raw["schema_version"].(float64); int(schemaVersion) == 2 {
		var state InstalledResourceStateV2
		if err := json.Unmarshal(data, &state); err != nil {
			return InstalledResourceStateV2{}, fmt.Errorf("malformed v2 state file (%s): %w", path, err)
		}
		return state, nil
	}
	var v1State struct {
		Entries []struct {
			CatalogID string `json:"catalog_id"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(data, &v1State); err != nil {
		return InstalledResourceStateV2{}, fmt.Errorf("malformed v1 state file (%s): %w", path, err)
	}
	entries := make([]InstalledResourceRecordV2, 0, len(v1State.Entries))
	for _, e := range v1State.Entries {
		entries = append(entries, InstalledResourceRecordV2{
			ResourceName: e.CatalogID,
			ProviderID:   "default",
			ManagedFiles: []ManagedFile{},
		})
	}
	return InstalledResourceStateV2{SchemaVersion: 2, Entries: entries}, nil
}

// PromoteInstalledState creates sth-state-v2.json from sth-state.json if needed.
// It is a no-op when sth-state-v2.json already exists or sth-state.json is absent.
// On v1→v2 promotion, a progress message is written to w. A nil w defaults to io.Discard.
func PromoteInstalledState(targetDir string, w io.Writer) error {
	if w == nil {
		w = io.Discard
	}
	v2Path := filepath.Join(filepath.Clean(targetDir), stateFileNameV2)
	if _, err := os.Stat(v2Path); err == nil {
		return nil
	}
	legacyPath := filepath.Join(filepath.Clean(targetDir), stateFileName)
	data, err := os.ReadFile(legacyPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", legacyPath, err)
	}
	var raw map[string]any
	if jsonErr := json.Unmarshal(data, &raw); jsonErr != nil {
		return fmt.Errorf("malformed state file (%s): %w", legacyPath, jsonErr)
	}
	schemaVersion, _ := raw["schema_version"].(float64)
	needsMigration := int(schemaVersion) != 2
	state, err := ReadInstalledStateV2(targetDir)
	if err != nil {
		return err
	}
	if err := WriteInstalledStateV2(targetDir, state); err != nil {
		if needsMigration {
			fmt.Fprintf(w, "[sth] migrating sth-state.json v1 → v2 … failed: %v\n", err)
		}
		return err
	}
	if needsMigration {
		fmt.Fprintf(w, "[sth] migrating sth-state.json v1 → v2 … done\n")
	}
	return nil
}

// WriteProjectConfigV2 persists a v2 project config to disk.
func WriteProjectConfigV2(cfg ProjectConfigV2) (string, error) {
	cfg.SchemaVersion = 2
	if cfg.Providers == nil {
		cfg.Providers = []ProviderEntry{}
	}
	if cfg.Resources == nil {
		cfg.Resources = []ResourceEntry{}
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to serialize v2 configuration: %w", err)
	}
	path, err := ProjectProfilePath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("failed to create project configuration directory: %w", err)
	}
	if err := atomicWriteFile(path, append(data, '\n'), 0o644); err != nil {
		return "", fmt.Errorf("failed to write %s: %w", path, err)
	}
	return path, nil
}

// WriteInstalledStateV2 persists a v2 installed state for targetDir to disk.
func WriteInstalledStateV2(targetDir string, state InstalledResourceStateV2) error {
	if err := validateTargetDir(targetDir); err != nil {
		return err
	}
	state.SchemaVersion = 2
	if state.Entries == nil {
		state.Entries = []InstalledResourceRecordV2{}
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize v2 state: %w", err)
	}
	statePath := filepath.Join(filepath.Clean(targetDir), stateFileNameV2)
	return atomicWriteFile(statePath, append(data, '\n'), 0o644)
}

// InstallCatalogEntryV2 writes the catalog entry to disk and records it in sth-state-v2.json.
// providerID identifies the source provider in the v2 state; pass "default" for legacy v1 installs.
func InstallCatalogEntryV2(targetDir string, entry CatalogEntry, content []byte, providerID string) (InstalledResourceRecordV2, error) {
	if providerID == "" {
		providerID = "default"
	}
	record, managedFile, err := SaveCatalogEntry(targetDir, entry, content)
	if err != nil {
		return InstalledResourceRecordV2{}, err
	}

	v2record := InstalledResourceRecordV2{
		ResourceName: record.CatalogID,
		ProviderID:   providerID,
		InstalledAt:  time.Now().UTC().Format(time.RFC3339),
		ManagedFiles: []ManagedFile{managedFile},
		SourcePath:   entry.SourcePath,
	}

	if err := UpsertInstalledResourceV2(targetDir, v2record); err != nil {
		return InstalledResourceRecordV2{}, err
	}

	return v2record, nil
}

// UpsertInstalledResourceV2 stores or replaces the given record in sth-state-v2.json.
func UpsertInstalledResourceV2(targetDir string, record InstalledResourceRecordV2) error {
	if err := EnsureTargetDir(targetDir); err != nil {
		return err
	}

	state, err := ReadInstalledStateV2(targetDir)
	if err != nil {
		return err
	}

	for i, existing := range state.Entries {
		if existing.ResourceName == record.ResourceName && existing.TargetID == record.TargetID {
			state.Entries[i] = record
			return WriteInstalledStateV2(targetDir, state)
		}
	}

	state.Entries = append(state.Entries, record)
	sort.Slice(state.Entries, func(left, right int) bool {
		if state.Entries[left].ResourceName != state.Entries[right].ResourceName {
			return state.Entries[left].ResourceName < state.Entries[right].ResourceName
		}
		return state.Entries[left].TargetID < state.Entries[right].TargetID
	})
	return WriteInstalledStateV2(targetDir, state)
}

// SaveInstalledResourceV2 writes an existing v2-tracked resource back to disk and updates its SHA256.
// If the record has no managed files, it is a no-op.
func SaveInstalledResourceV2(record InstalledResourceRecordV2, content []byte) (InstalledResourceRecordV2, error) {
	if len(record.ManagedFiles) == 0 {
		return record, nil
	}

	mf := record.ManagedFiles[0]
	installedPath := filepath.Clean(filepath.FromSlash(mf.Path))
	if err := os.MkdirAll(filepath.Dir(installedPath), 0o755); err != nil {
		return record, fmt.Errorf("failed to create directory %s: %w", filepath.Dir(installedPath), err)
	}

	fileContent := addManagedHeaderIfMarkdown(installedPath, content)
	if err := os.WriteFile(installedPath, fileContent, 0o644); err != nil {
		return record, fmt.Errorf("failed to write %s: %w", installedPath, err)
	}

	record.ManagedFiles[0].SHA256 = ComputeSHA256(fileContent)
	return record, nil
}

// RefreshManagedFileHashes recomputes the SHA256 for each managed file in the
// record based on content (as it would be written to disk), without performing
// any file I/O. For Markdown files the managed header is taken into account.
// Returns the updated record.
func RefreshManagedFileHashes(record InstalledResourceRecordV2, content []byte) InstalledResourceRecordV2 {
	for i, mf := range record.ManagedFiles {
		installedPath := filepath.Clean(filepath.FromSlash(mf.Path))
		fileContent := addManagedHeaderIfMarkdown(installedPath, content)
		record.ManagedFiles[i].SHA256 = ComputeSHA256(fileContent)
	}
	return record
}

// ResolveManagedFilePath returns the expected on-disk path for a catalog entry,
// using the same logic as SaveCatalogEntry/BuildInstalledPath.
func ResolveManagedFilePath(targetDir string, entry CatalogEntry) string {
	p, err := BuildInstalledPath(targetDir, entry)
	if err != nil {
		return filepath.ToSlash(filepath.Join(targetDir, entry.FolderName, filepath.Base(filepath.FromSlash(entry.SourcePath))))
	}
	return filepath.ToSlash(filepath.Clean(p))
}

// UpsertResourceEntry adds or replaces a ResourceEntry in cfg.Resources.
// Returns an error if entry.ResourceName is empty or entry.ProviderID does not
// reference an existing provider in cfg.Providers.
func UpsertResourceEntry(cfg *ProjectConfigV2, entry ResourceEntry) error {
	if strings.TrimSpace(entry.ResourceName) == "" {
		return errors.New("resource_name is required")
	}
	found := false
	for _, p := range cfg.Providers {
		if p.ID == entry.ProviderID {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("provider_id %q not found in configuration", entry.ProviderID)
	}
	for i, existing := range cfg.Resources {
		if existing.ResourceName == entry.ResourceName {
			cfg.Resources[i] = entry
			sort.Slice(cfg.Resources, func(a, b int) bool {
				return cfg.Resources[a].ResourceName < cfg.Resources[b].ResourceName
			})
			return nil
		}
	}
	cfg.Resources = append(cfg.Resources, entry)
	sort.Slice(cfg.Resources, func(a, b int) bool {
		return cfg.Resources[a].ResourceName < cfg.Resources[b].ResourceName
	})
	return nil
}
