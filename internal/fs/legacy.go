package fs

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	legacyProjectConfigDirName = ".loreskills"
	sthProjectConfigDirName    = ".sth"
	legacyStateFileName        = "lore-state.json"
	legacyStateFileNameV2      = "lore-state-v2.json"
)

// HasLegacyProjectDir reports whether a .loreskills/project.json exists in the
// current working directory but no .sth/project.json does. Returns false when
// the .sth directory is already present so the prompt is never repeated.
func HasLegacyProjectDir() (bool, error) {
	wd, err := os.Getwd()
	if err != nil {
		return false, fmt.Errorf("impossible de localiser le projet courant : %w", err)
	}

	sthPath := filepath.Join(wd, sthProjectConfigDirName, projectConfigFileName)
	if _, err := os.Stat(sthPath); err == nil {
		return false, nil
	}

	legacyPath := filepath.Join(wd, legacyProjectConfigDirName, projectConfigFileName)
	if _, err := os.Stat(legacyPath); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	return true, nil
}

// ReadProjectProfileIfPresentFrom loads the project profile from a specific
// directory (e.g. ".loreskills") rather than the default ".sth" / ".loreskills"
// path. Returns nil without error when the file does not exist.
func ReadProjectProfileIfPresentFrom(dir string) (*ProjectProfile, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("impossible de localiser le projet courant : %w", err)
	}

	path := filepath.Join(wd, dir, projectConfigFileName)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lecture de %s échouée : %w", path, err)
	}

	var profile ProjectProfile
	if jsonErr := json.Unmarshal(data, &profile); jsonErr == nil {
		if validateErr := validateProjectProfile(profile); validateErr == nil {
			profile.Provider = normalizeProvider(profile.Provider)
			profile.Ref = normalizeRef(profile.Ref)
			profile.CatalogPath = normalizeCatalogPath(profile.CatalogPath)
			return &profile, nil
		}
	}

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
			profile.Ref = normalizeRef(profile.Ref)
			profile.CatalogPath = normalizeCatalogPath(profile.CatalogPath)
			return &profile, nil
		}
	}

	return nil, fmt.Errorf("profil projet malformé (%s)", path)
}

// MigrateLegacyProjectDir copies .loreskills/project.json to .sth/project.json
// and renames any legacy state files (lore-state.json, lore-state-v2.json)
// inside targetDir to their STH-prefixed names. The .loreskills directory is
// left untouched so the user can inspect and remove it manually. Repeated
// invocations are short-circuited by HasLegacyProjectDir once .sth/project.json
// exists.
//
// Progress messages are written to w; a nil w defaults to io.Discard.
func MigrateLegacyProjectDir(targetDir string, w io.Writer) error {
	if w == nil {
		w = io.Discard
	}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("impossible de localiser le projet courant : %w", err)
	}

	legacyDir := filepath.Join(wd, legacyProjectConfigDirName)
	sthDir := filepath.Join(wd, sthProjectConfigDirName)

	srcPath := filepath.Join(legacyDir, projectConfigFileName)
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("lecture de %s échouée : %w", srcPath, err)
	}

	if err := os.MkdirAll(sthDir, 0o755); err != nil {
		return fmt.Errorf("impossible de créer %s : %w", sthDir, err)
	}

	dstPath := filepath.Join(sthDir, projectConfigFileName)
	if err := atomicWriteFile(dstPath, data, 0o644); err != nil {
		return fmt.Errorf("écriture de %s échouée : %w", dstPath, err)
	}

	if cleanTarget := filepath.Clean(targetDir); cleanTarget != "" && cleanTarget != "." {
		absTarget := cleanTarget
		if !filepath.IsAbs(absTarget) {
			absTarget = filepath.Join(wd, absTarget)
		}
		renameLegacyStateFile(absTarget, legacyStateFileName, stateFileName, w)
		renameLegacyStateFile(absTarget, legacyStateFileNameV2, stateFileNameV2, w)
	}

	fmt.Fprintf(w, "Configuration migrée de %s vers %s.\n", legacyProjectConfigDirName, sthProjectConfigDirName)
	fmt.Fprintf(w, "Vous pouvez supprimer %s/ manuellement quand vous le souhaitez.\n", legacyProjectConfigDirName)
	return nil
}

// renameLegacyStateFile renames an old <targetDir>/<oldName> to <targetDir>/<newName>.
// It is a no-op when the source is missing, and warns on any other error.
func renameLegacyStateFile(targetDir, oldName, newName string, w io.Writer) {
	src := filepath.Join(targetDir, oldName)
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return
	} else if err != nil {
		fmt.Fprintf(w, "avertissement: impossible de lire %s : %v\n", src, err)
		return
	}

	dst := filepath.Join(targetDir, newName)
	if _, err := os.Stat(dst); err == nil {
		fmt.Fprintf(w, "avertissement: %s existe déjà, %s laissé en place.\n", dst, src)
		return
	}

	if err := os.Rename(src, dst); err != nil {
		fmt.Fprintf(w, "avertissement: impossible de renommer %s en %s : %v\n", src, dst, err)
		return
	}
	fmt.Fprintf(w, "Renommé %s en %s.\n", oldName, newName)
}
