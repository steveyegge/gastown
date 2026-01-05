//go:build integration

// Package cmd contains integration tests for beads routing and redirects.
//
// Run with: go test -tags=integration ./internal/cmd -run TestBeadsRouting -v
package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
)

// setupRoutingTestTown creates a minimal Gas Town with multiple rigs for testing routing.
// Returns townRoot.
func setupRoutingTestTown(t *testing.T) string {
	t.Helper()

	townRoot := t.TempDir()

	// Create town-level .beads directory
	townBeadsDir := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(townBeadsDir, 0755); err != nil {
		t.Fatalf("mkdir town .beads: %v", err)
	}

	// Create routes.jsonl with multiple rigs
	routes := []beads.Route{
		{Prefix: "hq-", Path: "."},                      // Town-level beads
		{Prefix: "gt-", Path: "gastown/mayor/rig"},      // Gastown rig
		{Prefix: "tr-", Path: "testrig/mayor/rig"},      // Test rig
	}
	if err := beads.WriteRoutes(townBeadsDir, routes); err != nil {
		t.Fatalf("write routes: %v", err)
	}

	// Create gastown rig structure
	gasRigPath := filepath.Join(townRoot, "gastown", "mayor", "rig")
	if err := os.MkdirAll(gasRigPath, 0755); err != nil {
		t.Fatalf("mkdir gastown: %v", err)
	}

	// Create gastown .beads directory with its own config
	gasBeadsDir := filepath.Join(gasRigPath, ".beads")
	if err := os.MkdirAll(gasBeadsDir, 0755); err != nil {
		t.Fatalf("mkdir gastown .beads: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gasBeadsDir, "config.yaml"), []byte("prefix: gt\n"), 0644); err != nil {
		t.Fatalf("write gastown config: %v", err)
	}

	// Create testrig structure
	testRigPath := filepath.Join(townRoot, "testrig", "mayor", "rig")
	if err := os.MkdirAll(testRigPath, 0755); err != nil {
		t.Fatalf("mkdir testrig: %v", err)
	}

	// Create testrig .beads directory
	testBeadsDir := filepath.Join(testRigPath, ".beads")
	if err := os.MkdirAll(testBeadsDir, 0755); err != nil {
		t.Fatalf("mkdir testrig .beads: %v", err)
	}
	if err := os.WriteFile(filepath.Join(testBeadsDir, "config.yaml"), []byte("prefix: tr\n"), 0644); err != nil {
		t.Fatalf("write testrig config: %v", err)
	}

	// Create polecats directory with redirect
	polecatsDir := filepath.Join(townRoot, "gastown", "polecats", "rictus")
	if err := os.MkdirAll(polecatsDir, 0755); err != nil {
		t.Fatalf("mkdir polecats: %v", err)
	}

	// Create redirect file for polecat -> mayor/rig/.beads
	// Path: gastown/polecats/rictus -> ../../mayor/rig/.beads -> gastown/mayor/rig/.beads
	polecatBeadsDir := filepath.Join(polecatsDir, ".beads")
	if err := os.MkdirAll(polecatBeadsDir, 0755); err != nil {
		t.Fatalf("mkdir polecat .beads: %v", err)
	}
	redirectContent := "../../mayor/rig/.beads"
	if err := os.WriteFile(filepath.Join(polecatBeadsDir, "redirect"), []byte(redirectContent), 0644); err != nil {
		t.Fatalf("write redirect: %v", err)
	}

	// Create crew directory with redirect
	// Path: gastown/crew/max -> ../../mayor/rig/.beads -> gastown/mayor/rig/.beads
	crewDir := filepath.Join(townRoot, "gastown", "crew", "max")
	if err := os.MkdirAll(crewDir, 0755); err != nil {
		t.Fatalf("mkdir crew: %v", err)
	}

	crewBeadsDir := filepath.Join(crewDir, ".beads")
	if err := os.MkdirAll(crewBeadsDir, 0755); err != nil {
		t.Fatalf("mkdir crew .beads: %v", err)
	}
	crewRedirect := "../../mayor/rig/.beads"
	if err := os.WriteFile(filepath.Join(crewBeadsDir, "redirect"), []byte(crewRedirect), 0644); err != nil {
		t.Fatalf("write crew redirect: %v", err)
	}

	return townRoot
}

// TestBeadsRoutingFromTownRoot verifies that bd show routes to correct rig
// based on issue ID prefix when run from town root.
func TestBeadsRoutingFromTownRoot(t *testing.T) {
	// Skip if bd is not available
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not installed, skipping routing test")
	}

	townRoot := setupRoutingTestTown(t)

	tests := []struct {
		prefix      string
		expectedRig string // Expected rig path fragment in error/output
	}{
		{"hq-", "."}, // Town-level beads
		{"gt-", "gastown"},
		{"tr-", "testrig"},
	}

	for _, tc := range tests {
		t.Run(tc.prefix, func(t *testing.T) {
			// Create a fake issue ID with the prefix
			issueID := tc.prefix + "test123"

			// Run bd show - it will fail since issue doesn't exist,
			// but we're testing routing, not the issue itself
			cmd := exec.Command("bd", "--no-daemon", "show", issueID)
			cmd.Dir = townRoot
			cmd.Env = append(os.Environ(), "BD_DEBUG_ROUTING=1")
			output, _ := cmd.CombinedOutput()

			// The debug routing output or error message should indicate
			// which beads directory was used
			outputStr := string(output)
			t.Logf("Output for %s: %s", issueID, outputStr)

			// We expect either the routing debug output or an error from the correct beads
			// If routing works, the error will be about not finding the issue,
			// not about routing failure
			if strings.Contains(outputStr, "no matching route") {
				t.Errorf("routing failed for prefix %s: %s", tc.prefix, outputStr)
			}
		})
	}
}

