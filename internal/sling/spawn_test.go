package sling

import (
	"testing"

	"github.com/steveyegge/gastown/internal/config"
)

func TestGetSessionPane_ReturnsSessionName(t *testing.T) {
	// In K8s/coop mode, GetSessionPane returns the session name directly.
	pane, err := GetSessionPane("gt-test-session")
	if err != nil {
		t.Fatalf("GetSessionPane() unexpected error: %v", err)
	}
	if pane != "gt-test-session" {
		t.Errorf("GetSessionPane() = %q, want %q", pane, "gt-test-session")
	}
}

func TestResolveExecutionTarget_DefaultsLocal(t *testing.T) {
	// Without K8s env var, should default to local.
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	got := ResolveExecutionTarget("/nonexistent/rig", "")
	if got != config.ExecutionTargetLocal {
		t.Errorf("ResolveExecutionTarget() = %q, want %q", got, config.ExecutionTargetLocal)
	}
}

func TestResolveExecutionTarget_AutoDetectsK8s(t *testing.T) {
	// With KUBERNETES_SERVICE_HOST set (as in any K8s pod), should default to k8s.
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.96.0.1")
	got := ResolveExecutionTarget("/nonexistent/rig", "")
	if got != config.ExecutionTargetK8s {
		t.Errorf("ResolveExecutionTarget() = %q, want %q", got, config.ExecutionTargetK8s)
	}
}

func TestResolveExecutionTarget_OverrideTakesPrecedence(t *testing.T) {
	// Explicit override should win over K8s auto-detection.
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.96.0.1")
	got := ResolveExecutionTarget("/nonexistent/rig", "local")
	if got != config.ExecutionTargetLocal {
		t.Errorf("ResolveExecutionTarget() = %q, want %q", got, config.ExecutionTargetLocal)
	}
}

func TestGetSessionPane_Exported(t *testing.T) {
	// Verify GetSessionPane is exported and callable from external packages.
	// This is a compile-time check — the function signature is:
	//   func GetSessionPane(sessionName string) (string, error)
	var fn func(string) (string, error) = GetSessionPane
	if fn == nil {
		t.Fatal("GetSessionPane should be non-nil")
	}
}
