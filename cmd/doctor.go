package cmd

import (
	"context"
	"errors"
	"fmt"
	stdhttp "net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sth/internal/fs"
)

const doctorReachabilityTimeout = 5 * time.Second

// doctorCheck represents one named diagnostic with its outcome.
type doctorCheck struct {
	name        string
	status      string // "pass", "fail", "skip"
	detail      string
	remediation string
}

// runDoctor runs the four independent checks defined by FR-013 and prints a
// concise pass/fail/skip table. A single failed check yields a non-zero exit;
// skipped checks do not.
func (a *app) runDoctor(args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("usage : sth doctor")
	}

	cfg, cfgErr := fs.ReadProjectConfigV2()
	checks := []doctorCheck{
		a.doctorCheckProjectConfig(cfg, cfgErr),
	}
	checks = append(checks, a.doctorCheckStateFile(cfg, cfgErr))
	checks = append(checks, a.doctorCheckCredential(cfg, cfgErr))
	checks = append(checks, a.doctorChecksProviderReachability(cfg, cfgErr)...)

	failures := 0
	for _, c := range checks {
		marker := "[?]"
		switch c.status {
		case "pass":
			marker = "[✓]"
		case "fail":
			marker = "[✗]"
			failures++
		case "skip":
			marker = "[SKIP]"
		}
		line := fmt.Sprintf("%s %s", marker, c.name)
		if c.detail != "" {
			line = fmt.Sprintf("%s — %s", line, c.detail)
		}
		a.writeStdout(line)
		if c.status == "fail" && c.remediation != "" {
			a.writeStdout(fmt.Sprintf("    → %s", c.remediation))
		}
	}

	if failures > 0 {
		return fmt.Errorf("%d vérification(s) en échec", failures)
	}
	a.writeStdout("All checks passed.")
	return nil
}

func (a *app) doctorCheckProjectConfig(cfg fs.ProjectConfigV2, cfgErr error) doctorCheck {
	cfgPath, _ := fs.ProjectProfilePath()
	abs, _ := filepath.Abs(cfgPath)
	a.verbose.Path("project.json", abs)
	if cfgErr != nil {
		return doctorCheck{
			name:        "Project configuration",
			status:      "fail",
			detail:      cfgErr.Error(),
			remediation: fmt.Sprintf("Run `sth init` to create %s.", abs),
		}
	}
	if len(cfg.Providers) == 0 {
		return doctorCheck{
			name:        "Project configuration",
			status:      "fail",
			detail:      fmt.Sprintf("no provider configured in %s", abs),
			remediation: "Run `sth init` to register at least one provider.",
		}
	}
	return doctorCheck{
		name:   "Project configuration",
		status: "pass",
		detail: fmt.Sprintf("%d provider(s)", len(cfg.Providers)),
	}
}

func (a *app) doctorCheckStateFile(cfg fs.ProjectConfigV2, cfgErr error) doctorCheck {
	if cfgErr != nil || len(cfg.Providers) == 0 {
		return doctorCheck{name: "State file integrity", status: "skip", detail: "no project configured"}
	}
	targetDir := cfg.Providers[0].TargetDir
	if targetDir == "" {
		profile, profileErr := fs.ReadProjectProfile()
		if profileErr != nil {
			return doctorCheck{name: "State file integrity", status: "skip", detail: "target_dir empty"}
		}
		targetDir = profile.TargetDir
	}
	abs, _ := filepath.Abs(filepath.Join(targetDir, "sth-state-v2.json"))
	a.verbose.Path("sth-state-v2.json", abs)
	state, err := fs.ReadInstalledStateV2(targetDir)
	if err != nil {
		return doctorCheck{
			name:        "State file integrity",
			status:      "fail",
			detail:      err.Error(),
			remediation: fmt.Sprintf("Delete %s and re-install your resources.", abs),
		}
	}
	return doctorCheck{
		name:   "State file integrity",
		status: "pass",
		detail: fmt.Sprintf("%d resource(s) tracked", len(state.Entries)),
	}
}

func (a *app) doctorCheckCredential(cfg fs.ProjectConfigV2, cfgErr error) doctorCheck {
	if cfgErr != nil || len(cfg.Providers) == 0 {
		return doctorCheck{name: "Credential validity", status: "skip", detail: "no project configured"}
	}

	if token := strings.TrimSpace(os.Getenv("STH_TOKEN")); token != "" {
		return doctorCheck{
			name:   "Credential validity",
			status: "pass",
			detail: "STH_TOKEN is set",
		}
	}
	if oidcURL := strings.TrimSpace(os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL")); oidcURL != "" {
		return doctorCheck{
			name:   "Credential validity",
			status: "pass",
			detail: "GitHub Actions OIDC env present",
		}
	}
	return doctorCheck{
		name:        "Credential validity",
		status:      "fail",
		detail:      "no STH_TOKEN or OIDC env detected",
		remediation: "Set a valid STH_TOKEN or run `sth init`.",
	}
}