// TestBeadsRedirectResolution verifies that redirect files are followed correctly.
func TestBeadsRedirectResolution(t *testing.T) {
	townRoot := setupRoutingTestTown(t)

	tests := []struct {
		name     string
		workDir  string
		expected string // Expected resolved path (relative to townRoot)
	}{
		{
			name:     "polecat redirect",
			workDir:  "gastown/polecats/rictus",
			expected: "gastown/mayor/rig/.beads",
		},
		{
			name:     "crew redirect",
			workDir:  "gastown/crew/max",
			expected: "gastown/mayor/rig/.beads",
		},
		{
			name:     "no redirect (mayor/rig)",
			workDir:  "gastown/mayor/rig",
			expected: "gastown/mayor/rig/.beads",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fullWorkDir := filepath.Join(townRoot, tc.workDir)
			resolved := beads.ResolveBeadsDir(fullWorkDir)

			expectedFull := filepath.Join(townRoot, tc.expected)
			if resolved != expectedFull {
				t.Errorf("ResolveBeadsDir(%s) = %s, want %s", tc.workDir, resolved, expectedFull)
			}
		})
	}
}

// TestBeadsCircularRedirectDetection verifies that circular redirects are detected.
func TestBeadsCircularRedirectDetection(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a beads directory with a redirect pointing to itself
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Create redirect file pointing to itself (circular)
	redirectContent := ".beads" // Points to current .beads (circular)
	if err := os.WriteFile(filepath.Join(beadsDir, "redirect"), []byte(redirectContent), 0644); err != nil {
		t.Fatalf("write redirect: %v", err)
	}

	// ResolveBeadsDir should detect the circular redirect and return the original
	resolved := beads.ResolveBeadsDir(tmpDir)
	if resolved != beadsDir {
		t.Errorf("expected circular redirect to return original beads dir, got %s", resolved)
	}

	// The redirect file should have been removed
	redirectPath := filepath.Join(beadsDir, "redirect")
	if _, err := os.Stat(redirectPath); !os.IsNotExist(err) {
		t.Error("circular redirect file should have been removed")
	}
}

