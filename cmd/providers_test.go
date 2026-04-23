package cmd

import "testing"

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
	want := []string{"azure-devops", "bitbucket", "github", "gitlab"}
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

func TestIsSupportedHostedDomain(t *testing.T) {
	if !isSupportedHostedDomain("github", "github.com") {
		t.Fatal("expected github.com to be accepted for github")
	}
	if isSupportedHostedDomain("github", "git.example.com") {
		t.Fatal("unexpected self-hosted GitHub domain acceptance")
	}
}