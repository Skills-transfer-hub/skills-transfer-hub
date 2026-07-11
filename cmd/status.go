package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sth/internal/fs"
	sthhttp "sth/internal/http"
	"path/filepath"
)

func (a *app) runStatus(args []string) error {
	flags, _ := parseFlags(args, "--json")
	jsonOut := flags["--json"]

	statusErr := a.runStatusInner(jsonOut)
	if statusErr != nil && jsonOut {
		emitJSONError(a.stdout, statusErr, 1)
	}
	return statusErr
}

func (a *app) runStatusInner(jsonOut bool) error {
	if cfgPath, err := fs.ProjectProfilePath(); err == nil {
		if abs, absErr := filepath.Abs(cfgPath); absErr == nil {
			a.verbose.Path("project.json", abs)
		}
	}
	config, err := fs.ReadProjectConfigV2()
	if err != nil {
		return err
	}

	targetDir := ""
	if len(config.Providers) > 0 {
		targetDir = config.Providers[0].TargetDir
	}
	if targetDir == "" {
		profile, profileErr := fs.ReadProjectProfile()
		if profileErr != nil {
			return profileErr
		}
		targetDir = profile.TargetDir
	}

	state, err := fs.ReadInstalledStateV2(targetDir)
	if err != nil {
		return err
	}

	if len(state.Entries) == 0 {
		a.writeStdout("No resources installed.")
		return nil
	}

	var latestByPath map[string]string
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

	projectRoot, _ := filepath.Abs(".")
	results := fs.ComputeAllStatuses(state, config, latestByPath, projectRoot)

	if jsonOut {
		data, marshalErr := json.MarshalIndent(results, "", "  ")
		if marshalErr != nil {
			return marshalErr
		}
		a.writeStdout(string(data))
		return nil
	}

	for _, res := range results {
		line := fmt.Sprintf("%-30s  %s", res.ResourceName, res.Status)
		if res.Status == fs.ResourceStatusOutdated && !res.Pinned {
			line += fmt.Sprintf("  installed: %.8s  available: %.8s", res.InstalledVersion, res.LatestVersion)
		}
		if res.Pinned {
			line += fmt.Sprintf("  (pinned at %s)", res.PinnedVersion)
		}
		a.writeStdout(line)
		if res.Pinned && res.LatestVersion != "" && res.LatestVersion != res.InstalledVersion {
			a.writeStdout(fmt.Sprintf("  → Newer version available for %s (pinned at %s)", res.ResourceName, res.PinnedVersion))
		}
		if res.Status == fs.ResourceStatusModifiedLocally {
			for _, f := range res.ModifiedFiles {
				a.writeStdout(fmt.Sprintf("    modified: %s", f))
			}
		}
		if res.Status == fs.ResourceStatusUnknownBaseline {
			a.writeStdout(fmt.Sprintf("  → re-install with `sth install --force %s` to establish integrity tracking", res.ResourceName))
		}
	}

	return nil
}

// isProviderConfigError reports whether err is a ProviderConfigError —
// a configuration problem (missing provider, unsupported type, missing required
// fields) rather than a genuine network or HTTP failure. Such errors are
// non-fatal for the status command: the resource is reported with an unknown
// latest version.
func isProviderConfigError(err error) bool {
	var pce *sthhttp.ProviderConfigError
	return errors.As(err, &pce)
}

