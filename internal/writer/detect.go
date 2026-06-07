package writer

import (
	"os"
	"path/filepath"
)

type DetectedTarget struct {
	Kind     string
	RootDir  string
	Evidence string
}

func DetectAvailableTargets(projectRoot string) []DetectedTarget {
	out := make([]DetectedTarget, 0, len(AllTargetSpecs()))
	if projectRoot == "" {
		return out
	}
	for _, spec := range AllTargetSpecs() {
		for _, marker := range spec.DetectMarkers {
			if exists(filepath.Join(projectRoot, filepath.FromSlash(marker.Path))) {
				out = append(out, DetectedTarget{Kind: spec.Kind, RootDir: spec.RootDir, Evidence: marker.Evidence})
				break
			}
		}
	}
	return out
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
