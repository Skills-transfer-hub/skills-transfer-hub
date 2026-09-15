package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"sth/internal/fs"
)

type repositoryUpdateReport struct {
	CheckedAt time.Time                  `json:"checked_at"`
	Resources []repositoryUpdateResource `json:"resources"`
	Signature string                     `json:"-"`
	Error     string                     `json:"error,omitempty"`
}

type repositoryUpdateResource struct {
	Provider   string `json:"provider"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Installed  string `json:"installed_version"`
	Available  string `json:"available_version,omitempty"`
	NextAction string `json:"next_action,omitempty"`
}

func (a *app) prepareRepositoryUpdateCheck(force bool) (repositoryUpdateReport, func() repositoryUpdateReport) {
	config, err := fs.ReadProjectConfigV2ReadOnly()
	if err != nil {
		return repositoryUpdateReport{Error: err.Error()}, nil
	}
	directories, err := projectStateDirs(config)
	if err != nil {
		return repositoryUpdateReport{Error: err.Error()}, nil
	}
	state, err := fs.ReadInstalledStateAcrossDirs(directories)
	if err != nil {
		return repositoryUpdateReport{Error: err.Error()}, nil
	}
	serialized, err := json.Marshal(struct {
		Config fs.ProjectConfigV2
		State  fs.InstalledResourceStateV2
	}{config, state})
	if err != nil {
		return repositoryUpdateReport{Error: err.Error()}, nil
	}
	signature := fmt.Sprintf("%x", sha256.Sum256(serialized))
	if !force && a.repositoryUpdates != nil && a.repositoryUpdates.Signature == signature && time.Since(a.repositoryUpdates.CheckedAt) < 5*time.Minute {
		return *a.repositoryUpdates, nil
	}
	if len(state.Entries) == 0 {
		return repositoryUpdateReport{CheckedAt: time.Now().UTC(), Signature: signature, Resources: []repositoryUpdateResource{}}, nil
	}
	projectRoot, _ := filepath.Abs(".")
	return repositoryUpdateReport{}, func() repositoryUpdateReport {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		return a.checkRepositoryUpdates(ctx, config, state, projectRoot, signature)
	}
}

func (a *app) checkRepositoryUpdates(ctx context.Context, config fs.ProjectConfigV2, state fs.InstalledResourceStateV2, projectRoot, signature string) repositoryUpdateReport {
	report := repositoryUpdateReport{CheckedAt: time.Now().UTC(), Signature: signature, Resources: []repositoryUpdateResource{}}
	latest := map[string]string{}
	failures := map[string]error{}
	for _, record := range state.Entries {
		provider := fs.RecordProviderID(config, record)
		queryPath := fs.CommitQueryPath(record.SourcePath, record.ArtifactKind)
		key := fs.LatestVersionKey(provider, queryPath)
		if _, checked := failures[key]; checked {
			continue
		}
		requestContext, cancel := context.WithTimeout(ctx, 2*time.Second)
		version, err := a.client.GetLatestCommitSHAForPath(requestContext, config, provider, queryPath)
		cancel()
		latest[key], failures[key] = version, err
	}
	for _, result := range fs.GroupStatusesByResource(fs.ComputeAllStatuses(state, config, latest, projectRoot)) {
		provider := statusProviderLabel(config, result)
		resource := repositoryUpdateResource{Provider: provider, Name: installedDisplayName(result.ResourceName), Status: "up-to-date", Installed: result.InstalledVersion, Available: result.LatestVersion}
		switch {
		case result.LatestVersion == "":
			resource.Status = "unverified"
			resource.NextAction = "Connexion non vérifiée — ouvrez Diagnostic puis réessayez avec `sth check-updates`."
		case result.InstalledVersion == "":
			resource.Status = "unverified"
			resource.NextAction = "Version installée inconnue — ouvrez Diagnostic."
		case result.LatestVersion != result.InstalledVersion && result.Pinned:
			resource.Status = "pinned"
			resource.NextAction = "Nouvelle version disponible ; version épinglée conservée."
		case result.LatestVersion != result.InstalledVersion:
			resource.Status = "update-available"
			resource.NextAction = "Choisissez Mettre à jour pour appliquer la nouvelle version."
		}
		if result.Status == fs.ResourceStatusModifiedLocally {
			resource.NextAction = "Modifications personnelles protégées — copiez-les avant toute mise à jour."
		}
		report.Resources = append(report.Resources, resource)
	}
	return report
}

func repositoryUpdateLines(report repositoryUpdateReport) []string {
	if report.Error != "" {
		return []string{"Mises à jour des skills : non vérifiées — ouvrez Diagnostic."}
	}
	if report.CheckedAt.IsZero() {
		return []string{"Vérification des mises à jour des skills…"}
	}
	if len(report.Resources) == 0 {
		return nil
	}
	lines := []string{"Mises à jour des skills — vérification " + report.CheckedAt.Local().Format("15:04")}
	for _, resource := range report.Resources {
		label := "Déjà à jour"
		switch resource.Status {
		case "unverified":
			label = "connexion ou version non vérifiée"
		case "pinned":
			label = "mise à jour disponible · version épinglée"
		case "update-available":
			label = "mise à jour disponible"
		}
		lines = append(lines, fmt.Sprintf("  %s [%s] : %s", resource.Name, resource.Provider, label))
		if resource.NextAction != "" {
			lines = append(lines, "    "+resource.NextAction)
		}
	}
	return lines
}

func repositoryUpdateMenuLines(report repositoryUpdateReport) []string {
	if report.Error != "" || report.CheckedAt.IsZero() || len(report.Resources) == 0 {
		return repositoryUpdateLines(report)
	}
	available, current := 0, 0
	unverified := map[string]bool{}
	for _, resource := range report.Resources {
		switch resource.Status {
		case "update-available", "pinned":
			available++
		case "up-to-date":
			current++
		case "unverified":
			unverified[resource.Provider] = true
		}
	}
	lines := []string{fmt.Sprintf("Skills : %d mise(s) à jour disponible(s) · %d déjà à jour", available, current)}
	if len(unverified) > 0 {
		providers := make([]string, 0, len(unverified))
		for provider := range unverified {
			providers = append(providers, provider)
		}
		sort.Strings(providers)
		lines = append(lines, "Non vérifié : "+strings.Join(providers, ", ")+" — ouvrez Diagnostic.")
	}
	return append(lines, "Contrôle "+report.CheckedAt.Local().Format("15:04")+" · détails : Vérifier les mises à jour")
}

func (a *app) runCheckUpdates(args []string) error {
	flags, positional := parseFlags(args, "--json")
	if len(positional) > 0 {
		return fmt.Errorf("usage: sth check-updates [--json]")
	}
	report, check := a.prepareRepositoryUpdateCheck(true)
	if check != nil {
		report = check()
	}
	a.repositoryUpdates = &report
	if flags["--json"] {
		data, err := json.Marshal(report)
		if err != nil {
			return err
		}
		a.writeStdout(string(data))
	} else {
		a.writeStdout(repositoryUpdateLines(report)...)
	}
	if report.Error != "" {
		return fmt.Errorf("vérification impossible : %s; ouvrez Diagnostic", report.Error)
	}
	for _, resource := range report.Resources {
		if resource.Status == "unverified" {
			return fmt.Errorf("certaines sources ne sont pas vérifiées; ouvrez Diagnostic")
		}
	}
	return nil
}
