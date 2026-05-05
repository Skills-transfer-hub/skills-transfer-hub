// Package auth wraps OS keychain access for the STH Cloud provider.
package auth

import (
	"errors"
	"fmt"
	"os"

	"github.com/zalando/go-keyring"
)

const (
	// KeyringService is the constant identifier under which the API key is
	// stored in the OS keychain (Windows Credential Manager, macOS Keychain,
	// libsecret on Linux).
	KeyringService = "sth-cloud"
	defaultAccount = "default"

	// EnvAPIKey is the environment variable consulted as a fallback when no
	// keychain entry is found. Useful for CI and headless environments.
	EnvAPIKey = "STH_CLOUD_API_KEY"
)

// ErrKeyNotFound is returned when no API key is configured (keychain miss + env unset).
var ErrKeyNotFound = errors.New("aucune clé STH Cloud configurée — exécute `sth login`")

// SaveAPIKey persists the key to the OS keychain under (service=sth-cloud, user=default).
func SaveAPIKey(key string) error {
	if key == "" {
		return errors.New("clé vide")
	}
	if err := keyring.Set(KeyringService, defaultAccount, key); err != nil {
		return fmt.Errorf("écriture keychain échouée : %w", err)
	}
	return nil
}

// LoadAPIKey reads the key from the keychain, falling back to STH_CLOUD_API_KEY
// if the keychain has no entry. Returns ErrKeyNotFound if neither source has a value.
func LoadAPIKey() (string, error) {
	key, err := keyring.Get(KeyringService, defaultAccount)
	if err == nil && key != "" {
		return key, nil
	}
	if env := os.Getenv(EnvAPIKey); env != "" {
		return env, nil
	}
	if errors.Is(err, keyring.ErrNotFound) || err == nil {
		return "", ErrKeyNotFound
	}
	return "", fmt.Errorf("lecture keychain échouée : %w", err)
}

// DeleteAPIKey removes the keychain entry. Idempotent — returns nil if the
// entry doesn't exist.
func DeleteAPIKey() error {
	err := keyring.Delete(KeyringService, defaultAccount)
	if err == nil || errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return fmt.Errorf("suppression keychain échouée : %w", err)
}
