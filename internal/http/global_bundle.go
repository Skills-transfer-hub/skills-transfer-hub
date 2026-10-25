package http

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"
)

const GlobalBundleMaximumFiles = 4096
const GlobalBundleMaximumBytes int64 = 64 << 20

type GlobalBundleFile struct {
	Path    string `json:"path"`
	Content []byte `json:"-"`
	Mode    uint32 `json:"mode"`
}

type ResolvedSkillBundle struct {
	Source       GlobalSource
	Name         string
	Path         string
	Description  string
	Revision     string
	RequestedRef string
	Files        []GlobalBundleFile
	Provenance   []GlobalBundleFile
	Warnings     []string
}

func (c *Client) DownloadGlobalSkill(ctx context.Context, resolution GlobalResolution, candidate GlobalCandidate) (ResolvedSkillBundle, error) {
	if err := validateGlobalRelativePath(candidate.Path); err != nil {
		return ResolvedSkillBundle{}, err
	}
	if !globalSkillName.MatchString(candidate.Name) || path.Base(candidate.Path) != candidate.Name {
		return ResolvedSkillBundle{}, fmt.Errorf("invalid selected skill name or path")
	}
	tree, err := c.globalTree(ctx, resolution)
	if err != nil {
		return ResolvedSkillBundle{}, err
	}
	files, provenance, err := globalBundleEntries(tree, candidate.Path)
	if err != nil {
		return ResolvedSkillBundle{}, err
	}
	bundle := ResolvedSkillBundle{Source: resolution.Source, Name: candidate.Name, Path: candidate.Path, Revision: resolution.Revision, RequestedRef: resolution.RequestedRef, Files: []GlobalBundleFile{}, Provenance: []GlobalBundleFile{}, Warnings: []string{}}
	totalBytes := int64(0)
	entrypointFound := false
	for _, entry := range append(append([]globalTreeEntry{}, files...), provenance...) {
		content, err := c.globalDownload(ctx, resolution, entry.Path)
		if err != nil {
			return ResolvedSkillBundle{}, err
		}
		totalBytes += int64(len(content))
		if totalBytes > GlobalBundleMaximumBytes {
			return ResolvedSkillBundle{}, fmt.Errorf("bundle exceeds cumulative size limit of %d bytes", GlobalBundleMaximumBytes)
		}
		mode := uint32(0o644)
		if entry.Mode == "100755" {
			mode = 0o755
		}
		file := GlobalBundleFile{Path: entry.Path, Content: content, Mode: mode}
		if !strings.HasPrefix(entry.Path, candidate.Path+"/") {
			bundle.Provenance = append(bundle.Provenance, file)
			continue
		}
		file.Path = strings.TrimPrefix(entry.Path, candidate.Path+"/")
		if file.Path == "SKILL.md" {
			validated, warnings, err := validateGlobalSkillMetadata(candidate.Path, content)
			if err != nil {
				return ResolvedSkillBundle{}, err
			}
			if validated.Name != candidate.Name {
				return ResolvedSkillBundle{}, fmt.Errorf("selected skill name changed")
			}
			bundle.Description = validated.Description
			bundle.Warnings = append(bundle.Warnings, warnings...)
			entrypointFound = true
		}
		bundle.Files = append(bundle.Files, file)
	}
	if !entrypointFound {
		return ResolvedSkillBundle{}, fmt.Errorf("selected skill folder %q has no regular SKILL.md", candidate.Path)
	}
	for _, entry := range tree {
		if strings.HasSuffix(entry.Path, "/"+candidate.Name+"/SKILL.md") && path.Dir(entry.Path) != candidate.Path {
			bundle.Warnings = append(bundle.Warnings, fmt.Sprintf("alternative %s excluded; use --path to select it explicitly", path.Dir(entry.Path)))
		}
	}
	return bundle, nil
}

func globalBundleEntries(tree []globalTreeEntry, folder string) ([]globalTreeEntry, []globalTreeEntry, error) {
	files := []globalTreeEntry{}
	provenance := []globalTreeEntry{}
	totalBytes := int64(0)
	for _, entry := range tree {
		if (folder == entry.Path || strings.HasPrefix(folder, entry.Path+"/")) && (entry.Type != "tree" || entry.Mode != "040000") {
			return nil, nil, fmt.Errorf("source ancestor %q is not a regular directory", entry.Path)
		}
		inside := strings.HasPrefix(entry.Path, folder+"/")
		license := !strings.Contains(entry.Path, "/") && globalProvenanceName(entry.Path)
		if !inside && !license {
			continue
		}
		if entry.Type == "tree" && entry.Mode == "040000" {
			continue
		}
		if entry.Type != "blob" || (entry.Mode != "100644" && entry.Mode != "100755") {
			return nil, nil, fmt.Errorf("unsupported Git mode %s for %q: symlinks and submodules cannot be installed", entry.Mode, entry.Path)
		}
		if inside && globalActiveManifest(entry.Path) {
			return nil, nil, fmt.Errorf("skill contains active plugin manifest %q; plugin installation is outside global skills V1", entry.Path)
		}
		if entry.Size < 0 || entry.Size > defaultResponseLimitBytes {
			return nil, nil, fmt.Errorf("file %q exceeds HTTP size limit of %d bytes", entry.Path, defaultResponseLimitBytes)
		}
		totalBytes += entry.Size
		if totalBytes > GlobalBundleMaximumBytes {
			return nil, nil, fmt.Errorf("bundle exceeds cumulative size limit of %d bytes", GlobalBundleMaximumBytes)
		}
		if inside {
			files = append(files, entry)
		} else {
			provenance = append(provenance, entry)
		}
		if len(files)+len(provenance) > GlobalBundleMaximumFiles {
			return nil, nil, fmt.Errorf("bundle exceeds maximum file count of %d", GlobalBundleMaximumFiles)
		}
	}
	sort.Slice(files, func(left, right int) bool { return files[left].Path < files[right].Path })
	sort.Slice(provenance, func(left, right int) bool { return provenance[left].Path < provenance[right].Path })
	return files, provenance, nil
}

func globalProvenanceName(fileName string) bool {
	upper := strings.ToUpper(fileName)
	for _, prefix := range []string{"LICENSE", "LICENCE", "COPYING", "NOTICE"} {
		if upper == prefix || strings.HasPrefix(upper, prefix+".") || strings.HasPrefix(upper, prefix+"-") {
			return true
		}
	}
	return false
}

func globalActiveManifest(filePath string) bool {
	switch strings.ToLower(path.Base(filePath)) {
	case "plugin.json", "gemini-extension.json", "hooks.json":
		return true
	default:
		return false
	}
}
