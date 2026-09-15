package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"sth/internal/fs"
	sthhttp "sth/internal/http"
)

func (a *app) runStatus(args []string) error {
	flags, _ := parseFlags(args, "--json", "--local")
	jsonOut := flags["--json"]

	statusErr := a.runStatusInnerWithLocal(jsonOut, flags["--local"])
	if statusErr != nil && jsonOut {
		emitJSONError(a.stdout, statusErr, 1)
	}
	return statusErr
}

func (a *app) runStatusInner(jsonOut bool) error { return a.runStatusInnerWithLocal(jsonOut, false) }

func (a *app) runStatusInnerWithLocal(jsonOut, localOnly bool) error {
	if configPath, err := fs.ProjectProfilePath(); err == nil {
		if absolutePath, absolutePathErr := filepath.Abs(configPath); absolutePathErr == nil {
			a.verbose.Path("project.json", absolutePath)
		}
	}
	config, err := fs.ReadProjectConfigV2ReadOnly()
	if err != nil {
		return err
	}

	stateDirs, dirsErr := projectStateDirs(config)
	if dirsErr != nil {
		return dirsErr
	}

	state, err := fs.ReadInstalledStateAcrossDirs(stateDirs)
	if err != nil {
		return err
	}
	projectRoot, _ := filepath.Abs(".")
	v3State, v3Err := fs.ReadInstalledStateV3OrLegacy(projectRoot, stateDirs)
	if v3Err != nil {
		return v3Err
	}
	useV3 := len(v3State.Resources) > 0

	if len(state.Entries) == 0 && !useV3 {
		if jsonOut {
			a.writeStdout("[]")
		} else {
			a.writeStdout("No resources installed.")
		}
		return nil
	}

	var latestByPath map[string]string
	if !localOnly {
		if err := a.withAuthRecovery(context.Background(), func() error {
			resolved, resolveErr := a.latestVersionsByPath(context.Background(), config, state)
			if resolveErr != nil {
				return fmt.Errorf("network error fetching latest versions: %w", resolveErr)
			}
			latestByPath = resolved
			return nil
		}); err != nil {
			return err
		}

	}
	results := fs.GroupStatusesByResource(fs.ComputeAllStatuses(state, config, latestByPath, projectRoot))
	v3Results := []fs.StatusResultV3(nil)
	if useV3 {
		v3Results = fs.ComputeStatusesV3(v3State, projectRoot)
		for resultIndex := range v3Results {
			for _, legacyResult := range results {
				if legacyResult.ResourceName != v3Results[resultIndex].ResourceID || legacyResult.ProviderID != v3Results[resultIndex].ProviderID {
					continue
				}
				v3Results[resultIndex].LatestVersion = legacyResult.LatestVersion
				v3Results[resultIndex].Pinned = legacyResult.Pinned
				v3Results[resultIndex].PinnedVersion = legacyResult.PinnedVersion
				if v3Results[resultIndex].Status != fs.ResourceStatusModifiedLocally && legacyResult.Status == fs.ResourceStatusOutdated {
					v3Results[resultIndex].Status = fs.ResourceStatusOutdated
				}
				break
			}
		}
	}

	if jsonOut {
		jsonValue := any(results)
		if useV3 {
			jsonValue = v3Results
		}
		data, marshalErr := json.MarshalIndent(jsonValue, "", "  ")
		if marshalErr != nil {
			return marshalErr
		}
		a.writeStdout(string(data))
		return nil
	}
	if useV3 {
		for _, result := range v3Results {
			line := fmt.Sprintf("%-30s  %s", result.ResourceID, result.Status)
			if result.Status == fs.ResourceStatusOutdated && !result.Pinned {
				line += fmt.Sprintf("  installed: %.8s  available: %.8s", result.InstalledVersion, result.LatestVersion)
			}
			if result.Pinned {
				line += fmt.Sprintf("  (pinned at %s)", result.PinnedVersion)
			}
			a.writeStdout(line)
			if result.Pinned && result.LatestVersion != "" && result.LatestVersion != result.InstalledVersion {
				a.writeStdout(fmt.Sprintf("  → Newer version available for %s (pinned at %s)", result.ResourceID, result.PinnedVersion))
			}
			for _, modifiedOutput := range result.ModifiedOutputs {
				a.writeStdout(fmt.Sprintf("    modified: %s", modifiedOutput))
			}
		}
		return nil
	}

	showProvider := len(config.Providers) > 1
	for _, result := range results {
		line := fmt.Sprintf("%-30s  %s", result.ResourceName, result.Status)
		if showProvider {
			line = fmt.Sprintf("%-30s  %-16s  %s", result.ResourceName, statusProviderLabel(config, result), result.Status)
		}
		if result.Status == fs.ResourceStatusOutdated && !result.Pinned {
			line += fmt.Sprintf("  installed: %.8s  available: %.8s", result.InstalledVersion, result.LatestVersion)
		}
		if result.Pinned {
			line += fmt.Sprintf("  (pinned at %s)", result.PinnedVersion)
		}
		a.writeStdout(line)
		if result.Pinned && result.LatestVersion != "" && result.LatestVersion != result.InstalledVersion {
			a.writeStdout(fmt.Sprintf("  → Newer version available for %s (pinned at %s)", result.ResourceName, result.PinnedVersion))
		}
		if result.Status == fs.ResourceStatusModifiedLocally {
			for _, modifiedFile := range result.ModifiedFiles {
				a.writeStdout(fmt.Sprintf("    modified: %s", modifiedFile))
			}
		}
		if result.Status == fs.ResourceStatusUnknownBaseline {
			a.writeStdout(fmt.Sprintf("  → re-install with `sth install --force %s` to establish integrity tracking", result.ResourceName))
		}
	}

	return nil
}

func statusProviderLabel(config fs.ProjectConfigV2, result fs.StatusResult) string {
	if strings.TrimSpace(result.ProviderID) != "" {
		return result.ProviderID
	}
	return fs.DefaultProviderID(config)
}

func projectStateDirs(config fs.ProjectConfigV2) ([]string, error) {
	if len(config.Providers) > 0 {
		return fs.StateTargetDirs(config), nil
	}
	profile, err := fs.ReadProjectProfile()
	if err != nil {
		return nil, err
	}
	return []string{profile.TargetDir}, nil
}

func isProviderConfigError(err error) bool {
	var providerConfigErr *sthhttp.ProviderConfigError
	return errors.As(err, &providerConfigErr)
}
