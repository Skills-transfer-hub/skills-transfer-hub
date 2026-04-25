package fs

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSyncManagedBundle_rewritesManifestAndRemovesObsoleteFiles(t *testing.T) {
	paths := newTestPaths(t)
	entry := CatalogEntry{
		CatalogID:    CatalogID("bundle", "react", "form-wizard"),
		ResourceType: "bundle",
		Name:         "form-wizard",
		FolderName:   "react",
		SourcePath:   "react/form-wizard/SKILL.md",
	}

	writeInstalledStateForTest(t, paths.targetDir, InstalledResourceState{Entries: []InstalledResourceRecord{{
		CatalogID:     entry.CatalogID,
		ResourceType:  entry.ResourceType,
		Name:          entry.Name,
		FolderName:    entry.FolderName,
		InstalledPath: ".claude/resources/react/form-wizard",
		SourcePath:    entry.SourcePath,
		ManagedRoot:   ".claude/resources/react/form-wizard",
		SourceRoot:    "react/form-wizard",
		ManagedFiles: []ManagedFileRecord{
			{RelativePath: "SKILL.md", InstalledPath: ".claude/resources/react/form-wizard/SKILL.md", SourcePath: "react/form-wizard/SKILL.md"},
			{RelativePath: "docs/obsolete.md", InstalledPath: ".claude/resources/react/form-wizard/docs/obsolete.md", SourcePath: "react/form-wizard/docs/obsolete.md"},
		},
		Status: "installed",
	}}})

	if err := os.MkdirAll(filepath.Join(paths.projectDir, ".claude", "resources", "react", "form-wizard", "docs"), 0o755); err != nil {
		t.Fatalf("MkdirAll(bundle docs): %v", err)
	}
	if err := os.WriteFile(filepath.Join(paths.projectDir, ".claude", "resources", "react", "form-wizard", "SKILL.md"), []byte("old"), 0o644); err != nil {
		t.Fatalf("WriteFile(SKILL.md): %v", err)
	}
	if err := os.WriteFile(filepath.Join(paths.projectDir, ".claude", "resources", "react", "form-wizard", "docs", "obsolete.md"), []byte("obsolete"), 0o644); err != nil {
		t.Fatalf("WriteFile(obsolete.md): %v", err)
	}

	record, err := SyncManagedBundle(paths.targetDir, entry, map[string][]byte{
		"react/form-wizard/SKILL.md":             []byte("# Updated\n"),
		"react/form-wizard/templates/prompt.txt": []byte("prompt"),
	})
	if err != nil {
		t.Fatalf("SyncManagedBundle() error = %v", err)
	}
	if record.ManagedRoot != ".claude/resources/react/form-wizard" {
		t.Fatalf("record.ManagedRoot = %q, want .claude/resources/react/form-wizard", record.ManagedRoot)
	}
	if len(record.ManagedFiles) != 2 {
		t.Fatalf("len(record.ManagedFiles) = %d, want 2", len(record.ManagedFiles))
	}
	if _, err := os.Stat(filepath.Join(paths.projectDir, ".claude", "resources", "react", "form-wizard", "docs", "obsolete.md")); !os.IsNotExist(err) {
		t.Fatalf("obsolete managed file still exists, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(paths.projectDir, ".claude", "resources", "react", "form-wizard", "templates", "prompt.txt")); err != nil {
		t.Fatalf("Stat(prompt.txt): %v", err)
	}

	state, err := ReadInstalledState(paths.targetDir)
	if err != nil {
		t.Fatalf("ReadInstalledState() error = %v", err)
	}
	if len(state.Entries) != 1 || len(state.Entries[0].ManagedFiles) != 2 {
		t.Fatalf("state.Entries = %+v, want one bundle record with two managed files", state.Entries)
	}
}

func TestUpdateCheckState_roundTripsAndExpiresAfter24Hours(t *testing.T) {
	paths := newTestPaths(t)
	cacheDir := filepath.Join(paths.homeDir, "machine-cache")
	t.Setenv("STH_USER_CACHE_DIR", cacheDir)

	now := time.Date(2026, time.April, 23, 10, 0, 0, 0, time.UTC)
	state := NewAvailableUpdateCheckState(now, "v1.0.0", "v1.2.0", "https://example.invalid/asset", "https://example.invalid/checksum", "sth-windows-amd64.zip")

	cachePath, err := WriteUpdateCheckState(state)
	if err != nil {
		t.Fatalf("WriteUpdateCheckState() error = %v", err)
	}
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("Stat(%s): %v", cachePath, err)
	}

	loaded, err := ReadUpdateCheckState()
	if err != nil {
		t.Fatalf("ReadUpdateCheckState() error = %v", err)
	}
	if loaded == nil {
		t.Fatal("ReadUpdateCheckState() = nil, want cache state")
	}
	if loaded.LatestStableVersion != "v1.2.0" {
		t.Fatalf("loaded.LatestStableVersion = %q, want v1.2.0", loaded.LatestStableVersion)
	}
	if !loaded.IsFresh(now.Add(23*time.Hour), "v1.0.0") {
		t.Fatal("cache should still be fresh before 24 hours")
	}
	if loaded.IsFresh(now.Add(25*time.Hour), "v1.0.0") {
		t.Fatal("cache should be stale after 24 hours")
	}
	if loaded.IsFresh(now.Add(2*time.Hour), "v9.9.9") {
		t.Fatal("cache should be stale when the current binary version changes")
	}

	offer := StableUpdateOfferFromState(*loaded)
	if offer.Status != "fresh-update-available" {
		t.Fatalf("offer.Status = %q, want fresh-update-available", offer.Status)
	}
	if !offer.DownloadReady {
		t.Fatal("offer.DownloadReady = false, want true")
	}
	if offer.LatestStableVersion != "v1.2.0" {
		t.Fatalf("offer.LatestStableVersion = %q, want v1.2.0", offer.LatestStableVersion)
	}
}

func TestSyncManagedBundle_rejectsCaseMismatchedSourcePaths(t *testing.T) {
	paths := newTestPaths(t)
	entry := CatalogEntry{
		CatalogID:    CatalogID("bundle", "react", "form-wizard"),
		ResourceType: "bundle",
		Name:         "form-wizard",
		FolderName:   "react",
		SourcePath:   "react/form-wizard/SKILL.md",
	}

	_, err := SyncManagedBundle(paths.targetDir, entry, map[string][]byte{
		"React/form-wizard/SKILL.md": []byte("# Updated\n"),
	})
	if err == nil {
		t.Fatal("SyncManagedBundle() error = nil, want source path mismatch error")
	}
}