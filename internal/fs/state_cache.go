package fs

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	updateCacheDirName          = "sth"
	updateCacheFileName         = "update-check.json"
	defaultUpdateCacheTTLHours  = 24
	updateCheckStatusNoUpdate   = "no-update"
	updateCheckStatusAvailable  = "update-available"
	updateCheckStatusError      = "error"
)

// StableUpdateOffer describes the startup update message rendered by the guided experience.
type StableUpdateOffer struct {
	CurrentVersion      string   `json:"current_version"`
	LatestStableVersion string   `json:"latest_stable_version,omitempty"`
	Status              string   `json:"status"`
	Message             string   `json:"message"`
	Highlights          []string `json:"highlights,omitempty"`
	ReleaseNotesURL     string   `json:"release_notes_url,omitempty"`
	DownloadReady       bool     `json:"download_ready"`
}

func (offer StableUpdateOffer) Actionable() bool {
	return offer.Status == "fresh-update-available"
}

// UpdateCheckState stores the last automatic stable update check for the local machine.
type UpdateCheckState struct {
	LastCheckedAt       string   `json:"last_checked_at"`
	TTLHours            int      `json:"ttl_hours"`
	CurrentVersion      string   `json:"current_version"`
	LatestStableVersion string   `json:"latest_stable_version,omitempty"`
	DownloadURL         string   `json:"download_url,omitempty"`
	ChecksumURL         string   `json:"checksum_url,omitempty"`
	AssetName           string   `json:"asset_name,omitempty"`
	Highlights          []string `json:"highlights,omitempty"`
	ReleaseNotesURL     string   `json:"release_notes_url,omitempty"`
	Status              string   `json:"status"`
	ErrorMessage        string   `json:"error_message,omitempty"`
}

func NewNoUpdateCheckState(now time.Time, currentVersion string) UpdateCheckState {
	return UpdateCheckState{
		LastCheckedAt:  now.UTC().Format(time.RFC3339),
		TTLHours:       defaultUpdateCacheTTLHours,
		CurrentVersion: strings.TrimSpace(currentVersion),
		Status:         updateCheckStatusNoUpdate,
	}
}

func NewAvailableUpdateCheckState(now time.Time, currentVersion, latestStableVersion, downloadURL, checksumURL, assetName string) UpdateCheckState {
	return UpdateCheckState{
		LastCheckedAt:       now.UTC().Format(time.RFC3339),
		TTLHours:            defaultUpdateCacheTTLHours,
		CurrentVersion:      strings.TrimSpace(currentVersion),
		LatestStableVersion: strings.TrimSpace(latestStableVersion),
		DownloadURL:         strings.TrimSpace(downloadURL),
		ChecksumURL:         strings.TrimSpace(checksumURL),
		AssetName:           strings.TrimSpace(assetName),
		Status:              updateCheckStatusAvailable,
	}
}

// WithReleaseSummary attaches the user-facing release highlights and the
// release notes URL to an existing update-available state. Returns the state
// unchanged for non-available statuses so callers can chain it after any
// constructor.
func (state UpdateCheckState) WithReleaseSummary(highlights []string, releaseNotesURL string) UpdateCheckState {
	if strings.TrimSpace(state.Status) != updateCheckStatusAvailable {
		return state
	}
	trimmed := make([]string, 0, len(highlights))
	for _, h := range highlights {
		clean := strings.TrimSpace(h)
		if clean == "" {
			continue
		}
		trimmed = append(trimmed, clean)
	}
	state.Highlights = trimmed
	state.ReleaseNotesURL = strings.TrimSpace(releaseNotesURL)
	return state
}

func NewErrorUpdateCheckState(now time.Time, currentVersion, errorMessage string) UpdateCheckState {
	return UpdateCheckState{
		LastCheckedAt:  now.UTC().Format(time.RFC3339),
		TTLHours:       defaultUpdateCacheTTLHours,
		CurrentVersion: strings.TrimSpace(currentVersion),
		Status:         updateCheckStatusError,
		ErrorMessage:   strings.TrimSpace(errorMessage),
	}
}

func (state UpdateCheckState) IsFresh(now time.Time, currentVersion string) bool {
	if strings.TrimSpace(state.CurrentVersion) != strings.TrimSpace(currentVersion) {
		return false
	}
	if strings.TrimSpace(state.LastCheckedAt) == "" {
		return false
	}
	checkedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(state.LastCheckedAt))
	if err != nil {
		return false
	}
	ttlHours := state.TTLHours
	if ttlHours <= 0 {
		ttlHours = defaultUpdateCacheTTLHours
	}

	return now.UTC().Sub(checkedAt.UTC()) < time.Duration(ttlHours)*time.Hour
}

