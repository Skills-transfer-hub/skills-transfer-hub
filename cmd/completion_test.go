package cmd

import (
	"strings"
	"testing"
)

func TestCompletion_BashIncludesAllVisibleCommands(t *testing.T) {
	res := runCLI(t, "", "", "completion", "bash")
	if res.err != nil {
		t.Fatalf("err = %v", res.err)
	}
	if !strings.Contains(res.stdout, "complete -F _sth_completion sth") {
		t.Fatalf("missing complete invocation: %q", res.stdout)
	}
	for _, c := range visibleCommands() {
		if !strings.Contains(res.stdout, c.name) {
			t.Fatalf("missing command %q in bash completion", c.name)
		}
	}
}

func TestCompletion_BashFlagsForList(t *testing.T) {
	res := runCLI(t, "", "", "completion", "bash")
	if res.err != nil {
		t.Fatalf("err = %v", res.err)
	}
	if !strings.Contains(res.stdout, "list)") {
		t.Fatalf("missing 'list)' branch: %q", res.stdout)
	}
	if !strings.Contains(res.stdout, "--json") {
		t.Fatalf("missing --json flag: %q", res.stdout)
	}
}

func TestCompletion_ZshIncludesCompdef(t *testing.T) {
	res := runCLI(t, "", "", "completion", "zsh")
	if res.err != nil {
		t.Fatalf("err = %v", res.err)
	}
	if !strings.Contains(res.stdout, "#compdef sth") {
		t.Fatalf("missing #compdef header: %q", res.stdout)
	}
	if !strings.Contains(res.stdout, "compdef _sth sth") {
		t.Fatalf("missing compdef registration: %q", res.stdout)
	}
}

func TestCompletion_PowerShellRegistersArgumentCompleter(t *testing.T) {
	res := runCLI(t, "", "", "completion", "powershell")
	if res.err != nil {
		t.Fatalf("err = %v", res.err)
	}
	if !strings.Contains(res.stdout, "Register-ArgumentCompleter") {
		t.Fatalf("missing Register-ArgumentCompleter: %q", res.stdout)
	}
}

func TestCompletion_UnsupportedShellExitsNonZero(t *testing.T) {
	res := runCLI(t, "", "", "completion", "fish")
	if res.err == nil {
		t.Fatal("expected error for unsupported shell")
	}
	if !strings.Contains(res.stderr, "unsupported shell") {
		t.Fatalf("missing unsupported-shell message: %q", res.stderr)
	}
}

func TestCompletion_DoesNotIncludeStockholm(t *testing.T) {
	res := runCLI(t, "", "", "completion", "bash")
	if res.err != nil {
		t.Fatalf("err = %v", res.err)
	}
	// stockholm is not in commandCatalog → must not appear.
	if strings.Contains(res.stdout, " stockholm ") || strings.Contains(res.stdout, "\"stockholm\"") {
		t.Fatalf("stockholm leaked into bash completion: %q", res.stdout)
	}
}

func TestCompletion_RequiresArgument(t *testing.T) {
	res := runCLI(t, "", "", "completion")
	if res.err == nil {
		t.Fatal("expected error when shell arg missing")
	}
}
