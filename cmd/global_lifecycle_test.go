package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"sth/internal/fs"
)

type globalFixture struct {
	home          string
	configuration string
	project       string
	revision      atomic.Int32
	requests      atomic.Int32
	server        *httptest.Server
}

func newGlobalFixture(t *testing.T) *globalFixture {
	t.Helper()
	fixture := &globalFixture{home: t.TempDir(), configuration: t.TempDir(), project: t.TempDir()}
	fixture.revision.Store(1)
	fixture.server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		fixture.requests.Add(1)
		switch {
		case request.URL.Path == "/repos/example/skills":
			fmt.Fprint(response, `{"default_branch":"main"}`)
		case strings.HasPrefix(request.URL.Path, "/repos/example/skills/commits/"):
			reference := strings.TrimPrefix(request.URL.Path, "/repos/example/skills/commits/")
			if len(reference) != 40 {
				reference = strings.Repeat(fmt.Sprint(fixture.revision.Load()), 40)
			}
			fmt.Fprintf(response, `{"sha":%q}`, reference)
		case strings.HasPrefix(request.URL.Path, "/repos/example/skills/git/trees/"):
			fmt.Fprint(response, `{"truncated":false,"tree":[{"path":"skills/ponytail/SKILL.md","type":"blob","mode":"100644"},{"path":"skills/ponytail/run.sh","type":"blob","mode":"100755"},{"path":"skills/ponytail/icon.bin","type":"blob","mode":"100644"},{"path":"LICENSE","type":"blob","mode":"100644"},{"path":".openclaw/skills/ponytail/SKILL.md","type":"blob","mode":"100644"},{"path":"skills/plugin.json","type":"blob","mode":"100644"}]}`)
		case strings.HasPrefix(request.URL.Path, "/example/skills/"):
			parts := strings.SplitN(strings.TrimPrefix(request.URL.Path, "/example/skills/"), "/", 2)
			if len(parts) != 2 || len(parts[0]) != 40 {
				t.Errorf("download not pinned to SHA: %s", request.URL.Path)
				http.Error(response, "unpinned", 400)
				return
			}
			switch parts[1] {
			case "skills/ponytail/SKILL.md":
				fmt.Fprintf(response, "---\nname: ponytail\ndescription: Test global installation\n---\nRevision %s\n", parts[0])
			case "skills/ponytail/run.sh":
				fmt.Fprint(response, "#!/bin/sh\nexit 91\n")
			case "skills/ponytail/icon.bin":
				_, _ = response.Write([]byte{0, 255, 128, 1})
			case "LICENSE":
				fmt.Fprint(response, "Fixture license\n")
			case ".openclaw/skills/ponytail/SKILL.md":
				fmt.Fprint(response, "---\nname: ponytail\ndescription: Alternative\n---\n")
			default:
				http.NotFound(response, request)
			}
		default:
			http.NotFound(response, request)
		}
	}))
	t.Cleanup(fixture.server.Close)
	t.Setenv("STH_GITHUB_API_BASE_URL", fixture.server.URL)
	t.Setenv("STH_GITHUB_RAW_BASE_URL", fixture.server.URL)
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	return fixture
}

func (fixture *globalFixture) run(t *testing.T, arguments ...string) (globalResult, error) {
	t.Helper()
	arguments = append(arguments, "--global", "--json")
	result := runCLIWithOptions(t, cliOptions{workDir: fixture.project, homeDir: fixture.home, userConfigDir: fixture.configuration, args: arguments, skipTelemetrySeed: true})
	var envelope globalResult
	if err := json.Unmarshal([]byte(result.stdout), &envelope); err != nil {
		t.Fatalf("decode global result %q (%v): %v", result.stdout, result.err, err)
	}
	return envelope, result.err
}

