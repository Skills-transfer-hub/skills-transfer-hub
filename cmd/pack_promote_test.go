package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"sth/internal/fs"
)

func TestRunPackPromoteDryRunAndConfirmation(t *testing.T) {
	root := commandPromotionWorktree(t)
	item, err := fs.SaveLocalLibraryItem(root, fs.LocalLibrarySaveInput{Kind: "prompt", Title: "Review PR", Body: "Review the diff"})
	if err != nil {
		t.Fatal(err)
	}
	dryRun := runCLI(t, root, t.TempDir(), "pack", "promote", item.ID, "--dry-run")
	if dryRun.err != nil {
		t.Fatalf("dry-run: %v\nstderr=%s", dryRun.err, dryRun.stderr)
	}
	assertContains(t, dryRun.stdout, "skills/review-pr/SKILL.md")
	assertContains(t, dryRun.stdout, "No files written")
	if _, err := os.Lstat(filepath.Join(root, "skills")); !os.IsNotExist(err) {
		t.Fatalf("dry-run created skills: %v", err)
	}

	unconfirmed := runCLI(t, root, t.TempDir(), "pack", "promote", item.ID)
	if unconfirmed.err == nil || !strings.Contains(unconfirmed.err.Error(), "canceled") {
		t.Fatalf("unconfirmed promotion=%v", unconfirmed.err)
	}
	if _, err := os.Lstat(filepath.Join(root, "skills")); !os.IsNotExist(err) {
		t.Fatalf("unconfirmed promotion created skills: %v", err)
	}

	confirmed := runCLI(t, root, t.TempDir(), "pack", "promote", item.ID, "--yes")
	if confirmed.err != nil {
		t.Fatalf("confirmed promotion: %v\nstderr=%s", confirmed.err, confirmed.stderr)
	}
	assertContains(t, confirmed.stdout, "did not commit or push")
	assertContains(t, confirmed.stdout, "pull request")
	if _, err := os.Stat(filepath.Join(root, "skills", "review-pr", "references", "content.md")); err != nil {
		t.Fatalf("reference not materialized: %v", err)
	}
}

func TestRunBridgePromotionRequiresConfirmationAndKeepsPreviewReadOnly(t *testing.T) {
	root := commandPromotionWorktree(t)
	item, err := fs.SaveLocalLibraryItem(root, fs.LocalLibrarySaveInput{Kind: "artifact", Title: "Shared Result", Body: "Evidence"})
	if err != nil {
		t.Fatal(err)
	}
	input := strings.Join([]string{
		fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"promote","params":{"library_id":%q}}`, item.ID),
		fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"promote.preview","params":{"library_id":%q}}`, item.ID),
	}, "\n") + "\n"
	result := runCLIWithInput(t, root, t.TempDir(), input, "bridge", "--stdio")
	if result.err != nil {
		t.Fatal(result.err)
	}
	responses := decodeBridgeTestResponses(t, result.stdout)
	if errorValue := responses[0]["error"].(map[string]any); errorValue["code"] != float64(-32602) {
		t.Fatalf("unconfirmed error=%v", errorValue)
	}
	preview := responses[1]["result"].(map[string]any)
	if preview["output"] != "skills/shared-result" || len(preview["files"].([]any)) != 2 {
		t.Fatalf("preview=%v", preview)
	}
	if strings.Contains(result.stdout, "Evidence") {
		t.Fatal("bridge preview exposed captured content")
	}
	if _, err := os.Lstat(filepath.Join(root, "skills")); !os.IsNotExist(err) {
		t.Fatalf("bridge preview mutated worktree: %v", err)
	}

	mutation := fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"promote","params":{"library_id":%q,"confirmed":true}}`, item.ID) + "\n"
	result = runCLIWithInput(t, root, t.TempDir(), mutation, "bridge", "--stdio")
	if result.err != nil {
		t.Fatal(result.err)
	}
	promoted := decodeBridgeTestResponses(t, result.stdout)[0]["result"].(map[string]any)
	if promoted["promoted"] != true || !strings.Contains(promoted["guidance"].(string), "pull request") {
		t.Fatalf("promoted=%v", promoted)
	}
}

func commandPromotionWorktree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	command := exec.Command("git", "init", "--quiet", root)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v (%s)", err, output)
	}
	return root
}