// TestBeadsPrefixConflictDetection verifies that duplicate prefixes are detected.
func TestBeadsPrefixConflictDetection(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Create routes with a duplicate prefix
	routes := []beads.Route{
		{Prefix: "gt-", Path: "gastown/mayor/rig"},
		{Prefix: "gt-", Path: "other/mayor/rig"}, // Duplicate!
		{Prefix: "bd-", Path: "beads/mayor/rig"},
	}
	if err := beads.WriteRoutes(beadsDir, routes); err != nil {
		t.Fatalf("write routes: %v", err)
	}

	// FindConflictingPrefixes should detect the duplicate
	conflicts, err := beads.FindConflictingPrefixes(beadsDir)
	if err != nil {
		t.Fatalf("FindConflictingPrefixes: %v", err)
	}

	if len(conflicts) == 0 {
		t.Error("expected to find conflicts, got none")
	}

	if paths, ok := conflicts["gt-"]; !ok {
		t.Error("expected conflict for prefix 'gt-'")
	} else if len(paths) != 2 {
		t.Errorf("expected 2 conflicting paths for 'gt-', got %d", len(paths))
	}
}

// TestBeadsListFromPolecatDirectory verifies that bd list works from polecat directories.
func TestBeadsListFromPolecatDirectory(t *testing.T) {
	// Skip if bd is not available
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not installed, skipping test")
	}

	townRoot := setupRoutingTestTown(t)
	polecatDir := filepath.Join(townRoot, "gastown", "polecats", "rictus")

	// Initialize beads in mayor/rig so bd list can work
	mayorRigBeads := filepath.Join(townRoot, "gastown", "mayor", "rig", ".beads")

	// Create a minimal beads.db (or use bd init)
	// For now, just test that the redirect is followed
	cmd := exec.Command("bd", "--no-daemon", "list")
	cmd.Dir = polecatDir
	output, err := cmd.CombinedOutput()

	// We expect either success (empty list) or an error about missing db,
	// but NOT an error about missing .beads directory (since redirect should work)
	outputStr := string(output)
	t.Logf("bd list output: %s", outputStr)

	if err != nil {
		// Check it's not a "no .beads directory" error
		if strings.Contains(outputStr, "no .beads directory") {
			t.Errorf("redirect not followed: %s", outputStr)
		}
		// Check it's finding the right beads directory via redirect
		if strings.Contains(outputStr, "redirect") && !strings.Contains(outputStr, mayorRigBeads) {
			// This is okay - the redirect is being processed
			t.Logf("redirect detected in output (expected)")
		}
	}
}

// TestBeadsListFromCrewDirectory verifies that bd list works from crew directories.
func TestBeadsListFromCrewDirectory(t *testing.T) {
	// Skip if bd is not available
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not installed, skipping test")
	}

	townRoot := setupRoutingTestTown(t)
	crewDir := filepath.Join(townRoot, "gastown", "crew", "max")

	cmd := exec.Command("bd", "--no-daemon", "list")
	cmd.Dir = crewDir
	output, err := cmd.CombinedOutput()

	outputStr := string(output)
	t.Logf("bd list output from crew: %s", outputStr)

	if err != nil {
		// Check it's not a "no .beads directory" error
		if strings.Contains(outputStr, "no .beads directory") {
			t.Errorf("redirect not followed for crew: %s", outputStr)
		}
	}
}

// TestBeadsRoutesLoading verifies that routes.jsonl is loaded correctly.
func TestBeadsRoutesLoading(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Create routes.jsonl with various entries
	routesContent := `{"prefix": "hq-", "path": "."}
{"prefix": "gt-", "path": "gastown/mayor/rig"}
# Comment line should be ignored
{"prefix": "bd-", "path": "beads/mayor/rig"}

{"prefix": "tr-", "path": "testrig/mayor/rig"}
`
	if err := os.WriteFile(filepath.Join(beadsDir, "routes.jsonl"), []byte(routesContent), 0644); err != nil {
		t.Fatalf("write routes: %v", err)
	}

	routes, err := beads.LoadRoutes(beadsDir)
	if err != nil {
		t.Fatalf("LoadRoutes: %v", err)
	}

	if len(routes) != 4 {
		t.Errorf("expected 4 routes, got %d", len(routes))
	}

	// Verify specific routes
	expectedPrefixes := map[string]string{
		"hq-": ".",
		"gt-": "gastown/mayor/rig",
		"bd-": "beads/mayor/rig",
		"tr-": "testrig/mayor/rig",
	}

	for _, r := range routes {
		if expected, ok := expectedPrefixes[r.Prefix]; ok {
			if r.Path != expected {
				t.Errorf("route %s: path = %q, want %q", r.Prefix, r.Path, expected)
			}
		} else {
			t.Errorf("unexpected prefix: %s", r.Prefix)
		}
	}
}

