package cmd

import (
	"os"
	"strings"
)

// defaultDocsBaseURL is the production docs site. STH_DOCS_URL overrides it
// so dev/staging environments and tests can point at a different host.
const defaultDocsBaseURL = "https://sth-cloud.example"

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