func (fixture *globalFixture) state(t *testing.T) fs.GlobalState {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(fixture.configuration, "sth", "global", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var state fs.GlobalState
	if err := json.Unmarshal(content, &state); err != nil {
		t.Fatal(err)
	}
	return state
}

func TestGlobalInstallDryRunAndAllAgents(t *testing.T) {
	fixture := newGlobalFixture(t)
	preview, err := fixture.run(t, "install", "https://github.com/example/skills", "--skill", "ponytail", "--all-agents", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	for _, directory := range []string{fixture.home, fixture.project, fixture.configuration} {
		entries, err := os.ReadDir(directory)
		if err != nil || len(entries) != 0 {
			t.Fatalf("dry run wrote %s: %v %v", directory, entries, err)
		}
	}
	if len(preview.Resources) != 1 || len(preview.Resources[0].Placements) != 4 || len(preview.Resources[0].RequestedConsumers) != 8 {
		t.Fatalf("unexpected plan: %+v", preview)
	}
	result, err := fixture.run(t, "install", "github:example/skills", "--skill=ponytail", "--all-agents")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Resources) != 1 || result.Resources[0].ValidationLevel != "discovery-unverified" || result.Resources[0].FileState != "files-installed" {
		t.Fatalf("unexpected installation: %+v", result)
	}
	state := fixture.state(t)
	if len(state.Installations) != 1 || state.Installations[0].Source.Subdir != "skills/ponytail" || state.Installations[0].ResolvedRevision != strings.Repeat("1", 40) || len(state.Installations[0].Provenance) == 0 {
		t.Fatalf("unexpected state: %+v", state)
	}
	for _, placement := range state.Installations[0].Placements {
		skillDirectory := filepath.Join(placement.ResolvedRoot, placement.RelativePath)
		content, err := os.ReadFile(filepath.Join(skillDirectory, "icon.bin"))
		if err != nil || string(content) != string([]byte{0, 255, 128, 1}) {
			t.Fatalf("binary asset corrupted: %v %v", content, err)
		}
		info, err := os.Stat(filepath.Join(skillDirectory, "run.sh"))
		if err != nil || info.Mode().Perm() != 0o755 {
			t.Fatalf("executable permission lost: %v %v", info, err)
		}
	}
	entries, err := os.ReadDir(fixture.project)
	if err != nil || len(entries) != 0 {
		t.Fatalf("project changed: %v %v", entries, err)
	}
}

func TestGlobalLifecyclePreservesRevisionAndConsumers(t *testing.T) {
	fixture := newGlobalFixture(t)
	if _, err := fixture.run(t, "install", "github:example/skills", "--agent=codex"); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(fixture.configuration, "sth", "global", "state.json")
	before, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	result, err := fixture.run(t, "install", "github:example/skills", "--agent=codex")
	if err != nil || result.Resources[0].Outcome != "unchanged" {
		t.Fatalf("reinstall not idempotent: %+v %v", result, err)
	}
	after, err := os.ReadFile(statePath)
	if err != nil || string(before) != string(after) {
		t.Fatalf("idempotence rewrote state: %v", err)
	}
	fixture.revision.Store(2)
	if _, err := fixture.run(t, "install", "github:example/skills", "--agent=claude"); err != nil {
		t.Fatal(err)
	}
	state := fixture.state(t)
	installation := state.Installations[0]
	if installation.ResolvedRevision != strings.Repeat("1", 40) || len(installation.RequestedConsumers) != 2 || len(installation.Placements) != 2 {
		t.Fatalf("adding consumer advanced revision or lost consumer: %+v", installation)
	}
	requestCount := fixture.requests.Load()
	listed, err := fixture.run(t, "list")
	if err != nil || len(listed.Resources) != 1 || fixture.requests.Load() != requestCount {
		t.Fatalf("list should be offline: %+v %v", listed, err)
	}
	status, err := fixture.run(t, "status")
	if err != nil || status.Resources[0].RemoteState != "outdated" || status.Resources[0].FileState != "files-installed" {
		t.Fatalf("status: %+v %v", status, err)
	}
	preview, err := fixture.run(t, "update", "ponytail", "--dry-run")
	if err != nil || preview.Resources[0].ResolvedRevision != strings.Repeat("2", 40) || fixture.state(t).Installations[0].ResolvedRevision != strings.Repeat("1", 40) {
		t.Fatalf("update preview: %+v %v", preview, err)
	}
	updated, err := fixture.run(t, "update")
	if err != nil || updated.Resources[0].ResolvedRevision != strings.Repeat("2", 40) || len(updated.Resources[0].RequestedConsumers) != 2 {
		t.Fatalf("update: %+v %v", updated, err)
	}
	foreignPath := filepath.Join(fixture.home, ".agents", "skills", "ponytail", "my-notes.txt")
	if err := os.WriteFile(foreignPath, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.run(t, "remove", "ponytail", "--dry-run"); err != nil {
		t.Fatal(err)
	}
	if len(fixture.state(t).Installations) != 1 {
		t.Fatal("remove dry-run changed state")
	}
	fixture.server.Close()
	if _, err := fixture.run(t, "remove", "ponytail", "--yes"); err != nil {
		t.Fatal(err)
	}
	if len(fixture.state(t).Installations) != 0 {
		t.Fatal("remove left installation in state")
	}
	if content, err := os.ReadFile(foreignPath); err != nil || string(content) != "keep me" {
		t.Fatalf("remove lost foreign file: %s %v", content, err)
	}
}

func TestGlobalPinConflictAndOfflineStatus(t *testing.T) {
	fixture := newGlobalFixture(t)
	if _, err := fixture.run(t, "install", "github:example/skills", "--agent=codex", "--ref=stable"); err != nil {
		t.Fatal(err)
	}
	fixture.revision.Store(2)
	if _, err := fixture.run(t, "update", "ponytail"); err != nil {
		t.Fatal(err)
	}
	if installation := fixture.state(t).Installations[0]; !installation.Pinned || installation.RequestedRef != "stable" || installation.ResolvedRevision != strings.Repeat("1", 40) {
		t.Fatalf("pin moved: %+v", installation)
	}
	managedPath := filepath.Join(fixture.home, ".agents", "skills", "ponytail", "SKILL.md")
	if err := os.WriteFile(managedPath, []byte("local edit"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, operation := range []string{"update", "remove"} {
		if _, err := fixture.run(t, operation, "ponytail", "--yes"); err == nil {
			t.Fatalf("%s accepted local changes", operation)
		}
		if content, err := os.ReadFile(managedPath); err != nil || string(content) != "local edit" {
			t.Fatalf("local change lost: %s %v", content, err)
		}
	}
	forced, err := fixture.run(t, "update", "ponytail", "--force")
	if err != nil || forced.Resources[0].BackupPath == "" {
		t.Fatalf("force missing backup: %+v %v", forced, err)
	}
	if _, err := fixture.run(t, "install", "github:example/skills", "--agent=claude", "--ref=new-pin"); err != nil {
		t.Fatal(err)
	}
	if installation := fixture.state(t).Installations[0]; installation.ResolvedRevision != strings.Repeat("2", 40) || installation.RequestedRef != "new-pin" || len(installation.Placements) != 2 {
		t.Fatalf("explicit repin failed: %+v", installation)
	}
	fixture.server.Close()
	status, err := fixture.run(t, "status")
	if err != nil || len(status.Resources) != 1 || status.Resources[0].FileState != "files-installed" {
		t.Fatalf("offline status lost local state: %+v %v", status, err)
	}
}
