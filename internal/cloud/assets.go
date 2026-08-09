package cloud

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	stdhttp "net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"sth/internal/cloud/sthv2"
)

// AssetSummary is the CLI-facing projection of the canonical v2 read model.
// The generated wire DTO remains authoritative; Body is retained only for the
// local-library projection used by MCP.
type AssetSummary struct {
	ID              string `json:"id"`
	Family          string `json:"family"`
	Kind            string `json:"kind"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	Status          string `json:"status"`
	SourcePath      string `json:"source_path,omitempty"`
	Body            string `json:"body,omitempty"`
	LatestVersionID string `json:"latest_version_id,omitempty"`
	LatestVersion   string `json:"latest_version,omitempty"`
}

type ResourceVersionManifest = sthv2.ResourceVersionManifest
type PackageDownloads = sthv2.PackageDownloads

const (
	maxCloudPackageFiles     = 500
	maxCloudPackageFileBytes = 25 * 1024 * 1024
	maxCloudPackageBytes     = 100 * 1024 * 1024
)

func SearchAssets(
	ctx context.Context,
	httpClient *stdhttp.Client,
	query string,
	family string,
	kind string,
) ([]AssetSummary, error) {
	familyValue := trimmedStringPointer(family)
	queryValue := trimmedStringPointer(query)
	var kindValue *sthv2.AIAssetKind
	if normalized := strings.TrimSpace(kind); normalized != "" {
		value := sthv2.AIAssetKind(normalized)
		kindValue = &value
	}
	page, err := newGeneratedV2Client(httpClient).ListAssets(ctx, sthv2.ListAssetsInput{
		Family: familyValue,
		Kind:   kindValue,
		Q:      queryValue,
	})
	if err != nil {
		return nil, fmt.Errorf("recherche d'assets : %w", err)
	}
	assets := make([]AssetSummary, 0, len(page.Items))
	for _, item := range page.Items {
		asset, err := projectAssetSummary(item)
		if err != nil {
			return nil, fmt.Errorf("recherche d'assets : %w", err)
		}
		assets = append(assets, asset)
	}
	return assets, nil
}

func GetAsset(
	ctx context.Context,
	httpClient *stdhttp.Client,
	assetID string,
) (AssetSummary, error) {
	assetID = strings.TrimSpace(assetID)
	if assetID == "" {
		return AssetSummary{}, fmt.Errorf("assets: id requis")
	}
	item, err := newGeneratedV2Client(httpClient).GetAsset(ctx, sthv2.GetAssetInput{AssetID: assetID})
	if err != nil {
		return AssetSummary{}, fmt.Errorf("ouverture d'un asset : %w", err)
	}
	asset, err := projectAssetSummary(item)
	if err != nil {
		return AssetSummary{}, fmt.Errorf("ouverture d'un asset : %w", err)
	}
	if asset.ID != assetID {
		return AssetSummary{}, fmt.Errorf("ouverture d'un asset : identifiant incohérent")
	}
	return asset, nil
}

func GetResourceVersionManifest(
	ctx context.Context,
	httpClient *stdhttp.Client,
	assetID string,
	versionID string,
) (ResourceVersionManifest, error) {
	assetID = strings.TrimSpace(assetID)
	versionID = strings.TrimSpace(versionID)
	if assetID == "" || versionID == "" {
		return ResourceVersionManifest{}, fmt.Errorf("manifest: asset_id et version_id requis")
	}
	manifest, err := newGeneratedV2Client(httpClient).GetResourceVersionManifest(
		ctx,
		sthv2.GetResourceVersionManifestInput{AssetID: assetID, VersionID: versionID},
	)
	if err != nil {
		return ResourceVersionManifest{}, fmt.Errorf("lecture du manifest : %w", err)
	}
	if err := validateResourceVersionManifest(manifest, assetID, versionID); err != nil {
		return ResourceVersionManifest{}, fmt.Errorf("lecture du manifest : %w", err)
	}
	return manifest, nil
}

func CreateResourceVersionDownloads(
	ctx context.Context,
	httpClient *stdhttp.Client,
	assetID string,
	versionID string,
) (PackageDownloads, error) {
	assetID = strings.TrimSpace(assetID)
	versionID = strings.TrimSpace(versionID)
	if assetID == "" || versionID == "" {
		return PackageDownloads{}, fmt.Errorf("downloads: asset_id et version_id requis")
	}
	downloads, err := newGeneratedV2Client(httpClient).CreateResourceVersionDownloads(
		ctx,
		sthv2.CreateResourceVersionDownloadsInput{AssetID: assetID, VersionID: versionID},
	)
	if err != nil {
		return PackageDownloads{}, fmt.Errorf("création des téléchargements : %w", err)
	}
	if downloads.AssetID != assetID || downloads.VersionID != versionID || len(downloads.Downloads) < 1 || len(downloads.Downloads) > maxCloudPackageFiles {
		return PackageDownloads{}, fmt.Errorf("création des téléchargements : réponse incohérente")
	}
	seen := make(map[string]struct{}, len(downloads.Downloads))
	for _, download := range downloads.Downloads {
		if !validLogicalAssetPath(download.Path) || !validSHA256(download.SHA256) || download.Size < 0 || download.Size > maxCloudPackageFileBytes || !validMIME(download.MIME) {
			return PackageDownloads{}, fmt.Errorf("création des téléchargements : entrée invalide")
		}
		if _, duplicate := seen[download.Path]; duplicate {
			return PackageDownloads{}, fmt.Errorf("création des téléchargements : chemin dupliqué")
		}
		seen[download.Path] = struct{}{}
		if !validSignedDownloadURL(download.DownloadURL) || !signedDownloadMatchesCloudOrigin(download.DownloadURL) {
			return PackageDownloads{}, fmt.Errorf("création des téléchargements : URL signée invalide")
		}
		expiresAt, err := time.Parse(time.RFC3339, download.ExpiresAt)
		if err != nil || !expiresAt.After(time.Now()) {
			return PackageDownloads{}, fmt.Errorf("création des téléchargements : expiration invalide")
		}
	}
	return downloads, nil
}

// DownloadResourceVersionBundle verifies that the server-provided signed URL
// set is an exact projection of the immutable manifest, then downloads each
// file with redirects disabled and validates MIME, size and SHA-256 before
// making any content available to a writer.
func DownloadResourceVersionBundle(
	ctx context.Context,
	httpClient *stdhttp.Client,
	manifest ResourceVersionManifest,
	downloads PackageDownloads,
) (map[string][]byte, error) {
	if err := validateResourceVersionManifest(manifest, manifest.AssetID, manifest.VersionID); err != nil {
		return nil, fmt.Errorf("téléchargement du bundle : manifest invalide : %w", err)
	}
	if downloads.AssetID != manifest.AssetID || downloads.VersionID != manifest.VersionID || len(downloads.Downloads) != len(manifest.Files) {
		return nil, fmt.Errorf("téléchargement du bundle : liste de téléchargements incohérente")
	}

	downloadByPath := make(map[string]sthv2.PackageDownload, len(downloads.Downloads))
	for _, download := range downloads.Downloads {
		if _, duplicate := downloadByPath[download.Path]; duplicate {
			return nil, fmt.Errorf("téléchargement du bundle : chemin dupliqué")
		}
		downloadByPath[download.Path] = download
	}

	totalSize := int64(0)
	for _, file := range manifest.Files {
		if file.Size > maxCloudPackageFileBytes || totalSize > maxCloudPackageBytes-file.Size {
			return nil, fmt.Errorf("téléchargement du bundle : limites de taille dépassées")
		}
		totalSize += file.Size
		download, ok := downloadByPath[file.Path]
		if !ok || download.SHA256 != file.SHA256 || download.Size != file.Size || download.MIME != file.MIME {
			return nil, fmt.Errorf("téléchargement du bundle : métadonnées incohérentes pour %s", file.Path)
		}
		if !validSignedDownloadURL(download.DownloadURL) || !signedDownloadMatchesCloudOrigin(download.DownloadURL) {
			return nil, fmt.Errorf("téléchargement du bundle : URL signée invalide pour %s", file.Path)
		}
		expiresAt, err := time.Parse(time.RFC3339, download.ExpiresAt)
		if err != nil || !expiresAt.After(time.Now()) {
			return nil, fmt.Errorf("téléchargement du bundle : URL expirée pour %s", file.Path)
		}
	}

	client := noRedirectClient(httpClient)
	contents := make(map[string][]byte, len(manifest.Files))
	for _, file := range manifest.Files {
		download := downloadByPath[file.Path]
		request, err := stdhttp.NewRequestWithContext(ctx, stdhttp.MethodGet, download.DownloadURL, nil)
		if err != nil {
			return nil, fmt.Errorf("téléchargement du bundle : requête invalide pour %s", file.Path)
		}
		request.Header.Set("Accept", file.MIME)
		response, err := client.Do(request)
		if err != nil {
			return nil, fmt.Errorf("téléchargement du bundle : échec pour %s : %w", file.Path, err)
		}
		content, readErr := readVerifiedPackageFile(response, file)
		response.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		contents[file.Path] = content
	}
	return contents, nil
}

func readVerifiedPackageFile(response *stdhttp.Response, file sthv2.ManifestFile) ([]byte, error) {
	if response.StatusCode != stdhttp.StatusOK {
		return nil, fmt.Errorf("téléchargement du bundle : HTTP %d pour %s", response.StatusCode, file.Path)
	}
	if response.ContentLength >= 0 && response.ContentLength != file.Size {
		return nil, fmt.Errorf("téléchargement du bundle : taille HTTP invalide pour %s", file.Path)
	}
	contentType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || !strings.EqualFold(contentType, file.MIME) {
		return nil, fmt.Errorf("téléchargement du bundle : MIME invalide pour %s", file.Path)
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, file.Size+1))
	if err != nil {
		return nil, fmt.Errorf("téléchargement du bundle : lecture échouée pour %s : %w", file.Path, err)
	}
	if int64(len(content)) != file.Size {
		return nil, fmt.Errorf("téléchargement du bundle : taille invalide pour %s", file.Path)
	}
	digest := sha256.Sum256(content)
	if hex.EncodeToString(digest[:]) != file.SHA256 {
		return nil, fmt.Errorf("téléchargement du bundle : SHA-256 invalide pour %s", file.Path)
	}
	return content, nil
}

func projectAssetSummary(item sthv2.AIAssetSummary) (AssetSummary, error) {
	if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Name) == "" {
		return AssetSummary{}, fmt.Errorf("réponse incomplète")
	}
	if item.Family != "package" && item.Family != "library" {
		return AssetSummary{}, fmt.Errorf("famille inconnue")
	}
	if !validAssetKind(string(item.Kind)) || !validAssetStatus(item.Status) {
		return AssetSummary{}, fmt.Errorf("type ou statut inconnu")
	}
	latestID := dereferenceString(item.LatestVersionID)
	latestVersion := dereferenceString(item.LatestVersion)
	if (latestID == "") != (latestVersion == "") {
		return AssetSummary{}, fmt.Errorf("version courante incomplète")
	}
	return AssetSummary{
		ID:              item.ID,
		Family:          item.Family,
		Kind:            string(item.Kind),
		Name:            item.Name,
		Description:     dereferenceString(item.Description),
		Status:          item.Status,
		SourcePath:      dereferenceString(item.SourcePath),
		LatestVersionID: latestID,
		LatestVersion:   latestVersion,
	}, nil
}

func validateResourceVersionManifest(manifest ResourceVersionManifest, assetID string, versionID string) error {
	if manifest.SchemaVersion != "2" || manifest.AssetID != assetID || manifest.VersionID != versionID {
		return fmt.Errorf("identifiants incohérents")
	}
	if strings.TrimSpace(manifest.Version) == "" || !validAssetKind(manifest.Kind) {
		return fmt.Errorf("métadonnées incomplètes")
	}
	if !validLogicalAssetPath(manifest.Entrypoint) || len(manifest.Files) < 1 || len(manifest.Files) > maxCloudPackageFiles {
		return fmt.Errorf("fichiers invalides")
	}
	if !validGitCommit(manifest.Origin.Commit) || strings.TrimSpace(manifest.Origin.RepositoryURL) == "" {
		return fmt.Errorf("origine Git invalide")
	}
	if !validRepositoryURL(manifest.Origin.RepositoryURL) || !validManifestRef(manifest.Origin.Ref) || !validCompatibility(manifest.Compatibility) {
		return fmt.Errorf("origine Git ou compatibilité invalide")
	}
	seen := make(map[string]struct{}, len(manifest.Files))
	entrypointFound := false
	totalSize := int64(0)
	for _, file := range manifest.Files {
		if !validLogicalAssetPath(file.Path) || !validSHA256(file.SHA256) || file.Size < 0 || file.Size > maxCloudPackageFileBytes || !validMIME(file.MIME) {
			return fmt.Errorf("descripteur de fichier invalide")
		}
		if totalSize > maxCloudPackageBytes-file.Size {
			return fmt.Errorf("taille totale du manifest dépassée")
		}
		totalSize += file.Size
		if _, duplicate := seen[file.Path]; duplicate {
			return fmt.Errorf("chemin de fichier dupliqué")
		}
		seen[file.Path] = struct{}{}
		entrypointFound = entrypointFound || file.Path == manifest.Entrypoint
	}
	if !entrypointFound {
		return fmt.Errorf("entrypoint absent des fichiers")
	}
	return nil
}

func validLogicalAssetPath(value string) bool {
	if value == "" || len(value) > 512 {
		return false
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	return !strings.HasPrefix(value, "/") &&
		!strings.Contains(value, "\\") && !strings.Contains(value, ":") &&
		!strings.Contains(value, "//") && path.Clean(value) == value && value != "." &&
		!strings.HasPrefix(value, "../")
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func validGitCommit(value string) bool {
	return (len(value) == 40 || len(value) == 64) && validHex(value)
}

func validHex(value string) bool {
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func validMIME(value string) bool {
	mediaType, parameters, err := mime.ParseMediaType(value)
	return err == nil && len(parameters) == 0 && mediaType == value && strings.Contains(mediaType, "/")
}

func validRepositoryURL(value string) bool {
	if value == "" || len(value) > 2048 {
		return false
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Hostname() == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	switch parsed.Scheme {
	case "https":
		return parsed.User == nil
	case "ssh":
		if parsed.User == nil {
			return true
		}
		_, passwordSet := parsed.User.Password()
		return !passwordSet
	default:
		return false
	}
}

func validManifestRef(value string) bool {
	if strings.TrimSpace(value) == "" || len(value) > 512 {
		return false
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}

func validCompatibility(value map[string]string) bool {
	if len(value) > 32 {
		return false
	}
	for runtime, status := range value {
		if !validCompatibilityRuntime(runtime) || (status != "supported" && status != "unsupported" && status != "experimental") {
			return false
		}
	}
	return true
}

func validCompatibilityRuntime(value string) bool {
	if value == "" || len(value) > 64 || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for _, character := range value[1:] {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '_' && character != '-' {
			return false
		}
	}
	return true
}

func validSignedDownloadURL(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return false
	}
	if parsed.Scheme == "https" {
		return true
	}
	host := strings.ToLower(parsed.Hostname())
	return parsed.Scheme == "http" && (host == "localhost" || host == "127.0.0.1" || host == "::1")
}

func signedDownloadMatchesCloudOrigin(value string) bool {
	baseValue, err := validatedV2CloudBaseURL()
	if err != nil {
		return false
	}
	base, err := url.Parse(baseValue)
	if err != nil {
		return false
	}
	download, err := url.Parse(value)
	if err != nil {
		return false
	}
	return strings.EqualFold(base.Scheme, download.Scheme) && strings.EqualFold(base.Host, download.Host)
}

func validAssetKind(value string) bool {
	switch value {
	case "skill", "agent", "command", "instruction", "prompt", "artifact":
		return true
	default:
		return false
	}
}

func validAssetStatus(value string) bool {
	switch value {
	case "available", "draft", "published", "archived":
		return true
	default:
		return false
	}
}

func trimmedStringPointer(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func dereferenceString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
