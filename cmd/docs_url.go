package cmd

import (
	"os"
	"strings"
)

const defaultDocsBaseURL = "https://sth-dashboard.vercel.app"

// DocsBaseURL returns the docs base URL with the env override applied and
// any trailing slash stripped.
func DocsBaseURL() string {
	raw := strings.TrimSpace(os.Getenv("STH_DOCS_URL"))
	if raw == "" {
		raw = defaultDocsBaseURL
	}
	return strings.TrimRight(raw, "/")
}

// DocsURL returns the root docs landing URL — used in the "first init"
// reminder so the user knows where to find the manual.
func DocsURL() string { return DocsBaseURL() + "/docs" }

// TelemetryDocsURL points to the dedicated telemetry policy page. Linked
// from the consent prompt and from `sth telemetry status`.
func TelemetryDocsURL() string { return DocsBaseURL() + "/docs/telemetry" }

// AlphaLimitsDocsURL points to the Alpha-limits section of the docs.
// Surfaced when the CLI refuses to add a second Git provider.
func AlphaLimitsDocsURL() string { return DocsBaseURL() + "/docs#alphaLimits" }

// dashboardBaseURL returns the user-facing app URL — distinct from
// DocsBaseURL because the docs site and the dashboard live on different
// hosts in production. STH_DASHBOARD_URL overrides it for dev/staging.
func dashboardBaseURL() string {
	raw := strings.TrimSpace(os.Getenv("STH_DASHBOARD_URL"))
	if raw == "" {
		raw = "https://app.sth-cloud.example"
	}
	return strings.TrimRight(raw, "/")
}

// DashboardURL returns the dashboard root — useful for general "go check
// your account" messages.
func DashboardURL() string { return dashboardBaseURL() }

// DashboardLicensesURL returns the page where the user finds their
// `lsk_live_*` key. Linked from the init wizard cloud-login sub-prompt.
func DashboardLicensesURL() string { return dashboardBaseURL() + "/dashboard/licenses" }
