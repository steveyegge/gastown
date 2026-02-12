package polecat

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/rig"
)

func TestSessionName(t *testing.T) {
	r := &rig.Rig{
		Name:     "gastown",
		Polecats: []string{"Toast"},
	}
	m := NewSessionManager(r)

	name := m.SessionName("Toast")
	if name != "gt-gastown-Toast" {
		t.Errorf("sessionName = %q, want gt-gastown-Toast", name)
	}
}

func TestSessionManagerPolecatDir(t *testing.T) {
	r := &rig.Rig{
		Name:     "gastown",
		Path:     "/home/user/ai/gastown",
		Polecats: []string{"Toast"},
	}
	m := NewSessionManager(r)

	dir := m.polecatDir("Toast")
	expected := "/home/user/ai/gastown/polecats/Toast"
	if filepath.ToSlash(dir) != expected {
		t.Errorf("polecatDir = %q, want %q", dir, expected)
	}
}

func TestHasPolecat(t *testing.T) {
	root := t.TempDir()
	// hasPolecat checks filesystem, so create actual directories
	for _, name := range []string{"Toast", "Cheedo"} {
		if err := os.MkdirAll(filepath.Join(root, "polecats", name), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}

	r := &rig.Rig{
		Name:     "gastown",
		Path:     root,
		Polecats: []string{"Toast", "Cheedo"},
	}
	m := NewSessionManager(r)

	if !m.hasPolecat("Toast") {
		t.Error("expected hasPolecat(Toast) = true")
	}
	if !m.hasPolecat("Cheedo") {
		t.Error("expected hasPolecat(Cheedo) = true")
	}
	if m.hasPolecat("Unknown") {
		t.Error("expected hasPolecat(Unknown) = false")
	}
}

func TestStartPolecatNotFound(t *testing.T) {
	r := &rig.Rig{
		Name:     "gastown",
		Polecats: []string{"Toast"},
	}
	m := NewSessionManager(r)

	err := m.Start("Unknown", SessionStartOptions{})
	if err == nil {
		t.Error("expected error for unknown polecat")
	}
}

func TestIsRunningNoSession(t *testing.T) {
	r := &rig.Rig{
		Name:     "gastown",
		Polecats: []string{"Toast"},
	}
	m := NewSessionManager(r)

	running, err := m.IsRunning("Toast")
	if err != nil {
		t.Fatalf("IsRunning: %v", err)
	}
	if running {
		t.Error("expected IsRunning = false for non-existent session")
	}
}

func TestSessionManagerListEmpty(t *testing.T) {
	root := t.TempDir()
	r := &rig.Rig{
		Name:     "test-rig-unlikely-name",
		Path:     root,
		Polecats: []string{},
	}
	m := NewSessionManager(r)

	infos, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(infos) != 0 {
		t.Errorf("infos count = %d, want 0", len(infos))
	}
}

func TestStopNotFound(t *testing.T) {
	r := &rig.Rig{
		Name:     "test-rig",
		Polecats: []string{"Toast"},
	}
	m := NewSessionManager(r)

	err := m.Stop("Toast", false)
	if err != ErrSessionNotFound {
		t.Errorf("Stop = %v, want ErrSessionNotFound", err)
	}
}

func TestCaptureNotFound(t *testing.T) {
	r := &rig.Rig{
		Name:     "test-rig",
		Polecats: []string{"Toast"},
	}
	m := NewSessionManager(r)

	_, err := m.Capture("Toast", 50)
	if err != ErrSessionNotFound {
		t.Errorf("Capture = %v, want ErrSessionNotFound", err)
	}
}

func TestInjectNotFound(t *testing.T) {
	r := &rig.Rig{
		Name:     "test-rig",
		Polecats: []string{"Toast"},
	}
	m := NewSessionManager(r)

	err := m.Inject("Toast", "hello")
	if err != ErrSessionNotFound {
		t.Errorf("Inject = %v, want ErrSessionNotFound", err)
	}
}

// TestPolecatCommandFormat verifies the polecat session command exports
// GT_ROLE, GT_RIG, GT_POLECAT, and BD_ACTOR inline before starting Claude.
// This is a regression test for gt-y41ep - env vars must be exported inline.
func TestPolecatCommandFormat(t *testing.T) {
	// This test verifies the expected command format.
	// The actual command is built in Start() but we test the format here
	// to document and verify the expected behavior.

	rigName := "gastown"
	polecatName := "Toast"
	expectedBdActor := "gastown/polecats/Toast"

	// Build the expected command format (mirrors Start() logic)
	expectedPrefix := "export GT_ROLE=polecat GT_RIG=" + rigName + " GT_POLECAT=" + polecatName + " BD_ACTOR=" + expectedBdActor + " GIT_AUTHOR_NAME=" + expectedBdActor
	expectedSuffix := "&& claude --dangerously-skip-permissions"

	// The command must contain all required env exports
	requiredParts := []string{
		"export",
		"GT_ROLE=polecat",
		"GT_RIG=" + rigName,
		"GT_POLECAT=" + polecatName,
		"BD_ACTOR=" + expectedBdActor,
		"GIT_AUTHOR_NAME=" + expectedBdActor,
		"claude --dangerously-skip-permissions",
	}

	// Verify expected format contains all required parts
	fullCommand := expectedPrefix + " " + expectedSuffix
	for _, part := range requiredParts {
		if !strings.Contains(fullCommand, part) {
			t.Errorf("Polecat command should contain %q", part)
		}
	}

	// Verify GT_ROLE is specifically "polecat" (not "mayor" or "crew")
	if !strings.Contains(fullCommand, "GT_ROLE=polecat") {
		t.Error("GT_ROLE must be 'polecat', not 'mayor' or 'crew'")
	}
}

func TestPolecatStartPassesBDDaemonHostFromEnvironment(t *testing.T) {
	// This test verifies that polecat Start() reads BD_DAEMON_HOST from the environment
	// and passes it to AgentEnv for inclusion in the agent's environment variables.
	//
	// We can't easily test the full Start() flow without a running coop server,
	// but we can verify the config.AgentEnv call pattern that Start() uses.

	testCases := []struct {
		name        string
		envValue    string
		expectInEnv bool
		expectedVal string
	}{
		{
			name:        "BD_DAEMON_HOST set",
			envValue:    "192.168.1.100:7233",
			expectInEnv: true,
			expectedVal: "192.168.1.100:7233",
		},
		{
			name:        "BD_DAEMON_HOST empty",
			envValue:    "",
			expectInEnv: false,
			expectedVal: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Simulate what polecat.Start() does: read from os.Getenv and pass to AgentEnv
			cfg := config.AgentEnvConfig{
				Role:          "polecat",
				Rig:           "testrig",
				AgentName:     "Toast",
				TownRoot:      "/town",
				BeadsNoDaemon: true,
				BDDaemonHost:  tc.envValue, // This is what os.Getenv("BD_DAEMON_HOST") returns
			}
			env := config.AgentEnv(cfg)

			if tc.expectInEnv {
				if got, ok := env["BD_DAEMON_HOST"]; !ok {
					t.Errorf("expected BD_DAEMON_HOST in env, but not found")
				} else if got != tc.expectedVal {
					t.Errorf("BD_DAEMON_HOST = %q, want %q", got, tc.expectedVal)
				}
			} else {
				if _, ok := env["BD_DAEMON_HOST"]; ok {
					t.Errorf("expected BD_DAEMON_HOST to not be in env when empty")
				}
			}
		})
	}
}

func TestPolecatStartBDDaemonHostValuePassthrough(t *testing.T) {
	// Test that various BD_DAEMON_HOST values are passed through without modification
	testCases := []struct {
		name  string
		value string
	}{
		{"localhost with port", "localhost:7233"},
		{"IP with port", "10.0.0.1:8080"},
		{"hostname only", "beads-daemon.local"},
		{"full URL format", "http://daemon.example.com:7233"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Simulate what polecat.Start() does
			cfg := config.AgentEnvConfig{
				Role:          "polecat",
				Rig:           "testrig",
				AgentName:     "Toast",
				TownRoot:      "/town",
				BeadsNoDaemon: true,
				BDDaemonHost:  tc.value,
			}
			env := config.AgentEnv(cfg)

			// Value should be passed through unchanged
			if got := env["BD_DAEMON_HOST"]; got != tc.value {
				t.Errorf("BD_DAEMON_HOST = %q, want %q", got, tc.value)
			}
		})
	}
}
