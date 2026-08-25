package fs

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

const ReleaseEngineManifestName = "sth-code-engine.json"

type releaseEngineManifest struct {
	SchemaVersion  int    `json:"schema_version"`
	EngineVersion  string `json:"engine_version"`
	BridgeProtocol int    `json:"bridge_protocol"`
	Contract       string `json:"contract"`
}

func VerifyReleaseEngineManifest(releaseVersion string, manifestData, checksumManifest, checksumSignature []byte) error {
	if err := verifyChecksumSignature(checksumManifest, checksumSignature); err != nil {
		return err
	}
	expectedChecksum, err := checksumForAsset(checksumManifest, ReleaseEngineManifestName)
	if err != nil {
		return err
	}
	actualChecksum := sha256.Sum256(manifestData)
	if strings.ToLower(expectedChecksum) != hex.EncodeToString(actualChecksum[:]) {
		return fmt.Errorf("invalid checksum for %s", ReleaseEngineManifestName)
	}
	var manifest releaseEngineManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return fmt.Errorf("invalid %s: %w", ReleaseEngineManifestName, err)
	}
	if manifest.SchemaVersion != 1 || manifest.BridgeProtocol != 1 || manifest.Contract != "sth-code-bridge-v1" {
		return fmt.Errorf("invalid %s contract", ReleaseEngineManifestName)
	}
	expectedVersion := strings.TrimPrefix(strings.TrimSpace(releaseVersion), "v")
	if strings.TrimSpace(manifest.EngineVersion) != expectedVersion {
		return fmt.Errorf("release version %s does not match signed engine version %s", expectedVersion, strings.TrimSpace(manifest.EngineVersion))
	}
	return nil
}
