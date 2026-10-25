package fs

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestGlobalMutationRollsBackDestinationsAndState(t *testing.T) {
	context := globalTestContext(t)
	state, writes := globalTestInstall(context, []string{"agents", "claude"}, []byte("original"))
	globalApplyFixture(t, context, state, writes)
	next, nextWrites := globalTestInstall(context, []string{"agents", "claude"}, []byte("updated"))
	next.Installations[0].ResolvedRevision = strings.Repeat("b", 40)
	globalMutationHook = func(stage, target string) error {
		if stage == "before-write" && strings.Contains(target, ".claude") {
			return errors.New("injected second-destination failure")
		}
		return nil
	}
	t.Cleanup(func() { globalMutationHook = nil })
	if _, err := ApplyGlobalChange(context, state, next, nextWrites, GlobalMutationOptions{}); err == nil || !strings.Contains(err.Error(), "injected") {
		t.Fatalf("expected destination failure, got %v", err)
	}
	for _, rootID := range []string{"agents", "claude"} {
		content, err := os.ReadFile(filepath.Join(context.Roots[rootID], "ponytail", "SKILL.md"))
		if err != nil || string(content) != "original" {
			t.Fatalf("rollback lost %s: %q %v", rootID, content, err)
		}
	}
	if saved := globalReadSavedState(t, context); saved.Installations[0].ResolvedRevision != state.Installations[0].ResolvedRevision {
		t.Fatalf("state not restored: %+v", saved)
	}
}

func globalTestInstall(context GlobalContext, roots []string, content []byte) (GlobalState, []GlobalWrite) {
	installation := GlobalInstallation{
		ID:        "github:github.com/example/repository#skills/ponytail",
		SkillName: "ponytail", Source: GlobalSource{Provider: "github", Host: "github.com", Repository: "example/repository", Subdir: "skills/ponytail"},
		RequestedRef: "main", ResolvedRevision: strings.Repeat("a", 40), RequestedConsumers: roots,
	}
	writes := []GlobalWrite{}
	for _, rootID := range roots {
		file := GlobalFile{RootID: rootID, RelativePath: "ponytail/SKILL.md", SHA256: ComputeSHA256(content), Mode: 0644}
		installation.Placements = append(installation.Placements, GlobalPlacement{RootID: rootID, RelativePath: "ponytail", ResolvedRoot: context.Roots[rootID], Consumers: []string{rootID}, Files: []GlobalFile{file}})
		writes = append(writes, GlobalWrite{RootID: rootID, RelativePath: file.RelativePath, Data: content, Mode: file.Mode})
	}
	return GlobalState{SchemaVersion: 1, Installations: []GlobalInstallation{installation}}, writes
}

func globalEmptyState() GlobalState {
	return GlobalState{SchemaVersion: 1, Installations: []GlobalInstallation{}}
}

func globalApplyFixture(t *testing.T, context GlobalContext, state GlobalState, writes []GlobalWrite) {
	t.Helper()
	if _, err := ApplyGlobalChange(context, globalEmptyState(), state, writes, GlobalMutationOptions{}); err != nil {
		t.Fatal(err)
	}
}

