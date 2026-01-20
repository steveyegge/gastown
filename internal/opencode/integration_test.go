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
	"testing"

	"github.com/steveyegge/gastown/internal/mayor"
	"github.com/steveyegge/gastown/internal/testutil"
)

// TestOpenCodeMayorWorkflow is a real E2E test that verifies the Mayor
// can start correctly with the OpenCode runtime.
func TestOpenCodeMayorWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	testutil.RequireBeads(t)
	testutil.RequireBinary(t, "opencode")
	testutil.RequireGT(t)

	// Create test town just like users do
	f := testutil.NewTownFixture(t, "opencode")
	townRoot := f.Root

	// Start Mayor using the framework
	t.Log("Starting Mayor with OpenCode agent...")
	mgr := mayor.NewManager(townRoot)
	
	if err := mgr.Start("opencode"); err != nil {
		testutil.LogDiagnostic(t, mgr.SessionName())
		t.Fatalf("Failed to start Mayor: %v", err)
	}
	t.Cleanup(func() { mgr.Stop() })

	// Verify Mayor is running
	running, err := mgr.IsRunning()
	if err != nil || !running {
		t.Fatalf("Mayor should be running: err=%v, running=%v", err, running)
	}
	t.Log("✓ Mayor is running")

	// Verify plugin was installed
	// Note: Framework installs plugin in role-specific .opencode/plugin dir
	// In this case, mayor/.opencode/plugin/gastown.js
	pluginPath := f.Path("mayor/.opencode/plugin/gastown.js")
	if _, err := os.Stat(pluginPath); err != nil {
		t.Errorf("OpenCode plugin should be installed at %s: %v", pluginPath, err)
	} else {
		t.Log("✓ OpenCode plugin installed")
	}
}


	testutil.RequireBeads(t)
	testutil.RequireBinary(t, "opencode")
	testutil.RequireGT(t)

	// Create test town just like users do
	f := testutil.NewTownFixture(t, "opencode")
	townRoot := f.Root

	// Start Mayor using the framework
	t.Log("Starting Mayor with OpenCode agent...")
	mgr := mayor.NewManager(townRoot)
	
	if err := mgr.Start("opencode"); err != nil {
		testutil.LogDiagnostic(t, mgr.SessionName())
		t.Fatalf("Failed to start Mayor: %v", err)
	}
	t.Cleanup(func() { mgr.Stop() })

	// Verify Mayor is running
	running, err := mgr.IsRunning()
	if err != nil || !running {
		t.Fatalf("Mayor should be running: err=%v, running=%v", err, running)
	}
	t.Log("✓ Mayor is running")

	// Verify plugin was installed
	// Note: Framework installs plugin in role-specific .opencode/plugin dir
	// In this case, mayor/.opencode/plugin/gastown.js
	pluginPath := f.Path("mayor/.opencode/plugin/gastown.js")
	if _, err := os.Stat(pluginPath); err != nil {
		t.Errorf("OpenCode plugin should be installed at %s: %v", pluginPath, err)
	} else {
		t.Log("✓ OpenCode plugin installed")
	}
}


	testutil.RequireBeads(t)
	testutil.RequireBinary(t, "opencode")
	testutil.RequireGT(t)

	// Create test town just like users do
	f := testutil.NewTownFixture(t, "opencode")
	townRoot := f.Root

	// Start Mayor using the framework
	t.Log("Starting Mayor with OpenCode agent...")
	mgr := mayor.NewManager(townRoot)
	
	if err := mgr.Start("opencode"); err != nil {
		testutil.LogDiagnostic(t, mgr.SessionName())
		t.Fatalf("Failed to start Mayor: %v", err)
	}
	t.Cleanup(func() { mgr.Stop() })

	// Verify Mayor is running
	running, err := mgr.IsRunning()
	if err != nil || !running {
		t.Fatalf("Mayor should be running: err=%v, running=%v", err, running)
	}
	t.Log("✓ Mayor is running")

	// Verify plugin was installed
	// Note: Framework installs plugin in role-specific .opencode/plugin dir
	// In this case, mayor/.opencode/plugin/gastown.js
	pluginPath := f.Path("mayor/.opencode/plugin/gastown.js")
	if _, err := os.Stat(pluginPath); err != nil {
		t.Errorf("OpenCode plugin should be installed at %s: %v", pluginPath, err)
	} else {
		t.Log("✓ OpenCode plugin installed")
	}
}


	testutil.RequireBeads(t)
	testutil.RequireBinary(t, "opencode")
	testutil.RequireGT(t)

	// Create test town just like users do
	f := testutil.NewTownFixture(t, "opencode")
	townRoot := f.Root

	// Start Mayor using the framework
	t.Log("Starting Mayor with OpenCode agent...")
	mgr := mayor.NewManager(townRoot)

	if err := mgr.Start("opencode"); err != nil {
		testutil.LogDiagnostic(t, mgr.SessionName())
		t.Fatalf("Failed to start Mayor: %v", err)
	}
	t.Cleanup(func() { mgr.Stop() })

	// Verify Mayor is running
	running, err := mgr.IsRunning()
	if err != nil || !running {
		t.Fatalf("Mayor should be running: err=%v, running=%v", err, running)
	}
	t.Log("✓ Mayor is running")

	// Verify plugin was installed
	pluginPath := f.Path(".opencode/plugin/gastown.js")
	// Note: Framework installs plugin in role-specific .opencode/plugin dir
	// In this case, mayor/.opencode/plugin/gastown.js
	pluginPath = f.Path("mayor/.opencode/plugin/gastown.js")
	if _, err := os.Stat(pluginPath); err != nil {
		t.Errorf("OpenCode plugin should be installed at %s: %v", pluginPath, err)
	} else {
		t.Log("✓ OpenCode plugin installed")
	}
}
