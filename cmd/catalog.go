package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"sth/internal/fs"
	"sth/internal/scanner"
)

func (a *app) runCatalog(args []string) error {
	if len(args) == 0 {
		return errors.New("usage : sth catalog init [--force]")
	}
	if isHelpFlag(args[0]) {
		a.writeStdout(commandHelpText("catalog"))
		return nil
	}
	switch args[0] {
	case "init":
		if len(args) >= 2 && isHelpFlag(args[1]) {
			a.writeStdout(commandHelpText("catalog"))
			return nil
		}
		return a.runCatalogInit(args[1:])
	}
	return fmt.Errorf("commande catalog inconnue : %q", args[0])
}

func (a *app) runCatalogInit(args []string) error {
	flags, positional := parseFlags(args, "--force")
	if len(positional) > 0 {
		return fmt.Errorf("sth catalog init n'accepte pas d'argument positionnel (reçu : %v)", positional)
	}
	force := flags["--force"]

	tree, err := fs.WalkLocalSkillTree(".")
	if err != nil {
		return err
	}

	artifacts, scanErr := scanLocalArtifacts(tree)
	if scanErr != nil {
		return errors.New("No skills found matching the expected structure")
	}

	entries := make([]fs.CatalogEntry, 0, len(artifacts))
	for _, artifact := range artifacts {
		entries = append(entries, fs.CatalogEntry{
			CatalogID:    fs.CatalogID(string(artifact.Kind), artifact.Category, artifact.Name),
			ResourceType: string(artifact.Kind),
			Name:         artifact.Name,
			FolderName:   artifact.Category,
			SourcePath:   artifact.Entrypoint,
			Description:  fs.ReadSkillDescription(artifact.Entrypoint),
		})
	}

	sort.Slice(entries, func(left, right int) bool {
		if entries[left].FolderName == entries[right].FolderName {
			return entries[left].Name < entries[right].Name
		}
		return entries[left].FolderName < entries[right].FolderName
	})

	if !force {
		if _, statErr := os.Stat("SKILLS.md"); statErr == nil {
			if !a.confirmSkillsOverwrite() {
				return errors.New("génération annulée : SKILLS.md existant conservé")
			}
		}
	}

	if err := os.WriteFile("SKILLS.md", []byte(fs.RenderSkillsMarkdown(entries)), 0o644); err != nil {
		return fmt.Errorf("écriture de SKILLS.md échouée : %w", err)
	}

	a.writeStdout(fmt.Sprintf(
		"SKILLS.md generated with %d skill(s) across %d categorie(s).",
		len(entries), countCategories(entries),
	))
	return nil
}

func scanLocalArtifacts(tree []scanner.TreeEntry) ([]scanner.Artifact, error) {
	flatScanner := scanner.FlatSkillScanner{}
	if flatScanner.Detect(tree) {
		return flatScanner.Scan(tree)
	}
	multiStackScanner := scanner.MultiStackScanner{}
	if multiStackScanner.Detect(tree) {
		return multiStackScanner.Scan(tree)
	}
	return nil, errors.New("aucun layout de skills détecté")
}

func (a *app) confirmSkillsOverwrite() bool {
	fmt.Fprint(a.stdout, "SKILLS.md already exists. Overwrite? [y/N] ")
	inputScanner := bufio.NewScanner(a.stdin)
	if !inputScanner.Scan() {
		return false
	}
	answer := strings.TrimSpace(inputScanner.Text())
	return answer == "y" || answer == "Y"
}

func countCategories(entries []fs.CatalogEntry) int {
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		seen[entry.FolderName] = struct{}{}
	}
	return len(seen)
}
