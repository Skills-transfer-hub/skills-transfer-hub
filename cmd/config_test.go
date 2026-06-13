package cmd

import (
	"bytes"
	"io"
	stdhttp "net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"sth/internal/auth"
	"sth/internal/fs"
)

func chdirTemp(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
}

const orgConfigBody = `{"config":{"schema_version":2,` +
	`"providers":[{"id":"acme","provider":"github","catalog_ref":"release/1.x"}],` +
	`"resources":[]},"config_version":3}`

func TestRunConfigPull_DryRunWritesNothing(t *testing.T) {
	chdirTemp(t)
	t.Setenv("STH_USER_CONFIG_DIR", t.TempDir())
	t.Setenv(auth.EnvAPIKey, "lsk_live_TEST")
	server := httptest.NewServer(stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
		if request.Method == stdhttp.MethodGet && request.URL.Path == "/v1/org-config" {
			_, _ = io.WriteString(writer, orgConfigBody)
			return
		}
		stdhttp.NotFound(writer, request)
	}))
	defer server.Close()
	t.Setenv("STH_CLOUD_BASE_URL", server.URL)

	var stdout bytes.Buffer
	application := newApp(nil, &stdout, io.Discard, nil)
	if err := application.runConfigPull(true); err != nil {
		t.Fatalf("runConfigPull: %v", err)
	}

	written, err := fs.ReadProjectConfigV2()
	if err != nil {
		t.Fatalf("ReadProjectConfigV2: %v", err)
	}
	if len(written.Providers) != 0 {
		t.Fatalf("--dry-run ne doit rien écrire, providers = %+v", written.Providers)
	}
	if !strings.Contains(stdout.String(), "Aucune écriture") {
		t.Fatalf("attendu la mention dry-run, got %q", stdout.String())
	}
}

func TestRunConfigPull_AppliesOrgConfig(t *testing.T) {
	chdirTemp(t)
	t.Setenv("STH_USER_CONFIG_DIR", t.TempDir())
	t.Setenv(auth.EnvAPIKey, "lsk_live_TEST")
	server := httptest.NewServer(stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
		if request.Method == stdhttp.MethodGet && request.URL.Path == "/v1/org-config" {
			_, _ = io.WriteString(writer, orgConfigBody)
			return
		}
		stdhttp.NotFound(writer, request)
	}))
	defer server.Close()
	t.Setenv("STH_CLOUD_BASE_URL", server.URL)

	var stdout bytes.Buffer
	application := newApp(nil, &stdout, io.Discard, nil)
	if err := application.runConfigPull(false); err != nil {
		t.Fatalf("runConfigPull: %v", err)
	}

	written, err := fs.ReadProjectConfigV2()
	if err != nil {
		t.Fatalf("ReadProjectConfigV2: %v", err)
	}
	provider, found := providerByIDInProject(written, "acme")
	if !found || provider.CatalogRef != "release/1.x" {
		t.Fatalf("la config org doit être appliquée localement, got %+v", written.Providers)
	}
}

func TestRunConfigPull_EmptyConfigDegrades(t *testing.T) {
	chdirTemp(t)
	t.Setenv("STH_USER_CONFIG_DIR", t.TempDir())
	t.Setenv(auth.EnvAPIKey, "lsk_live_TEST")
	server := httptest.NewServer(stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
		_, _ = io.WriteString(writer, `{"config":null,"config_version":0}`)
	}))
	defer server.Close()
	t.Setenv("STH_CLOUD_BASE_URL", server.URL)

	var stdout bytes.Buffer
	application := newApp(nil, &stdout, io.Discard, nil)
	if err := application.runConfigPull(false); err != nil {
		t.Fatalf("runConfigPull doit dégrader proprement (exit 0), got %v", err)
	}
	if !strings.Contains(stdout.String(), "Aucune configuration partagée") {
		t.Fatalf("attendu le message d'absence de config, got %q", stdout.String())
	}
}

func TestRunConfigPush_SendsVersionAndPrints(t *testing.T) {
	chdirTemp(t)
	t.Setenv("STH_USER_CONFIG_DIR", t.TempDir())
	t.Setenv("STH_PRO_GATE", "0")
	t.Setenv(auth.EnvAPIKey, "lsk_live_TEST")
	var sentIfMatch string
	server := httptest.NewServer(stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
		switch request.Method {
		case stdhttp.MethodGet:
			_, _ = io.WriteString(writer, `{"config":null,"config_version":3}`)
		case stdhttp.MethodPut:
			sentIfMatch = request.Header.Get("If-Match")
			_, _ = io.WriteString(writer, `{"config_version":4}`)
		default:
			stdhttp.NotFound(writer, request)
		}
	}))
	defer server.Close()
	t.Setenv("STH_CLOUD_BASE_URL", server.URL)

	var stdout bytes.Buffer
	application := newApp(nil, &stdout, io.Discard, nil)
	if err := application.runConfigPush(false); err != nil {
		t.Fatalf("runConfigPush: %v", err)
	}
	if sentIfMatch != "3" {
		t.Fatalf("If-Match doit valoir la version courante, got %q", sentIfMatch)
	}
	if !strings.Contains(stdout.String(), "version 4") {
		t.Fatalf("attendu la nouvelle version, got %q", stdout.String())
	}
}

func TestRunConfigPush_VersionConflict(t *testing.T) {
	chdirTemp(t)
	t.Setenv("STH_USER_CONFIG_DIR", t.TempDir())
	t.Setenv("STH_PRO_GATE", "0")
	t.Setenv(auth.EnvAPIKey, "lsk_live_TEST")
	server := httptest.NewServer(stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
		switch request.Method {
		case stdhttp.MethodGet:
			_, _ = io.WriteString(writer, `{"config":null,"config_version":3}`)
		case stdhttp.MethodPut:
			writer.WriteHeader(stdhttp.StatusConflict)
			_, _ = io.WriteString(writer, `{"error":"version_conflict"}`)
		default:
			stdhttp.NotFound(writer, request)
		}
	}))
	defer server.Close()
	t.Setenv("STH_CLOUD_BASE_URL", server.URL)

	var stdout bytes.Buffer
	application := newApp(nil, &stdout, io.Discard, nil)
	err := application.runConfigPush(false)
	if err == nil {
		t.Fatal("un conflit de version doit échouer")
	}
	if !strings.Contains(err.Error(), "pull") {
		t.Fatalf("l'erreur doit inviter à re-pull, got %v", err)
	}
}

func providerByIDInProject(cfg fs.ProjectConfigV2, id string) (fs.ProviderEntry, bool) {
	for _, provider := range cfg.Providers {
		if provider.ID == id {
			return provider, true
		}
	}
	return fs.ProviderEntry{}, false
}
