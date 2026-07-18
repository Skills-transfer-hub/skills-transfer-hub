package cmd

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"sth/internal/cloud"
	"sth/internal/scanner"
)

const (
	bridgePublicationMaxFiles     = 500
	bridgePublicationMaxFileBytes = 25 * 1024 * 1024
	bridgePublicationMaxBytes     = 100 * 1024 * 1024
	bridgeGitMetadataMaxBytes     = 2 * 1024 * 1024
)

type bridgeCloudPublicationPlan struct {
	sourcePath     string
	commit         string
	manifestSHA256 string
	manifest       cloud.PublicationManifestInput
	contents       map[string][]byte
	changes        []bridgeCloudPublicationChange
	warnings       []string
}

type bridgeCloudPublicationChange struct {
	Action string `json:"action"`
	Path   string `json:"path"`
	Target string `json:"target"`
}

type bridgeGitTreeFile struct {
	repositoryPath string
	logicalPath    string
	objectID       string
	size           int64
}

func previewBridgeCloudPublication(sourcePath, folderName, version string) (bridgeCloudPublicationPlan, error) {
	return buildBridgeCloudPublication(sourcePath, folderName, version)
}

func publishBridgeCloudPublication(
	sourcePath, folderName, version, expectedCommit, expectedManifestSHA256 string,
) (cloud.Publication, bridgeCloudPublicationPlan, error) {
	plan, err := buildBridgeCloudPublication(sourcePath, folderName, version)
	if err != nil {
		return cloud.Publication{}, bridgeCloudPublicationPlan{}, err
	}
	if plan.commit != expectedCommit || plan.manifestSHA256 != expectedManifestSHA256 {
		return cloud.Publication{}, bridgeCloudPublicationPlan{}, fmt.Errorf("HEAD ou le manifest a changé depuis le preview ; relancez le preview")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	publication, err := cloud.CreatePackagePublication(ctx, nil, plan.manifest)
	if err != nil {
		return cloud.Publication{}, bridgeCloudPublicationPlan{}, err
	}
	uploads, err := cloud.CreatePackagePublicationUploads(ctx, nil, publication)
	if err != nil {
		return cloud.Publication{}, bridgeCloudPublicationPlan{}, err
	}
	if err := cloud.UploadPackagePublicationFiles(ctx, nil, plan.manifest, uploads, plan.contents); err != nil {
		return cloud.Publication{}, bridgeCloudPublicationPlan{}, err
	}
	finalized, err := cloud.FinalizePackagePublication(ctx, nil, publication)
	if err != nil {
		return cloud.Publication{}, bridgeCloudPublicationPlan{}, err
	}
	return finalized, plan, nil
}

func buildBridgeCloudPublication(sourcePath, folderName, version string) (bridgeCloudPublicationPlan, error) {
	sourcePath, err := normalizeBridgePublicationSourcePath(sourcePath)
	if err != nil {
		return bridgeCloudPublicationPlan{}, err
	}
	if !validBridgePublicationComponent(folderName) {
		return bridgeCloudPublicationPlan{}, fmt.Errorf("folder_name doit être un identifiant lowercase sûr")
	}
	if !validBridgePublicationVersion(version) {
		return bridgeCloudPublicationPlan{}, fmt.Errorf("version invalide")
	}
	slug := path.Base(sourcePath)
	if !validBridgePublicationComponent(slug) {
		return bridgeCloudPublicationPlan{}, fmt.Errorf("le dossier du skill doit être un slug lowercase sûr")
	}

	workingDirectory, err := canonicalBridgeProjectRoot()
	if err != nil {
		return bridgeCloudPublicationPlan{}, err
	}
	repositoryRootBytes, err := bridgeGitOutput(workingDirectory, 8*1024, "rev-parse", "--show-toplevel")
	if err != nil {
		return bridgeCloudPublicationPlan{}, fmt.Errorf("ce projet n’est pas un dépôt Git exploitable")
	}
	repositoryRoot := strings.TrimSpace(string(repositoryRootBytes))
	canonicalRepositoryRoot, err := filepath.EvalSymlinks(repositoryRoot)
	if err != nil || canonicalRepositoryRoot == "" {
		return bridgeCloudPublicationPlan{}, fmt.Errorf("racine Git invalide")
	}
	commitBytes, err := bridgeGitOutput(canonicalRepositoryRoot, 256, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return bridgeCloudPublicationPlan{}, fmt.Errorf("HEAD ne pointe pas vers un commit publiable")
	}
	commit := strings.TrimSpace(string(commitBytes))
	if !validBridgeGitOID(commit) {
		return bridgeCloudPublicationPlan{}, fmt.Errorf("OID Git HEAD invalide")
	}
	ref := commit
	if refBytes, refErr := bridgeGitOutput(canonicalRepositoryRoot, 1024, "symbolic-ref", "--quiet", "--short", "HEAD"); refErr == nil {
		candidate := strings.TrimSpace(string(refBytes))
		if validBridgeGitRef(candidate) {
			ref = candidate
		}
	}
	remoteBytes, err := bridgeGitOutput(canonicalRepositoryRoot, 4096, "config", "--get", "remote.origin.url")
	if err != nil {
		return bridgeCloudPublicationPlan{}, fmt.Errorf("remote Git origin manquant")
	}
	remoteURL, err := normalizeBridgeGitRemote(strings.TrimSpace(string(remoteBytes)))
	if err != nil {
		return bridgeCloudPublicationPlan{}, err
	}

	treeBytes, err := bridgeGitOutput(
		canonicalRepositoryRoot,
		bridgeGitMetadataMaxBytes,
		"ls-tree", "-r", "-z", "-l", "--full-tree", commit, "--", ":(literal)"+sourcePath,
	)
	if err != nil {
		return bridgeCloudPublicationPlan{}, fmt.Errorf("lecture de l’arbre Git HEAD échouée")
	}
	treeFiles, err := parseBridgeGitTree(sourcePath, treeBytes)
	if err != nil {
		return bridgeCloudPublicationPlan{}, err
	}
	contents := make(map[string][]byte, len(treeFiles))
	publicationFiles := make([]cloud.PublicationFile, 0, len(treeFiles))
	changes := make([]bridgeCloudPublicationChange, 0, len(treeFiles))
	var entrypointContent []byte
	for _, treeFile := range treeFiles {
		content, err := bridgeGitOutput(canonicalRepositoryRoot, int(treeFile.size)+1, "cat-file", "blob", treeFile.objectID)
		if err != nil || int64(len(content)) != treeFile.size {
			return bridgeCloudPublicationPlan{}, fmt.Errorf("lecture du blob Git HEAD échouée pour %s", treeFile.logicalPath)
		}
		digest := sha256.Sum256(content)
		publicationFiles = append(publicationFiles, cloud.PublicationFile{
			Path: treeFile.logicalPath, SHA256: hex.EncodeToString(digest[:]), Size: treeFile.size,
			MIME: bridgePublicationMIME(treeFile.logicalPath),
		})
		contents[treeFile.logicalPath] = content
		changes = append(changes, bridgeCloudPublicationChange{Action: "upload", Path: treeFile.repositoryPath, Target: "sth-cloud"})
		if treeFile.logicalPath == "SKILL.md" {
			entrypointContent = content
		}
	}
	if entrypointContent == nil {
		return bridgeCloudPublicationPlan{}, fmt.Errorf("le dossier tracké à HEAD ne contient pas SKILL.md")
	}

	name, description := bridgePublicationMetadata(entrypointContent, slug)
	manifest := cloud.PublicationManifestInput{
		Asset: cloud.PublicationAssetInput{
			Kind: "skill", Slug: slug, FolderName: folderName, Name: name, Description: description,
		},
		Version:    version,
		Entrypoint: "SKILL.md",
		Files:      publicationFiles,
		// Only the Claude and Codex mappings are certified in the initial
		// team-brain vertical; export-only targets are intentionally omitted.
		Compatibility: map[string]string{"claude": "supported", "codex": "supported"},
	}
	manifest.Origin.RepositoryURL = remoteURL
	manifest.Origin.Ref = ref
	manifest.Origin.Commit = commit
	manifestSHA256, err := cloud.CanonicalPublicationManifestSHA256(manifest)
	if err != nil {
		return bridgeCloudPublicationPlan{}, err
	}
	warnings := []string{}
	dirty, statusErr := bridgeGitHasOutput(
		canonicalRepositoryRoot,
		"status", "--porcelain=v1", "-z", "--untracked-files=all", "--", ":(literal)"+sourcePath,
	)
	if statusErr != nil {
		return bridgeCloudPublicationPlan{}, fmt.Errorf("vérification du worktree échouée")
	}
	if dirty {
		warnings = append(warnings, "Le worktree diffère de HEAD ; seuls les octets du commit affiché seront publiés.")
	}
	return bridgeCloudPublicationPlan{
		sourcePath: sourcePath, commit: commit, manifestSHA256: manifestSHA256,
		manifest: manifest, contents: contents, changes: changes, warnings: warnings,
	}, nil
}

func normalizeBridgePublicationSourcePath(value string) (string, error) {
	if value == "" || len(value) > 512 || strings.TrimSpace(value) != value || strings.Contains(value, "\\") || strings.Contains(value, ":") || strings.Contains(value, "//") || strings.HasPrefix(value, "/") {
		return "", fmt.Errorf("source_path doit être un dossier relatif POSIX sûr")
	}
	clean := path.Clean(value)
	if clean != value || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("source_path doit être un dossier relatif POSIX sûr")
	}
	segments := strings.Split(clean, "/")
	if len(segments) > 32 {
		return "", fmt.Errorf("source_path dépasse 32 segments")
	}
	for _, segment := range segments {
		if !validBridgePublicationSourceSegment(segment) {
			return "", fmt.Errorf("source_path contient un segment non sûr")
		}
	}
	return clean, nil
}

