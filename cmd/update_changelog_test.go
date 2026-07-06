package cmd

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

const bundleChangelogFixture = `# Changelog

## [1.3.0] - 2026-07-01

### Added
- Nested field arrays

## [1.2.1] - 2026-06-20

### Fixed
- Validation reset
`

func TestPrintBundleChangelog_ShowsEntriesSincePreviousVersion(t *testing.T) {
	var stdout bytes.Buffer
	application := newApp(nil, &stdout, io.Discard, nil)
	bundleContents := map[string][]byte{
		"skills/react-forms/SKILL.md":     []byte("skill"),
		"skills/react-forms/CHANGELOG.md": []byte(bundleChangelogFixture),
	}

	application.printBundleChangelog("react-forms", bundleContents, "1.2.1")

	output := stdout.String()
	if !strings.Contains(output, "What's new in react-forms:") {
		t.Errorf("header manquant : %q", output)
	}
	if !strings.Contains(output, "## [1.3.0] - 2026-07-01") || !strings.Contains(output, "Nested field arrays") {
		t.Errorf("entrée 1.3.0 manquante : %q", output)
	}
	if strings.Contains(output, "Validation reset") {
		t.Errorf("entrée déjà installée affichée : %q", output)
	}
}

func TestPrintBundleChangelog_SilentWithoutPreviousVersion(t *testing.T) {
	var stdout bytes.Buffer
	application := newApp(nil, &stdout, io.Discard, nil)
	bundleContents := map[string][]byte{
		"skills/react-forms/CHANGELOG.md": []byte(bundleChangelogFixture),
	}

	application.printBundleChangelog("react-forms", bundleContents, "")

	if stdout.Len() != 0 {
		t.Errorf("sortie inattendue sans baseline : %q", stdout.String())
	}
}

func TestPrintBundleChangelog_SilentWithoutChangelogFile(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	application := newApp(nil, &stdout, &stderr, nil)
	bundleContents := map[string][]byte{
		"skills/react-forms/SKILL.md": []byte("skill"),
	}

	application.printBundleChangelog("react-forms", bundleContents, "1.2.1")

	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Errorf("sortie inattendue sans CHANGELOG.md : stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestPrintBundleChangelog_WarnsOnUnparseableChangelog(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	application := newApp(nil, &stdout, &stderr, nil)
	bundleContents := map[string][]byte{
		"skills/react-forms/CHANGELOG.md": []byte("v3 plein de trucs\nv2 moins de trucs\n"),
	}

	application.printBundleChangelog("react-forms", bundleContents, "1.2.1")

	if !strings.Contains(stderr.String(), "Warning: Could not parse changelog for react-forms") {
		t.Errorf("warning manquant : %q", stderr.String())
	}
	if strings.Contains(stdout.String(), "What's new") {
		t.Errorf("section affichée malgré le parse raté : %q", stdout.String())
	}
}

func TestPrintBundleChangelog_ShowsAllEntriesWhenBaselineIsContentSHA(t *testing.T) {
	var stdout bytes.Buffer
	application := newApp(nil, &stdout, io.Discard, nil)
	bundleContents := map[string][]byte{
		"skills/react-forms/CHANGELOG.md": []byte(bundleChangelogFixture),
	}

	application.printBundleChangelog("react-forms", bundleContents, "0bee89b07a248e27c83fc3d5951213c1")

	output := stdout.String()
	if !strings.Contains(output, "## [1.3.0] - 2026-07-01") || !strings.Contains(output, "## [1.2.1] - 2026-06-20") {
		t.Errorf("toutes les entrées attendues quand la baseline est incomparable : %q", output)
	}
}

func TestPrintBundleChangelog_AppendsTruncationNote(t *testing.T) {
	var stdout bytes.Buffer
	application := newApp(nil, &stdout, io.Discard, nil)
	padding := strings.Repeat("- ligne de padding assez longue pour dépasser la limite\n", 1200)
	oversized := "## [9.9.9] - 2026-07-01\n\n" + padding
	if len(oversized) <= 50*1024 {
		t.Fatal("fixture trop petite pour le test de troncature")
	}
	bundleContents := map[string][]byte{
		"skills/react-forms/CHANGELOG.md": []byte(oversized),
	}

	application.printBundleChangelog("react-forms", bundleContents, "1.0.0")

	if !strings.Contains(stdout.String(), "(changelog truncated)") {
		t.Errorf("note de troncature manquante : %q", stdout.String())
	}
}
