package cloud

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	stdhttp "net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"sth/internal/cloud/sthv2"
)

type PublicationManifestInput = sthv2.PublicationManifestInput
type PublicationAssetInput = sthv2.PublicationAssetInput
type PublicationFile = sthv2.PublicationFile
type Publication = sthv2.Publication
type BlobUpload = sthv2.BlobUpload

func CanonicalPublicationManifestSHA256(manifest PublicationManifestInput) (string, error) {
	if err := validatePublicationManifestInput(manifest); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return "", fmt.Errorf("publication manifest: encoding failed: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func CreatePackagePublication(ctx context.Context, httpClient *stdhttp.Client, manifest PublicationManifestInput) (Publication, error) {
	expectedHash, err := CanonicalPublicationManifestSHA256(manifest)
	if err != nil {
		return Publication{}, err
	}
	publication, err := newGeneratedV2Client(httpClient).CreatePublication(ctx, sthv2.CreatePublicationInput{
		Body: sthv2.PublicationCreate{Manifest: manifest},
	})
	if err != nil {
		return Publication{}, fmt.Errorf("creating publication: %w", err)
	}
	if err := validatePublicationResponse(publication, manifest, expectedHash, false); err != nil {
		return Publication{}, fmt.Errorf("creating publication: invalid response: %w", err)
	}
	return publication, nil
}

func CreatePackagePublicationUploads(ctx context.Context, httpClient *stdhttp.Client, publication Publication) ([]BlobUpload, error) {
	if !validCanonicalUUID(publication.ID) {
		return nil, fmt.Errorf("publication uploads: invalid identifier")
	}
	response, err := newGeneratedV2Client(httpClient).CreatePublicationUploads(ctx, sthv2.CreatePublicationUploadsInput{PublicationID: publication.ID})
	if err != nil {
		return nil, fmt.Errorf("publication uploads: %w", err)
	}
	if len(response.Uploads) != len(publication.Manifest.Files) {
		return nil, fmt.Errorf("publication uploads: incomplete mapping")
	}
	files := make(map[string]PublicationFile, len(publication.Manifest.Files))
	for _, file := range publication.Manifest.Files {
		files[file.Path] = file
	}
	seen := make(map[string]struct{}, len(response.Uploads))
	for _, upload := range response.Uploads {
		file, ok := files[upload.Path]
		if !ok || upload.SHA256 != file.SHA256 {
			return nil, fmt.Errorf("publication uploads: inconsistent mapping")
		}
		if _, duplicate := seen[upload.Path]; duplicate {
			return nil, fmt.Errorf("publication uploads: duplicate path")
		}
		seen[upload.Path] = struct{}{}
		if !validSignedDownloadURL(upload.UploadURL) || !signedDownloadMatchesCloudOrigin(upload.UploadURL) {
			return nil, fmt.Errorf("publication uploads: invalid signed URL")
		}
		expiresAt, parseErr := time.Parse(time.RFC3339, upload.ExpiresAt)
		if parseErr != nil || !expiresAt.After(time.Now()) {
			return nil, fmt.Errorf("publication uploads: invalid expiration")
		}
	}
	return response.Uploads, nil
}

func UploadPackagePublicationFiles(
	ctx context.Context,
	httpClient *stdhttp.Client,
	manifest PublicationManifestInput,
	uploads []BlobUpload,
	contents map[string][]byte,
) error {
	if err := validatePublicationManifestInput(manifest); err != nil {
		return err
	}
	if len(contents) != len(manifest.Files) || len(uploads) != len(manifest.Files) {
		return fmt.Errorf("publication upload: incomplete content")
	}
	uploadByPath := make(map[string]BlobUpload, len(uploads))
	for _, upload := range uploads {
		if _, duplicate := uploadByPath[upload.Path]; duplicate {
			return fmt.Errorf("publication upload: duplicate path")
		}
		uploadByPath[upload.Path] = upload
	}
	for _, file := range manifest.Files {
		content, ok := contents[file.Path]
		upload, uploadOK := uploadByPath[file.Path]
		if !ok || !uploadOK || int64(len(content)) != file.Size || upload.SHA256 != file.SHA256 || sha256Hex(content) != file.SHA256 {
			return fmt.Errorf("publication upload: inconsistent content or mapping for %s", file.Path)
		}
		if !validSignedDownloadURL(upload.UploadURL) || !signedDownloadMatchesCloudOrigin(upload.UploadURL) {
			return fmt.Errorf("publication upload: invalid signed URL for %s", file.Path)
		}
		expiresAt, err := time.Parse(time.RFC3339, upload.ExpiresAt)
		if err != nil || !expiresAt.After(time.Now()) {
			return fmt.Errorf("publication upload: expired URL for %s", file.Path)
		}
	}

	client := noRedirectClient(httpClient)
	for _, file := range manifest.Files {
		upload := uploadByPath[file.Path]
		request, err := stdhttp.NewRequestWithContext(ctx, stdhttp.MethodPut, upload.UploadURL, bytes.NewReader(contents[file.Path]))
		if err != nil {
			return fmt.Errorf("publication upload: invalid request for %s", file.Path)
		}
		request.Header.Set("Content-Type", file.MIME)
		response, err := client.Do(request)
		if err != nil {
			return fmt.Errorf("publication upload failed for %s: %w", file.Path, err)
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64*1024))
		response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return fmt.Errorf("publication upload: HTTP %d for %s", response.StatusCode, file.Path)
		}
	}
	return nil
}

