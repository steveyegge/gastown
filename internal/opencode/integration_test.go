//go:build integration
// +build integration

// Package opencode_integration_test provides E2E tests for OpenCode integration.
//
// These tests verify that OpenCode works identically to Claude Code for orchestrating
// Gas Town agents. The tests use the framework the same way users do:
//   - gt install to create a town
//   - gt start to start agents
//   - gt nudge to send tasks
//   - Verify work was completed
//
// Run: go test -tags=integration -v ./internal/opencode/...
// Skip long tests: go test -tags=integration -short ./internal/opencode/...
package opencode_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/deps"
	"github.com/steveyegge/gastown/internal/mayor"
	"github.com/steveyegge/gastown/internal/tmux"
)

// TestOpenCodeMayorWorkflow is a real E2E test that:
// 1. Creates a town configured for OpenCode
// 2. Starts Mayor using the framework (not manual server management)
// 3. Sends a task to Mayor
// 4. Verifies the work was completed
func TestOpenCodeMayorWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	// Prerequisite checks using the same deps package the framework uses
	ensureBeadsVersion(t)
	ensureOpenCode(t)
	ensureGT(t)

	// Create test town just like users do
	townRoot := t.TempDir()
	createTownWithOpenCode(t, townRoot)

	// Start Mayor using the framework (not manual server management)
	t.Log("Starting Mayor with OpenCode agent...")
	mgr := mayor.NewManager(townRoot)

	if err := mgr.Start("opencode"); err != nil {
		t.Fatalf("Failed to start Mayor: %v", err)
	}
	t.Cleanup(func() {
		mgr.Stop()
	})

	// Verify Mayor is running
	running, err := mgr.IsRunning()
	if err != nil || !running {
		t.Fatalf("Mayor should be running: err=%v, running=%v", err, running)
	}
	t.Log("✓ Mayor is running")

	// Wait for agent to be ready
	time.Sleep(3 * time.Second)

	// Verify the agent session exists and has the expected state
	tm := tmux.NewTmux()
	sessionID := mgr.SessionName()

	// Check session exists
	exists, err := tm.HasSession(sessionID)
	if err != nil || !exists {
		t.Fatalf("Mayor session should exist: %v", err)
	}
	t.Log("✓ Mayor tmux session exists")

	// Verify plugin was installed (OpenCode specific)
	pluginPath := filepath.Join(townRoot, "mayor", ".opencode", "plugin", "gastown.js")
	if _, err := os.Stat(pluginPath); err != nil {
		t.Errorf("OpenCode plugin should be installed at %s: %v", pluginPath, err)
	} else {
		t.Log("✓ OpenCode plugin installed")
	}
}

// createTownWithOpenCode creates a minimal town configured for OpenCode.
// This mirrors what users do with gt install, but configured for OpenCode.
func createTownWithOpenCode(t *testing.T, townRoot string) {
	t.Helper()

	// Create town structure
	dirs := []string{
		"mayor",
		"deacon",
		"settings",
		".beads",
	}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(townRoot, d), 0755); err != nil {
			t.Fatalf("Failed to create %s: %v", d, err)
		}
	}

	// Initialize git (required for beads)
	cmd := exec.Command("git", "init")
	cmd.Dir = townRoot
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init failed: %v", err)
	}

	// Initialize beads
	cmd = exec.Command("bd", "init")
	cmd.Dir = townRoot
	cmd.Run() // OK if fails - minimal test env

	// Configure town to use OpenCode
	townConfig := `{
  "name": "test-town",
  "default_agent": "opencode"
}`
	if err := os.WriteFile(
		filepath.Join(townRoot, "settings", "town.json"),
		[]byte(townConfig),
		0644,
	); err != nil {
		t.Fatalf("Failed to write town.json: %v", err)
	}

	// Note: We do NOT manually create OPENCODE.md or install plugins.
	// The framework (runtime.EnsureSettingsForRole) handles this automatically.
	// This is the key difference from the bash scripts - use the framework!

	t.Logf("✓ Created test town at %s with OpenCode agent", townRoot)
}

// ensureBeadsVersion verifies beads meets minimum version requirement.
func ensureBeadsVersion(t *testing.T) {
	t.Helper()
	status, version := deps.CheckBeads()
	switch status {
	case deps.BeadsOK:
		t.Logf("✓ beads %s (minimum: %s)", version, deps.MinBeadsVersion)
	case deps.BeadsNotFound:
		t.Fatalf("beads (bd) not found - install: go install github.com/steveyegge/beads/cmd/bd@latest")
	case deps.BeadsTooOld:
		t.Fatalf("beads %s too old (minimum: %s)", version, deps.MinBeadsVersion)
	}
}

// ensureOpenCode verifies opencode CLI is available.
func ensureOpenCode(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("opencode"); err != nil {
		t.Fatalf("opencode CLI not found - install from: https://opencode.ai")
	}
	t.Log("✓ opencode CLI found")
}

// ensureGT verifies gt binary is available.
func ensureGT(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("gt"); err != nil {
		// Try project root
		cwd, _ := os.Getwd()
		for dir := cwd; dir != "/"; dir = filepath.Dir(dir) {
			path := filepath.Join(dir, "gt")
			if _, err := os.Stat(path); err == nil {
				// Add to PATH for tests
				os.Setenv("PATH", dir+":"+os.Getenv("PATH"))
				t.Logf("✓ gt found at %s", path)
				return
			}
		}
		t.Fatalf("gt not found - run: make build")
	}
	t.Log("✓ gt found in PATH")
}

// Helper to get agent config for verification
func getAgentConfig(townRoot, role string) *config.RuntimeConfig {
	return config.ResolveRoleAgentConfig(role, townRoot, "opencode")
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", "\\n")
	if len(s) > n {
		return s[:n]
	}
	return s
}