func (a *app) doctorChecksProviderReachability(cfg fs.ProjectConfigV2, cfgErr error) []doctorCheck {
	if cfgErr != nil || len(cfg.Providers) == 0 {
		return []doctorCheck{{name: "Provider reachability", status: "skip", detail: "no project configured"}}
	}
	out := make([]doctorCheck, 0, len(cfg.Providers))
	for _, p := range cfg.Providers {
		out = append(out, a.doctorCheckOneProvider(p))
	}
	return out
}

func (a *app) doctorCheckOneProvider(p fs.ProviderEntry) doctorCheck {
	url := providerReachabilityURL(p)
	if url == "" {
		return doctorCheck{
			name:   fmt.Sprintf("Provider reachability (%s)", p.ID),
			status: "skip",
			detail: fmt.Sprintf("no reachability URL for provider %q", p.Provider),
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), doctorReachabilityTimeout)
	defer cancel()
	req, err := stdhttp.NewRequestWithContext(ctx, stdhttp.MethodHead, url, nil)
	if err != nil {
		return doctorCheck{
			name:   fmt.Sprintf("Provider reachability (%s)", p.ID),
			status: "fail",
			detail: err.Error(),
		}
	}
	resp, err := stdhttp.DefaultClient.Do(req)
	if err != nil {
		a.verbose.HTTP(stdhttp.MethodHead, url, 0)
		return doctorCheck{
			name:        fmt.Sprintf("Provider reachability (%s)", p.ID),
			status:      "fail",
			detail:      fmt.Sprintf("%s: %s", url, err.Error()),
			remediation: "Check network access and provider credentials.",
		}
	}
	defer resp.Body.Close()
	a.verbose.HTTP(stdhttp.MethodHead, url, resp.StatusCode)
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return doctorCheck{
			name:   fmt.Sprintf("Provider reachability (%s)", p.ID),
			status: "pass",
			detail: fmt.Sprintf("%s → %d", url, resp.StatusCode),
		}
	}
	if resp.StatusCode == stdhttp.StatusUnauthorized || resp.StatusCode == stdhttp.StatusForbidden {
		// Reachable but not authorized — surface as fail with credential hint.
		return doctorCheck{
			name:        fmt.Sprintf("Provider reachability (%s)", p.ID),
			status:      "fail",
			detail:      fmt.Sprintf("%s → %d (auth required)", url, resp.StatusCode),
			remediation: "Set a valid STH_TOKEN or provider-specific token.",
		}
	}
	return doctorCheck{
		name:        fmt.Sprintf("Provider reachability (%s)", p.ID),
		status:      "fail",
		detail:      fmt.Sprintf("%s → %d", url, resp.StatusCode),
		remediation: "Check network access and provider credentials.",
	}
}

// providerReachabilityURL returns a no-auth URL to HEAD for a sanity check.
// Honors STH_*_BASE_URL overrides so test servers (httptest) work.
func providerReachabilityURL(p fs.ProviderEntry) string {
	switch strings.ToLower(strings.TrimSpace(p.Provider)) {
	case "github":
		base := strings.TrimRight(getEnvOrDefault("STH_GITHUB_API_BASE_URL", "https://api.github.com"), "/")
		if id := strings.TrimSpace(p.RepositoryID); id != "" {
			return fmt.Sprintf("%s/repos/%s", base, id)
		}
		return base
	case "gitlab":
		base := strings.TrimRight(getEnvOrDefault("STH_GITLAB_API_BASE_URL", "https://gitlab.com/api/v4"), "/")
		if id := strings.TrimSpace(p.RepositoryID); id != "" {
			return fmt.Sprintf("%s/projects/%s", base, neturl.PathEscape(id))
		}
		return base
	case "azure-devops":
		return strings.TrimRight(getEnvOrDefault("STH_AZURE_DEVOPS_BASE_URL", "https://dev.azure.com"), "/")
	case "bitbucket":
		base := strings.TrimRight(getEnvOrDefault("STH_BITBUCKET_API_BASE_URL", "https://api.bitbucket.org/2.0"), "/")
		if id := strings.TrimSpace(p.RepositoryID); id != "" {
			return fmt.Sprintf("%s/repositories/%s", base, id)
		}
		return base
	case "sth-cloud":
		return strings.TrimRight(getEnvOrDefault("STH_CLOUD_BASE_URL", "https://api.sth-cloud.example/cli"), "/")
	default:
		return ""
	}
}

// getEnvOrDefault is a local copy of the http package helper to avoid an
// import cycle (cmd → http → cmd doesn't exist, but we need this in cmd).
func getEnvOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

// errProjectAbsent is returned when the project config is missing — used
// internally to differentiate "no project" from harder errors.
var errProjectAbsent = errors.New("no project configured")
