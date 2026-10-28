package fs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func globalTestContext(t *testing.T) GlobalContext {
	t.Helper()
	home := t.TempDir()
	t.Setenv("STH_USER_CONFIG_DIR", filepath.Join(home, "config"))
	context, err := NewGlobalContext(home, map[string]string{
		"agents": filepath.Join(home, ".agents", "skills"),
		"claude": filepath.Join(home, ".claude", "skills"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return context
}

func TestGlobalStateAbsentIsReadOnly(t *testing.T) {
	context := globalTestContext(t)
	state, err := ReadGlobalState(context)
	if err != nil || state.SchemaVersion != 1 || len(state.Installations) != 0 {
		t.Fatalf("state=%+v err=%v", state, err)
	}
	if context.StateRoot != filepath.Join(context.Home, "config", "sth", "global") {
		t.Fatalf("unexpected state root %s", context.StateRoot)
	}
	entries, err := os.ReadDir(context.Home)
	if err != nil || len(entries) != 0 {
		t.Fatalf("read created persistent files: %v %v", entries, err)
	}
}

func TestGlobalStateRejectsInvalidDocuments(t *testing.T) {
	for _, document := range []string{"", "{", `{"schema_version":2,"installations":[]}`, `{"schema_version":1,"installations":null,"unexpected":true}`, "{\"schema_version\":1,\"installations\":[{\"id\":\"\xff\"}]}"} {
		t.Run(document, func(t *testing.T) {
			context := globalTestContext(t)
			if err := os.MkdirAll(context.StateRoot, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(context.StateRoot, "state.json"), []byte(document), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := ReadGlobalState(context); err == nil {
				t.Fatalf("invalid state accepted: %s", document)
			}
		})
	}
}

func TestGlobalContextRejectsBroadAndEscapingRoots(t *testing.T) {
	home := t.TempDir()
	for _, root := range []string{home, filepath.Dir(home), filepath.Join(home, ".agents", ".."), "relative"} {
		if _, err := NewGlobalContext(home, map[string]string{"agents": root}); err == nil {
			t.Fatalf("unsafe root accepted: %s", root)
		}
	}
}

func TestGlobalStateRejectsSymlinkedStateDirectory(t *testing.T) {
	context := globalTestContext(t)
	if err := os.MkdirAll(filepath.Dir(context.StateRoot), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), context.StateRoot); err != nil {
		t.Skip(err)
	}
	if _, err := ReadGlobalState(context); err == nil || !strings.Contains(err.Error(), "link") {
		t.Fatalf("symlinked state directory accepted: %v", err)
	}
}

func TestGlobalContextCanonicalizesIndependentConfigBase(t *testing.T) {
	home := t.TempDir()
	configBase := t.TempDir()
	alias := filepath.Join(t.TempDir(), "config-base")
	if err := os.Symlink(configBase, alias); err != nil {
		t.Skip(err)
	}
	t.Setenv("STH_USER_CONFIG_DIR", alias)
	context, err := NewGlobalContext(home, map[string]string{"agents": filepath.Join(home, ".agents", "skills")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReadGlobalState(context); err != nil {
		t.Fatalf("trusted config base alias rejected: %v", err)
	}
}

func TestGlobalContextKeepsPrivateStateOutsideAgentRoots(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".agents", "skills")
	t.Setenv("STH_USER_CONFIG_DIR", root)
	if _, err := NewGlobalContext(home, map[string]string{"agents": root}); err == nil {
		t.Fatal("private backup directory inside agent discovery root accepted")
	}
}
