package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun_initCreatesDefaultConfig(t *testing.T) {
	homeDir := t.TempDir()

	result := runCLI(t, "", homeDir, "init")

	if result.err != nil {
		t.Fatalf("Run() error = %v", result.err)
	}
	if !strings.Contains(result.stdout, "Fichier de configuration créé") {
		t.Fatalf("stdout %q does not contain success message", result.stdout)
	}

	configPath := filepath.Join(homeDir, ".loreskillsrc")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", configPath, err)
	}

	var config map[string]string
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("json.Unmarshal(): %v", err)
	}
	if config["repo_raw_url"] == "" {
		t.Fatal("repo_raw_url is empty")
	}
	if config["target_dir"] == "" {
		t.Fatal("target_dir is empty")
	}
}

func TestRun_initRefusesExistingConfig(t *testing.T) {
	homeDir := t.TempDir()
	configPath := filepath.Join(homeDir, ".loreskillsrc")
	original := []byte("{\"repo_raw_url\":\"https://example.invalid\",\"target_dir\":\".claude/skills\"}\n")
	if err := os.WriteFile(configPath, original, 0o600); err != nil {
		t.Fatalf("WriteFile(%s): %v", configPath, err)
	}

	result := runCLI(t, "", homeDir, "init")

	if result.err == nil {
		t.Fatal("Run() error = nil, want error")
	}
	if !strings.Contains(result.stderr, "configuration existe déjà") {
		t.Fatalf("stderr %q does not contain overwrite refusal", result.stderr)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", configPath, err)
	}
	if string(data) != string(original) {
		t.Fatalf("config was modified:\n got: %q\nwant: %q", data, original)
	}
}
