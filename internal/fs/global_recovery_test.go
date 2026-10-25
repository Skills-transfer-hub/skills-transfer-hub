package fs

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGlobalLockChild(t *testing.T) {
	if os.Getenv("STH_GLOBAL_TEST_LOCK") != "1" {
		return
	}
	home := os.Getenv("STH_GLOBAL_TEST_HOME")
	context, err := NewGlobalContext(home, map[string]string{"agents": filepath.Join(home, ".agents", "skills")})
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintln(os.Stdout, "waiting")
	release, err := acquireGlobalLock(context)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	fmt.Fprintln(os.Stdout, "acquired")
}

func TestGlobalLockSerializesProcesses(t *testing.T) {
	context := globalTestContext(t)
	release, err := acquireGlobalLock(context)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	command := exec.Command(os.Args[0], "-test.run=^TestGlobalLockChild$")
	command.Env = append(os.Environ(), "STH_GLOBAL_TEST_LOCK=1", "STH_GLOBAL_TEST_HOME="+context.Home)
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { command.Process.Kill() })
	scanner := bufio.NewScanner(output)
	if !scanner.Scan() || scanner.Text() != "waiting" {
		t.Fatalf("child did not reach lock: %s", scanner.Text())
	}
	acquired := make(chan string, 1)
	go func() {
		if scanner.Scan() {
			acquired <- scanner.Text()
			return
		}
		acquired <- "child exited without lock"
	}()
	select {
	case result := <-acquired:
		t.Fatalf("child bypassed held lock: %s", result)
	case <-time.After(150 * time.Millisecond):
	}
	release()
	select {
	case result := <-acquired:
		if result != "acquired" {
			t.Fatalf("child failed after release: %s", result)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("child remained blocked after lock release")
	}
	if err := command.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestGlobalInterruptedChild(t *testing.T) {
	if os.Getenv("STH_GLOBAL_TEST_CRASH") != "1" {
		return
	}
	home := os.Getenv("STH_GLOBAL_TEST_HOME")
	context, err := NewGlobalContext(home, map[string]string{"agents": filepath.Join(home, ".agents", "skills"), "claude": filepath.Join(home, ".claude", "skills")})
	if err != nil {
		t.Fatal(err)
	}
	before, err := ReadGlobalState(context)
	if err != nil {
		t.Fatal(err)
	}
	after, writes := globalTestInstall(context, []string{"agents", "claude"}, []byte("updated"))
	after.Installations[0].ResolvedRevision = strings.Repeat("b", 40)
	globalMutationHook = func(stage, target string) error {
		if stage == "before-state" {
			os.Exit(71)
		}
		return nil
	}
	if _, err := ApplyGlobalChange(context, before, after, writes, GlobalMutationOptions{}); err != nil {
		t.Fatal(err)
	}
	t.Fatal("crash point not reached")
}

func globalInterruptedFixture(t *testing.T) (GlobalContext, GlobalState) {
	t.Helper()
	context := globalTestContext(t)
	state, writes := globalTestInstall(context, []string{"agents", "claude"}, []byte("original"))
	globalApplyFixture(t, context, state, writes)
	command := exec.Command(os.Args[0], "-test.run=^TestGlobalInterruptedChild$")
	command.Env = append(os.Environ(), "STH_GLOBAL_TEST_CRASH=1", "STH_GLOBAL_TEST_HOME="+context.Home)
	output, err := command.CombinedOutput()
	exitError, ok := err.(*exec.ExitError)
	if !ok || exitError.ExitCode() != 71 {
		t.Fatalf("child did not stop at crash point: %s %v", output, err)
	}
	return context, state
}

func TestGlobalRecoveryRestoresInterruptedTransaction(t *testing.T) {
	context, state := globalInterruptedFixture(t)
	if err := RecoverGlobalTransactions(context); err != nil {
		t.Fatal(err)
	}
	for _, rootID := range []string{"agents", "claude"} {
		content, err := os.ReadFile(filepath.Join(context.Roots[rootID], "ponytail", "SKILL.md"))
		if err != nil || string(content) != "original" {
			t.Fatalf("interrupted %s not restored: %q %v", rootID, content, err)
		}
	}
	if saved := globalReadSavedState(t, context); !globalStatesEqual(saved, state) {
		t.Fatal("interrupted state not restored")
	}
	entries, err := os.ReadDir(filepath.Join(context.StateRoot, "transactions"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("completed recovery journal remains: %v %v", entries, err)
	}
}

func TestGlobalRecoveryPreservesPostInterruptionEdits(t *testing.T) {
	context, _ := globalInterruptedFixture(t)
	target := filepath.Join(context.Roots["agents"], "ponytail", "SKILL.md")
	if err := os.WriteFile(target, []byte("work after interruption"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := RecoverGlobalTransactions(context); err == nil || !strings.Contains(err.Error(), "diverged") {
		t.Fatalf("post-interruption edit accepted for overwrite: %v", err)
	}
	content, err := os.ReadFile(target)
	if err != nil || string(content) != "work after interruption" {
		t.Fatalf("post-interruption edit lost: %q %v", content, err)
	}
	other, err := os.ReadFile(filepath.Join(context.Roots["claude"], "ponytail", "SKILL.md"))
	if err != nil || string(other) != "updated" {
		t.Fatalf("recovery mutated before checking all divergences: %q %v", other, err)
	}
	entries, err := os.ReadDir(filepath.Join(context.StateRoot, "transactions"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("blocked recovery lost journal: %v %v", entries, err)
	}
}

func TestGlobalRecoveryRejectsTamperedJournalPaths(t *testing.T) {
	context, _ := globalInterruptedFixture(t)
	transactionsRoot := filepath.Join(context.StateRoot, "transactions")
	entries, err := os.ReadDir(transactionsRoot)
	if err != nil || len(entries) != 1 {
		t.Fatalf("missing journal: %v %v", entries, err)
	}
	journalPath := filepath.Join(transactionsRoot, entries[0].Name(), "journal.json")
	content, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	var journal map[string]any
	if err := json.Unmarshal(content, &journal); err != nil {
		t.Fatal(err)
	}
	changes := journal["changes"].([]any)
	changes[0].(map[string]any)["relative_path"] = "../escape"
	tampered, err := json.Marshal(journal)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(journalPath, tampered, 0600); err != nil {
		t.Fatal(err)
	}
	if err := RecoverGlobalTransactions(context); err == nil {
		t.Fatal("tampered journal path accepted")
	}
}

func TestGlobalMutationRecoversBeforePlanning(t *testing.T) {
	context, before := globalInterruptedFixture(t)
	after, writes := globalTestInstall(context, []string{"agents", "claude"}, []byte("latest"))
	after.Installations[0].ResolvedRevision = strings.Repeat("c", 40)
	if _, err := ApplyGlobalChange(context, before, after, writes, GlobalMutationOptions{}); err != nil {
		t.Fatalf("next mutation did not recover: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(context.Roots["agents"], "ponytail", "SKILL.md"))
	if err != nil || string(content) != "latest" {
		t.Fatalf("next mutation did not publish: %q %v", content, err)
	}
}

func TestGlobalMutationIgnoresUntouchedInstallations(t *testing.T) {
	context := globalTestContext(t)
	state, writes := globalTestInstall(context, []string{"agents", "claude"}, []byte("original"))
	other := state.Installations[0]
	other.ID = "github:github.com/example/other#skills/other"
	other.SkillName = "other"
	other.Source = GlobalSource{Provider: "github", Host: "github.com", Repository: "example/other", Subdir: "skills/other"}
	other.Placements = []GlobalPlacement{{RootID: "agents", RelativePath: "other", ResolvedRoot: context.Roots["agents"], Consumers: []string{"agents"}, Files: []GlobalFile{{RootID: "agents", RelativePath: "other/SKILL.md", SHA256: ComputeSHA256([]byte("other")), Mode: 0644}}}}
	state.Installations = append(state.Installations, other)
	writes = append(writes, GlobalWrite{RootID: "agents", RelativePath: "other/SKILL.md", Data: []byte("other"), Mode: 0644})
	globalApplyFixture(t, context, state, writes)
	otherPath := filepath.Join(context.Roots["agents"], "other", "SKILL.md")
	if err := os.WriteFile(otherPath, []byte("local edits"), 0644); err != nil {
		t.Fatal(err)
	}
	after := GlobalState{SchemaVersion: 1, Installations: []GlobalInstallation{other}}
	if _, err := ApplyGlobalChange(context, state, after, nil, GlobalMutationOptions{}); err != nil {
		t.Fatalf("unrelated modification blocked remove: %v", err)
	}
	content, err := os.ReadFile(otherPath)
	if err != nil || string(content) != "local edits" {
		t.Fatalf("unrelated edits changed: %q %v", content, err)
	}
}

func TestGlobalRecoveryRejectsTrailingJournalJSON(t *testing.T) {
	context, _ := globalInterruptedFixture(t)
	root := filepath.Join(context.StateRoot, "transactions")
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 {
		t.Fatalf("missing journal: %v %v", entries, err)
	}
	journalPath := filepath.Join(root, entries[0].Name(), "journal.json")
	file, err := os.OpenFile(journalPath, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("{}"); err != nil {
		t.Fatal(err)
	}
	file.Close()
	if err := RecoverGlobalTransactions(context); err == nil {
		t.Fatal("trailing journal JSON accepted")
	}
}
