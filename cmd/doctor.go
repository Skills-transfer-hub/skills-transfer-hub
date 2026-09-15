package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	stdhttp "net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sth/internal/fs"
	sthhttp "sth/internal/http"
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
func (a *app) runDoctor(args []string) (resultErr error) {
	flags, positional := parseFlags(args, "--json")
	jsonOut := flags["--json"]
	defer func() { a.recordOperationResult("doctor", "", resultErr) }()
	if len(positional) > 0 {
		return fmt.Errorf("usage: sth doctor")
	}

	cfg, cfgErr := fs.ReadProjectConfigV2()
	checks := []doctorCheck{
		a.doctorCheckProjectConfig(cfg, cfgErr),
	}
	checks = append(checks, a.doctorCheckStateFile(cfg, cfgErr))
	checks = append(checks, a.doctorCheckCredential(cfg, cfgErr))
	checks = append(checks, a.doctorCheckLicenseExpiry())
	checks = append(checks, a.doctorChecksProviderReachability(cfg, cfgErr)...)

	failures := 0
	if jsonOut {
		output := make([]map[string]string, 0, len(checks))
		for _, check := range checks {
			output = append(output, map[string]string{"name": check.name, "status": check.status, "detail": check.detail, "next_action": check.remediation})
			if check.status == "fail" {
				failures++
			}
		}
		payload, err := json.Marshal(output)
		if err != nil {
			return err
		}
		a.writeStdout(string(payload))
		if failures > 0 {
			return fmt.Errorf("%d check(s) failed; follow each next_action and rerun `sth doctor`", failures)
		}
		return nil
	}
	for _, check := range checks {
		marker := "[?]"
		switch check.status {
		case "pass":
			marker = "[✓]"
		case "fail":
			marker = "[✗]"
			failures++
		case "skip":
			marker = "[SKIP]"
		}
		line := fmt.Sprintf("%s %s", marker, check.name)
		if check.detail != "" {
			line = fmt.Sprintf("%s — %s", line, check.detail)
		}
		a.writeStdout(line)
		if check.status == "fail" && check.remediation != "" {
			a.writeStdout(fmt.Sprintf("    → %s", check.remediation))
		}
	}

	if failures > 0 {
		return fmt.Errorf("%d check(s) failed", failures)
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

	// Resolve a global fallback target_dir once, in case some providers
	// omit their own. Reading the profile per-provider would be wasteful.
	var globalFallback string
	for _, p := range cfg.Providers {
		if p.TargetDir == "" {
			if profile, err := fs.ReadProjectProfile(); err == nil {
				globalFallback = profile.TargetDir
			}
			break
		}
	}

	// Collect unique target dirs across all configured providers.
	seen := map[string]bool{}
	var targetDirs []string
	for _, p := range cfg.Providers {
		td := p.TargetDir
		if td == "" {
			td = globalFallback
		}
		if td == "" || seen[td] {
			continue
		}
		seen[td] = true
		targetDirs = append(targetDirs, td)
	}
	if len(targetDirs) == 0 {
		return doctorCheck{name: "State file integrity", status: "skip", detail: "target_dir empty"}
	}

	var totalEntries int
	var failures []string
	for _, targetDir := range targetDirs {
		abs, _ := filepath.Abs(filepath.Join(targetDir, "sth-state-v2.json"))
		a.verbose.Path("sth-state-v2.json", abs)
		state, err := fs.ReadInstalledStateV2(targetDir)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %s", abs, err.Error()))
			continue
		}
		totalEntries += len(state.Entries)
	}

	if len(failures) > 0 {
		return doctorCheck{
			name:        "State file integrity",
			status:      "fail",
			detail:      strings.Join(failures, "; "),
			remediation: "Back up the state files and restore a known-good copy; keep your installed files until ownership is recovered.",
		}
	}
	return doctorCheck{
		name:   "State file integrity",
		status: "pass",
		detail: fmt.Sprintf("%d resource(s) tracked across %d target dir(s)", totalEntries, len(targetDirs)),
	}
}

func (a *app) doctorCheckCredential(cfg fs.ProjectConfigV2, cfgErr error) doctorCheck {
	if cfgErr != nil || len(cfg.Providers) == 0 {
		return doctorCheck{name: "Credential validity", status: "skip", detail: "no project configured"}
	}

	for _, provider := range cfg.Providers {
		if IsCloudProvider(provider.Provider) || provider.Private {
			return doctorCheck{name: "Credential validity", status: "skip", detail: "Credentials checked when accessing each configured source; token presence alone does not verify access"}
		}
	}
	return doctorCheck{name: "Credential validity", status: "pass", detail: "Public repositories support anonymous access; no token required"}
}

