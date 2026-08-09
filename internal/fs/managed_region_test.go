package fs

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestBuildManagedMarkdownRegion(t *testing.T) {
	block, err := BuildManagedMarkdownRegion("codex-instructions-v1", []byte("Règle persistante"))
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("<!-- sth:managed:codex-instructions-v1:start -->\nRègle persistante\n<!-- sth:managed:codex-instructions-v1:end -->")
	if !bytes.Equal(block, want) {
		t.Fatalf("bloc inattendu\nobtenu: %q\nattendu: %q", block, want)
	}
}

func TestBuildManagedMarkdownRegionRejectsUnsafeInput(t *testing.T) {
	invalidIDs := []string{"", "Codex", "codex_instructions", "codex instructions", "-codex", "codex:instructions", "codex\ninstructions"}
	for _, invalidID := range invalidIDs {
		if _, err := BuildManagedMarkdownRegion(invalidID, []byte("contenu")); err == nil {
			t.Errorf("identifiant invalide accepté : %q", invalidID)
		}
	}
	if _, err := BuildManagedMarkdownRegion("codex", []byte("<!-- sth:managed:codex:end -->")); !errors.Is(err, ErrManagedMarkdownRegionMalformed) {
		t.Fatalf("injection de marqueur acceptée : %v", err)
	}
}

