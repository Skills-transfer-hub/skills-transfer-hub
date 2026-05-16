package cmd

import (
	"context"
	"strings"
	"testing"
)

// On dev builds (no release metadata baked in) the Pro gate is intentionally
// inert — same policy as the alpha gate — so `go test ./...` never hits a
// gated error path. The matching production code path is the
// "STH_PRO_GATE=0" escape hatch, which we exercise below as a documented
// bypass for power users.
func TestEnforceProGate_BypassesOnDevBuild(t *testing.T) {
	a := &app{}
	if err := a.enforceProGate(context.Background()); err != nil {
		t.Errorf("dev build should bypass Pro gate, got %v", err)
	}
}

func TestEnforceProGate_EnvVarBypass(t *testing.T) {
	t.Setenv(proGateSkipEnvVar, "0")
	a := &app{}
	if err := a.enforceProGate(context.Background()); err != nil {
		t.Errorf("STH_PRO_GATE=0 should bypass, got %v", err)
	}
}

// proGateNotEntitledError is the message users see when they hit the gate on
// a release build without an active entitlement. The wording is part of the
// product contract — keep it stable so docs / Pro upsell can quote it.
func TestProGateNotEntitledError_MentionsEnvVarAndDocs(t *testing.T) {
	err := proGateNotEntitledError()
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	msg := err.Error()
	if !strings.Contains(msg, proGateSkipEnvVar) {
		t.Errorf("error should mention the bypass env var %q, got %q", proGateSkipEnvVar, msg)
	}
	if !strings.Contains(msg, "Pro") {
		t.Errorf("error should mention Pro, got %q", msg)
	}
}