func TestGlobalInstallDryRunIdempotenceAndRemove(t *testing.T) {
	context := globalTestContext(t)
	content := []byte{0, 255, 32, 10, 77}
	state, writes := globalTestInstall(context, []string{"agents", "claude"}, content)
	result, err := ApplyGlobalChange(context, globalEmptyState(), state, writes, GlobalMutationOptions{DryRun: true})
	if err != nil || !result.Changed {
		t.Fatalf("dry-run: %+v %v", result, err)
	}
	entries, err := os.ReadDir(context.Home)
	if err != nil || len(entries) != 0 {
		t.Fatalf("dry-run wrote: %v %v", entries, err)
	}
	globalApplyFixture(t, context, state, writes)
	installedPath := filepath.Join(context.Roots["agents"], "ponytail", "SKILL.md")
	contentAfter, err := os.ReadFile(installedPath)
	if err != nil || !bytes.Equal(content, contentAfter) {
		t.Fatalf("binary content changed: %v %v", contentAfter, err)
	}
	beforeInfo, err := os.Stat(installedPath)
	if err != nil {
		t.Fatal(err)
	}
	result, err = ApplyGlobalChange(context, state, state, writes, GlobalMutationOptions{})
	if err != nil || result.Changed {
		t.Fatalf("idempotent result: %+v %v", result, err)
	}
	afterInfo, err := os.Stat(installedPath)
	if err != nil || !beforeInfo.ModTime().Equal(afterInfo.ModTime()) {
		t.Fatalf("idempotent install rewrote file: %v", err)
	}
	foreignPath := filepath.Join(filepath.Dir(installedPath), "notes.txt")
	if err := os.WriteFile(foreignPath, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyGlobalChange(context, state, globalEmptyState(), nil, GlobalMutationOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(installedPath); !os.IsNotExist(err) {
		t.Fatalf("managed file remains: %v", err)
	}
	foreign, err := os.ReadFile(foreignPath)
	if err != nil || string(foreign) != "keep" {
		t.Fatalf("foreign file lost: %q %v", foreign, err)
	}
}

func TestGlobalMutationProtectsThirdPartyAndLocalChanges(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run("third-party-"+map[bool]string{true: "force", false: "normal"}[force], func(t *testing.T) {
			context := globalTestContext(t)
			state, writes := globalTestInstall(context, []string{"agents"}, []byte("original"))
			target := filepath.Join(context.Roots["agents"], "ponytail")
			if err := os.MkdirAll(target, 0755); err != nil {
				t.Fatal(err)
			}
			if _, err := ApplyGlobalChange(context, globalEmptyState(), state, writes, GlobalMutationOptions{Force: force}); err == nil || !strings.Contains(err.Error(), "third-party") {
				t.Fatalf("third-party directory accepted: %v", err)
			}
		})
	}
	context := globalTestContext(t)
	state, writes := globalTestInstall(context, []string{"agents"}, []byte("original"))
	globalApplyFixture(t, context, state, writes)
	target := filepath.Join(context.Roots["agents"], "ponytail", "SKILL.md")
	if err := os.WriteFile(target, []byte("local work"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyGlobalChange(context, state, globalEmptyState(), nil, GlobalMutationOptions{}); err == nil || !strings.Contains(err.Error(), "modified") {
		t.Fatalf("local modification removed: %v", err)
	}
	result, err := ApplyGlobalChange(context, state, state, writes, GlobalMutationOptions{Force: true})
	if err != nil || result.BackupPath == "" {
		t.Fatalf("force did not save backup: %+v %v", result, err)
	}
	backup, err := os.ReadFile(filepath.Join(result.BackupPath, "agents", "ponytail", "SKILL.md"))
	if err != nil || string(backup) != "local work" {
		t.Fatalf("local work not backed up: %q %v", backup, err)
	}
}

func TestGlobalMutationRejectsStaleStateAndSourceCollision(t *testing.T) {
	context := globalTestContext(t)
	state, writes := globalTestInstall(context, []string{"agents"}, []byte("original"))
	globalApplyFixture(t, context, state, writes)
	if _, err := ApplyGlobalChange(context, globalEmptyState(), state, writes, GlobalMutationOptions{}); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("stale state accepted: %v", err)
	}
	other := state
	other.Installations = append([]GlobalInstallation(nil), state.Installations...)
	other.Installations[0].ID = "github:github.com/another/repository#skills/ponytail"
	other.Installations[0].Source.Repository = "another/repository"
	if _, err := ApplyGlobalChange(context, state, other, writes, GlobalMutationOptions{Force: true}); err == nil || !strings.Contains(err.Error(), "source") {
		t.Fatalf("source takeover accepted: %v", err)
	}
}

func TestGlobalMutationRejectsUnsafeStatePaths(t *testing.T) {
	for _, relative := range []string{"../escape", "/absolute", `ponytail\escape`, "ponytail/CON", "ponytail/file.", "ponytail/file:stream", "ponytail/../other"} {
		t.Run(relative, func(t *testing.T) {
			context := globalTestContext(t)
			state, writes := globalTestInstall(context, []string{"agents"}, []byte("original"))
			state.Installations[0].Placements[0].Files[0].RelativePath = relative
			writes[0].RelativePath = relative
			if _, err := ApplyGlobalChange(context, globalEmptyState(), state, writes, GlobalMutationOptions{}); err == nil {
				t.Fatalf("unsafe state path accepted: %s", relative)
			}
		})
	}
}

func TestGlobalInspectMissingModifiedAndRelocated(t *testing.T) {
	context := globalTestContext(t)
	state, writes := globalTestInstall(context, []string{"agents", "claude"}, []byte("original"))
	globalApplyFixture(t, context, state, writes)
	if err := os.Remove(filepath.Join(context.Roots["agents"], "ponytail", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(context.Roots["claude"], "ponytail", "SKILL.md"), []byte("edited"), 0644); err != nil {
		t.Fatal(err)
	}
	statuses, err := InspectGlobalInstallation(context, state.Installations[0])
	if err != nil || len(statuses) != 2 || statuses[0].Status != "missing" || statuses[1].Status != "modified-locally" {
		t.Fatalf("unexpected status: %+v %v", statuses, err)
	}
	state.Installations[0].Placements[0].ResolvedRoot = filepath.Join(t.TempDir(), "skills")
	if _, err := InspectGlobalInstallation(context, state.Installations[0]); err == nil || !strings.Contains(err.Error(), "relocat") {
		t.Fatalf("relocated root accepted: %v", err)
	}
}

func TestGlobalMutationRejectsRootCaseCollision(t *testing.T) {
	context := globalTestContext(t)
	if err := os.MkdirAll(filepath.Join(context.Home, ".AGENTS", "skills"), 0755); err != nil {
		t.Fatal(err)
	}
	state, writes := globalTestInstall(context, []string{"agents"}, []byte("original"))
	if _, err := ApplyGlobalChange(context, globalEmptyState(), state, writes, GlobalMutationOptions{DryRun: true}); err == nil || !strings.Contains(err.Error(), "case collision") {
		t.Fatalf("root case collision accepted: %v", err)
	}
}

func TestGlobalMutationChecksDestinationsBeforeFirstWrite(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows directory writability is controlled by ACLs, not POSIX permission bits")
	}
	context := globalTestContext(t)
	state, writes := globalTestInstall(context, []string{"agents", "claude"}, []byte("original"))
	if err := os.MkdirAll(context.Roots["claude"], 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(context.Roots["claude"], 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(context.Roots["claude"], 0755) })
	if _, err := ApplyGlobalChange(context, globalEmptyState(), state, writes, GlobalMutationOptions{}); err == nil || !strings.Contains(err.Error(), "writable") {
		t.Fatalf("non-writable destination not rejected in preflight: %v", err)
	}
	if _, err := os.Stat(context.Roots["agents"]); !os.IsNotExist(err) {
		t.Fatalf("first destination modified before second destination validated: %v", err)
	}
}

func TestGlobalBinaryExecutableAndProvenancePreserved(t *testing.T) {
	context := globalTestContext(t)
	state, writes := globalTestInstall(context, []string{"agents"}, []byte("skill"))
	asset := []byte{0, 255, 128, 10, 13}
	scriptFile := GlobalFile{RootID: "agents", RelativePath: "ponytail/scripts/runner", SHA256: ComputeSHA256(asset), Mode: 0755}
	state.Installations[0].Placements[0].Files = append(state.Installations[0].Placements[0].Files, scriptFile)
	writes = append(writes, GlobalWrite{RootID: scriptFile.RootID, RelativePath: scriptFile.RelativePath, Data: asset, Mode: scriptFile.Mode})
	license := GlobalFile{RootID: "provenance", RelativePath: "test-identity/LICENSE", SHA256: ComputeSHA256([]byte("license")), Mode: 0644}
	state.Installations[0].Provenance = []GlobalFile{license}
	writes = append(writes, GlobalWrite{RootID: license.RootID, RelativePath: license.RelativePath, Data: []byte("license"), Mode: license.Mode})
	globalApplyFixture(t, context, state, writes)
	scriptPath := filepath.Join(context.Roots["agents"], filepath.FromSlash(scriptFile.RelativePath))
	content, err := os.ReadFile(scriptPath)
	if err != nil || !bytes.Equal(content, asset) {
		t.Fatalf("binary asset changed: %v %v", content, err)
	}
	info, err := os.Stat(scriptPath)
	if err != nil || !globalModesEqual(uint32(info.Mode().Perm()), 0755) {
		t.Fatalf("executable permissions changed: %v %v", info, err)
	}
	licensePath := filepath.Join(context.StateRoot, "provenance", filepath.FromSlash(license.RelativePath))
	if content, err := os.ReadFile(licensePath); err != nil || string(content) != "license" {
		t.Fatalf("provenance missing: %q %v", content, err)
	}
}

func TestGlobalMutationPreservesEditAfterPreflight(t *testing.T) {
	context := globalTestContext(t)
	state, writes := globalTestInstall(context, []string{"agents"}, []byte("original"))
	globalApplyFixture(t, context, state, writes)
	after, nextWrites := globalTestInstall(context, []string{"agents"}, []byte("upstream"))
	globalMutationHook = func(stage, target string) error {
		if stage == "before-write" {
			return os.WriteFile(target, []byte("concurrent edit"), 0644)
		}
		return nil
	}
	t.Cleanup(func() { globalMutationHook = nil })
	if _, err := ApplyGlobalChange(context, state, after, nextWrites, GlobalMutationOptions{}); err == nil {
		t.Fatal("concurrent local edit overwritten")
	}
	content, err := os.ReadFile(filepath.Join(context.Roots["agents"], "ponytail", "SKILL.md"))
	if err != nil || string(content) != "concurrent edit" {
		t.Fatalf("concurrent local edit lost: %q %v", content, err)
	}
}

func TestGlobalMutationRejectsUnicodeAndReservedPathAliases(t *testing.T) {
	for _, relative := range []string{"ponytail/COM¹.txt", "ponytail/LPT²", "ponytail/invalid-\xff"} {
		t.Run(relative, func(t *testing.T) {
			context := globalTestContext(t)
			state, writes := globalTestInstall(context, []string{"agents"}, []byte("original"))
			state.Installations[0].Placements[0].Files[0].RelativePath = relative
			writes[0].RelativePath = relative
			if _, err := ApplyGlobalChange(context, globalEmptyState(), state, writes, GlobalMutationOptions{DryRun: true}); err == nil {
				t.Fatalf("unsafe unicode path accepted: %q", relative)
			}
		})
	}
	context := globalTestContext(t)
	state, writes := globalTestInstall(context, []string{"agents"}, []byte("original"))
	for _, relative := range []string{"ponytail/café.txt", "ponytail/café.txt"} {
		file := GlobalFile{RootID: "agents", RelativePath: relative, SHA256: ComputeSHA256([]byte("asset")), Mode: 0644}
		state.Installations[0].Placements[0].Files = append(state.Installations[0].Placements[0].Files, file)
		writes = append(writes, GlobalWrite{RootID: file.RootID, RelativePath: relative, Data: []byte("asset"), Mode: 0644})
	}
	if _, err := ApplyGlobalChange(context, globalEmptyState(), state, writes, GlobalMutationOptions{DryRun: true}); err == nil {
		t.Fatal("canonically identical Unicode destinations accepted")
	}
}

func TestGlobalPermissionComparisonUsesPlatformCapabilities(t *testing.T) {
	for _, expectation := range []struct {
		platform string
		actual   uint32
		expected uint32
		equal    bool
	}{
		{"windows", 0666, 0644, true},
		{"windows", 0666, 0755, true},
		{"windows", 0444, 0644, false},
		{"windows", 0444, 0555, true},
		{"darwin", 0644, 0755, false},
		{"linux", 0644, 0644, true},
	} {
		if actual := globalModesEqualForOS(expectation.platform, expectation.actual, expectation.expected); actual != expectation.equal {
			t.Fatalf("%s modes %o/%o equal=%v, expected %v", expectation.platform, expectation.actual, expectation.expected, actual, expectation.equal)
		}
	}
}

func TestGlobalMutationPreservesStateEditedBeforePublication(t *testing.T) {
	context := globalTestContext(t)
	state, writes := globalTestInstall(context, []string{"agents"}, []byte("original"))
	globalApplyFixture(t, context, state, writes)
	after, nextWrites := globalTestInstall(context, []string{"agents"}, []byte("upstream"))
	globalMutationHook = func(stage, target string) error {
		if stage == "before-state" {
			return os.WriteFile(target, []byte("external state edit"), 0600)
		}
		return nil
	}
	t.Cleanup(func() { globalMutationHook = nil })
	if _, err := ApplyGlobalChange(context, state, after, nextWrites, GlobalMutationOptions{}); err == nil {
		t.Fatal("concurrent global state edit overwritten")
	}
	content, err := os.ReadFile(filepath.Join(context.StateRoot, "state.json"))
	if err != nil || string(content) != "external state edit" {
		t.Fatalf("concurrent global state edit lost: %q %v", content, err)
	}
}

func TestGlobalMutationDoesNotAdoptForeignNestedDirectory(t *testing.T) {
	context := globalTestContext(t)
	state, writes := globalTestInstall(context, []string{"agents"}, []byte("original"))
	globalApplyFixture(t, context, state, writes)
	foreignDirectory := filepath.Join(context.Roots["agents"], "ponytail", "scripts")
	if err := os.Mkdir(foreignDirectory, 0755); err != nil {
		t.Fatal(err)
	}
	after, nextWrites := globalTestInstall(context, []string{"agents"}, []byte("original"))
	file := GlobalFile{RootID: "agents", RelativePath: "ponytail/scripts/runner", SHA256: ComputeSHA256([]byte("script")), Mode: 0755}
	after.Installations[0].Placements[0].Files = append(after.Installations[0].Placements[0].Files, file)
	nextWrites = append(nextWrites, GlobalWrite{RootID: file.RootID, RelativePath: file.RelativePath, Data: []byte("script"), Mode: file.Mode})
	if _, err := ApplyGlobalChange(context, state, after, nextWrites, GlobalMutationOptions{Force: true}); err == nil || !strings.Contains(err.Error(), "third-party directory") {
		t.Fatalf("foreign nested directory implicitly adopted: %v", err)
	}
}

func globalReadSavedState(t *testing.T, context GlobalContext) GlobalState {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(context.StateRoot, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var state GlobalState
	if err := json.Unmarshal(content, &state); err != nil {
		t.Fatal(err)
	}
	return state
}