func TestMergeManagedMarkdownRegionIntoEmptyDocument(t *testing.T) {
	merged, err := MergeManagedMarkdownRegion(nil, "codex-instructions-v1", []byte("Toujours tester.\n"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := BuildManagedMarkdownRegion("codex-instructions-v1", []byte("Toujours tester.\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(merged, want) {
		t.Fatalf("document vide mal fusionné : %q", merged)
	}
	removed, err := RemoveManagedMarkdownRegion(merged, "codex-instructions-v1")
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 0 {
		t.Fatalf("le retrait du seul bloc doit rendre un document vide : %q", removed)
	}
}

func TestMergeReplaceRemoveManagedMarkdownRegionPreservesExteriorBytes(t *testing.T) {
	prefix := append([]byte{0xef, 0xbb, 0xbf}, []byte("# Préface\r\n")...)
	suffix := []byte("\r\nAprès\x00")
	oldBlock, err := BuildManagedMarkdownRegion("gemini-instructions-v1", []byte("Ancien"))
	if err != nil {
		t.Fatal(err)
	}
	document := append(append(append([]byte{}, prefix...), oldBlock...), suffix...)

	updated, err := MergeManagedMarkdownRegion(document, "gemini-instructions-v1", []byte("Nouveau\ncontenu"))
	if err != nil {
		t.Fatal(err)
	}
	newBlock, err := buildManagedMarkdownRegion("gemini-instructions-v1", []byte("Nouveau\ncontenu"), true)
	if err != nil {
		t.Fatal(err)
	}
	want := append(append(append([]byte{}, prefix...), newBlock...), suffix...)
	if !bytes.Equal(updated, want) {
		t.Fatalf("octets extérieurs modifiés\nobtenu: %q\nattendu: %q", updated, want)
	}

	replaced, err := ReplaceManagedMarkdownRegion(updated, "gemini-instructions-v1", []byte("Final"))
	if err != nil {
		t.Fatal(err)
	}
	removed, err := RemoveManagedMarkdownRegion(replaced, "gemini-instructions-v1")
	if err != nil {
		t.Fatal(err)
	}
	exterior := append(append([]byte{}, prefix...), suffix...)
	if !bytes.Equal(removed, exterior) {
		t.Fatalf("retrait destructif\nobtenu: %q\nattendu: %q", removed, exterior)
	}
	if !bytes.HasPrefix(updated, []byte{0xef, 0xbb, 0xbf}) {
		t.Fatalf("BOM perdu : %x", updated[:3])
	}
}

func TestMergeManagedMarkdownRegionsIndependently(t *testing.T) {
	document := []byte("préexistant")
	withCodex, err := MergeManagedMarkdownRegion(document, "codex-instructions-v1", []byte("Codex"))
	if err != nil {
		t.Fatal(err)
	}
	withBoth, err := MergeManagedMarkdownRegion(withCodex, "gemini-instructions-v1", []byte("Gemini"))
	if err != nil {
		t.Fatal(err)
	}
	withoutCodex, err := RemoveManagedMarkdownRegion(withBoth, "codex-instructions-v1")
	if err != nil {
		t.Fatal(err)
	}
	want, err := MergeManagedMarkdownRegion(document, "gemini-instructions-v1", []byte("Gemini"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(withoutCodex, want) {
		t.Fatalf("retrait d'une région a modifié l'autre\nobtenu: %q\nattendu: %q", withoutCodex, want)
	}
}

func TestManagedMarkdownRegionOperationsRejectMissingMarkers(t *testing.T) {
	document := []byte("sans région")
	if _, err := ReplaceManagedMarkdownRegion(document, "codex-instructions-v1", []byte("contenu")); !errors.Is(err, ErrManagedMarkdownRegionNotFound) {
		t.Fatalf("ReplaceManagedMarkdownRegion : %v", err)
	}
	if _, err := RemoveManagedMarkdownRegion(document, "codex-instructions-v1"); !errors.Is(err, ErrManagedMarkdownRegionNotFound) {
		t.Fatalf("RemoveManagedMarkdownRegion : %v", err)
	}
	if _, err := ManagedMarkdownRegionSHA256(document, "codex-instructions-v1"); !errors.Is(err, ErrManagedMarkdownRegionNotFound) {
		t.Fatalf("ManagedMarkdownRegionSHA256 : %v", err)
	}
}

func TestManagedMarkdownRegionOperationsRejectMalformedMarkers(t *testing.T) {
	startMarker := "<!-- sth:managed:codex-instructions-v1:start -->"
	endMarker := "<!-- sth:managed:codex-instructions-v1:end -->"
	malformedDocuments := map[string][]byte{
		"début sans fin":     []byte(startMarker + "\ncontenu"),
		"fin sans début":     []byte("contenu\n" + endMarker),
		"début dupliqué":     []byte(startMarker + startMarker + endMarker),
		"fin dupliquée":      []byte(startMarker + endMarker + endMarker),
		"marqueurs inversés": []byte(endMarker + "\ncontenu\n" + startMarker),
	}
	for scenario, document := range malformedDocuments {
		t.Run(scenario, func(t *testing.T) {
			if _, err := MergeManagedMarkdownRegion(document, "codex-instructions-v1", []byte("nouveau")); !errors.Is(err, ErrManagedMarkdownRegionMalformed) {
				t.Errorf("MergeManagedMarkdownRegion : %v", err)
			}
			if _, err := ReplaceManagedMarkdownRegion(document, "codex-instructions-v1", []byte("nouveau")); !errors.Is(err, ErrManagedMarkdownRegionMalformed) {
				t.Errorf("ReplaceManagedMarkdownRegion : %v", err)
			}
			if _, err := RemoveManagedMarkdownRegion(document, "codex-instructions-v1"); !errors.Is(err, ErrManagedMarkdownRegionMalformed) {
				t.Errorf("RemoveManagedMarkdownRegion : %v", err)
			}
			if _, err := ManagedMarkdownRegionSHA256(document, "codex-instructions-v1"); !errors.Is(err, ErrManagedMarkdownRegionMalformed) {
				t.Errorf("ManagedMarkdownRegionSHA256 : %v", err)
			}
		})
	}
}

func TestManagedMarkdownRegionSHA256CoversOnlyManagedBlock(t *testing.T) {
	firstDocument, err := MergeManagedMarkdownRegion([]byte("extérieur A"), "codex-instructions-v1", []byte("contenu"))
	if err != nil {
		t.Fatal(err)
	}
	secondDocument, err := MergeManagedMarkdownRegion([]byte("extérieur B très différent"), "codex-instructions-v1", []byte("contenu"))
	if err != nil {
		t.Fatal(err)
	}
	firstSHA256, err := ManagedMarkdownRegionSHA256(firstDocument, "codex-instructions-v1")
	if err != nil {
		t.Fatal(err)
	}
	secondSHA256, err := ManagedMarkdownRegionSHA256(secondDocument, "codex-instructions-v1")
	if err != nil {
		t.Fatal(err)
	}
	if firstSHA256 != secondSHA256 {
		t.Fatalf("le contenu extérieur affecte le SHA : %s != %s", firstSHA256, secondSHA256)
	}
	block, err := buildManagedMarkdownRegion("codex-instructions-v1", []byte("contenu"), true)
	if err != nil {
		t.Fatal(err)
	}
	wantDigest := sha256.Sum256(block)
	wantSHA256 := stringHexDigest(wantDigest)
	if firstSHA256 != wantSHA256 {
		t.Fatalf("SHA du bloc inattendu : %s != %s", firstSHA256, wantSHA256)
	}
}

func TestManagedMarkdownRegionSeparatesGeminiImportAndRestoresExactBytes(t *testing.T) {
	original := []byte("@./user.md")
	merged, err := MergeManagedMarkdownRegion(original, "gemini-instructions-v1", []byte("@./.gemini/sth/instructions/generated.md\n"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(merged, []byte("@./user.md<!--")) || !bytes.Contains(merged, []byte("@./user.md\n<!-- sth:managed:gemini-instructions-v1:start -->")) {
		t.Fatalf("import Gemini non séparé : %q", merged)
	}
	replaced, err := ReplaceManagedMarkdownRegion(merged, "gemini-instructions-v1", []byte("@./.gemini/sth/instructions/replaced.md\n"))
	if err != nil {
		t.Fatal(err)
	}
	removed, err := RemoveManagedMarkdownRegion(replaced, "gemini-instructions-v1")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(removed, original) {
		t.Fatalf("octets Gemini extérieurs modifiés : %q", removed)
	}
}

func TestWriteAndRemoveManagedMarkdownRegionFilePreserveModeAndExterior(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "AGENTS.md")
	original := append([]byte{0xef, 0xbb, 0xbf}, []byte("# Règles locales")...)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}

	region, err := WriteManagedMarkdownRegion(path, "codex-instructions-v1", []byte("Règle gérée"))
	if err != nil {
		t.Fatal(err)
	}
	if region.Path != path || region.ID != "codex-instructions-v1" || region.SHA256 == "" {
		t.Fatalf("métadonnées de région invalides : %+v", region)
	}
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && fileInfo.Mode().Perm() != 0o600 {
		t.Fatalf("permissions modifiées : %o", fileInfo.Mode().Perm())
	}

	updated, err := RemoveManagedMarkdownRegionFile(path, "codex-instructions-v1")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(updated, original) {
		t.Fatalf("contenu extérieur modifié : %q", updated)
	}
	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(onDisk, original) {
		t.Fatalf("contenu disque inattendu : %q", onDisk)
	}
}

func TestWriteAndRemoveOnlyManagedMarkdownRegionLeavesEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "GEMINI.md")
	if _, err := WriteManagedMarkdownRegion(path, "gemini-instructions-v1", []byte("Règle")); err != nil {
		t.Fatal(err)
	}
	updated, err := RemoveManagedMarkdownRegionFile(path, "gemini-instructions-v1")
	if err != nil {
		t.Fatal(err)
	}
	if len(bytes.TrimSpace(updated)) != 0 {
		t.Fatalf("le caller ne peut pas détecter le fichier vide : %q", updated)
	}
}

func TestInstalledResourceRecordV2ManagedRegionsJSON(t *testing.T) {
	emptyRecord := InstalledResourceRecordV2{
		ResourceName: "instruction",
		ManagedFiles: []ManagedFile{},
	}
	emptyData, err := json.Marshal(emptyRecord)
	if err != nil {
		t.Fatal(err)
	}
	var emptyRaw map[string]any
	if err := json.Unmarshal(emptyData, &emptyRaw); err != nil {
		t.Fatal(err)
	}
	if _, exists := emptyRaw["managed_regions"]; exists {
		t.Fatalf("managed_regions doit être omis : %s", emptyData)
	}

	want := InstalledResourceRecordV2{
		ResourceName: "instruction",
		ManagedFiles: []ManagedFile{},
		ManagedRegions: []ManagedRegion{{
			Path:   "AGENTS.md",
			ID:     "codex-instructions-v1",
			SHA256: "deadbeef",
		}},
	}
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got InstalledResourceRecordV2
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.ManagedRegions, want.ManagedRegions) {
		t.Fatalf("round-trip ManagedRegions invalide : %+v", got.ManagedRegions)
	}
}

func stringHexDigest(digest [sha256.Size]byte) string {
	const hexadecimal = "0123456789abcdef"
	encoded := make([]byte, len(digest)*2)
	for index, value := range digest {
		encoded[index*2] = hexadecimal[value>>4]
		encoded[index*2+1] = hexadecimal[value&0x0f]
	}
	return string(encoded)
}
