package cmd

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	sthfs "sth/internal/fs"
)

func newGlobalAcceptanceFixture(t *testing.T) *globalFixture {
	return newGlobalAcceptanceFileFixture(t, false)
}

func newGlobalAcceptanceFileFixture(t *testing.T, evolving bool) *globalFixture {
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
			revision := strings.TrimPrefix(request.URL.Path, "/repos/example/skills/git/trees/")
			entries := []map[string]string{}
			for _, name := range globalAcceptanceSkillNames(revision) {
				if evolving && name == "audit" && strings.HasPrefix(revision, "2") {
					continue
				}
				entries = append(entries,
					map[string]string{"path": "skills/" + name + "/SKILL.md", "type": "blob", "mode": "100644"},
					map[string]string{"path": ".openclaw/skills/" + name + "/SKILL.md", "type": "blob", "mode": "100644"})
			}
			entries = append(entries, map[string]string{"path": "skills/plugin.json", "type": "blob", "mode": "100644"})
			if evolving {
				filePath := "skills/review/obsolete.txt"
				if strings.HasPrefix(revision, "2") {
					filePath = "skills/review/references/new.bin"
				}
				entries = append(entries, map[string]string{"path": filePath, "type": "blob", "mode": "100644"})
			}
			if err := json.NewEncoder(response).Encode(map[string]any{"truncated": false, "tree": entries}); err != nil {
				t.Error(err)
			}
		case strings.HasPrefix(request.URL.Path, "/example/skills/"):
			parts := strings.SplitN(strings.TrimPrefix(request.URL.Path, "/example/skills/"), "/", 2)
			if len(parts) != 2 || len(parts[0]) != 40 {
				t.Errorf("unpinned download: %s", request.URL.Path)
				http.Error(response, "unpinned", http.StatusBadRequest)
				return
			}
			if evolving && (parts[1] == "skills/review/obsolete.txt" || parts[1] == "skills/review/references/new.bin") {
				_, _ = response.Write([]byte{0, 128, 255})
				return
			}
			for _, name := range globalAcceptanceSkillNames(parts[0]) {
				if evolving && name == "audit" && strings.HasPrefix(parts[0], "2") {
					continue
				}
				if parts[1] == "skills/"+name+"/SKILL.md" {
					fmt.Fprintf(response, "---\nname: %s\ndescription: Acceptance fixture\n---\nRevision %s\n", name, parts[0])
					return
				}
			}
			http.NotFound(response, request)
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

func globalAcceptanceSkillNames(revision string) []string {
	names := []string{"audit", "debug", "plan", "ponytail", "review", "test"}
	if strings.HasPrefix(revision, "2") {
		names = append(names, "newcomer")
	}
	return names
}

func globalAcceptanceSnapshot(t *testing.T, directory string) map[string]string {
	t.Helper()
	snapshot := map[string]string{}
	err := filepath.WalkDir(directory, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			relativePath, err := filepath.Rel(directory, filePath)
			if err != nil {
				return err
			}
			snapshot[relativePath] = "directory"
			return nil
		}
		content, err := os.ReadFile(filePath)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		relativePath, err := filepath.Rel(directory, filePath)
		if err != nil {
			return err
		}
		snapshot[relativePath] = fmt.Sprintf("%o:%d:%s", info.Mode().Perm(), info.ModTime().UnixNano(), content)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestGlobalAcceptanceInitializedProjectUnchangedAcrossLifecycle(t *testing.T) {
	fixture := newGlobalFixture(t)
	if err := os.MkdirAll(filepath.Join(fixture.project, ".sth"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"project.json": "existing project sentinel", "last-operation.json": `{"operation":"previous"}`} {
		if err := os.WriteFile(filepath.Join(fixture.project, ".sth", name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	before := globalAcceptanceSnapshot(t, fixture.project)
	for _, arguments := range [][]string{
		{"install", "github:example/skills", "--agent", "codex", "--dry-run"},
		{"install", "github:example/skills", "--agent", "codex"},
		{"list"}, {"status"}, {"update", "--dry-run"}, {"update"},
		{"remove", "ponytail", "--dry-run"}, {"remove", "ponytail"},
	} {
		if _, err := fixture.run(t, arguments...); err != nil {
			t.Fatalf("%v: %v", arguments, err)
		}
		if after := globalAcceptanceSnapshot(t, fixture.project); !reflect.DeepEqual(before, after) {
			t.Fatalf("%v changed current project: before %v, after %v", arguments, before, after)
		}
	}
}

func TestGlobalAcceptanceSyntaxErrorsHaveVersionedJSONAndCodeTwo(t *testing.T) {
	fixture := newGlobalFixture(t)
	for _, arguments := range [][]string{
		{"install", "github:example/skills", "--agent"},
		{"install", "github:example/skills", "--agent", "unknown"},
		{"install", "github:example/skills", "--skill", "missing", "--agent", "codex"},
		{"install", "github:example/skills", "--path", "../escape", "--agent", "codex"},
		{"status", "--wat"}, {"update", "self"},
	} {
		result, err := fixture.run(t, arguments...)
		if ExitCode(err) != 2 || result.SchemaVersion != 1 || result.Scope != "global" || len(result.Errors) == 0 {
			t.Errorf("%v: code %d, result %+v, error %v", arguments, ExitCode(err), result, err)
		}
	}
	for _, directory := range []string{fixture.home, fixture.configuration, fixture.project} {
		entries, err := os.ReadDir(directory)
		if err != nil || len(entries) != 0 {
			t.Fatalf("invalid invocation wrote %s: %v, %v", directory, entries, err)
		}
	}
}

func TestGlobalAcceptanceWildcardSelectsSixCanonicalSkills(t *testing.T) {
	fixture := newGlobalAcceptanceFixture(t)
	result, err := fixture.run(t, "install", "github:example/skills", "--path", "skills", "--skill", "*", "--all-agents")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Resources) != 6 || len(fixture.state(t).Installations) != 6 {
		t.Fatalf("expected six canonical skills: %+v", result)
	}
	for _, resource := range result.Resources {
		if resource.Source.Subdir != "skills/"+resource.SkillName || len(resource.Placements) != 4 || len(resource.RequestedConsumers) != 8 {
			t.Fatalf("wrong canonical projection: %+v", resource)
		}
		if resource.ValidationLevel != "discovery-unverified" {
			t.Fatalf("unverified runtime claimed: %+v", resource)
		}
	}
	for filePath, entry := range globalAcceptanceSnapshot(t, fixture.home) {
		if entry == "directory" {
			continue
		}
		if !strings.HasSuffix(filePath, "SKILL.md") || strings.Contains(filePath, "openclaw") || strings.Contains(filePath, "plugin") {
			t.Fatalf("unexpected installed file %s", filePath)
		}
	}
}

func TestGlobalAcceptanceForcedLifecycleDryRunsLeaveEveryFileUntouched(t *testing.T) {
	fixture := newGlobalFixture(t)
	if _, err := fixture.run(t, "install", "github:example/skills", "--agent", "codex"); err != nil {
		t.Fatal(err)
	}
	managedFile := filepath.Join(fixture.home, ".agents", "skills", "ponytail", "SKILL.md")
	if err := os.WriteFile(managedFile, []byte("user edit"), 0o644); err != nil {
		t.Fatal(err)
	}
	fixture.revision.Store(2)
	for _, directory := range []string{fixture.home, fixture.configuration, fixture.project} {
		before := globalAcceptanceSnapshot(t, directory)
		for _, arguments := range [][]string{{"update", "ponytail", "--force", "--dry-run"}, {"remove", "ponytail", "--force", "--dry-run"}} {
			if _, err := fixture.run(t, arguments...); err != nil {
				t.Fatalf("%v: %v", arguments, err)
			}
			if after := globalAcceptanceSnapshot(t, directory); !reflect.DeepEqual(before, after) {
				t.Fatalf("%v modified %s: before %v, after %v", arguments, directory, before, after)
			}
		}
	}
}

func TestGlobalAcceptanceMultiSkillConflictIsAtomic(t *testing.T) {
	fixture := newGlobalAcceptanceFixture(t)
	foreignFile := filepath.Join(fixture.home, ".claude", "skills", "review", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(foreignFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(foreignFile, []byte("foreign skill"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := globalAcceptanceSnapshot(t, fixture.home)
	result, err := fixture.run(t, "install", "github:example/skills", "--path", "skills", "--skill", "*", "--all-agents", "--force", "--yes")
	if ExitCode(err) != 1 || len(result.Errors) == 0 {
		t.Fatalf("third-party conflict not reported: %+v, %v", result, err)
	}
	if after := globalAcceptanceSnapshot(t, fixture.home); !reflect.DeepEqual(before, after) {
		t.Fatalf("partial installation persisted: before %v, after %v", before, after)
	}
	if _, err := os.Stat(filepath.Join(fixture.configuration, "sth", "global", "state.json")); !os.IsNotExist(err) {
		t.Fatalf("failed installation published state: %v", err)
	}
}

func TestGlobalAcceptanceUpdateAggregatesFailuresAndKeepsConsumers(t *testing.T) {
	fixture := newGlobalAcceptanceFixture(t)
	if _, err := fixture.run(t, "install", "github:example/skills", "--skill", "audit", "--skill", "review", "--agent", "codex", "--agent", "claude"); err != nil {
		t.Fatal(err)
	}
	modified := filepath.Join(fixture.home, ".claude", "skills", "audit", "SKILL.md")
	if err := os.WriteFile(modified, []byte("user changes"), 0o644); err != nil {
		t.Fatal(err)
	}
	fixture.revision.Store(2)
	result, err := fixture.run(t, "update")
	if ExitCode(err) != 1 || len(result.Errors) == 0 || len(result.Resources) != 2 {
		t.Fatalf("expected partial failure with both resources: %+v, %v", result, err)
	}
	for _, installation := range fixture.state(t).Installations {
		expectedRevision := strings.Repeat("2", 40)
		if installation.SkillName == "audit" {
			expectedRevision = strings.Repeat("1", 40)
		}
		if installation.ResolvedRevision != expectedRevision || !reflect.DeepEqual(installation.RequestedConsumers, []string{"claude", "codex"}) {
			t.Fatalf("unexpected post-update installation: %+v", installation)
		}
	}
	content, err := os.ReadFile(modified)
	if err != nil || string(content) != "user changes" {
		t.Fatalf("local edit lost: %q, %v", content, err)
	}
}

func TestGlobalAcceptanceNewIdentityUsesCurrentBranchWhileExistingKeepsRevision(t *testing.T) {
	fixture := newGlobalAcceptanceFixture(t)
	if _, err := fixture.run(t, "install", "github:example/skills", "--skill", "audit", "--agent", "codex"); err != nil {
		t.Fatal(err)
	}
	fixture.revision.Store(2)
	if _, err := fixture.run(t, "install", "github:example/skills", "--skill", "newcomer", "--agent", "claude"); err != nil {
		t.Fatalf("new skill must be discoverable on current default branch: %v", err)
	}
	if _, err := fixture.run(t, "install", "github:example/skills", "--skill", "audit", "--agent", "claude"); err != nil {
		t.Fatal(err)
	}
	for _, installation := range fixture.state(t).Installations {
		expectedRevision := strings.Repeat("2", 40)
		if installation.SkillName == "audit" {
			expectedRevision = strings.Repeat("1", 40)
		}
		if installation.ResolvedRevision != expectedRevision {
			t.Fatalf("wrong identity revision: %+v", installation)
		}
	}
}

func TestGlobalAcceptanceCorruptStateBlocksWithoutMutation(t *testing.T) {
	for _, stateContent := range []string{"invalid", `{"schema_version":999,"installations":[]}`} {
		t.Run(stateContent, func(t *testing.T) {
			fixture := newGlobalFixture(t)
			stateDirectory := filepath.Join(fixture.configuration, "sth", "global")
			if err := os.MkdirAll(stateDirectory, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(stateDirectory, "state.json"), []byte(stateContent), 0o600); err != nil {
				t.Fatal(err)
			}
			before := globalAcceptanceSnapshot(t, fixture.configuration)
			for _, arguments := range [][]string{{"list"}, {"install", "github:example/skills", "--agent", "codex"}} {
				if result, err := fixture.run(t, arguments...); ExitCode(err) != 1 || len(result.Errors) == 0 {
					t.Fatalf("%v accepted invalid state: %+v, %v", arguments, result, err)
				}
			}
			if after := globalAcceptanceSnapshot(t, fixture.configuration); !reflect.DeepEqual(before, after) {
				t.Fatalf("invalid state mutated: before %v, after %v", before, after)
			}
		})
	}
}

func TestGlobalAcceptanceForgedStatePathCannotRemoveForeignFile(t *testing.T) {
	fixture := newGlobalFixture(t)
	if _, err := fixture.run(t, "install", "github:example/skills", "--agent", "codex"); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(fixture.home, "foreign.txt")
	if err := os.WriteFile(foreign, []byte("preserve me"), 0o644); err != nil {
		t.Fatal(err)
	}
	state := fixture.state(t)
	state.Installations[0].Placements[0].Files[0] = sthfs.GlobalFile{RootID: "agents", RelativePath: "../../foreign.txt", SHA256: sthfs.ComputeSHA256([]byte("preserve me")), Mode: 0o644}
	content, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixture.configuration, "sth", "global", "state.json"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	before := globalAcceptanceSnapshot(t, fixture.home)
	if _, err := fixture.run(t, "remove", "ponytail", "--force"); ExitCode(err) != 1 {
		t.Fatalf("forged state accepted: %v", err)
	}
	if after := globalAcceptanceSnapshot(t, fixture.home); !reflect.DeepEqual(before, after) {
		t.Fatalf("forged state modified files: before %v, after %v", before, after)
	}
}

func TestGlobalAcceptanceResultExposesUnrequestedReaders(t *testing.T) {
	fixture := newGlobalFixture(t)
	result, err := fixture.run(t, "install", "github:example/skills", "--agent", "codex", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Resources []struct {
			Destinations []struct {
				RequestedConsumers []string `json:"requested_consumers"`
				PossibleReaders    []string `json:"possible_readers"`
			} `json:"destinations"`
		} `json:"resources"`
	}
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Resources) != 1 || len(envelope.Resources[0].Destinations) != 1 {
		t.Fatalf("missing physical destination details: %s", encoded)
	}
	destination := envelope.Resources[0].Destinations[0]
	if !reflect.DeepEqual(destination.RequestedConsumers, []string{"codex"}) || !reflect.DeepEqual(destination.PossibleReaders, []string{"codex", "copilot-cli", "copilot-vscode", "cursor", "gemini"}) {
		t.Fatalf("requested consumers and incidental readers not distinguished: %s", encoded)
	}
}

func TestGlobalAcceptanceUpdateRepairsMissingAndRetiresRemovedUpstreamFiles(t *testing.T) {
	fixture := newGlobalAcceptanceFileFixture(t, true)
	if _, err := fixture.run(t, "install", "github:example/skills", "--skill", "review", "--agent", "codex"); err != nil {
		t.Fatal(err)
	}
	skillDirectory := filepath.Join(fixture.home, ".agents", "skills", "review")
	entrypoint := filepath.Join(skillDirectory, "SKILL.md")
	if err := os.Remove(entrypoint); err != nil {
		t.Fatal(err)
	}
	status, err := fixture.run(t, "status", "review")
	if err != nil || len(status.Resources) != 1 || status.Resources[0].FileState != "missing" {
		t.Fatalf("missing file not reported: %+v, %v", status, err)
	}
	if _, err := fixture.run(t, "update", "review"); err != nil {
		t.Fatal(err)
	}
	if content, err := os.ReadFile(entrypoint); err != nil || !strings.Contains(string(content), strings.Repeat("1", 40)) {
		t.Fatalf("missing entrypoint was not repaired: %q, %v", content, err)
	}
	foreign := filepath.Join(skillDirectory, "my-notes.txt")
	if err := os.WriteFile(foreign, []byte("user notes"), 0o600); err != nil {
		t.Fatal(err)
	}
	fixture.revision.Store(2)
	if _, err := fixture.run(t, "update", "review"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(skillDirectory, "obsolete.txt")); !os.IsNotExist(err) {
		t.Fatalf("obsolete tracked file remains: %v", err)
	}
	if content, err := os.ReadFile(filepath.Join(skillDirectory, "references", "new.bin")); err != nil || !reflect.DeepEqual(content, []byte{0, 128, 255}) {
		t.Fatalf("new binary asset differs: %v, %v", content, err)
	}
	if content, err := os.ReadFile(foreign); err != nil || string(content) != "user notes" {
		t.Fatalf("foreign file was modified: %q, %v", content, err)
	}
}

func TestGlobalAcceptanceAddAgentToSeveralRevisionsAfterUpstreamRemoval(t *testing.T) {
	fixture := newGlobalAcceptanceFileFixture(t, true)
	if _, err := fixture.run(t, "install", "github:example/skills", "--skill", "audit", "--agent", "codex"); err != nil {
		t.Fatal(err)
	}
	fixture.revision.Store(2)
	if _, err := fixture.run(t, "install", "github:example/skills", "--skill", "newcomer", "--agent", "codex"); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.run(t, "install", "github:example/skills", "--skill", "audit", "--skill", "newcomer", "--agent", "claude"); err != nil {
		t.Fatalf("existing identities should resolve independently at their recorded revisions: %v", err)
	}
	for _, installation := range fixture.state(t).Installations {
		expected := strings.Repeat("2", 40)
		if installation.SkillName == "audit" {
			expected = strings.Repeat("1", 40)
		}
		if installation.ResolvedRevision != expected || !reflect.DeepEqual(installation.RequestedConsumers, []string{"claude", "codex"}) {
			t.Fatalf("recorded revision or consumers lost: %+v", installation)
		}
	}
}

func stageGlobalAcceptanceInterruption(t *testing.T, fixture *globalFixture) (string, string, []byte) {
	t.Helper()
	if _, err := fixture.run(t, "install", "github:example/skills", "--agent", "codex"); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(fixture.configuration, "sth", "global", "state.json")
	stateBefore, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	stateAfter := fixture.state(t)
	installation := &stateAfter.Installations[0]
	managedFile := filepath.Join(fixture.home, ".agents", "skills", "ponytail", "SKILL.md")
	before, err := os.ReadFile(managedFile)
	if err != nil {
		t.Fatal(err)
	}
	after := []byte(strings.ReplaceAll(string(before), strings.Repeat("1", 40), strings.Repeat("2", 40)))
	installation.ResolvedRevision = strings.Repeat("2", 40)
	for index := range installation.Placements[0].Files {
		file := &installation.Placements[0].Files[index]
		if file.RelativePath == "ponytail/SKILL.md" {
			file.SHA256 = sthfs.ComputeSHA256(after)
		}
	}
	encodedAfter, err := json.Marshal(stateAfter)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := json.Marshal(map[string]any{
		"schema_version": 1,
		"changes": []map[string]any{{
			"root_id": "agents", "relative_path": "ponytail/SKILL.md",
			"before": map[string]any{"data": before, "mode": 0o644},
			"after":  map[string]any{"data": after, "mode": 0o644},
		}},
		"state_before": map[string]any{"data": stateBefore, "mode": 0o600},
		"state_after":  map[string]any{"data": encodedAfter, "mode": 0o600},
	})
	if err != nil {
		t.Fatal(err)
	}
	journalPath := filepath.Join(fixture.configuration, "sth", "global", "transactions", "interrupted", "journal.json")
	if err := os.MkdirAll(filepath.Dir(journalPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(journalPath, journal, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(managedFile, after, 0o644); err != nil {
		t.Fatal(err)
	}
	return managedFile, journalPath, before
}

func TestGlobalAcceptanceRecoversInterruptedFilesBeforeNetworkFailure(t *testing.T) {
	fixture := newGlobalFixture(t)
	managedFile, journalPath, before := stageGlobalAcceptanceInterruption(t, fixture)
	fixture.server.Close()
	if _, err := fixture.run(t, "update", "ponytail"); ExitCode(err) != 1 {
		t.Fatalf("offline update unexpectedly succeeded: %v", err)
	}
	if content, err := os.ReadFile(managedFile); err != nil || !reflect.DeepEqual(content, before) {
		t.Fatalf("interrupted files were not restored before attempting network: %q, %v", content, err)
	}
	if _, err := os.Stat(journalPath); !os.IsNotExist(err) {
		t.Fatalf("successful recovery left journal: %v", err)
	}
}

func TestGlobalAcceptanceRecoveryPreservesEditsAfterInterruption(t *testing.T) {
	fixture := newGlobalFixture(t)
	managedFile, journalPath, _ := stageGlobalAcceptanceInterruption(t, fixture)
	if err := os.WriteFile(managedFile, []byte("work after interruption"), 0o644); err != nil {
		t.Fatal(err)
	}
	beforeHome := globalAcceptanceSnapshot(t, fixture.home)
	beforeConfiguration := globalAcceptanceSnapshot(t, fixture.configuration)
	if result, err := fixture.run(t, "remove", "ponytail", "--force"); ExitCode(err) != 1 || !strings.Contains(strings.Join(result.Errors, " "), "diverged") {
		t.Fatalf("divergent recovery should block even with force: %+v, %v", result, err)
	}
	if after := globalAcceptanceSnapshot(t, fixture.home); !reflect.DeepEqual(after, beforeHome) {
		t.Fatal("recovery changed files after interruption")
	}
	if after := globalAcceptanceSnapshot(t, fixture.configuration); !reflect.DeepEqual(after, beforeConfiguration) {
		t.Fatal("blocked recovery changed state or journal")
	}
	if _, err := os.Stat(journalPath); err != nil {
		t.Fatalf("recovery evidence not preserved: %v", err)
	}
}