func validBridgePublicationSourceSegment(value string) bool {
	if value == "" || len(value) > 128 || value == "." || value == ".." || strings.EqualFold(value, ".git") {
		return false
	}
	for _, character := range value {
		if (character < 'A' || character > 'Z') && (character < 'a' || character > 'z') &&
			(character < '0' || character > '9') && character != '.' && character != '_' && character != '-' && character != ' ' {
			return false
		}
	}
	return true
}

func validBridgePublicationComponent(value string) bool {
	return validBridgeCloudIDSegment(value)
}

func validBridgePublicationVersion(value string) bool {
	if value == "" || len(value) > 128 || strings.TrimSpace(value) != value ||
		((value[0] < 'A' || value[0] > 'Z') && (value[0] < 'a' || value[0] > 'z') && (value[0] < '0' || value[0] > '9')) {
		return false
	}
	for _, character := range value {
		if (character < 'A' || character > 'Z') && (character < 'a' || character > 'z') &&
			(character < '0' || character > '9') && character != '.' && character != '_' && character != '+' && character != '-' {
			return false
		}
	}
	return true
}

func validBridgeGitOID(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func validBridgeGitRef(value string) bool {
	if value == "" || len(value) > 512 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}

func normalizeBridgeGitRemote(value string) (string, error) {
	if value == "" || len(value) > 2048 {
		return "", fmt.Errorf("remote Git origin invalide")
	}
	if strings.Contains(value, "://") {
		parsed, err := url.Parse(value)
		if err != nil || parsed.Hostname() == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return "", fmt.Errorf("remote Git origin invalide")
		}
		if parsed.Scheme == "https" && parsed.User == nil {
			return value, nil
		}
		if parsed.Scheme == "ssh" {
			if parsed.User != nil {
				if _, passwordSet := parsed.User.Password(); passwordSet {
					return "", fmt.Errorf("remote Git origin avec credentials refusé")
				}
			}
			return value, nil
		}
		return "", fmt.Errorf("remote Git origin doit utiliser HTTPS ou SSH")
	}
	at := strings.IndexByte(value, '@')
	colon := strings.IndexByte(value, ':')
	if at < 1 || colon <= at+1 || colon == len(value)-1 {
		return "", fmt.Errorf("remote Git origin doit utiliser HTTPS ou SSH")
	}
	user, host, repositoryPath := value[:at], value[at+1:colon], value[colon+1:]
	if !validBridgeRemoteToken(user) || !validBridgeRemoteToken(host) || !validBridgeRemotePath(repositoryPath) {
		return "", fmt.Errorf("remote Git origin SCP invalide")
	}
	normalized := url.URL{Scheme: "ssh", User: url.User(user), Host: host, Path: "/" + repositoryPath}
	return normalized.String(), nil
}

