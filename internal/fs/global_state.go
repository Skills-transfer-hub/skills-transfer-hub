package fs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

type GlobalContext struct {
	Home      string
	StateRoot string
	Roots     map[string]string
}

type GlobalSource struct {
	Provider   string `json:"provider"`
	Host       string `json:"host"`
	Repository string `json:"repository"`
	Subdir     string `json:"subdir"`
}

type GlobalFile struct {
	RootID       string `json:"root_id"`
	RelativePath string `json:"relative_path"`
	SHA256       string `json:"sha256"`
	Mode         uint32 `json:"mode"`
}

type GlobalPlacement struct {
	RootID       string       `json:"root_id"`
	RelativePath string       `json:"relative_path"`
	ResolvedRoot string       `json:"resolved_root"`
	Consumers    []string     `json:"consumers"`
	Files        []GlobalFile `json:"files"`
}

type GlobalInstallation struct {
	ID                 string            `json:"id"`
	SkillName          string            `json:"skill_name"`
	Source             GlobalSource      `json:"source"`
	RequestedRef       string            `json:"requested_ref"`
	Pinned             bool              `json:"pinned"`
	ResolvedRevision   string            `json:"resolved_revision"`
	PreviousRevision   string            `json:"previous_revision,omitempty"`
	RequestedConsumers []string          `json:"requested_consumers"`
	AllAgents          bool              `json:"all_agents"`
	Placements         []GlobalPlacement `json:"placements"`
	Provenance         []GlobalFile      `json:"provenance,omitempty"`
	InstalledAt        string            `json:"installed_at"`
	UpdatedAt          string            `json:"updated_at"`
}

type GlobalState struct {
	SchemaVersion int                  `json:"schema_version"`
	Installations []GlobalInstallation `json:"installations"`
}

type GlobalWrite struct {
	RootID       string
	RelativePath string
	Data         []byte
	Mode         uint32
}

type GlobalMutationOptions struct {
	DryRun bool
	Force  bool
}

type GlobalMutationResult struct {
	Changed    bool
	BackupPath string
	Warnings   []string
}

type GlobalFileStatus struct {
	GlobalFile
	Path   string `json:"path"`
	Status string `json:"status"`
}

func NewGlobalContext(home string, roots map[string]string) (GlobalContext, error) {
	if !filepath.IsAbs(home) || filepath.Clean(home) != home {
		return GlobalContext{}, fmt.Errorf("global home must be an absolute clean path")
	}
	canonicalHome, err := filepath.EvalSymlinks(home)
	if err != nil {
		return GlobalContext{}, fmt.Errorf("resolve global home: %w", err)
	}
	context := GlobalContext{Home: canonicalHome, Roots: make(map[string]string)}
	for rootID, root := range roots {
		if rootID == "" || rootID == "provenance" || strings.ContainsAny(rootID, `/\\`) {
			return GlobalContext{}, fmt.Errorf("invalid global root ID %q", rootID)
		}
		if !filepath.IsAbs(root) || filepath.Clean(root) != root {
			return GlobalContext{}, fmt.Errorf("global root must be an absolute clean path: %s", root)
		}
		relative, err := filepath.Rel(home, root)
		if err != nil || relative == "." || strings.HasPrefix(relative, "..") || len(strings.Split(filepath.ToSlash(relative), "/")) < 2 {
			return GlobalContext{}, fmt.Errorf("global root must be a dedicated skills directory below home: %s", root)
		}
		context.Roots[rootID] = filepath.Join(canonicalHome, relative)
	}
	settingsPath, err := UserSettingsPath()
	if err != nil {
		return GlobalContext{}, err
	}
	settingsPath, err = filepath.Abs(settingsPath)
	if err != nil {
		return GlobalContext{}, err
	}
	configBase, err := canonicalGlobalConfigBase(filepath.Dir(filepath.Dir(settingsPath)))
	if err != nil {
		return GlobalContext{}, err
	}
	context.StateRoot = filepath.Join(configBase, filepath.Base(filepath.Dir(settingsPath)), "global")
	for _, root := range context.Roots {
		stateFolded := strings.ToLower(context.StateRoot)
		rootFolded := strings.ToLower(root)
		if stateFolded == rootFolded || strings.HasPrefix(stateFolded, rootFolded+string(filepath.Separator)) || strings.HasPrefix(rootFolded, stateFolded+string(filepath.Separator)) {
			return GlobalContext{}, fmt.Errorf("private global state must be outside agent discovery roots: %s", context.StateRoot)
		}
	}
	return context, nil
}

func canonicalGlobalConfigBase(base string) (string, error) {
	current := base
	missing := []string{}
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for index := len(missing) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, missing[index])
			}
			return resolved, nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", err
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

func ReadGlobalState(context GlobalContext) (GlobalState, error) {
	if err := validateGlobalAbsolutePath(context.StateRoot); err != nil {
		return GlobalState{}, err
	}
	statePath := filepath.Join(context.StateRoot, "state.json")
	if err := validateGlobalAbsolutePath(statePath); err != nil {
		return GlobalState{}, err
	}
	content, err := os.ReadFile(statePath)
	if os.IsNotExist(err) {
		return GlobalState{SchemaVersion: 1, Installations: []GlobalInstallation{}}, nil
	}
	if err != nil {
		return GlobalState{}, fmt.Errorf("read global state: %w", err)
	}
	if !utf8.Valid(content) {
		return GlobalState{}, fmt.Errorf("invalid UTF-8 in global state")
	}
	var state GlobalState
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return GlobalState{}, fmt.Errorf("invalid global state: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return GlobalState{}, fmt.Errorf("invalid trailing data in global state")
	}
	if state.SchemaVersion != 1 {
		return GlobalState{}, fmt.Errorf("unsupported global state schema version %d", state.SchemaVersion)
	}
	if state.Installations == nil {
		state.Installations = []GlobalInstallation{}
	}
	return state, nil
}

func validateGlobalAbsolutePath(target string) error {
	if !filepath.IsAbs(target) || filepath.Clean(target) != target {
		return fmt.Errorf("invalid absolute global path %q", target)
	}
	current := target
	for {
		info, err := os.Lstat(current)
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink is not allowed in global path: %s", current)
		}
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("inspect global path %s: %w", current, err)
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return nil
}
