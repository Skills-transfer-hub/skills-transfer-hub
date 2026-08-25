package cmd

import (
	"errors"
	"fmt"
	"os"
	"sort"

	"sth/internal/fs"
	"sth/internal/scanner"
	"sth/internal/tui"
)

func (a *app) runCatalog(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: sth catalog init [--force]")
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
	return fmt.Errorf("unknown catalog command: %q", args[0])
}

func (a *app) runCatalogInit(args []string) error {
	flags, positional := parseFlags(args, "--force")
	if len(positional) > 0 {
		return fmt.Errorf("sth catalog init does not accept positional arguments (received: %v)", positional)
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
				return errors.New("generation canceled: existing SKILLS.md preserved")
			}
		}
	}

	if err := os.WriteFile("SKILLS.md", []byte(fs.RenderSkillsMarkdown(entries)), 0o644); err != nil {
		return fmt.Errorf("failed to write SKILLS.md: %w", err)
	}

	a.writeStdout(fmt.Sprintf(
		"SKILLS.md generated with %d skill(s) across %d category or categories.",
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
	return nil, errors.New("no skill layout detected")
}

func (a *app) confirmSkillsOverwrite() bool {
	session := tui.NewSession(a.stdin, a.stdout)
	confirmed, err := tui.Confirm(session, tui.ConfirmConfig{
		Question:   "SKILLS.md already exists. Replace it?",
		DefaultYes: false,
	})
	if err != nil {
		return false
	}
	return confirmed
}

func countCategories(entries []fs.CatalogEntry) int {
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		seen[entry.FolderName] = struct{}{}
	}
	return len(seen)
}
