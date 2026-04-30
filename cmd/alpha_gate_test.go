package cmd

import (
	"encoding/json"
	"fmt"
	stdhttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	sthhttp "sth/internal/http"
	"sync/atomic"
	"testing"
	"time"

	"sth/internal/fs"
)

// withReleaseMetadata overrides the build-time release vars and restores them
// after the test. Tests that need a "released" build call this; the default
// (version="dev", releaseAPIURL="") is what `go test` and `go run` see.
func withReleaseMetadata(t *testing.T, ver, apiURL string) {
	t.Helper()
	prevVersion := version
	prevAPIURL := releaseAPIURL
	prevAssetName := releaseAssetName
	version = ver
	releaseAPIURL = apiURL
	releaseAssetName = defaultReleaseAssetName()
	t.Cleanup(func() {
		version = prevVersion
		releaseAPIURL = prevAPIURL
		releaseAssetName = prevAssetName
	})
}

// releasesServer mocks the GitHub releases endpoint with a configurable
// behaviour and a hit counter so tests can assert cache reuse.
func releasesServer(t *testing.T, behavior string, latestTag string) (*httptest.Server, *int32) {
	t.Helper()
	var hits int32
	srv := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		atomic.AddInt32(&hits, 1)
		switch behavior {
		case "ok":
			assetName := defaultReleaseAssetName()
			fmt.Fprintf(w,
				`[{"tag_name":%q,"prerelease":false,"assets":[{"name":%q,"browser_download_url":"%s/asset"},{"name":"SHA256SUMS","browser_download_url":"%s/checksum"}]}]`,
				latestTag, assetName, r.Host, r.Host,
			)
		case "500":
			stdhttp.Error(w, "boom", stdhttp.StatusInternalServerError)
		case "hang":
			// Sleep longer than the gate's 3s timeout so the request fails.
			time.Sleep(5 * time.Second)
		default:
			t.Fatalf("unknown server behavior %q", behavior)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// preWriteCache puts an UpdateCheckState into the test's user cache dir so the
// next gate invocation reads it.
func preWriteCache(t *testing.T, cacheDir string, state fs.UpdateCheckState) {
	t.Helper()
	t.Setenv("STH_USER_CACHE_DIR", cacheDir)
	if _, err := fs.WriteUpdateCheckState(state); err != nil {
		t.Fatalf("WriteUpdateCheckState: %v", err)
	}
}

func readCachedStatus(t *testing.T, cacheDir string) string {
	t.Helper()
	t.Setenv("STH_USER_CACHE_DIR", cacheDir)
	state, err := fs.ReadUpdateCheckState()
	if err != nil {
		t.Fatalf("ReadUpdateCheckState: %v", err)
	}
	if state == nil {
		return ""
	}
	return state.Status
}

func TestShouldBypassAlphaGate(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"no args", []string{}, false},
		{"version", []string{"version"}, true},
		{"help", []string{"help"}, true},
		{"help with target", []string{"help", "list"}, true},
		{"--help", []string{"--help"}, true},
		{"-h", []string{"-h"}, true},
		{"logout", []string{"logout"}, true},
		{"update self", []string{"update", "self"}, true},
		{"update <resource>", []string{"update"}, false},
		{"update self extra", []string{"update", "self", "--dry-run"}, true},
		{"install", []string{"install", "skills/foo"}, false},
		{"login", []string{"login"}, false},
		{"list", []string{"list"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldBypassAlphaGate(tc.args); got != tc.want {
				t.Fatalf("shouldBypassAlphaGate(%v) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}

func TestAlphaGate_devBuildSkipsCheck(t *testing.T) {
	// Default test setup: version="dev", releaseAPIURL="". Server should never
	// be hit even for non-bypass commands.
	srv, hits := releasesServer(t, "500", "")

	result := runCLIWithOptions(t, cliOptions{
		args:         []string{"list"},
		client:       sthhttp.NewClient(srv.Client()),
		userCacheDir: t.TempDir(),
	})

	if atomic.LoadInt32(hits) != 0 {
		t.Fatalf("server hits = %d, want 0 (dev build skips the gate)", *hits)
	}
	// list will still error (no project config), but not because of the gate.
	if result.err != nil {
		assertNotContains(t, result.stderr, "Alpha :")
	}
}

func TestAlphaGate_envEscapeHatchSkipsCheck(t *testing.T) {
	srv, hits := releasesServer(t, "500", "")
	withReleaseMetadata(t, "v1.0.0", srv.URL+"/releases")
	t.Setenv(alphaGateSkipEnvVar, "0")

	result := runCLIWithOptions(t, cliOptions{
		args:         []string{"list"},
		client:       sthhttp.NewClient(srv.Client()),
		userCacheDir: t.TempDir(),
	})

	if atomic.LoadInt32(hits) != 0 {
		t.Fatalf("server hits = %d, want 0 (env hatch skips the gate)", *hits)
	}
	if result.err != nil {
		assertNotContains(t, result.stderr, "Alpha :")
	}
}

func TestAlphaGate_bypassCommands(t *testing.T) {
	// Read-only / recovery commands must not trigger the gate at all. We
	// exclude `update self` here because it legitimately hits the releases
	// endpoint as part of its own work — the assertion of interest there is
	// "the gate didn't add an extra block", not "no network calls".
	for _, args := range [][]string{
		{"version"},
		{"help"},
		{"--help"},
		{"-h"},
		{"logout"},
	} {
		t.Run(args[0], func(t *testing.T) {
			srv, hits := releasesServer(t, "500", "")
			withReleaseMetadata(t, "v1.0.0", srv.URL+"/releases")

			result := runCLIWithOptions(t, cliOptions{
				args:         args,
				client:       sthhttp.NewClient(srv.Client()),
				userCacheDir: t.TempDir(),
			})

			if atomic.LoadInt32(hits) != 0 {
				t.Fatalf("server hits for bypass cmd %v = %d, want 0", args, *hits)
			}
			if result.err != nil {
				assertNotContains(t, result.stderr, "Alpha :")
			}
		})
	}
}

func TestAlphaGate_updateSelfNotBlocked(t *testing.T) {
	// Even on an outdated binary with the server returning errors, `update self`
	// must reach its own runSelfUpdate code path (it can fail there for other
	// reasons, but it must never be stopped by the gate).
	srv, _ := releasesServer(t, "500", "")
	withReleaseMetadata(t, "v1.0.0", srv.URL+"/releases")

	result := runCLIWithOptions(t, cliOptions{
		args:         []string{"update", "self"},
		client:       sthhttp.NewClient(srv.Client()),
		userCacheDir: t.TempDir(),
	})
	// runSelfUpdate may surface its own error against the 500 server, but the
	// error must NOT be the gate's "Alpha :" prefix.
	assertNotContains(t, result.stderr, "Alpha :")
}

func TestAlphaGate_releasedUpToDate_passesAndCachesNoUpdate(t *testing.T) {
	srv, hits := releasesServer(t, "ok", "v1.0.0")
	withReleaseMetadata(t, "v1.0.0", srv.URL+"/releases")
	cacheDir := t.TempDir()

	result := runCLIWithOptions(t, cliOptions{
		args:         []string{"list"},
		client:       sthhttp.NewClient(srv.Client()),
		userCacheDir: cacheDir,
	})

	if atomic.LoadInt32(hits) != 1 {
		t.Fatalf("server hits = %d, want 1", *hits)
	}
	if result.err != nil {
		assertNotContains(t, result.stderr, "Alpha :")
	}
	if got := readCachedStatus(t, cacheDir); got != "no-update" {
		t.Fatalf("cached status = %q, want %q", got, "no-update")
	}
}

func TestAlphaGate_releasedOutdated_blocksAndCachesAvailable(t *testing.T) {
	srv, _ := releasesServer(t, "ok", "v1.2.0")
	withReleaseMetadata(t, "v1.0.0", srv.URL+"/releases")
	cacheDir := t.TempDir()

	result := runCLIWithOptions(t, cliOptions{
		args:         []string{"list"},
		client:       sthhttp.NewClient(srv.Client()),
		userCacheDir: cacheDir,
	})

	if result.err == nil {
		t.Fatalf("Run() err = nil, want gate block")
	}
	assertContains(t, result.stderr, "Alpha :")
	assertContains(t, result.stderr, "obsolète")
	assertContains(t, result.stderr, "v1.2.0")
	if got := readCachedStatus(t, cacheDir); got != "update-available" {
		t.Fatalf("cached status = %q, want %q", got, "update-available")
	}
}

func TestAlphaGate_releasedNetworkError_blocksAndCachesError(t *testing.T) {
	srv, _ := releasesServer(t, "500", "")
	withReleaseMetadata(t, "v1.0.0", srv.URL+"/releases")
	cacheDir := t.TempDir()

	result := runCLIWithOptions(t, cliOptions{
		args:         []string{"list"},
		client:       sthhttp.NewClient(srv.Client()),
		userCacheDir: cacheDir,
	})

	if result.err == nil {
		t.Fatalf("Run() err = nil, want gate block")
	}
	assertContains(t, result.stderr, "impossible de vérifier")
	if got := readCachedStatus(t, cacheDir); got != "error" {
		t.Fatalf("cached status = %q, want %q", got, "error")
	}
}

func TestAlphaGate_cacheHitNoUpdate_skipsNetwork(t *testing.T) {
	srv, hits := releasesServer(t, "500", "")
	withReleaseMetadata(t, "v1.0.0", srv.URL+"/releases")

	cacheDir := t.TempDir()
	preWriteCache(t, cacheDir, fs.UpdateCheckState{
		LastCheckedAt:  time.Now().UTC().Format(time.RFC3339),
		TTLHours:       alphaGateCacheTTLh,
		CurrentVersion: "v1.0.0",
		Status:         "no-update",
	})

	result := runCLIWithOptions(t, cliOptions{
		args:         []string{"list"},
		client:       sthhttp.NewClient(srv.Client()),
		userCacheDir: cacheDir,
	})

	if atomic.LoadInt32(hits) != 0 {
		t.Fatalf("server hits = %d, want 0 (cache hit should skip network)", *hits)
	}
	if result.err != nil {
		assertNotContains(t, result.stderr, "Alpha :")
	}
}

func TestAlphaGate_cacheHitUpdateAvailable_blocksWithoutNetwork(t *testing.T) {
	srv, hits := releasesServer(t, "500", "")
	withReleaseMetadata(t, "v1.0.0", srv.URL+"/releases")

	cacheDir := t.TempDir()
	preWriteCache(t, cacheDir, fs.UpdateCheckState{
		LastCheckedAt:       time.Now().UTC().Format(time.RFC3339),
		TTLHours:            alphaGateCacheTTLh,
		CurrentVersion:      "v1.0.0",
		LatestStableVersion: "v1.5.0",
		DownloadURL:         "https://example.invalid/asset",
		ChecksumURL:         "https://example.invalid/checksum",
		AssetName:           "sth-linux-amd64.tar.gz",
		Status:              "update-available",
	})

	result := runCLIWithOptions(t, cliOptions{
		args:         []string{"list"},
		client:       sthhttp.NewClient(srv.Client()),
		userCacheDir: cacheDir,
	})

	if atomic.LoadInt32(hits) != 0 {
		t.Fatalf("server hits = %d, want 0 (cache hit should skip network)", *hits)
	}
	if result.err == nil {
		t.Fatalf("Run() err = nil, want gate block")
	}
	assertContains(t, result.stderr, "obsolète")
	assertContains(t, result.stderr, "v1.5.0")
}

func TestAlphaGate_cacheHitError_rechecksFresh(t *testing.T) {
	// Fresh cached error → don't trust it; re-check live.
	srv, hits := releasesServer(t, "ok", "v1.0.0")
	withReleaseMetadata(t, "v1.0.0", srv.URL+"/releases")

	cacheDir := t.TempDir()
	preWriteCache(t, cacheDir, fs.UpdateCheckState{
		LastCheckedAt:  time.Now().UTC().Format(time.RFC3339),
		TTLHours:       alphaGateCacheTTLh,
		CurrentVersion: "v1.0.0",
		Status:         "error",
		ErrorMessage:   "previous failure",
	})

	result := runCLIWithOptions(t, cliOptions{
		args:         []string{"list"},
		client:       sthhttp.NewClient(srv.Client()),
		userCacheDir: cacheDir,
	})

	if atomic.LoadInt32(hits) != 1 {
		t.Fatalf("server hits = %d, want 1 (cached error must trigger re-check)", *hits)
	}
	// Fresh check succeeded so the gate should not block.
	if result.err != nil {
		assertNotContains(t, result.stderr, "Alpha :")
	}
}

func TestAlphaGate_staleCache_refreshes(t *testing.T) {
	srv, hits := releasesServer(t, "ok", "v1.0.0")
	withReleaseMetadata(t, "v1.0.0", srv.URL+"/releases")

	cacheDir := t.TempDir()
	preWriteCache(t, cacheDir, fs.UpdateCheckState{
		LastCheckedAt:  time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339),
		TTLHours:       alphaGateCacheTTLh,
		CurrentVersion: "v1.0.0",
		Status:         "no-update",
	})

	result := runCLIWithOptions(t, cliOptions{
		args:         []string{"list"},
		client:       sthhttp.NewClient(srv.Client()),
		userCacheDir: cacheDir,
	})

	if atomic.LoadInt32(hits) != 1 {
		t.Fatalf("server hits = %d, want 1 (stale cache must refresh)", *hits)
	}
	if result.err != nil {
		assertNotContains(t, result.stderr, "Alpha :")
	}
}

func TestAlphaGate_writesCacheWithGateTTL(t *testing.T) {
	srv, _ := releasesServer(t, "ok", "v1.0.0")
	withReleaseMetadata(t, "v1.0.0", srv.URL+"/releases")
	cacheDir := t.TempDir()

	_ = runCLIWithOptions(t, cliOptions{
		args:         []string{"list"},
		client:       sthhttp.NewClient(srv.Client()),
		userCacheDir: cacheDir,
	})

	t.Setenv("STH_USER_CACHE_DIR", cacheDir)
	path, err := fs.UpdateCheckStatePath()
	if err != nil {
		t.Fatalf("UpdateCheckStatePath: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Fatalf("cache dir: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile cache: %v", err)
	}
	var raw struct {
		TTLHours int    `json:"ttl_hours"`
		Status   string `json:"status"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal cache: %v", err)
	}
	if raw.TTLHours != alphaGateCacheTTLh {
		t.Fatalf("cached TTLHours = %d, want %d (gate-shorter TTL)", raw.TTLHours, alphaGateCacheTTLh)
	}
	if raw.Status != "no-update" {
		t.Fatalf("cached status = %q, want no-update", raw.Status)
	}
}
