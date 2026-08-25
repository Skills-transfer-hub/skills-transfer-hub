package writer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodexAgentRenderingProducesRequiredTOMLFields(t *testing.T) {
	installedPath := filepath.Join(t.TempDir(), ".codex", "agents", "reviewer.toml")
	source := []byte("---\nname: ignored\ndescription: Review correctness and security\n---\nInspect the change.\nReturn evidence.\n")

	result, err := (CodexWriter{}).Write(WriteRequest{InstalledPath: installedPath, Content: source, AddManagedHeader: true})
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(installedPath)
	if err != nil {
		t.Fatal(err)
	}
	expectedParts := []string{
		codexManagedTOMLMarker,
		`name = "reviewer"`,
		`description = "Review correctness and security"`,
		`developer_instructions = "Inspect the change.\nReturn evidence."`,
	}
	for _, expectedPart := range expectedParts {
		if !strings.Contains(string(content), expectedPart) {
			t.Errorf("rendered agent does not contain %q: %s", expectedPart, content)
		}
	}
	if result.SHA256 != computeSHA256(content) {
		t.Fatalf("SHA-256 = %s, want %s", result.SHA256, computeSHA256(content))
	}
}

func TestCodexCommandRenderingProducesValidSkillMetadata(t *testing.T) {
	installedPath := filepath.Join(t.TempDir(), ".agents", "skills", "deploy", "SKILL.md")
	source := []byte("---\ndescription: Deploy safely\nargument-hint: '[environment]'\n---\nDeploy the requested environment.\n")

	_, err := (CodexWriter{}).Write(WriteRequest{InstalledPath: installedPath, Content: source, ArtifactKind: "command", ArtifactName: "deploy", AddManagedHeader: true})
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(installedPath)
	if err != nil {
		t.Fatal(err)
	}
	expectedParts := []string{
		"---\n",
		`name: "deploy"`,
		`description: "Deploy safely"`,
		minimalManagedMarker,
		"Deploy the requested environment.",
	}
	for _, expectedPart := range expectedParts {
		if !strings.Contains(string(content), expectedPart) {
			t.Errorf("rendered skill does not contain %q: %s", expectedPart, content)
		}
	}
}

func TestCodexCommandRenderingReplacesCommandFrontmatter(t *testing.T) {
	installedPath := filepath.Join(t.TempDir(), ".agents", "skills", "deploy", "SKILL.md")
	source := []byte("---\nname: legacy-command\ndescription: Deploy safely\nargument-hint: '[environment]'\n---\nDeploy now.\n")

	rendered, err := (CodexWriter{}).Render(WriteRequest{InstalledPath: installedPath, Content: source, ArtifactKind: "command", ArtifactName: "deploy", AddManagedHeader: true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rendered), "legacy-command") || strings.Contains(string(rendered), "argument-hint") {
		t.Fatalf("command frontmatter was not normalized: %s", rendered)
	}
	if !strings.Contains(string(rendered), `name: "deploy"`) {
		t.Fatalf("normalized command has no canonical name: %s", rendered)
	}
}

func TestCodexNativeSkillWithRequiredMetadataIsPreserved(t *testing.T) {
	installedPath := filepath.Join(t.TempDir(), ".agents", "skills", "review", "SKILL.md")
	source := []byte("---\nname: review\ndescription: Review code\n---\nReview the change.\n")

	rendered, err := (CodexWriter{}).Render(WriteRequest{InstalledPath: installedPath, Content: source, AddManagedHeader: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(rendered), "---\nname: review\ndescription: Review code\n---\n"+minimalManagedMarker) {
		t.Fatalf("native skill metadata changed: %s", rendered)
	}
}

func TestCodexSkillRejectsNameDifferentFromDirectory(t *testing.T) {
	installedPath := filepath.Join(t.TempDir(), ".agents", "skills", "review", "SKILL.md")
	source := []byte("---\nname: other\ndescription: Review code\n---\nReview the change.\n")

	_, err := (CodexWriter{}).Render(WriteRequest{InstalledPath: installedPath, Content: source, AddManagedHeader: true})
	if err == nil || !strings.Contains(err.Error(), "differs from directory") {
		t.Fatalf("mismatched skill name error = %v", err)
	}
}

func TestCodexSkillRejectsInvalidDirectoryName(t *testing.T) {
	installedPath := filepath.Join(t.TempDir(), ".agents", "skills", "Invalid_Name", "SKILL.md")
	source := []byte("---\ndescription: Review code\n---\nReview the change.\n")

	_, err := (CodexWriter{}).Render(WriteRequest{InstalledPath: installedPath, Content: source, AddManagedHeader: true})
	if err == nil || !strings.Contains(err.Error(), "invalid Codex skill name") {
		t.Fatalf("invalid skill name error = %v", err)
	}
}

func TestCodexTOMLRenderingRejectsInvalidUTF8(t *testing.T) {
	_, err := (CodexWriter{}).Render(WriteRequest{
		InstalledPath: filepath.Join(t.TempDir(), ".codex", "agents", "reviewer.toml"),
		Content:       []byte{0xff, 0xfe},
	})
	if err == nil {
		t.Fatal("invalid UTF-8 should fail")
	}
}
