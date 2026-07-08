package cmd

import (
	"strings"
	"testing"
)

func TestFindProviderGuide_returnsBitbucketCloudMetadata(t *testing.T) {
	guide, ok := findProviderGuide("bitbucket")
	if !ok {
		t.Fatal("findProviderGuide(bitbucket) = not found, want guide")
	}
	if guide.HostedDomain != "bitbucket.org" {
		t.Fatalf("guide.HostedDomain = %q, want bitbucket.org", guide.HostedDomain)
	}
	if guide.CredentialKind != "api-token" {
		t.Fatalf("guide.CredentialKind = %q, want api-token", guide.CredentialKind)
	}
	if guide.ActionURL != "https://id.atlassian.com/manage-profile/security/api-tokens" {
		t.Fatalf("guide.ActionURL = %q", guide.ActionURL)
	}
}

func TestSupportedProviderKeys_areStableAndComplete(t *testing.T) {
	want := []string{"azure-devops", "bitbucket", "github", "gitlab", "sth-cloud"}
	got := supportedProviderKeys()
	if len(got) != len(want) {
		t.Fatalf("len(got) = %d, want %d", len(got), len(want))
	}
	for index, key := range want {
		if got[index] != key {
			t.Fatalf("got[%d] = %q, want %q", index, got[index], key)
		}
	}
}

func TestProviderGuides_containNoPlaceholderDomains(t *testing.T) {
	for _, guide := range supportedProviders() {
		for _, value := range []string{guide.HostedDomain, guide.DocumentationURL, guide.ActionURL} {
			if strings.Contains(value, ".example") {
				t.Fatalf("guide %s exposes a placeholder domain: %q", guide.Key, value)
			}
		}
	}
}

func TestFindProviderGuide_sthCloudPointsAtRealServices(t *testing.T) {
	guide, ok := findProviderGuide("sth-cloud")
	if !ok {
		t.Fatal("findProviderGuide(sth-cloud) = not found, want guide")
	}
	if guide.DocumentationURL != DocsURL() {
		t.Fatalf("guide.DocumentationURL = %q, want %q", guide.DocumentationURL, DocsURL())
	}
	if guide.ActionURL != DashboardLicensesURL() {
		t.Fatalf("guide.ActionURL = %q, want %q", guide.ActionURL, DashboardLicensesURL())
	}
	if guide.HostedDomain == "" {
		t.Fatal("guide.HostedDomain = empty, want the sth-cloud API host")
	}
}

func TestIsSupportedHostedDomain(t *testing.T) {
	if !isSupportedHostedDomain("github", "github.com") {
		t.Fatal("expected github.com to be accepted for github")
	}
	if isSupportedHostedDomain("github", "git.example.com") {
		t.Fatal("unexpected self-hosted GitHub domain acceptance")
	}
}
