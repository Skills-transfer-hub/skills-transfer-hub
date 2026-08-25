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
	updateCacheDirName         = "sth"
	updateCacheFileName        = "update-check.json"
	defaultUpdateCacheTTLHours = 72
	errorUpdateCacheTTLHours   = 1
	updateCheckStatusNoUpdate  = "no-update"
	updateCheckStatusAvailable = "update-available"
	updateCheckStatusError     = "error"
)

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

func (state UpdateCheckState) WithReleaseSummary(highlights []string, releaseNotesURL string) UpdateCheckState {
	if strings.TrimSpace(state.Status) != updateCheckStatusAvailable {
		return state
	}
	trimmed := make([]string, 0, len(highlights))
	for _, highlight := range highlights {
		clean := strings.TrimSpace(highlight)
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
		TTLHours:       errorUpdateCacheTTLHours,
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
			offer.Message = "A stable update is available. Run `sth update self`."
		} else {
			offer.Message = fmt.Sprintf("A stable update is available: %s (run `sth update self`).", DisplayVersion(offer.LatestStableVersion))
		}
	case updateCheckStatusError:
		offer.Status = "error"
		offer.Message = "Update check unavailable."
		if strings.TrimSpace(state.ErrorMessage) != "" {
			offer.Message = fmt.Sprintf("Update check unavailable: %s", strings.TrimSpace(state.ErrorMessage))
		}
	default:
		offer.Status = "fresh-no-update"
		offer.Message = "No stable update is available."
	}

	return offer
}

func UpdateCheckStatePath() (string, error) {
	baseDir := strings.TrimSpace(os.Getenv("STH_USER_CACHE_DIR"))
	if baseDir == "" {
		userCacheDir, err := os.UserCacheDir()
		if err != nil {
			return "", fmt.Errorf("unable to locate the user cache: %w", err)
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
		return nil, fmt.Errorf("unable to read %s: %w", cachePath, err)
	}

	var state UpdateCheckState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("malformed update cache file (%s): %w", cachePath, err)
	}
	if err := validateUpdateCheckState(state); err != nil {
		return nil, fmt.Errorf("invalid update cache file (%s): %w", cachePath, err)
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
		return "", fmt.Errorf("unable to create the update cache directory: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return "", fmt.Errorf("unable to serialize the update cache: %w", err)
	}
	if err := os.WriteFile(cachePath, append(data, '\n'), 0o644); err != nil {
		return "", fmt.Errorf("unable to write %s: %w", cachePath, err)
	}

	return cachePath, nil
}

func validateUpdateCheckState(state UpdateCheckState) error {
	if strings.TrimSpace(state.LastCheckedAt) == "" {
		return errors.New("last_checked_at is required")
	}
	if _, err := time.Parse(time.RFC3339, strings.TrimSpace(state.LastCheckedAt)); err != nil {
		return fmt.Errorf("invalid last_checked_at: %w", err)
	}
	if state.TTLHours <= 0 {
		return errors.New("ttl_hours must be greater than zero")
	}
	if strings.TrimSpace(state.CurrentVersion) == "" {
		return errors.New("current_version is required")
	}
	switch strings.TrimSpace(state.Status) {
	case updateCheckStatusNoUpdate:
	case updateCheckStatusAvailable:
		if strings.TrimSpace(state.LatestStableVersion) == "" {
			return errors.New("latest_stable_version is required for update-available")
		}
		if strings.TrimSpace(state.DownloadURL) == "" || strings.TrimSpace(state.ChecksumURL) == "" || strings.TrimSpace(state.AssetName) == "" {
			return errors.New("download_url, checksum_url, and asset_name are required for update-available")
		}
	case updateCheckStatusError:
		if strings.TrimSpace(state.ErrorMessage) == "" {
			return errors.New("error_message is required for error")
		}
	default:
		return fmt.Errorf("invalid update status: %s", state.Status)
	}

	return nil
}

func SyncManagedBundle(targetDir string, entry CatalogEntry, files map[string][]byte) (InstalledResourceRecord, error) {
	if err := validateTargetDir(targetDir); err != nil {
		return InstalledResourceRecord{}, err
	}
	if err := validateCatalogEntry(entry); err != nil {
		return InstalledResourceRecord{}, err
	}
	if len(files) == 0 {
		return InstalledResourceRecord{}, fmt.Errorf("no bundle files to synchronize for %s", entry.CatalogID)
	}

	sourceRoot := path.Dir(path.Clean(strings.ReplaceAll(strings.TrimSpace(entry.SourcePath), "\\", "/")))
	if sourceRoot == "." || sourceRoot == "/" || sourceRoot == "" {
		return InstalledResourceRecord{}, fmt.Errorf("invalid bundle source_path: %q", entry.SourcePath)
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
			return InstalledResourceRecord{}, fmt.Errorf("unable to resolve bundle file %s: %w", sourcePath, relErr)
		}
		if relativePath == "." || relativePath == "" || relativePath == ".." || strings.HasPrefix(relativePath, "../") {
			return InstalledResourceRecord{}, fmt.Errorf("bundle file outside managed root: %s", sourcePath)
		}

		installedPath, resolveErr := ResolveManagedPath(managedRoot, relativePath)
		if resolveErr != nil {
			return InstalledResourceRecord{}, resolveErr
		}
		if err := os.MkdirAll(filepath.Dir(installedPath), 0o755); err != nil {
			return InstalledResourceRecord{}, fmt.Errorf("failed to create directory %s: %w", filepath.Dir(installedPath), err)
		}
		if err := os.WriteFile(installedPath, addManagedHeaderIfMarkdown(installedPath, files[sourcePath]), 0o644); err != nil {
			return InstalledResourceRecord{}, fmt.Errorf("failed to write %s: %w", installedPath, err)
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
				return InstalledResourceRecord{}, fmt.Errorf("failed to remove %s: %w", managedFile.InstalledPath, err)
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