func (a *app) doctorCheckLicenseExpiry() doctorCheck {
	state, expiresAt := licenseExpiryStatus()
	switch state {
	case licenseExpiryExpired:
		return doctorCheck{
			name:        "License expiry",
			status:      "fail",
			detail:      fmt.Sprintf("expired on %s", expiresAt.Format("2006-01-02")),
			remediation: "Regenerate your key on the dashboard, then run `sth login` with the new key.",
		}
	case licenseExpiryNear:
		return doctorCheck{
			name:   "License expiry",
			status: "pass",
			detail: fmt.Sprintf("expires in %d day(s) (%s) — regenerate it soon on the dashboard", licenseExpiryDaysRemaining(expiresAt), expiresAt.Format("2006-01-02")),
		}
	case licenseExpiryValid:
		return doctorCheck{
			name:   "License expiry",
			status: "pass",
			detail: fmt.Sprintf("valid until %s", expiresAt.Format("2006-01-02")),
		}
	default:
		return doctorCheck{name: "License expiry", status: "skip", detail: "no license cache (run `sth login`)"}
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

func (a *app) doctorCheckOneProvider(entry fs.ProviderEntry) doctorCheck {
	url := providerReachabilityURL(entry)
	if url == "" {
		return doctorCheck{
			name:   fmt.Sprintf("Provider reachability (%s)", entry.ID),
			status: "skip",
			detail: fmt.Sprintf("no reachability URL for provider %q", entry.Provider),
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), doctorReachabilityTimeout)
	defer cancel()
	if !IsCloudProvider(entry.Provider) {
		profile := fs.ProjectProfileFromProvider(entry, ".sth")
		if err := a.client.CheckProjectAccess(ctx, profile); err != nil {
			remediation := "Check the repository and branch, restore network access, then rerun `sth doctor`."
			if _, authentication := sthhttp.IsAuthError(err); authentication {
				remediation = "Check the credentials configured for this source, then rerun `sth doctor`."
			}
			if strings.Contains(err.Error(), "rate limit") {
				remediation = "Wait until the provider quota resets, then rerun `sth doctor`; no token is required for public access."
			}
			return doctorCheck{name: fmt.Sprintf("Provider reachability (%s)", entry.ID), status: "fail", detail: err.Error(), remediation: remediation}
		}
		return doctorCheck{name: fmt.Sprintf("Provider reachability (%s)", entry.ID), status: "pass", detail: "Catalog access verified using the same access method as installation"}
	}
	req, err := stdhttp.NewRequestWithContext(ctx, stdhttp.MethodHead, url, nil)
	if err != nil {
		return doctorCheck{
			name:   fmt.Sprintf("Provider reachability (%s)", entry.ID),
			status: "fail",
			detail: err.Error(),
		}
	}
	resp, err := stdhttp.DefaultClient.Do(req)
	if err != nil {
		a.verbose.HTTP(stdhttp.MethodHead, url, 0)
		return doctorCheck{
			name:        fmt.Sprintf("Provider reachability (%s)", entry.ID),
			status:      "fail",
			detail:      fmt.Sprintf("%s: %s", url, err.Error()),
			remediation: "Check network access and provider credentials.",
		}
	}
	defer resp.Body.Close()
	a.verbose.HTTP(stdhttp.MethodHead, url, resp.StatusCode)
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return doctorCheck{
			name:   fmt.Sprintf("Provider reachability (%s)", entry.ID),
			status: "pass",
			detail: fmt.Sprintf("%s → %d", url, resp.StatusCode),
		}
	}
	if resp.StatusCode == stdhttp.StatusTooManyRequests || resp.StatusCode == stdhttp.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0" {
		return doctorCheck{name: fmt.Sprintf("Provider reachability (%s)", entry.ID), status: "fail", detail: "Provider rate limit reached", remediation: "Wait until the provider quota resets, then rerun `sth doctor`; public access does not require a token."}
	}
	if resp.StatusCode == stdhttp.StatusUnauthorized || resp.StatusCode == stdhttp.StatusForbidden {
		if IsCloudProvider(entry.Provider) {
			return doctorCheck{
				name:   fmt.Sprintf("Provider reachability (%s)", entry.ID),
				status: "pass",
				detail: fmt.Sprintf("%s → %d (reachable, authentication required)", url, resp.StatusCode),
			}
		}
		return doctorCheck{
			name:        fmt.Sprintf("Provider reachability (%s)", entry.ID),
			status:      "fail",
			detail:      fmt.Sprintf("%s → %d (auth required)", url, resp.StatusCode),
			remediation: "Set a valid STH_TOKEN or provider-specific token.",
		}
	}
	return doctorCheck{
		name:        fmt.Sprintf("Provider reachability (%s)", entry.ID),
		status:      "fail",
		detail:      fmt.Sprintf("%s → %d", url, resp.StatusCode),
		remediation: "Check network access and provider credentials.",
	}
}

func providerReachabilityURL(entry fs.ProviderEntry) string {
	switch strings.ToLower(strings.TrimSpace(entry.Provider)) {
	case "github":
		base := strings.TrimRight(getEnvOrDefault("STH_GITHUB_API_BASE_URL", "https://api.github.com"), "/")
		if id := strings.TrimSpace(entry.RepositoryID); id != "" {
			return fmt.Sprintf("%s/repos/%s", base, id)
		}
		return base
	case "gitlab":
		base := strings.TrimRight(getEnvOrDefault("STH_GITLAB_API_BASE_URL", "https://gitlab.com/api/v4"), "/")
		if id := strings.TrimSpace(entry.RepositoryID); id != "" {
			return fmt.Sprintf("%s/projects/%s", base, neturl.PathEscape(id))
		}
		return base
	case "azure-devops":
		return strings.TrimRight(getEnvOrDefault("STH_AZURE_DEVOPS_BASE_URL", "https://dev.azure.com"), "/")
	case "bitbucket":
		base := strings.TrimRight(getEnvOrDefault("STH_BITBUCKET_API_BASE_URL", "https://api.bitbucket.org/2.0"), "/")
		if id := strings.TrimSpace(entry.RepositoryID); id != "" {
			return fmt.Sprintf("%s/repositories/%s", base, id)
		}
		return base
	case "sth-cloud":
		return sthhttp.CloudBaseURL()
	default:
		return ""
	}
}

func getEnvOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

// errProjectAbsent is returned when the project config is missing — used
// internally to differentiate "no project" from harder errors.
var errProjectAbsent = errors.New("no project configured")