func validBridgeRemoteToken(value string) bool {
	if value == "" || len(value) > 253 {
		return false
	}
	for _, character := range value {
		if (character < 'A' || character > 'Z') && (character < 'a' || character > 'z') &&
			(character < '0' || character > '9') && character != '.' && character != '_' && character != '-' {
			return false
		}
	}
	return true
}

func validBridgeRemotePath(value string) bool {
	if value == "" || strings.HasPrefix(value, "/") || strings.Contains(value, "//") {
		return false
	}
	for _, character := range value {
		if character < 0x21 || character == 0x7f || character == '?' || character == '#' || character == '\\' {
			return false
		}
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

func parseBridgeGitTree(sourceRoot string, encoded []byte) ([]bridgeGitTreeFile, error) {
	records := bytes.Split(encoded, []byte{0})
	files := make([]bridgeGitTreeFile, 0, len(records))
	totalSize := int64(0)
	for _, record := range records {
		if len(record) == 0 {
			continue
		}
		tab := bytes.IndexByte(record, '\t')
		if tab < 0 {
			return nil, fmt.Errorf("arbre Git HEAD malformé")
		}
		metadata := strings.Fields(string(record[:tab]))
		if len(metadata) != 4 || (metadata[0] != "100644" && metadata[0] != "100755") || metadata[1] != "blob" || !validBridgeGitOID(metadata[2]) {
			return nil, fmt.Errorf("symlink, submodule ou entrée Git non régulière refusé")
		}
		size, err := strconv.ParseInt(metadata[3], 10, 64)
		if err != nil || size < 0 || size > bridgePublicationMaxFileBytes || totalSize > bridgePublicationMaxBytes-size {
			return nil, fmt.Errorf("limites de taille de publication dépassées")
		}
		totalSize += size
		repositoryPath := string(record[tab+1:])
		prefix := sourceRoot + "/"
		if !utf8.ValidString(repositoryPath) || !strings.HasPrefix(repositoryPath, prefix) {
			return nil, fmt.Errorf("chemin Git hors bundle refusé")
		}
		logicalPath := strings.TrimPrefix(repositoryPath, prefix)
		if !validBridgePublicationLogicalPath(logicalPath) {
			return nil, fmt.Errorf("chemin Git non sûr refusé")
		}
		files = append(files, bridgeGitTreeFile{repositoryPath: repositoryPath, logicalPath: logicalPath, objectID: metadata[2], size: size})
		if len(files) > bridgePublicationMaxFiles {
			return nil, fmt.Errorf("la publication dépasse %d fichiers", bridgePublicationMaxFiles)
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("aucun fichier tracké à HEAD dans source_path")
	}
	sort.Slice(files, func(i, j int) bool { return files[i].logicalPath < files[j].logicalPath })
	return files, nil
}

func validBridgePublicationLogicalPath(value string) bool {
	if value == "" || len(value) > 512 || path.Clean(value) != value || strings.HasPrefix(value, "/") || strings.Contains(value, "\\") || strings.Contains(value, ":") || strings.Contains(value, "//") || value == "." || value == ".." || strings.HasPrefix(value, "../") {
		return false
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}

func bridgePublicationMIME(filePath string) string {
	switch strings.ToLower(path.Ext(filePath)) {
	case ".md", ".mdx":
		return "text/markdown"
	case ".txt":
		return "text/plain"
	case ".json":
		return "application/json"
	case ".yaml", ".yml":
		return "application/yaml"
	case ".html", ".htm":
		return "text/html"
	case ".css":
		return "text/css"
	case ".js", ".mjs", ".cjs":
		return "text/javascript"
	case ".ts", ".tsx":
		return "text/typescript"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	case ".pdf":
		return "application/pdf"
	default:
		return "application/octet-stream"
	}
}

func bridgePublicationMetadata(content []byte, slug string) (string, *string) {
	frontmatter, ok := scanner.ExtractFrontmatter(content)
	if !ok {
		return slug, nil
	}
	name := slug
	if validBridgePublicationText(frontmatter.Name, 200) {
		name = strings.TrimSpace(frontmatter.Name)
	}
	var description *string
	if validBridgePublicationText(frontmatter.Description, 2000) {
		value := strings.TrimSpace(frontmatter.Description)
		description = &value
	}
	return name, description
}

func validBridgePublicationText(value string, maximum int) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > maximum || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

type bridgeBoundedBuffer struct {
	buffer bytes.Buffer
	limit  int
}

func (buffer *bridgeBoundedBuffer) Write(content []byte) (int, error) {
	if len(content) > buffer.limit-buffer.buffer.Len() {
		remaining := buffer.limit - buffer.buffer.Len()
		if remaining > 0 {
			_, _ = buffer.buffer.Write(content[:remaining])
		}
		return 0, fmt.Errorf("sortie Git trop volumineuse")
	}
	return buffer.buffer.Write(content)
}

func bridgeGitOutput(directory string, maximum int, arguments ...string) ([]byte, error) {
	command := exec.Command("git", arguments...)
	command.Dir = directory
	command.Env = bridgeGitEnvironment()
	output := &bridgeBoundedBuffer{limit: maximum}
	command.Stdout = output
	command.Stderr = io.Discard
	if err := command.Run(); err != nil {
		return nil, err
	}
	return append([]byte(nil), output.buffer.Bytes()...), nil
}

func bridgeGitHasOutput(directory string, arguments ...string) (bool, error) {
	command := exec.Command("git", arguments...)
	command.Dir = directory
	command.Env = bridgeGitEnvironment()
	writer := &bridgePresenceWriter{}
	command.Stdout = writer
	command.Stderr = io.Discard
	if err := command.Run(); err != nil {
		return false, err
	}
	return writer.present, nil
}

type bridgePresenceWriter struct{ present bool }

func (writer *bridgePresenceWriter) Write(content []byte) (int, error) {
	writer.present = writer.present || len(content) > 0
	return len(content), nil
}

func bridgeGitEnvironment() []string {
	environment := make([]string, 0, len(os.Environ())+7)
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, "GIT_") {
			continue
		}
		environment = append(environment, entry)
	}
	environment = append(environment,
		"GIT_NO_REPLACE_OBJECTS=1",
		"GIT_NO_LAZY_FETCH=1",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL="+os.DevNull,
	)
	return environment
}
