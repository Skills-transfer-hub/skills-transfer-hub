package http

import (
	"context"
	"encoding/json"
	"fmt"
	stdhttp "net/http"
	"reflect"
	"strings"
	"testing"
)

func globalSkillText(name string) []byte {
	return []byte("---\nname: " + name + "\ndescription: A useful skill\n---\nUse it.\n")
}

func globalRepositoryClient(t *testing.T, paths map[string]string, bodies map[string][]byte, truncated bool) (*Client, GlobalResolution, *[]string) {
	t.Helper()
	requests := []string{}
	client := globalTestClient(t, func(response stdhttp.ResponseWriter, request *stdhttp.Request) {
		requests = append(requests, request.URL.Path)
		switch {
		case request.URL.Path == "/repos/owner/repo/commits/main":
			json.NewEncoder(response).Encode(map[string]string{"sha": globalTestRevision})
		case request.URL.Path == "/repos/owner/repo/git/trees/"+globalTestRevision:
			entries := []map[string]any{}
			for filePath, mode := range paths {
				entryType := "blob"
				if mode == "160000" {
					entryType = "commit"
				}
				if mode == "040000" {
					entryType = "tree"
				}
				entries = append(entries, map[string]any{"path": filePath, "mode": mode, "type": entryType, "size": len(bodies[filePath])})
			}
			json.NewEncoder(response).Encode(map[string]any{"tree": entries, "truncated": truncated})
		case strings.HasPrefix(request.URL.Path, "/raw/owner/repo/"+globalTestRevision+"/"):
			filePath := strings.TrimPrefix(request.URL.Path, "/raw/owner/repo/"+globalTestRevision+"/")
			body, exists := bodies[filePath]
			if !exists {
				t.Errorf("unexpected file download %q", filePath)
				response.WriteHeader(404)
				return
			}
			response.Write(body)
		default:
			t.Errorf("request not pinned to revision: %s", request.URL.Path)
			response.WriteHeader(404)
		}
	})
	source, _ := ParseGlobalSource("github:owner/repo")
	resolution, err := client.ResolveGlobalSource(context.Background(), source, "main")
	if err != nil {
		t.Fatal(err)
	}
	return client, resolution, &requests
}

func TestGlobalDiscoveryPrefersCanonicalSkillsAndHonorsExplicitPath(t *testing.T) {
	paths := map[string]string{".openclaw/skills/ponytail/SKILL.md": "100644", "skills/plugin.json": "100644"}
	bodies := map[string][]byte{".openclaw/skills/ponytail/SKILL.md": globalSkillText("ponytail")}
	for _, name := range []string{"ponytail", "review", "audit", "plan", "fix", "test"} {
		paths["skills/"+name+"/SKILL.md"] = "100644"
		bodies["skills/"+name+"/SKILL.md"] = globalSkillText(name)
	}
	client, resolution, _ := globalRepositoryClient(t, paths, bodies, false)
	candidates, err := client.DiscoverGlobalSkills(context.Background(), resolution, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 6 {
		t.Fatalf("wanted exactly six canonical skills: %#v", candidates)
	}
	selected, err := SelectGlobalSkills(candidates, []string{"ponytail"})
	if err != nil || len(selected) != 1 || selected[0].Path != "skills/ponytail" {
		t.Fatalf("wrong Ponytail selection: %#v, %v", selected, err)
	}
	all, err := SelectGlobalSkills(candidates, []string{"*"})
	if err != nil || len(all) != 6 {
		t.Fatalf("wrong wildcard selection: %#v, %v", all, err)
	}
	candidates, err = client.DiscoverGlobalSkills(context.Background(), resolution, ".openclaw/skills/ponytail")
	if err != nil || len(candidates) != 1 || candidates[0].Path != ".openclaw/skills/ponytail" {
		t.Fatalf("explicit scope not honored: %#v %v", candidates, err)
	}
}

func TestGlobalSelectionNeverResolvesAmbiguityArbitrarily(t *testing.T) {
	candidates := []GlobalCandidate{{Name: "review", Path: "skills/review"}, {Name: "review", Path: "other/review"}, {Name: "audit", Path: "skills/audit"}}
	for _, names := range [][]string{nil, {"review"}, {"*"}, {"missing"}, {"*", "audit"}} {
		if _, err := SelectGlobalSkills(candidates, names); err == nil {
			t.Fatalf("accepted ambiguous/invalid selection: %v", names)
		}
	}
	selected, err := SelectGlobalSkills(candidates, []string{"audit", "audit"})
	if err != nil || !reflect.DeepEqual(selected, []GlobalCandidate{candidates[2]}) {
		t.Fatalf("unique selection failed %#v %v", selected, err)
	}
	if _, err := SelectGlobalSkills(nil, nil); err == nil {
		t.Fatal("empty discovery accepted")
	}
}

func TestGlobalDiscoveryRejectsIncompleteAndUnsafeTrees(t *testing.T) {
	for _, testCase := range []struct {
		name, path, mode string
		truncated        bool
	}{
		{name: "truncated", path: "skills/valid/SKILL.md", mode: "100644", truncated: true},
		{name: "root", path: "SKILL.md", mode: "100644"},
		{name: "traversal", path: "skills/../valid/SKILL.md", mode: "100644"},
		{name: "backslash", path: `skills\valid\SKILL.md`, mode: "100644"},
		{name: "absolute", path: "/skills/valid/SKILL.md", mode: "100644"},
		{name: "symlink entrypoint", path: "skills/valid/SKILL.md", mode: "120000"},
		{name: "reserved", path: "skills/CON/SKILL.md", mode: "100644"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			client, resolution, _ := globalRepositoryClient(t, map[string]string{testCase.path: testCase.mode}, map[string][]byte{testCase.path: globalSkillText("valid")}, testCase.truncated)
			if _, err := client.DiscoverGlobalSkills(context.Background(), resolution, ""); err == nil {
				t.Fatal("unsafe tree accepted")
			}
		})
	}
}

