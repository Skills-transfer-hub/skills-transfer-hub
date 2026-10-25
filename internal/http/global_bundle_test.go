package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	stdhttp "net/http"
	"strings"
	"testing"
)

func TestGlobalBundlePreservesBinaryExecutableAndLicenseAtOneRevision(t *testing.T) {
	paths := map[string]string{
		"skills/ponytail/SKILL.md":                "100644",
		"skills/ponytail/scripts/run.sh":          "100755",
		"skills/ponytail/assets/picture.bin":      "100644",
		"skills/ponytail/references/hash#name.md": "100644",
		"LICENSE":                            "100644",
		".openclaw/skills/ponytail/SKILL.md": "100644",
		"skills/plugin.json":                 "100644",
	}
	bodies := map[string][]byte{
		"skills/ponytail/SKILL.md":                globalSkillText("ponytail"),
		"skills/ponytail/scripts/run.sh":          []byte("#!/bin/sh\nprintf should-not-run\n"),
		"skills/ponytail/assets/picture.bin":      {0, 1, 255, 128, 13, 10, 0},
		"skills/ponytail/references/hash#name.md": []byte("reference"),
		"LICENSE": []byte("License remains outside skill"),
	}
	client, resolution, requests := globalRepositoryClient(t, paths, bodies, false)
	candidates, err := client.DiscoverGlobalSkills(context.Background(), resolution, "")
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := client.DownloadGlobalSkill(context.Background(), resolution, candidates[0])
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Name != "ponytail" || bundle.Path != "skills/ponytail" || bundle.Revision != globalTestRevision || len(bundle.Files) != 4 {
		t.Fatalf("wrong bundle: %#v", bundle)
	}
	for _, file := range bundle.Files {
		if !bytes.Equal(file.Content, bodies["skills/ponytail/"+file.Path]) {
			t.Errorf("content changed for %s", file.Path)
		}
		expectedMode := uint32(0o644)
		if file.Path == "scripts/run.sh" {
			expectedMode = 0o755
		}
		if file.Mode != expectedMode {
			t.Errorf("mode changed for %s: %o", file.Path, file.Mode)
		}
	}
	if len(bundle.Provenance) != 1 || bundle.Provenance[0].Path != "LICENSE" || !bytes.Equal(bundle.Provenance[0].Content, bodies["LICENSE"]) {
		t.Fatalf("lost license %#v", bundle.Provenance)
	}
	if len(bundle.Warnings) == 0 {
		t.Fatal("excluded alternative not reported")
	}
	treeRequests := 0
	for _, request := range *requests {
		if strings.Contains(request, "/git/trees/") {
			treeRequests++
		}
	}
	if treeRequests != 1 {
		t.Fatalf("tree not shared between discovery and download: %v", *requests)
	}
}

func TestGlobalBundleRejectsLinksSubmodulesActivePluginsAndOversizedFiles(t *testing.T) {
	for _, testCase := range []struct{ name, path, mode string }{
		{name: "symlink", path: "link", mode: "120000"},
		{name: "submodule", path: "module", mode: "160000"},
		{name: "plugin", path: ".claude-plugin/plugin.json", mode: "100644"},
		{name: "gemini extension", path: "gemini-extension.json", mode: "100644"},
		{name: "unknown mode", path: "executable", mode: "100777"},
		{name: "case collision", path: "skill.MD", mode: "100644"},
		{name: "reserved filename", path: "NUL.txt", mode: "100644"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			paths := map[string]string{"skills/valid/SKILL.md": "100644", "skills/valid/" + testCase.path: testCase.mode}
			bodies := map[string][]byte{"skills/valid/SKILL.md": globalSkillText("valid"), "skills/valid/" + testCase.path: []byte("payload")}
			client, resolution, _ := globalRepositoryClient(t, paths, bodies, false)
			if _, err := client.DownloadGlobalSkill(context.Background(), resolution, GlobalCandidate{Name: "valid", Path: "skills/valid"}); err == nil {
				t.Fatal("unsafe bundle accepted")
			}
		})
	}
}