func StableUpdateOfferFromState(state UpdateCheckState) StableUpdateOffer {
	offer := StableUpdateOffer{
		CurrentVersion:      strings.TrimSpace(state.CurrentVersion),
		LatestStableVersion: strings.TrimSpace(state.LatestStableVersion),
	}

	switch strings.TrimSpace(state.Status) {
	case updateCheckStatusAvailable:
		offer.Status = "fresh-update-available"
		offer.DownloadReady = strings.TrimSpace(state.DownloadURL) != "" && strings.TrimSpace(state.ChecksumURL) != "" && strings.TrimSpace(state.AssetName) != ""
		offer.Highlights = append(offer.Highlights, state.Highlights...)
		offer.ReleaseNotesURL = strings.TrimSpace(state.ReleaseNotesURL)
		if offer.LatestStableVersion == "" {
			offer.Message = "Mise à jour stable disponible. Utilisez `sth update self`."
		} else {
			offer.Message = fmt.Sprintf("Mise à jour stable disponible : %s (utilisez `sth update self`).", DisplayVersion(offer.LatestStableVersion))
		}
	case updateCheckStatusError:
		offer.Status = "error"
		offer.Message = "Vérification de mise à jour indisponible."
		if strings.TrimSpace(state.ErrorMessage) != "" {
			offer.Message = fmt.Sprintf("Vérification de mise à jour indisponible : %s", strings.TrimSpace(state.ErrorMessage))
		}
	default:
		offer.Status = "fresh-no-update"
		offer.Message = "Aucune mise à jour stable disponible."
	}

	return offer
}

func UpdateCheckStatePath() (string, error) {
	baseDir := strings.TrimSpace(os.Getenv("STH_USER_CACHE_DIR"))
	if baseDir == "" {
		userCacheDir, err := os.UserCacheDir()
		if err != nil {
			return "", fmt.Errorf("impossible de localiser le cache utilisateur : %w", err)
		}
		baseDir = userCacheDir
	}

	return filepath.Join(filepath.Clean(baseDir), updateCacheDirName, updateCacheFileName), nil
}

func ReadUpdateCheckState() (*UpdateCheckState, error) {
	cachePath, err := UpdateCheckStatePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(cachePath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lecture de %s échouée : %w", cachePath, err)
	}

	var state UpdateCheckState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("fichier de cache update malformé (%s) : %w", cachePath, err)
	}
	if err := validateUpdateCheckState(state); err != nil {
		return nil, fmt.Errorf("fichier de cache update invalide (%s) : %w", cachePath, err)
	}

	return &state, nil
}

func WriteUpdateCheckState(state UpdateCheckState) (string, error) {
	if err := validateUpdateCheckState(state); err != nil {
		return "", err
	}

	cachePath, err := UpdateCheckStatePath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		return "", fmt.Errorf("création du dossier de cache update échouée : %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return "", fmt.Errorf("sérialisation du cache update échouée : %w", err)
	}
	if err := os.WriteFile(cachePath, append(data, '\n'), 0o644); err != nil {
		return "", fmt.Errorf("écriture de %s échouée : %w", cachePath, err)
	}

	return cachePath, nil
}

func validateUpdateCheckState(state UpdateCheckState) error {
	if strings.TrimSpace(state.LastCheckedAt) == "" {
		return errors.New("last_checked_at doit être renseigné")
	}
	if _, err := time.Parse(time.RFC3339, strings.TrimSpace(state.LastCheckedAt)); err != nil {
		return fmt.Errorf("last_checked_at invalide : %w", err)
	}
	if state.TTLHours <= 0 {
		return errors.New("ttl_hours doit être supérieur à zéro")
	}
	if strings.TrimSpace(state.CurrentVersion) == "" {
		return errors.New("current_version doit être renseigné")
	}
	switch strings.TrimSpace(state.Status) {
	case updateCheckStatusNoUpdate:
	case updateCheckStatusAvailable:
		if strings.TrimSpace(state.LatestStableVersion) == "" {
			return errors.New("latest_stable_version doit être renseigné pour update-available")
		}
		if strings.TrimSpace(state.DownloadURL) == "" || strings.TrimSpace(state.ChecksumURL) == "" || strings.TrimSpace(state.AssetName) == "" {
			return errors.New("download_url, checksum_url et asset_name sont requis pour update-available")
		}
	case updateCheckStatusError:
		if strings.TrimSpace(state.ErrorMessage) == "" {
			return errors.New("error_message doit être renseigné pour error")
		}
	default:
		return fmt.Errorf("status update invalide : %s", state.Status)
	}

	return nil
}

