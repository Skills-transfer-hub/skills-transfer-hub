package fs

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
)

const releaseSigningPublicKeyBase64 = "WHTBjBqQkH59ZpUfEFlYdIjrimM0VXQu5J+AQpzxpE0="

func releaseSigningPublicKey() ed25519.PublicKey {
	if override := strings.TrimSpace(os.Getenv("STH_RELEASE_SIGNING_PUBKEY")); override != "" {
		return decodeReleaseSigningPublicKey(override)
	}
	return decodeReleaseSigningPublicKey(releaseSigningPublicKeyBase64)
}

func decodeReleaseSigningPublicKey(encoded string) ed25519.PublicKey {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return nil
	}
	return ed25519.PublicKey(raw)
}

func verifyChecksumSignature(checksumManifest, encodedSignature []byte) error {
	publicKey := releaseSigningPublicKey()
	if publicKey == nil {
		return errors.New("clé publique de signature de release non configurée")
	}
	signature, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(encodedSignature)))
	if err != nil {
		return fmt.Errorf("signature du manifeste de checksums illisible : %w", err)
	}
	if len(signature) != ed25519.SignatureSize {
		return errors.New("taille de signature du manifeste de checksums invalide")
	}
	if !ed25519.Verify(publicKey, checksumManifest, signature) {
		return errors.New("signature du manifeste de checksums invalide")
	}
	return nil
}
