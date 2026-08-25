package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"strings"

	"sth/internal/fs"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	genKey := flag.Bool("genkey", false, "generate an ed25519 keypair and print the secret and public key")
	verify := flag.Bool("verify", false, "verify an ed25519 signature with the release public key")
	keyEnv := flag.String("key-env", "STH_RELEASE_SIGNING_KEY", "environment variable holding the base64 ed25519 private key")
	inputPath := flag.String("in", "", "path of the checksum manifest to sign")
	outputPath := flag.String("out", "", "path of the detached signature to write")
	signaturePath := flag.String("signature", "", "path of the detached signature to verify")
	flag.Parse()

	if *genKey {
		return generateKeyPair()
	}
	if *verify {
		return verifyManifest(*inputPath, *signaturePath)
	}
	return signManifest(*keyEnv, *inputPath, *outputPath)
}

func generateKeyPair() error {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	fmt.Printf("STH_RELEASE_SIGNING_KEY=%s\n", base64.StdEncoding.EncodeToString(privateKey))
	fmt.Printf("releaseSigningPublicKeyBase64=%s\n", base64.StdEncoding.EncodeToString(publicKey))
	return nil
}

func signManifest(keyEnv, inputPath, outputPath string) error {
	if strings.TrimSpace(inputPath) == "" || strings.TrimSpace(outputPath) == "" {
		return fmt.Errorf("--in and --out are required")
	}
	encodedKey := strings.TrimSpace(os.Getenv(keyEnv))
	if encodedKey == "" {
		return fmt.Errorf("environment variable %s is missing or empty", keyEnv)
	}
	privateKeyBytes, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil {
		return fmt.Errorf("unable to decode private key: %w", err)
	}
	if len(privateKeyBytes) != ed25519.PrivateKeySize {
		return fmt.Errorf("invalid private key size: %d bytes", len(privateKeyBytes))
	}
	manifest, err := os.ReadFile(inputPath)
	if err != nil {
		return err
	}
	signature := ed25519.Sign(ed25519.PrivateKey(privateKeyBytes), manifest)
	return os.WriteFile(outputPath, []byte(base64.StdEncoding.EncodeToString(signature)+"\n"), 0o600)
}

func verifyManifest(inputPath, signaturePath string) error {
	if strings.TrimSpace(inputPath) == "" || strings.TrimSpace(signaturePath) == "" {
		return fmt.Errorf("--in and --signature are required")
	}
	manifest, err := os.ReadFile(inputPath)
	if err != nil {
		return err
	}
	signature, err := os.ReadFile(signaturePath)
	if err != nil {
		return err
	}
	return fs.VerifyChecksumSignature(manifest, signature)
}
