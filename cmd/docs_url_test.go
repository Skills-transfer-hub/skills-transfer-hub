package cmd

import "testing"

func TestDocsAndDashboardURLs_defaultToProductionDashboard(t *testing.T) {
	t.Setenv("STH_DOCS_URL", "")
	t.Setenv("STH_DASHBOARD_URL", "")

	checks := []struct {
		name string
		got  string
		want string
	}{
		{"DocsBaseURL", DocsBaseURL(), "https://sth-dashboard.vercel.app"},
		{"DocsURL", DocsURL(), "https://sth-dashboard.vercel.app/docs"},
		{"TelemetryDocsURL", TelemetryDocsURL(), "https://sth-dashboard.vercel.app/docs/telemetry"},
		{"AlphaLimitsDocsURL", AlphaLimitsDocsURL(), "https://sth-dashboard.vercel.app/docs#alphaLimits"},
		{"DashboardURL", DashboardURL(), "https://sth-dashboard.vercel.app"},
		{"DashboardLicensesURL", DashboardLicensesURL(), "https://sth-dashboard.vercel.app/dashboard/licenses"},
	}
	for _, check := range checks {
		if check.got != check.want {
			t.Fatalf("%s = %q, want %q", check.name, check.got, check.want)
		}
	}
}

func TestDocsAndDashboardURLs_honorEnvOverrides(t *testing.T) {
	t.Setenv("STH_DOCS_URL", "https://docs.dev.local/")
	t.Setenv("STH_DASHBOARD_URL", "https://dash.dev.local/")

	if got := DocsURL(); got != "https://docs.dev.local/docs" {
		t.Fatalf("DocsURL = %q, want https://docs.dev.local/docs", got)
	}
	if got := TelemetryDocsURL(); got != "https://docs.dev.local/docs/telemetry" {
		t.Fatalf("TelemetryDocsURL = %q, want https://docs.dev.local/docs/telemetry", got)
	}
	if got := DashboardURL(); got != "https://dash.dev.local" {
		t.Fatalf("DashboardURL = %q, want https://dash.dev.local", got)
	}
	if got := DashboardLicensesURL(); got != "https://dash.dev.local/dashboard/licenses" {
		t.Fatalf("DashboardLicensesURL = %q, want https://dash.dev.local/dashboard/licenses", got)
	}
}
