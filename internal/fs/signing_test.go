package fs

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"
)

func TestVerifyChecksumSignature(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey: %v", err)
	}
	manifest := []byte("abc123  sth-linux-amd64.tar.gz\n")
	signature := []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, manifest)))

	t.Run("valid signature passes", func(t *testing.T) {
		t.Setenv("STH_RELEASE_SIGNING_PUBKEY", base64.StdEncoding.EncodeToString(publicKey))
		if err := verifyChecksumSignature(manifest, signature); err != nil {
			t.Fatalf("expected valid signature, got %v", err)
		}
	})

	t.Run("tampered manifest fails", func(t *testing.T) {
		t.Setenv("STH_RELEASE_SIGNING_PUBKEY", base64.StdEncoding.EncodeToString(publicKey))
		if err := verifyChecksumSignature([]byte("tampered  sth-linux-amd64.tar.gz\n"), signature); err == nil {
			t.Fatal("expected error for tampered manifest")
		}
	})

	t.Run("wrong key fails", func(t *testing.T) {
		otherPublicKey, _, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatalf("ed25519.GenerateKey: %v", err)
		}
		t.Setenv("STH_RELEASE_SIGNING_PUBKEY", base64.StdEncoding.EncodeToString(otherPublicKey))
		if err := verifyChecksumSignature(manifest, signature); err == nil {
			t.Fatal("expected error for wrong key")
		}
	})

	t.Run("unconfigured key fails", func(t *testing.T) {
		t.Setenv("STH_RELEASE_SIGNING_PUBKEY", "")
		if err := verifyChecksumSignature(manifest, signature); err == nil {
			t.Fatal("expected error when signing key is not configured")
		}
	})
}
