package fs

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
)

func TestVerifyReleaseEngineManifestRejectsSignedReplayAnnouncedAsNewer(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey: %v", err)
	}
	t.Setenv("STH_RELEASE_SIGNING_PUBKEY", base64.StdEncoding.EncodeToString(publicKey))
	manifestData := []byte("{\"schema_version\":1,\"engine_version\":\"1.2.3\",\"bridge_protocol\":1,\"contract\":\"sth-code-bridge-v1\"}\n")
	manifestChecksum := sha256.Sum256(manifestData)
	checksumManifest := []byte(fmt.Sprintf("%s  %s\n", hex.EncodeToString(manifestChecksum[:]), ReleaseEngineManifestName))
	checksumSignature := []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, checksumManifest)))

	err = VerifyReleaseEngineManifest("v9.9.9", manifestData, checksumManifest, checksumSignature)
	if err == nil || !strings.Contains(err.Error(), "does not match signed engine version") {
		t.Fatalf("VerifyReleaseEngineManifest() error = %v", err)
	}
}

func TestVerifyReleaseEngineManifestAcceptsMatchingSignedVersion(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey: %v", err)
	}
	t.Setenv("STH_RELEASE_SIGNING_PUBKEY", base64.StdEncoding.EncodeToString(publicKey))
	manifestData := []byte("{\"schema_version\":1,\"engine_version\":\"1.2.3\",\"bridge_protocol\":1,\"contract\":\"sth-code-bridge-v1\"}\n")
	manifestChecksum := sha256.Sum256(manifestData)
	checksumManifest := []byte(fmt.Sprintf("%s  %s\n", hex.EncodeToString(manifestChecksum[:]), ReleaseEngineManifestName))
	checksumSignature := []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, checksumManifest)))

	if err := VerifyReleaseEngineManifest("v1.2.3", manifestData, checksumManifest, checksumSignature); err != nil {
		t.Fatalf("VerifyReleaseEngineManifest() error = %v", err)
	}
}