func TestGlobalBundleEnforcesCountAndSizeBeforeDownloading(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		count    int
		fileSize int64
	}{
		{name: "file count", count: 4097, fileSize: 1},
		{name: "response size", count: 2, fileSize: 17 << 20},
		{name: "total size", count: 10, fileSize: 8 << 20},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			client := globalTestClient(t, func(response stdhttp.ResponseWriter, request *stdhttp.Request) {
				if strings.Contains(request.URL.Path, "/commits/") {
					json.NewEncoder(response).Encode(map[string]string{"sha": globalTestRevision})
					return
				}
				if strings.Contains(request.URL.Path, "/git/trees/") {
					entries := []map[string]any{{"path": "skills/valid/SKILL.md", "type": "blob", "mode": "100644", "size": 1}}
					for index := 1; index < testCase.count; index++ {
						entries = append(entries, map[string]any{"path": fmt.Sprintf("skills/valid/file-%d", index), "type": "blob", "mode": "100644", "size": testCase.fileSize})
					}
					json.NewEncoder(response).Encode(map[string]any{"tree": entries})
					return
				}
				t.Errorf("download occurred before bounds validated: %s", request.URL.Path)
				response.WriteHeader(500)
			})
			source, _ := ParseGlobalSource("github:owner/repo")
			resolution, err := client.ResolveGlobalSource(context.Background(), source, "main")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := client.DownloadGlobalSkill(context.Background(), resolution, GlobalCandidate{Name: "valid", Path: "skills/valid"}); err == nil {
				t.Fatal("oversized bundle accepted")
			}
		})
	}
}

func TestGlobalBundlePreservesExtensionMetadataWithWarnings(t *testing.T) {
	body := []byte("---\nname: valid\ndescription: works\nallowed-tools: Read, Bash\nuser-invocable: true\nunknown-host-field: kept\n---\nbody\n")
	client, resolution, _ := globalRepositoryClient(t, map[string]string{"skills/valid/SKILL.md": "100644"}, map[string][]byte{"skills/valid/SKILL.md": body}, false)
	bundle, err := client.DownloadGlobalSkill(context.Background(), resolution, GlobalCandidate{Name: "valid", Path: "skills/valid"})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Warnings) < 2 || !bytes.Equal(bundle.Files[0].Content, body) {
		t.Fatalf("metadata rewritten or unreported %#v", bundle)
	}
}

func TestGlobalBundleRejectsMutatedResolutionAfterDiscovery(t *testing.T) {
	client, resolution, _ := globalRepositoryClient(t, map[string]string{"skills/valid/SKILL.md": "100644"}, map[string][]byte{"skills/valid/SKILL.md": globalSkillText("valid")}, false)
	candidates, err := client.DiscoverGlobalSkills(context.Background(), resolution, "")
	if err != nil {
		t.Fatal(err)
	}
	resolution.Revision = "main"
	if _, err := client.DownloadGlobalSkill(context.Background(), resolution, candidates[0]); err == nil {
		t.Fatal("cached tree allowed mutable ref download")
	}
}

func TestGlobalBundleRejectsNonDirectorySourceAncestors(t *testing.T) {
	for _, mode := range []string{"120000", "160000", "100644"} {
		client, resolution, _ := globalRepositoryClient(t, map[string]string{"skills/valid": "" + mode, "skills/valid/SKILL.md": "100644"}, map[string][]byte{"skills/valid/SKILL.md": globalSkillText("valid")}, false)
		if _, err := client.DownloadGlobalSkill(context.Background(), resolution, GlobalCandidate{Name: "valid", Path: "skills/valid"}); err == nil {
			t.Fatalf("non-directory source ancestor mode %s accepted", mode)
		}
	}
}

func TestGlobalBundleRejectsUnicodeEquivalentPathsAndWindowsDeviceVariants(t *testing.T) {
	for _, paths := range []map[string]string{
		{"skills/valid/SKILL.md": "100644", "skills/valid/caf\u00e9.txt": "100644", "skills/valid/cafe\u0301.txt": "100644"},
		{"skills/valid/SKILL.md": "100644", "skills/valid/COM\u00b9.txt": "100644"},
	} {
		client, resolution, _ := globalRepositoryClient(t, paths, map[string][]byte{"skills/valid/SKILL.md": globalSkillText("valid")}, false)
		if _, err := client.DownloadGlobalSkill(context.Background(), resolution, GlobalCandidate{Name: "valid", Path: "skills/valid"}); err == nil {
			t.Fatal("platform-unsafe paths accepted")
		}
	}
}

func TestGlobalBundleReportsUnknownFrontmatterExtensions(t *testing.T) {
	body := []byte("---\r\nname: valid\r\ndescription: works\r\nfuture-agent-setting: true\r\n---\r\nbody\r\n")
	client, resolution, _ := globalRepositoryClient(t, map[string]string{"skills/valid/SKILL.md": "100644"}, map[string][]byte{"skills/valid/SKILL.md": body}, false)
	bundle, err := client.DownloadGlobalSkill(context.Background(), resolution, GlobalCandidate{Name: "valid", Path: "skills/valid"})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Warnings) != 1 || !strings.Contains(bundle.Warnings[0], "future-agent-setting") || !bytes.Equal(bundle.Files[0].Content, body) {
		t.Fatalf("unknown field not reported unchanged: %#v", bundle)
	}
}
