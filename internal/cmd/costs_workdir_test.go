package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/workspace"
)

// filterGTEnv removes GT_* and BD_* environment variables to isolate test subprocess.
// This prevents tests from inheriting the parent workspace's Gas Town configuration.
func filterGTEnv(env []string) []string {
	filtered := make([]string, 0, len(env))
	for _, e := range env {
		if strings.HasPrefix(e, "GT_") || strings.HasPrefix(e, "BD_") {
			continue
		}
		filtered = append(filtered, e)
	}
	return filtered
}

// TestQuerySessionEvents_FindsEventsFromAllLocations verifies that querySessionEvents
// finds session.ended events from both town-level and rig-level beads databases.
//
// Bug: Events created by rig-level agents (polecats, witness, etc.) are stored in
// the rig's .beads database. Events created by town-level agents (mayor, deacon)
// are stored in the town's .beads database. querySessionEvents must query ALL
// beads locations to find all events.
//
// This test:
// 1. Creates a town with a rig
// 2. Creates session.ended events in both town and rig beads
// 3. Verifies querySessionEvents finds events from both locations
func TestQuerySessionEvents_FindsEventsFromAllLocations(t *testing.T) {
	// Skip if gt and bd are not installed
	if _, err := exec.LookPath("gt"); err != nil {
		t.Skip("gt not installed, skipping integration test")
	}
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not installed, skipping integration test")
	}

	// Skip when running inside a Gas Town workspace - this integration test
	// creates a separate workspace and the subprocesses can interact with
	// the parent workspace's daemon, causing hangs.
	if os.Getenv("GT_TOWN_ROOT") != "" || os.Getenv("BD_ACTOR") != "" {
		t.Skip("skipping integration test inside Gas Town workspace (use 'go test' outside workspace)")
	}

	// Create a temporary directory structure
	tmpDir := t.TempDir()
	townRoot := filepath.Join(tmpDir, "test-town")

	// Create town directory
	if err := os.MkdirAll(townRoot, 0755); err != nil {
		t.Fatalf("creating town directory: %v", err)
	}

	// Initialize a git repo (required for gt install)
	gitInitCmd := exec.Command("git", "init")
	gitInitCmd.Dir = townRoot
	if out, err := gitInitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}

	// Use gt install to set up the town
	// Clear GT environment variables to isolate test from parent workspace
	gtInstallCmd := exec.Command("gt", "install")
	gtInstallCmd.Dir = townRoot
	gtInstallCmd.Env = filterGTEnv(os.Environ())
	if out, err := gtInstallCmd.CombinedOutput(); err != nil {
		t.Fatalf("gt install: %v\n%s", err, out)
	}

	// Create a bare repo to use as the rig source
	bareRepo := filepath.Join(tmpDir, "bare-repo.git")
	bareInitCmd := exec.Command("git", "init", "--bare", bareRepo)
	if out, err := bareInitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v\n%s", err, out)
	}

	// Create a temporary clone to add initial content (bare repos need content)
	tempClone := filepath.Join(tmpDir, "temp-clone")
	cloneCmd := exec.Command("git", "clone", bareRepo, tempClone)
	if out, err := cloneCmd.CombinedOutput(); err != nil {
		t.Fatalf("git clone bare: %v\n%s", err, out)
	}

	// Add initial commit to bare repo
	initFileCmd := exec.Command("bash", "-c", "echo 'test' > README.md && git add . && git commit -m 'init'")
	initFileCmd.Dir = tempClone
	if out, err := initFileCmd.CombinedOutput(); err != nil {
		t.Fatalf("initial commit: %v\n%s", err, out)
	}
	pushCmd := exec.Command("git", "push", "origin", "main")
	pushCmd.Dir = tempClone
	// Try main first, fall back to master
	if _, err := pushCmd.CombinedOutput(); err != nil {
		pushCmd2 := exec.Command("git", "push", "origin", "master")
		pushCmd2.Dir = tempClone
		if out, err := pushCmd2.CombinedOutput(); err != nil {
			t.Fatalf("git push: %v\n%s", err, out)
		}
	}

	// Add rig using gt rig add
	rigAddCmd := exec.Command("gt", "rig", "add", "testrig", bareRepo, "--prefix=tr")
	rigAddCmd.Dir = townRoot
	rigAddCmd.Env = filterGTEnv(os.Environ())
	if out, err := rigAddCmd.CombinedOutput(); err != nil {
		t.Fatalf("gt rig add: %v\n%s", err, out)
	}

	// Find the rig path
	rigPath := filepath.Join(townRoot, "testrig")

	// Verify rig has its own .beads
	rigBeadsPath := filepath.Join(rigPath, ".beads")
	if _, err := os.Stat(rigBeadsPath); os.IsNotExist(err) {
		t.Fatalf("rig .beads not created at %s", rigBeadsPath)
	}

	// Create a session.ended event in TOWN beads (simulating mayor/deacon)
	townEventPayload := `{"cost_usd":1.50,"session_id":"hq-mayor","role":"mayor","ended_at":"2026-01-12T10:00:00Z"}`
	townEventCmd := exec.Command("bd", "create",
		"--type=event",
		"--title=Town session ended",
		"--event-category=session.ended",
		"--event-payload="+townEventPayload,
		"--json",
	)
	townEventCmd.Dir = townRoot
	townEventCmd.Env = filterGTEnv(os.Environ())
	townOut, err := townEventCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("creating town event: %v\n%s", err, townOut)
	}
	t.Logf("Created town event: %s", string(townOut))

	// Create a session.ended event in RIG beads (simulating polecat)
	rigEventPayload := `{"cost_usd":2.50,"session_id":"gt-testrig-toast","role":"polecat","rig":"testrig","worker":"toast","ended_at":"2026-01-12T11:00:00Z"}`
	rigEventCmd := exec.Command("bd", "create",
		"--type=event",
		"--title=Rig session ended",
		"--event-category=session.ended",
		"--event-payload="+rigEventPayload,
		"--json",
	)
	rigEventCmd.Dir = rigPath
	rigEventCmd.Env = filterGTEnv(os.Environ())
	rigOut, err := rigEventCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("creating rig event: %v\n%s", err, rigOut)
	}
	t.Logf("Created rig event: %s", string(rigOut))

	// Verify events are in separate databases by querying each directly
	townListCmd := exec.Command("bd", "list", "--type=event", "--all", "--json")
	townListCmd.Dir = townRoot
	townListCmd.Env = filterGTEnv(os.Environ())
	townListOut, err := townListCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("listing town events: %v\n%s", err, townListOut)
	}

	rigListCmd := exec.Command("bd", "list", "--type=event", "--all", "--json")
	rigListCmd.Dir = rigPath
	rigListCmd.Env = filterGTEnv(os.Environ())
	rigListOut, err := rigListCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("listing rig events: %v\n%s", err, rigListOut)
	}

	var townEvents, rigEvents []struct{ ID string }
	json.Unmarshal(townListOut, &townEvents)
	json.Unmarshal(rigListOut, &rigEvents)

	t.Logf("Town beads has %d events", len(townEvents))
	t.Logf("Rig beads has %d events", len(rigEvents))

	// Both should have events (they're in separate DBs)
	if len(townEvents) == 0 {
		t.Error("Expected town beads to have events")
	}
	if len(rigEvents) == 0 {
		t.Error("Expected rig beads to have events")
	}

	// Save current directory and change to town root for query
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getting current directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(origDir); err != nil {
			t.Errorf("restoring directory: %v", err)
		}
	}()

	if err := os.Chdir(townRoot); err != nil {
		t.Fatalf("changing to town root: %v", err)
	}

	// Verify workspace discovery works
	foundTownRoot, wsErr := workspace.FindFromCwdOrError()
	if wsErr != nil {
		t.Fatalf("workspace.FindFromCwdOrError failed: %v", wsErr)
	}
	if foundTownRoot != townRoot {
		t.Errorf("workspace.FindFromCwdOrError returned %s, expected %s", foundTownRoot, townRoot)
	}

	// Call querySessionEvents - this should find events from ALL locations
	entries := querySessionEvents()

	t.Logf("querySessionEvents returned %d entries", len(entries))

	// We created 2 session.ended events (one town, one rig)
	// The fix should find BOTH
	if len(entries) < 2 {
		t.Errorf("querySessionEvents found %d entries, expected at least 2 (one from town, one from rig)", len(entries))
		t.Log("This indicates the bug: querySessionEvents only queries town-level beads, missing rig-level events")
	}

	// Verify we found both the mayor and polecat sessions
	var foundMayor, foundPolecat bool
	for _, e := range entries {
		t.Logf("  Entry: session=%s role=%s cost=$%.2f", e.SessionID, e.Role, e.CostUSD)
		if e.Role == "mayor" {
			foundMayor = true
		}
		if e.Role == "polecat" {
			foundPolecat = true
		}
	}

	if !foundMayor {
		t.Error("Missing mayor session from town beads")
	}
	if !foundPolecat {
		t.Error("Missing polecat session from rig beads")
	}
}
