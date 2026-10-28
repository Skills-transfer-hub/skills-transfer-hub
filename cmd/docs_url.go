package cmd

import (
	"strings"
)

const defaultDashboardBaseURL = "https://www.skillsth.com"

const defaultDocsBaseURL = defaultDashboardBaseURL

func DocsBaseURL() string {
	return strings.TrimRight(getEnvOrDefault("STH_DOCS_URL", defaultDocsBaseURL), "/")
}

func DocsURL() string { return DocsBaseURL() + "/docs" }

func TelemetryDocsURL() string { return DocsBaseURL() + "/docs/telemetry" }

func AlphaLimitsDocsURL() string { return DocsBaseURL() + "/docs#alphaLimits" }

func dashboardBaseURL() string {
	return strings.TrimRight(getEnvOrDefault("STH_DASHBOARD_URL", defaultDashboardBaseURL), "/")
}

func DashboardURL() string { return dashboardBaseURL() }

func DashboardLicensesURL() string { return dashboardBaseURL() + "/dashboard/licenses" }

func DashboardPricingURL() string { return dashboardBaseURL() + "/pricing" }
