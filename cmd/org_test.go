package cmd

import (
	"bytes"
	"io"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sth/internal/auth"
	"sth/internal/fs"
)

func orgsServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
		if request.URL.Path != "/v1/orgs" {
			stdhttp.NotFound(writer, request)
			return
		}
		_, _ = io.WriteString(writer, body)
	}))
}

const twoOrgsBody = `{"orgs":[` +
	`{"id":"org_a","name":"Acme Corp","role":"admin","plan":"team","active":true},` +
	`{"id":"org_b","name":"Side Project","role":"owner","plan":"pro","active":false}]}`

func TestRunOrgUse_PersistsSelection(t *testing.T) {
	t.Setenv("STH_USER_CONFIG_DIR", t.TempDir())
	t.Setenv("STH_PRO_GATE", "0")
	t.Setenv(auth.EnvAPIKey, "lsk_live_TEST")
	server := orgsServer(t, twoOrgsBody)
	defer server.Close()
	t.Setenv("STH_CLOUD_BASE_URL", server.URL)

	var stdout bytes.Buffer
	application := newApp(nil, &stdout, io.Discard, nil)
	if err := application.runOrgUse([]string{"Side Project"}); err != nil {
		t.Fatalf("runOrgUse: %v", err)
	}

	active, err := fs.ReadActiveOrg()
	if err != nil {
		t.Fatalf("ReadActiveOrg: %v", err)
	}
	if active == nil || active.OrgID != "org_b" {
		t.Fatalf("sélection non persistée, got %+v", active)
	}
	if !strings.Contains(stdout.String(), "Side Project") {
		t.Fatalf("confirmation attendue, got %q", stdout.String())
	}
}

func TestRunOrgUse_NonMemberFailsAndLeavesSelection(t *testing.T) {
	t.Setenv("STH_USER_CONFIG_DIR", t.TempDir())
	t.Setenv("STH_PRO_GATE", "0")
	t.Setenv(auth.EnvAPIKey, "lsk_live_TEST")
	server := orgsServer(t, `{"orgs":[{"id":"org_a","name":"Acme Corp","role":"admin","plan":"team","active":true}]}`)
	defer server.Close()
	t.Setenv("STH_CLOUD_BASE_URL", server.URL)

	var stdout bytes.Buffer
	application := newApp(nil, &stdout, io.Discard, nil)
	if err := application.runOrgUse([]string{"org_unknown"}); err == nil {
		t.Fatal("une org non-membre doit échouer")
	}
	if active, _ := fs.ReadActiveOrg(); active != nil {
		t.Fatalf("aucune sélection ne doit être persistée, got %+v", active)
	}
}

func TestRunOrgList_MarksActiveSelection(t *testing.T) {
	t.Setenv("STH_USER_CONFIG_DIR", t.TempDir())
	t.Setenv("STH_PRO_GATE", "0")
	t.Setenv(auth.EnvAPIKey, "lsk_live_TEST")
	if err := fs.SaveActiveOrg(fs.ActiveOrgSelection{OrgID: "org_b", OrgName: "Side Project", Role: "owner", Plan: "pro"}); err != nil {
		t.Fatalf("seed active org: %v", err)
	}
	server := orgsServer(t, twoOrgsBody)
	defer server.Close()
	t.Setenv("STH_CLOUD_BASE_URL", server.URL)

	var stdout bytes.Buffer
	application := newApp(nil, &stdout, io.Discard, nil)
	if err := application.runOrgList(false); err != nil {
		t.Fatalf("runOrgList: %v", err)
	}
	if !strings.Contains(stdout.String(), "* Side Project") {
		t.Fatalf("l'org active doit être marquée d'une étoile, got %q", stdout.String())
	}
}

func TestRunOrgCurrent_ServesCacheOffline(t *testing.T) {
	t.Setenv("STH_USER_CONFIG_DIR", t.TempDir())
	if err := fs.SaveActiveOrg(fs.ActiveOrgSelection{OrgID: "org_b", OrgName: "Side Project", Role: "owner", Plan: "pro"}); err != nil {
		t.Fatalf("seed active org: %v", err)
	}

	var stdout bytes.Buffer
	application := newApp(nil, &stdout, io.Discard, nil)
	if err := application.runOrgCurrent(false); err != nil {
		t.Fatalf("runOrgCurrent: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "Side Project") || !strings.Contains(output, "sélectionnée") {
		t.Fatalf("attendu l'org en cache marquée sélectionnée, got %q", output)
	}
}

func TestRunOrg_UnknownSubcommand(t *testing.T) {
	t.Setenv("STH_EXPERIMENTAL", "1")
	var stdout bytes.Buffer
	application := newApp(nil, &stdout, io.Discard, nil)
	if err := application.runOrg([]string{"bogus"}); err == nil {
		t.Fatal("attendu une erreur pour un sous-commande org inconnue")
	}
}

func TestRunOrg_BlockedWithoutExperimentalFlag(t *testing.T) {
	t.Setenv("STH_EXPERIMENTAL", "")
	var stdout bytes.Buffer
	application := newApp(nil, &stdout, io.Discard, nil)
	err := application.runOrg([]string{"current"})
	if err == nil {
		t.Fatal("sth org doit être bloqué sans STH_EXPERIMENTAL")
	}
	if !strings.Contains(err.Error(), "préversion") {
		t.Fatalf("erreur de préversion attendue, got %v", err)
	}
}