func FinalizePackagePublication(
	ctx context.Context,
	httpClient *stdhttp.Client,
	publication Publication,
) (Publication, error) {
	expectedHash, err := CanonicalPublicationManifestSHA256(publication.Manifest)
	if err != nil {
		return Publication{}, err
	}
	finalized, err := newGeneratedV2Client(httpClient).FinalizePublication(ctx, sthv2.FinalizePublicationInput{PublicationID: publication.ID})
	if err != nil {
		return Publication{}, fmt.Errorf("finalizing publication: %w", err)
	}
	if err := validatePublicationResponse(finalized, publication.Manifest, expectedHash, true); err != nil {
		return Publication{}, fmt.Errorf("finalizing publication: invalid response: %w", err)
	}
	if finalized.ID != publication.ID || finalized.AssetID != publication.AssetID {
		return Publication{}, fmt.Errorf("finalizing publication: inconsistent identity")
	}
	return finalized, nil
}

func validatePublicationManifestInput(manifest PublicationManifestInput) error {
	asset := manifest.Asset
	if asset.Kind != "skill" || !validPackageComponent(asset.Slug) || !validPackageComponent(asset.FolderName) {
		return fmt.Errorf("publication manifest: invalid asset")
	}
	if !validHumanText(asset.Name, 200) || (asset.Description != nil && !validHumanText(*asset.Description, 2000)) {
		return fmt.Errorf("publication manifest: invalid name or description")
	}
	if !validPublicationVersion(manifest.Version) || !validRepositoryURL(manifest.Origin.RepositoryURL) ||
		!validManifestRef(manifest.Origin.Ref) || !validGitCommit(manifest.Origin.Commit) {
		return fmt.Errorf("publication manifest: invalid origin or version")
	}
	if manifest.Entrypoint != "SKILL.md" || len(manifest.Files) < 1 || len(manifest.Files) > maxCloudPackageFiles || !validCompatibility(manifest.Compatibility) {
		return fmt.Errorf("publication manifest: invalid content")
	}
	seen := make(map[string]struct{}, len(manifest.Files))
	total := int64(0)
	entrypointFound := false
	for _, file := range manifest.Files {
		if !validLogicalAssetPath(file.Path) || !validSHA256(file.SHA256) || file.Size < 0 || file.Size > maxCloudPackageFileBytes || !validMIME(file.MIME) {
			return fmt.Errorf("publication manifest: invalid file")
		}
		if total > maxCloudPackageBytes-file.Size {
			return fmt.Errorf("publication manifest: total size exceeded")
		}
		total += file.Size
		if _, duplicate := seen[file.Path]; duplicate {
			return fmt.Errorf("publication manifest: duplicate path")
		}
		seen[file.Path] = struct{}{}
		entrypointFound = entrypointFound || file.Path == manifest.Entrypoint
	}
	if !entrypointFound {
		return fmt.Errorf("publication manifest: SKILL.md is missing")
	}
	return nil
}

func validatePublicationResponse(
	publication Publication,
	expectedManifest PublicationManifestInput,
	expectedManifestHash string,
	finalized bool,
) error {
	if !validCanonicalUUID(publication.ID) || publication.AssetID != expectedAssetID(expectedManifest) {
		return fmt.Errorf("invalid identity")
	}
	manifestHash, err := CanonicalPublicationManifestSHA256(publication.Manifest)
	if err != nil || manifestHash != expectedManifestHash {
		return fmt.Errorf("inconsistent returned manifest")
	}
	if _, err := time.Parse(time.RFC3339, publication.CreatedAt); err != nil {
		return fmt.Errorf("invalid created_at")
	}
	if !finalized {
		if publication.Status != "draft" || publication.VersionID != nil || publication.FinalizedAt != nil {
			return fmt.Errorf("invalid initial state")
		}
		return nil
	}
	if publication.Status != "published" || publication.VersionID == nil || !validCanonicalUUID(*publication.VersionID) || publication.FinalizedAt == nil {
		return fmt.Errorf("invalid final state")
	}
	if _, err := time.Parse(time.RFC3339, *publication.FinalizedAt); err != nil {
		return fmt.Errorf("invalid finalized_at")
	}
	return nil
}

func expectedAssetID(manifest PublicationManifestInput) string {
	return manifest.Asset.Kind + "::" + manifest.Asset.FolderName + "::" + manifest.Asset.Slug
}

func validPackageComponent(value string) bool {
	if value == "" || len(value) > 128 || !asciiLowerAlphaNumeric(value[0]) || !asciiLowerAlphaNumeric(value[len(value)-1]) {
		return false
	}
	if len(value) == 1 {
		return true
	}
	for _, character := range value[1 : len(value)-1] {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '.' && character != '_' && character != '-' {
			return false
		}
	}
	return true
}

func asciiLowerAlphaNumeric(value byte) bool {
	return (value >= 'a' && value <= 'z') || (value >= '0' && value <= '9')
}

func validPublicationVersion(value string) bool {
	if value == "" || len(value) > 128 || strings.TrimSpace(value) != value {
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

func validHumanText(value string, maximum int) bool {
	if strings.TrimSpace(value) == "" || len(value) > maximum || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validCanonicalUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	if value[14] < '1' || value[14] > '5' || !strings.ContainsRune("89ab", rune(value[19])) {
		return false
	}
	for index, character := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			continue
		}
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func sha256Hex(content []byte) string {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}
