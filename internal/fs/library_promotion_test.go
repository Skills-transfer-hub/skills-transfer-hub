package fs

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLocalLibraryPromotionIsDeterministicAndDryPlanDoesNotMutate(t *testing.T) {
	root := promotionTestWorktree(t)
	item, err := SaveLocalLibraryItem(root, LocalLibrarySaveInput{
		Kind: "prompt", Title: "Review PR", Body: "Review this diff.\r\nKeep evidence.", Tags: []string{"git"},
	})
	if err != nil {
		t.Fatal(err)
	}
	before := promotionTreeSnapshot(t, root)
	first, err := PlanLocalLibraryPromotion(root, item.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := PlanLocalLibraryPromotion(filepath.Join(root, ".sth"), item.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	after := promotionTreeSnapshot(t, root)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("preview mutated worktree\nbefore=%v\nafter=%v", before, after)
	}
	if first.Output != "skills/review-pr" || !reflect.DeepEqual(first.Files, second.Files) {
		t.Fatalf("non-deterministic plan\nfirst=%+v\nsecond=%+v", first, second)
	}
	if len(first.Files) != 2 || first.Files[0].Path != "skills/review-pr/SKILL.md" || first.Files[1].Path != "skills/review-pr/references/content.md" {
		t.Fatalf("unexpected multi-file bundle: %+v", first.Files)
	}
	if bytes.Contains(first.Files[0].Content, []byte("Review this diff")) || !bytes.Contains(first.Files[1].Content, []byte("Review this diff.\nKeep evidence.\n")) {
		t.Fatal("captured body must live only in the reference file")
	}
}

func TestLocalLibraryPromotionAppliesWithoutOverwrite(t *testing.T) {
	root := promotionTestWorktree(t)
	item, err := SaveLocalLibraryItem(root, LocalLibrarySaveInput{Kind: "artifact", Title: "Release Notes", Body: "Reusable result"})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := PlanLocalLibraryPromotion(root, item.ID, "skills/release-notes")
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyLocalLibraryPromotion(plan); err != nil {
		t.Fatal(err)
	}
	for _, file := range plan.Files {
		data, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(file.Path)))
		if readErr != nil || !bytes.Equal(data, file.Content) {
			t.Fatalf("materialized %s mismatch: %v", file.Path, readErr)
		}
	}
	if err := ApplyLocalLibraryPromotion(plan); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("second apply should refuse overwrite, got %v", err)
	}
}

func TestLocalLibraryPromotionRejectsTraversalSymlinkAndNonGitDirectory(t *testing.T) {
	root := promotionTestWorktree(t)
	item, err := SaveLocalLibraryItem(root, LocalLibrarySaveInput{Kind: "prompt", Title: "Safe Skill", Body: "Safe body"})
	if err != nil {
		t.Fatal(err)
	}
	for _, output := range []string{"../outside", "/tmp/outside", "skills/../outside", "skills/a/b", `skills\\escape`} {
		if _, err := PlanLocalLibraryPromotion(root, item.ID, output); err == nil {
			t.Fatalf("unsafe output %q accepted", output)
		}
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "skills")); err != nil {
		t.Fatal(err)
	}
	if _, err := PlanLocalLibraryPromotion(root, item.ID, "skills/safe-skill"); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("symlink path accepted: %v", err)
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatalf("promotion escaped through symlink: entries=%v err=%v", entries, err)
	}
	if _, err := PlanLocalLibraryPromotion(t.TempDir(), item.ID, ""); err == nil || !strings.Contains(err.Error(), "Git worktree") {
		t.Fatalf("non-Git directory accepted: %v", err)
	}
}

func promotionTestWorktree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	command := exec.Command("git", "init", "--quiet", root)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v (%s)", err, output)
	}
	return root
}

func promotionTreeSnapshot(t *testing.T, root string) []string {
	t.Helper()
	paths := []string{}
	err := filepath.WalkDir(root, func(current string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, current)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return paths
}