// SyncManagedBundle writes all files for a managed bundle, removes obsolete tracked files, and updates sth-state.json.
func SyncManagedBundle(targetDir string, entry CatalogEntry, files map[string][]byte) (InstalledResourceRecord, error) {
	if err := validateTargetDir(targetDir); err != nil {
		return InstalledResourceRecord{}, err
	}
	if err := validateCatalogEntry(entry); err != nil {
		return InstalledResourceRecord{}, err
	}
	if len(files) == 0 {
		return InstalledResourceRecord{}, fmt.Errorf("aucun fichier bundle à synchroniser pour %s", entry.CatalogID)
	}

	sourceRoot := path.Dir(path.Clean(strings.ReplaceAll(strings.TrimSpace(entry.SourcePath), "\\", "/")))
	if sourceRoot == "." || sourceRoot == "/" || sourceRoot == "" {
		return InstalledResourceRecord{}, fmt.Errorf("source_path bundle invalide : %q", entry.SourcePath)
	}

	managedRoot, err := BuildManagedRoot(targetDir, entry.FolderName, entry.Name)
	if err != nil {
		return InstalledResourceRecord{}, err
	}

	previousRecord, err := installedRecordByCatalogID(targetDir, entry.CatalogID)
	if err != nil {
		return InstalledResourceRecord{}, err
	}

	sourcePaths := make([]string, 0, len(files))
	for sourcePath := range files {
		sourcePaths = append(sourcePaths, sourcePath)
	}
	sort.Strings(sourcePaths)

	managedFiles := make([]ManagedFileRecord, 0, len(sourcePaths))
	installedPaths := make(map[string]struct{}, len(sourcePaths))
	for _, sourcePath := range sourcePaths {
		normalizedSourcePath := path.Clean(strings.ReplaceAll(strings.TrimSpace(sourcePath), "\\", "/"))
		relativePath, relErr := relativeRepositoryPath(sourceRoot, normalizedSourcePath)
		if relErr != nil {
			return InstalledResourceRecord{}, fmt.Errorf("impossible de résoudre le fichier bundle %s : %w", sourcePath, relErr)
		}
		if relativePath == "." || relativePath == "" || relativePath == ".." || strings.HasPrefix(relativePath, "../") {
			return InstalledResourceRecord{}, fmt.Errorf("fichier bundle hors racine gérée : %s", sourcePath)
		}

		installedPath, resolveErr := ResolveManagedPath(managedRoot, relativePath)
		if resolveErr != nil {
			return InstalledResourceRecord{}, resolveErr
		}
		if err := os.MkdirAll(filepath.Dir(installedPath), 0o755); err != nil {
			return InstalledResourceRecord{}, fmt.Errorf("création du dossier %s échouée : %w", filepath.Dir(installedPath), err)
		}
		if err := os.WriteFile(installedPath, addManagedHeaderIfMarkdown(installedPath, files[sourcePath]), 0o644); err != nil {
			return InstalledResourceRecord{}, fmt.Errorf("écriture de %s échouée : %w", installedPath, err)
		}

		installedPath = filepath.ToSlash(installedPath)
		installedPaths[installedPath] = struct{}{}
		managedFiles = append(managedFiles, ManagedFileRecord{
			RelativePath:  relativePath,
			InstalledPath: installedPath,
			SourcePath:    normalizedSourcePath,
		})
	}

	if previousRecord != nil {
		for _, managedFile := range previousRecord.ManagedFiles {
			if _, stillTracked := installedPaths[managedFile.InstalledPath]; stillTracked {
				continue
			}
			if err := os.Remove(filepath.Clean(filepath.FromSlash(managedFile.InstalledPath))); err != nil && !os.IsNotExist(err) {
				return InstalledResourceRecord{}, fmt.Errorf("suppression de %s échouée : %w", managedFile.InstalledPath, err)
			}
		}
	}

	record := InstalledResourceRecord{
		CatalogID:     entry.CatalogID,
		ResourceType:  entry.ResourceType,
		Name:          entry.Name,
		FolderName:    entry.FolderName,
		InstalledPath: filepath.ToSlash(managedRoot),
		SourcePath:    path.Clean(strings.TrimSpace(entry.SourcePath)),
		ManagedRoot:   filepath.ToSlash(managedRoot),
		SourceRoot:    sourceRoot,
		ManagedFiles:  managedFiles,
		Status:        "installed",
	}

	if err := UpsertInstalledResource(targetDir, record); err != nil {
		return InstalledResourceRecord{}, err
	}

	return record, nil
}

func installedRecordByCatalogID(targetDir, catalogID string) (*InstalledResourceRecord, error) {
	state, err := ReadInstalledState(targetDir)
	if err != nil {
		return nil, err
	}
	for _, entry := range state.Entries {
		if entry.CatalogID == catalogID {
			record := entry
			return &record, nil
		}
	}

	return nil, nil
}
