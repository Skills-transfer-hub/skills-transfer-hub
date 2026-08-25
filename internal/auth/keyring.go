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
var ErrKeyNotFound = errors.New("no STH Cloud key is configured — run `sth login`")

// SaveAPIKey persists the key to the OS keychain under (service=sth-cloud, user=default).
func SaveAPIKey(key string) error {
	if key == "" {
		return errors.New("key is empty")
	}
	if err := keyring.Set(KeyringService, defaultAccount, key); err != nil {
		return fmt.Errorf("failed to write to the OS keychain: %w", err)
	}
	return nil
}

// LoadAPIKey reads the key from the keychain, falling back to STH_CLOUD_API_KEY
// if the keychain has no entry or is unreachable. Returns ErrKeyNotFound when
// no key is available from either source.
//
// Any error returned by the keyring backend (not-found, missing libsecret on
// headless Linux, locked KDE wallet, etc.) is treated as "no keychain entry"
// — if the env fallback is also empty, the caller gets a single clean
// ErrKeyNotFound and the standard "run `sth login`" message. The distinction
// between OS-level keychain errors and a true miss is not actionable for the
// CLI user, who would see the same recovery path either way.
func LoadAPIKey() (string, error) {
	key, err := keyring.Get(KeyringService, defaultAccount)
	if err == nil && key != "" {
		return key, nil
	}
	if env := os.Getenv(EnvAPIKey); env != "" {
		return env, nil
	}
	return "", ErrKeyNotFound
}

// DeleteAPIKey removes the keychain entry. Idempotent — returns nil if the
// entry doesn't exist.
func DeleteAPIKey() error {
	err := keyring.Delete(KeyringService, defaultAccount)
	if err == nil || errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return fmt.Errorf("failed to remove the key from the OS keychain: %w", err)
}
