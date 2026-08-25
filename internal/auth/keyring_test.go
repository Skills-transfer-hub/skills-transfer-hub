package auth

import (
	"errors"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestLoadAPIKeyReportsEnglishRecoveryMessage(t *testing.T) {
	keyring.MockInit()
	t.Setenv(EnvAPIKey, "")

	_, err := LoadAPIKey()
	if !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("LoadAPIKey() error = %v, want ErrKeyNotFound", err)
	}
	if err.Error() != "no STH Cloud key is configured — run `sth login`" {
		t.Fatalf("LoadAPIKey() error = %q", err)
	}
}

func TestSaveAPIKeyReportsEnglishEmptyKeyMessage(t *testing.T) {
	err := SaveAPIKey("")
	if err == nil || err.Error() != "key is empty" {
		t.Fatalf("SaveAPIKey() error = %v", err)
	}
}

func TestSaveAPIKeyReportsEnglishKeychainFailure(t *testing.T) {
	backendErr := errors.New("backend unavailable")
	keyring.MockInitWithError(backendErr)

	err := SaveAPIKey("lsk_live_test")
	if !errors.Is(err, backendErr) {
		t.Fatalf("SaveAPIKey() error = %v, want wrapped backend error", err)
	}
	if err.Error() != "failed to write to the OS keychain: backend unavailable" {
		t.Fatalf("SaveAPIKey() error = %q", err)
	}
}

func TestDeleteAPIKeyReportsEnglishKeychainFailure(t *testing.T) {
	backendErr := errors.New("backend unavailable")
	keyring.MockInitWithError(backendErr)

	err := DeleteAPIKey()
	if !errors.Is(err, backendErr) {
		t.Fatalf("DeleteAPIKey() error = %v, want wrapped backend error", err)
	}
	if err.Error() != "failed to remove the key from the OS keychain: backend unavailable" {
		t.Fatalf("DeleteAPIKey() error = %q", err)
	}
}
