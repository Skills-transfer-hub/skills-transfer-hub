package cmd

import (
	"errors"
	stdhttp "net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	sthhttp "sth/internal/http"
)

func TestReleasedLocalCommandNeverChecksNetworkOrBlocks(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, _ *stdhttp.Request) {
		hits.Add(1)
		stdhttp.Error(writer, "offline", stdhttp.StatusServiceUnavailable)
	}))
	defer server.Close()
	previousVersion, previousAPIURL, previousAsset := version, releaseAPIURL, releaseAssetName
	version, releaseAPIURL, releaseAssetName = "v1.0.0", server.URL+"/releases", defaultReleaseAssetName()
	t.Cleanup(func() { version, releaseAPIURL, releaseAssetName = previousVersion, previousAPIURL, previousAsset })

	result := runCLIWithOptions(t, cliOptions{
		args: []string{"status"}, client: sthhttp.NewClient(server.Client()),
		workDir: t.TempDir(), homeDir: t.TempDir(), userCacheDir: t.TempDir(),
	})
	if hits.Load() != 0 {
		t.Fatalf("local status performed %d release checks", hits.Load())
	}
	if result.err != nil {
		assertNotContains(t, result.stderr, "Alpha :")
		assertNotContains(t, result.stderr, "connexion")
	}
}

func TestUpgradeCommandPackageManagerHints(t *testing.T) {
	cases := []struct {
		path string
		err  error
		want string
	}{
		{"/opt/homebrew/Cellar/sth/0.1.0/bin/sth", nil, "brew upgrade skills-transfer-hub/sth/sth"},
		{`C:\Users\hugo\scoop\apps\sth\current\sth.exe`, nil, "scoop update sth"},
		{"/usr/local/bin/sth", nil, "sth update self"},
		{"", errors.New("unavailable"), "sth update self"},
	}
	for _, test := range cases {
		previous := osExecutable
		osExecutable = func() (string, error) { return test.path, test.err }
		if got := upgradeCommand(); got != test.want {
			t.Fatalf("upgradeCommand(%q) = %q, want %q", test.path, got, test.want)
		}
		osExecutable = previous
	}
}