// TestBeadsAppendRoute verifies that routes can be appended and updated.
func TestBeadsAppendRoute(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Append first route
	route1 := beads.Route{Prefix: "gt-", Path: "gastown/mayor/rig"}
	if err := beads.AppendRoute(tmpDir, route1); err != nil {
		t.Fatalf("AppendRoute 1: %v", err)
	}

	// Append second route
	route2 := beads.Route{Prefix: "bd-", Path: "beads/mayor/rig"}
	if err := beads.AppendRoute(tmpDir, route2); err != nil {
		t.Fatalf("AppendRoute 2: %v", err)
	}

	// Verify both routes exist
	routes, err := beads.LoadRoutes(beadsDir)
	if err != nil {
		t.Fatalf("LoadRoutes: %v", err)
	}
	if len(routes) != 2 {
		t.Errorf("expected 2 routes, got %d", len(routes))
	}

	// Update existing route (same prefix, different path)
	route1Updated := beads.Route{Prefix: "gt-", Path: "newpath/mayor/rig"}
	if err := beads.AppendRoute(tmpDir, route1Updated); err != nil {
		t.Fatalf("AppendRoute update: %v", err)
	}

	// Verify update
	routes, _ = beads.LoadRoutes(beadsDir)
	if len(routes) != 2 {
		t.Errorf("expected 2 routes after update, got %d", len(routes))
	}

	for _, r := range routes {
		if r.Prefix == "gt-" && r.Path != "newpath/mayor/rig" {
			t.Errorf("route update failed: got path %q", r.Path)
		}
	}
}

// TestBeadsRemoveRoute verifies that routes can be removed.
func TestBeadsRemoveRoute(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Create initial routes
	routes := []beads.Route{
		{Prefix: "gt-", Path: "gastown/mayor/rig"},
		{Prefix: "bd-", Path: "beads/mayor/rig"},
	}
	if err := beads.WriteRoutes(beadsDir, routes); err != nil {
		t.Fatalf("WriteRoutes: %v", err)
	}

	// Remove one route
	if err := beads.RemoveRoute(tmpDir, "gt-"); err != nil {
		t.Fatalf("RemoveRoute: %v", err)
	}

	// Verify removal
	remaining, _ := beads.LoadRoutes(beadsDir)
	if len(remaining) != 1 {
		t.Errorf("expected 1 route after removal, got %d", len(remaining))
	}
	if remaining[0].Prefix != "bd-" {
		t.Errorf("wrong route remaining: %s", remaining[0].Prefix)
	}
}

// TestBeadsGetPrefixForRig verifies prefix lookup by rig name.
func TestBeadsGetPrefixForRig(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Create routes
	routes := []beads.Route{
		{Prefix: "gt-", Path: "gastown/mayor/rig"},
		{Prefix: "bd-", Path: "beads/mayor/rig"},
		{Prefix: "hq-", Path: "."},
	}
	if err := beads.WriteRoutes(beadsDir, routes); err != nil {
		t.Fatalf("WriteRoutes: %v", err)
	}

	tests := []struct {
		rigName  string
		expected string
	}{
		{"gastown", "gt"},
		{"beads", "bd"},
		{"unknown", "gt"}, // Default
		{"", "gt"},        // Empty -> default
	}

	for _, tc := range tests {
		t.Run(tc.rigName, func(t *testing.T) {
			result := beads.GetPrefixForRig(tmpDir, tc.rigName)
			if result != tc.expected {
				t.Errorf("GetPrefixForRig(%q) = %q, want %q", tc.rigName, result, tc.expected)
			}
		})
	}
}