func TestGlobalDiscoveryRejectsCaseCollisionsAndUnsafeScope(t *testing.T) {
	paths := map[string]string{"skills/valid/SKILL.md": "100644", "skills/VALID/SKILL.md": "100644"}
	bodies := map[string][]byte{"skills/valid/SKILL.md": globalSkillText("valid"), "skills/VALID/SKILL.md": globalSkillText("valid")}
	client, resolution, _ := globalRepositoryClient(t, paths, bodies, false)
	if _, err := client.DiscoverGlobalSkills(context.Background(), resolution, ""); err == nil {
		t.Fatal("case collision accepted")
	}
	for _, scope := range []string{"../skills", "/skills", `skills\valid`, ".", "skills/../skills"} {
		if _, err := client.DiscoverGlobalSkills(context.Background(), resolution, scope); err == nil {
			t.Errorf("accepted scope %q", scope)
		}
	}
}

func TestGlobalDiscoveryUsesCatalogAndValidatesMetadata(t *testing.T) {
	paths := map[string]string{"SKILLS.md": "100644", "custom/review/SKILL.md": "100644", "skills/other/SKILL.md": "100644"}
	bodies := map[string][]byte{"SKILLS.md": []byte("| Type | Name | Folder | Source |\n| --- | --- | --- | --- |\n| bundle | review | custom | custom/review/SKILL.md |\n"), "custom/review/SKILL.md": globalSkillText("review")}
	client, resolution, _ := globalRepositoryClient(t, paths, bodies, false)
	candidates, err := client.DiscoverGlobalSkills(context.Background(), resolution, "")
	if err != nil || len(candidates) != 1 || candidates[0].Path != "custom/review" {
		t.Fatalf("catalog was ignored: %#v %v", candidates, err)
	}
	for _, body := range []string{"# no frontmatter", "---\nname: wrong\ndescription: valid\n---\n", "---\nname: valid\n---\n", "---\nname: valid\ndescription: ready\nhooks:\n  SessionStart: run.sh\n---\n"} {
		t.Run(fmt.Sprint(len(body)), func(t *testing.T) {
			client, resolution, _ := globalRepositoryClient(t, map[string]string{"skills/valid/SKILL.md": "100644"}, map[string][]byte{"skills/valid/SKILL.md": []byte(body)}, false)
			if _, err := client.DiscoverGlobalSkills(context.Background(), resolution, ""); err == nil {
				t.Fatalf("invalid metadata accepted: %s", body)
			}
		})
	}
}
