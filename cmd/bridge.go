package cmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sth/internal/fs"
	"strings"
)

const (
	bridgeProtocolVersion = 1
	bridgeMaxFrameBytes   = 4 * 1024 * 1024
)

type bridgeRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type bridgeResponse struct {
	JSONRPC string       `json:"jsonrpc"`
	ID      int64        `json:"id"`
	Result  any          `json:"result,omitempty"`
	Error   *bridgeError `json:"error,omitempty"`
}

type bridgeError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type bridgeProject struct {
	Root       string              `json:"root"`
	ConfigPath string              `json:"config_path"`
	Configured bool                `json:"configured"`
	Config     *fs.ProjectConfigV2 `json:"config"`
}

type bridgeResource struct {
	ResourceRef string   `json:"resource_ref"`
	SkillName   string   `json:"skill_name,omitempty"`
	Kind        string   `json:"kind"`
	Status      string   `json:"status"`
	Version     string   `json:"version,omitempty"`
	Targets     []string `json:"targets"`
}

type bridgeCatalogResource struct {
	ResourceRef string `json:"resource_ref"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

func (a *app) runBridge(args []string) error {
	if len(args) != 1 || args[0] != "--stdio" {
		return errors.New("usage : sth bridge --stdio")
	}

	scanner := bufio.NewScanner(a.stdin)
	scanner.Buffer(make([]byte, 64*1024), bridgeMaxFrameBytes)
	encoder := json.NewEncoder(a.stdout)
	for scanner.Scan() {
		response := a.handleBridgeFrame(scanner.Bytes())
		if err := encoder.Encode(response); err != nil {
			return fmt.Errorf("failed to write bridge output: %w", err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to read bridge input: %w", err)
	}
	return nil
}

func (a *app) handleBridgeFrame(frame []byte) bridgeResponse {
	request := bridgeRequest{}
	if err := json.Unmarshal(frame, &request); err != nil || request.JSONRPC != "2.0" || request.ID < 1 || strings.TrimSpace(request.Method) == "" {
		return bridgeFailure(request.ID, -32600, "Invalid JSON-RPC request.")
	}

	result, rpcError := a.routeBridgeRequest(request)
	if rpcError != nil {
		return bridgeResponse{JSONRPC: "2.0", ID: request.ID, Error: rpcError}
	}
	return bridgeResponse{JSONRPC: "2.0", ID: request.ID, Result: result}
}

func (a *app) routeBridgeRequest(request bridgeRequest) (any, *bridgeError) {
	switch request.Method {
	case "handshake":
		var params struct {
			ProtocolVersion int `json:"protocol_version"`
		}
		if err := decodeBridgeParams(request.Params, &params); err != nil || params.ProtocolVersion != bridgeProtocolVersion {
			return nil, &bridgeError{Code: -32001, Message: "Incompatible bridge protocol version."}
		}
		return map[string]any{
			"protocol_version": bridgeProtocolVersion,
			"bridge_version":   valueOrDefault(version, "dev"),
			"methods": []string{
				"handshake", "project", "status", "catalogue", "preview", "install",
				"cloud.preview", "cloud.install", "cloud.publish.preview", "cloud.publish",
				"update.preview", "update", "remove.preview", "remove",
				"library.list", "library.get", "library.save", "capture",
				"promote.preview", "promote",
			},
		}, nil
	case "project":
		project, err := readBridgeProject()
		if err != nil {
			return nil, bridgeInternalError(err)
		}
		return project, nil
	case "status":
		project, err := readBridgeProject()
		if err != nil {
			return nil, bridgeInternalError(err)
		}
		resources, err := readBridgeResources(project.Config)
		if err != nil {
			return nil, bridgeInternalError(err)
		}
		return map[string]any{"project": project, "resources": resources}, nil
	case "catalogue":
		resources, err := a.readBridgeCatalogue()
		if err != nil {
			return nil, &bridgeError{Code: -32012, Message: err.Error()}
		}
		return map[string]any{"resources": resources}, nil
	case "preview":
		var params struct {
			ResourceRef string `json:"resource_ref"`
			Force       bool   `json:"force"`
		}
		if err := decodeBridgeParams(request.Params, &params); err != nil || strings.TrimSpace(params.ResourceRef) == "" {
			return nil, &bridgeError{Code: -32602, Message: "The resource_ref parameter is required."}
		}
		changes, err := a.previewBridgeResource(strings.TrimSpace(params.ResourceRef), params.Force)
		if err != nil {
			return nil, &bridgeError{Code: -32010, Message: err.Error()}
		}
		return map[string]any{
			"resource_ref": strings.TrimSpace(params.ResourceRef),
			"changes":      changes,
			"warnings":     []string{},
		}, nil
	case "install":
		var params struct {
			ResourceRef string `json:"resource_ref"`
			Force       bool   `json:"force"`
			Confirmed   bool   `json:"confirmed"`
		}
		if err := decodeBridgeParams(request.Params, &params); err != nil || strings.TrimSpace(params.ResourceRef) == "" || !params.Confirmed {
			return nil, &bridgeError{Code: -32602, Message: "A reference and explicit confirmation are required."}
		}
		resourceRef := strings.TrimSpace(params.ResourceRef)
		if err := a.installBridgeResource(resourceRef, params.Force); err != nil {
			return nil, &bridgeError{Code: -32011, Message: err.Error()}
		}
		return map[string]any{"resource_ref": resourceRef, "installed": true}, nil
	case "cloud.preview":
		var params struct {
			AssetID string `json:"asset_id"`
			Force   bool   `json:"force"`
		}
		if err := decodeBridgeParams(request.Params, &params); err != nil || strings.TrimSpace(params.AssetID) == "" {
			return nil, &bridgeError{Code: -32602, Message: "The asset_id parameter is required."}
		}
		assetID := strings.TrimSpace(params.AssetID)
		versionID, changes, warnings, err := previewBridgeCloudSkill(assetID, params.Force)
		if err != nil {
			return nil, &bridgeError{Code: -32030, Message: err.Error()}
		}
		return map[string]any{
			"asset_id":   assetID,
			"version_id": versionID,
			"changes":    changes,
			"warnings":   warnings,
		}, nil
	case "cloud.install":
		var params struct {
			AssetID           string `json:"asset_id"`
			ExpectedVersionID string `json:"expected_version_id"`
			Force             bool   `json:"force"`
			Confirmed         bool   `json:"confirmed"`
		}
		if err := decodeBridgeParams(request.Params, &params); err != nil || strings.TrimSpace(params.AssetID) == "" ||
			strings.TrimSpace(params.ExpectedVersionID) == "" || !params.Confirmed {
			return nil, &bridgeError{Code: -32602, Message: "An asset_id, expected_version_id, and explicit confirmation are required."}
		}
		assetID := strings.TrimSpace(params.AssetID)
		versionID, err := installBridgeCloudSkill(assetID, strings.TrimSpace(params.ExpectedVersionID), params.Force)
		if err != nil {
			return nil, &bridgeError{Code: -32031, Message: err.Error()}
		}
		return map[string]any{"asset_id": assetID, "version_id": versionID, "installed": true}, nil
	case "cloud.publish.preview":
		var params struct {
			SourcePath string `json:"source_path"`
			FolderName string `json:"folder_name"`
			Version    string `json:"version"`
		}
		if err := decodeBridgeParams(request.Params, &params); err != nil || params.SourcePath == "" || params.FolderName == "" || params.Version == "" {
			return nil, &bridgeError{Code: -32602, Message: "source_path, folder_name, and version are required."}
		}
		plan, err := previewBridgeCloudPublication(params.SourcePath, params.FolderName, params.Version)
		if err != nil {
			return nil, &bridgeError{Code: -32032, Message: err.Error()}
		}
		return map[string]any{
			"commit":          plan.commit,
			"manifest_sha256": plan.manifestSHA256,
			"changes":         plan.changes,
			"warnings":        plan.warnings,
		}, nil
	case "cloud.publish":
		var params struct {
			SourcePath             string `json:"source_path"`
			FolderName             string `json:"folder_name"`
			Version                string `json:"version"`
			ExpectedCommit         string `json:"expected_commit"`
			ExpectedManifestSHA256 string `json:"expected_manifest_sha256"`
			Confirmed              bool   `json:"confirmed"`
		}
		if err := decodeBridgeParams(request.Params, &params); err != nil || params.SourcePath == "" || params.FolderName == "" || params.Version == "" ||
			!validBridgeGitOID(params.ExpectedCommit) || len(params.ExpectedManifestSHA256) != 64 || !validBridgeGitOID(params.ExpectedManifestSHA256) || !params.Confirmed {
			return nil, &bridgeError{Code: -32602, Message: "The previewed manifest and explicit confirmation are required."}
		}
		finalized, plan, err := publishBridgeCloudPublication(
			params.SourcePath, params.FolderName, params.Version, params.ExpectedCommit, params.ExpectedManifestSHA256,
		)
		if err != nil {
			return nil, &bridgeError{Code: -32033, Message: err.Error()}
		}
		return map[string]any{
			"published":       true,
			"asset_id":        finalized.AssetID,
			"version_id":      *finalized.VersionID,
			"commit":          plan.commit,
			"manifest_sha256": plan.manifestSHA256,
		}, nil
	case "update.preview":
		changes, err := a.previewBridgeUpdate()
		if err != nil {
			return nil, &bridgeError{Code: -32013, Message: err.Error()}
		}
		return map[string]any{"changes": changes, "warnings": []string{}}, nil
	case "update":
		var params struct {
			Confirmed bool `json:"confirmed"`
		}
		if err := decodeBridgeParams(request.Params, &params); err != nil || !params.Confirmed {
			return nil, &bridgeError{Code: -32602, Message: "Explicit confirmation is required."}
		}
		if err := a.updateBridgeResources(); err != nil {
			return nil, &bridgeError{Code: -32014, Message: err.Error()}
		}
		return map[string]any{"updated": true}, nil
	case "remove.preview":
		var params struct {
			ResourceRef string `json:"resource_ref"`
		}
		if err := decodeBridgeParams(request.Params, &params); err != nil || strings.TrimSpace(params.ResourceRef) == "" {
			return nil, &bridgeError{Code: -32602, Message: "The resource_ref parameter is required."}
		}
		resource, err := previewBridgeRemoval(strings.TrimSpace(params.ResourceRef))
		if err != nil {
			return nil, &bridgeError{Code: -32015, Message: err.Error()}
		}
		return map[string]any{
			"resource": resource,
			"warnings": []string{"Files managed by this resource will be removed after confirmation."},
		}, nil
	case "remove":
		var params struct {
			ResourceRef string `json:"resource_ref"`
			Confirmed   bool   `json:"confirmed"`
		}
		if err := decodeBridgeParams(request.Params, &params); err != nil || strings.TrimSpace(params.ResourceRef) == "" || !params.Confirmed {
			return nil, &bridgeError{Code: -32602, Message: "A reference and explicit confirmation are required."}
		}
		resourceRef := strings.TrimSpace(params.ResourceRef)
		if err := a.removeBridgeResource(resourceRef); err != nil {
			return nil, &bridgeError{Code: -32016, Message: err.Error()}
		}
		return map[string]any{"resource_ref": resourceRef, "removed": true}, nil
	case "library.list":
		items, err := fs.ReadLocalLibrary(".")
		if err != nil {
			return nil, &bridgeError{Code: -32020, Message: "The local library cannot be read."}
		}
		return map[string]any{"items": items}, nil
	case "library.get":
		var params struct {
			ID string `json:"id"`
		}
		if err := decodeBridgeParams(request.Params, &params); err != nil || strings.TrimSpace(params.ID) == "" {
			return nil, &bridgeError{Code: -32602, Message: "The id parameter is required."}
		}
		items, err := fs.ReadLocalLibrary(".")
		if err != nil {
			return nil, &bridgeError{Code: -32020, Message: "The local library cannot be read."}
		}
		for _, item := range items {
			if item.ID == strings.TrimSpace(params.ID) {
				return item, nil
			}
		}
		return nil, &bridgeError{Code: -32021, Message: "Local item not found."}
	case "library.save", "capture":
		var params struct {
			Kind      string   `json:"kind"`
			Title     string   `json:"title"`
			Body      string   `json:"body"`
			Tags      []string `json:"tags"`
			Confirmed bool     `json:"confirmed"`
		}
		if err := decodeBridgeParams(request.Params, &params); err != nil || !params.Confirmed {
			return nil, &bridgeError{Code: -32602, Message: "A capture and explicit confirmation are required."}
		}
		item, err := fs.SaveLocalLibraryItem(".", fs.LocalLibrarySaveInput{
			Kind: params.Kind, Title: params.Title, Body: params.Body, Tags: params.Tags,
		})
		if err != nil {
			return nil, &bridgeError{Code: -32022, Message: err.Error()}
		}
		return item, nil
	case "promote.preview":
		var params struct {
			LibraryID string `json:"library_id"`
			Output    string `json:"output"`
		}
		if err := decodeBridgeParams(request.Params, &params); err != nil || strings.TrimSpace(params.LibraryID) == "" {
			return nil, &bridgeError{Code: -32602, Message: "The library_id parameter is required."}
		}
		plan, err := fs.PlanLocalLibraryPromotion(".", params.LibraryID, params.Output)
		if err != nil {
			return nil, &bridgeError{Code: -32023, Message: err.Error()}
		}
		return map[string]any{
			"library_id": plan.LibraryID,
			"output":     plan.Output,
			"files":      plan.Files,
			"warnings":   []string{"Git remains the source of truth; no files were written."},
		}, nil
	case "promote":
		var params struct {
			LibraryID string `json:"library_id"`
			Output    string `json:"output"`
			Confirmed bool   `json:"confirmed"`
		}
		if err := decodeBridgeParams(request.Params, &params); err != nil || strings.TrimSpace(params.LibraryID) == "" || !params.Confirmed {
			return nil, &bridgeError{Code: -32602, Message: "A library_id and explicit confirmation are required."}
		}
		plan, err := fs.PlanLocalLibraryPromotion(".", params.LibraryID, params.Output)
		if err != nil {
			return nil, &bridgeError{Code: -32023, Message: err.Error()}
		}
		if err := fs.ApplyLocalLibraryPromotion(plan); err != nil {
			return nil, &bridgeError{Code: -32024, Message: err.Error()}
		}
		return map[string]any{
			"library_id": plan.LibraryID,
			"output":     plan.Output,
			"files":      plan.Files,
			"promoted":   true,
			"guidance":   "Add the bundle to Git, commit it, then open a pull request; STH did not commit or push.",
		}, nil
	default:
		return nil, &bridgeError{Code: -32601, Message: "Unknown bridge method."}
	}
}

func (a *app) readBridgeCatalogue() ([]bridgeCatalogResource, error) {
	profile, err := fs.ReadProjectProfile()
	if err != nil {
		return nil, err
	}
	var entries []fs.CatalogEntry
	if err := a.withAuthRecovery(context.Background(), func() error {
		var listErr error
		entries, listErr = a.client.ListCatalog(context.Background(), *profile)
		return listErr
	}); err != nil {
		return nil, err
	}
	resources := make([]bridgeCatalogResource, 0, len(entries))
	for _, entry := range entries {
		resources = append(resources, bridgeCatalogResource{
			ResourceRef: entry.CatalogID,
			Kind:        entry.ResourceType,
			Name:        entry.Name,
			Description: entry.Description,
		})
	}
	sort.Slice(resources, func(leftIndex, rightIndex int) bool {
		if resources[leftIndex].Name == resources[rightIndex].Name {
			return resources[leftIndex].ResourceRef < resources[rightIndex].ResourceRef
		}
		return resources[leftIndex].Name < resources[rightIndex].Name
	})
	return resources, nil
}

func (a *app) previewBridgeUpdate() ([]fs.DryRunChange, error) {
	var stdout bytes.Buffer
	previewApp := newApp(strings.NewReader(""), &stdout, io.Discard, a.client)
	if err := previewApp.runUpdate([]string{"--dry-run", "--json"}); err != nil {
		return nil, err
	}
	output := bytes.TrimSpace(stdout.Bytes())
	if len(output) == 0 || !bytes.HasPrefix(output, []byte("[")) {
		return []fs.DryRunChange{}, nil
	}
	changes := []fs.DryRunChange{}
	if err := json.Unmarshal(output, &changes); err != nil {
		return nil, errors.New("the update engine produced an invalid preview")
	}
	return changes, nil
}

func (a *app) updateBridgeResources() error {
	updateApp := newApp(strings.NewReader(""), io.Discard, io.Discard, a.client)
	return updateApp.runUpdate([]string{"--json"})
}

func previewBridgeRemoval(resourceRef string) (bridgeResource, error) {
	project, err := readBridgeProject()
	if err != nil {
		return bridgeResource{}, err
	}
	resources, err := readBridgeResources(project.Config)
	if err != nil {
		return bridgeResource{}, err
	}
	for _, resource := range resources {
		if resource.ResourceRef == resourceRef || resource.SkillName == resourceRef {
			return resource, nil
		}
	}
	return bridgeResource{}, fmt.Errorf("resource %q not found", resourceRef)
}

func (a *app) removeBridgeResource(resourceRef string) error {
	removed, err := removeBridgeCloudResource(resourceRef)
	if err != nil {
		return err
	}
	if removed {
		return nil
	}
	removeApp := newApp(strings.NewReader(""), io.Discard, io.Discard, a.client)
	return removeApp.runRemove([]string{resourceRef, "--yes"})
}

func readBridgeProject() (bridgeProject, error) {
	root, err := filepath.Abs(".")
	if err != nil {
		return bridgeProject{}, err
	}
	configPath, err := fs.ProjectProfilePath()
	if err != nil {
		return bridgeProject{}, err
	}
	configured := true
	if _, err := os.Stat(configPath); err != nil {
		if !os.IsNotExist(err) {
			return bridgeProject{}, err
		}
		configured = false
	}
	if !configured {
		return bridgeProject{Root: root, ConfigPath: configPath, Configured: false, Config: nil}, nil
	}
	config, err := fs.ReadProjectConfigV2ReadOnly()
	if err != nil {
		return bridgeProject{}, err
	}
	return bridgeProject{Root: root, ConfigPath: configPath, Configured: true, Config: &config}, nil
}

func readBridgeResources(config *fs.ProjectConfigV2) ([]bridgeResource, error) {
	targets := []fs.TargetEntry{
		{ID: "sth-code-claude", Kind: "claude", RootDir: ".claude", Enabled: true},
		{ID: "sth-code-codex", Kind: "codex", RootDir: ".codex", Enabled: true},
	}
	if config != nil && len(config.Targets) > 0 {
		targets = fs.EffectiveTargets(*config)
	}
	targetByID := make(map[string]fs.TargetEntry, len(targets))
	for _, target := range targets {
		targetByID[target.ID] = target
	}

	stateDirectories := []string{bridgeCloudStateDirectory}
	seenStateDirectories := map[string]struct{}{bridgeCloudStateDirectory: {}}
	if config != nil {
		for _, provider := range config.Providers {
			stateDirectory := strings.TrimSpace(provider.TargetDir)
			if stateDirectory == "" {
				continue
			}
			if _, seen := seenStateDirectories[stateDirectory]; seen {
				continue
			}
			seenStateDirectories[stateDirectory] = struct{}{}
			stateDirectories = append(stateDirectories, stateDirectory)
		}
	}
	type aggregate struct {
		bridgeResource
		targets map[string]struct{}
	}
	resources := map[string]*aggregate{}
	for _, stateDirectory := range stateDirectories {
		state, err := fs.ReadInstalledStateV2(stateDirectory)
		if err != nil {
			return nil, err
		}
		for _, record := range state.Entries {
			key := record.ResourceName
			current := resources[key]
			if current == nil {
				kind := strings.TrimSpace(record.ArtifactKind)
				if kind == "" {
					kind = "skill"
				}
				current = &aggregate{
					bridgeResource: bridgeResource{
						ResourceRef: key,
						SkillName:   bridgeInstalledSkillName(record, kind),
						Kind:        kind,
						Status:      "installed",
						Version:     record.ResolvedVersion,
						Targets:     []string{},
					},
					targets: map[string]struct{}{},
				}
				resources[key] = current
			}
			if current.SkillName == "" {
				current.SkillName = bridgeInstalledSkillName(record, current.Kind)
			}
			for _, targetKind := range bridgeInstalledTargetKinds(record, targets, targetByID) {
				current.targets[targetKind] = struct{}{}
			}
		}
	}
	keys := make([]string, 0, len(resources))
	for key := range resources {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]bridgeResource, 0, len(keys))
	for _, key := range keys {
		current := resources[key]
		for target := range current.targets {
			current.Targets = append(current.Targets, target)
		}
		sort.Strings(current.Targets)
		result = append(result, current.bridgeResource)
	}
	return result, nil
}

func bridgeInstalledTargetKinds(
	record fs.InstalledResourceRecordV2,
	targets []fs.TargetEntry,
	targetByID map[string]fs.TargetEntry,
) []string {
	if targetID := strings.TrimSpace(record.TargetID); targetID != "" {
		if target, ok := targetByID[targetID]; ok && strings.TrimSpace(target.Kind) != "" {
			return []string{strings.TrimSpace(target.Kind)}
		}
		return []string{targetID}
	}

	hasClaudePlacement := bridgeRecordHasManagedPrefix(record, ".claude/skills/")
	hasAgentsPlacement := bridgeRecordHasManagedPrefix(record, ".agents/skills/")
	result := make([]string, 0, len(targets))
	for _, target := range targets {
		if !target.Enabled {
			continue
		}
		kind := strings.TrimSpace(target.Kind)
		if (hasClaudePlacement && (kind == "claude" || kind == "cursor" || kind == "copilot")) ||
			(hasAgentsPlacement && (kind == "codex" || kind == "antigravity" || kind == "gemini")) {
			result = append(result, kind)
		}
	}
	return result
}

func bridgeRecordHasManagedPrefix(record fs.InstalledResourceRecordV2, prefix string) bool {
	for _, file := range record.ManagedFiles {
		normalized := strings.TrimPrefix(filepath.ToSlash(filepath.Clean(file.Path)), "./")
		if strings.HasPrefix(normalized, strings.TrimPrefix(prefix, "./")) {
			return true
		}
	}
	return false
}

func bridgeInstalledSkillName(record fs.InstalledResourceRecordV2, kind string) string {
	if kind != "skill" {
		return ""
	}
	const marker = ".agents/skills/"
	for _, file := range record.ManagedFiles {
		normalized := strings.TrimPrefix(filepath.ToSlash(filepath.Clean(file.Path)), "./")
		index := strings.Index(normalized, marker)
		if index < 0 {
			continue
		}
		remainder := strings.TrimPrefix(normalized[index+len(marker):], "/")
		segments := strings.Split(remainder, "/")
		if len(segments) >= 2 && strings.TrimSpace(segments[0]) != "" {
			return strings.TrimSpace(segments[0])
		}
	}
	return ""
}

func (a *app) previewBridgeResource(resourceRef string, force bool) ([]fs.DryRunChange, error) {
	var stdout bytes.Buffer
	previewApp := newApp(strings.NewReader(""), &stdout, io.Discard, a.client)
	args := []string{resourceRef, "--dry-run", "--json"}
	if force {
		args = append(args, "--force")
	}
	if err := previewApp.runInstall(args); err != nil {
		return nil, err
	}
	changes := []fs.DryRunChange{}
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &changes); err != nil {
		return nil, errors.New("the installation engine produced an invalid preview")
	}
	return changes, nil
}

func (a *app) installBridgeResource(resourceRef string, force bool) error {
	installApp := newApp(strings.NewReader(""), io.Discard, io.Discard, a.client)
	args := []string{resourceRef, "--json"}
	if force {
		args = append(args, "--force")
	}
	return installApp.runInstall(args)
}

func decodeBridgeParams(raw json.RawMessage, target any) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = json.RawMessage(`{}`)
	}
	return json.Unmarshal(raw, target)
}

func bridgeFailure(id int64, code int, message string) bridgeResponse {
	return bridgeResponse{JSONRPC: "2.0", ID: id, Error: &bridgeError{Code: code, Message: message}}
}

func bridgeInternalError(error) *bridgeError {
	return &bridgeError{Code: -32000, Message: "The local STH engine could not read this project."}
}
