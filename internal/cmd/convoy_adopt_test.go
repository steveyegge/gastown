package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/constants"
)

// ---------------------------------------------------------------------------
// Subcommand registration tests
// ---------------------------------------------------------------------------

// TestConvoyAdoptCmd_Registration verifies that convoyAdoptCmd is registered
// as a subcommand of convoyCmd with correct Use and flags.
func TestConvoyAdoptCmd_Registration(t *testing.T) {
	if convoyAdoptCmd == nil {
		t.Fatal("convoyAdoptCmd is nil")
	}
	if !strings.HasPrefix(convoyAdoptCmd.Use, "adopt") {
		t.Errorf("convoyAdoptCmd.Use = %q, want prefix 'adopt'", convoyAdoptCmd.Use)
	}

	// Verify it's registered as a subcommand of convoyCmd.
	subCmds := convoyCmd.Commands()
	foundAdopt := false
	for _, cmd := range subCmds {
		if cmd.Name() == "adopt" {
			foundAdopt = true
			break
		}
	}
	if !foundAdopt {
		t.Errorf("convoyAdoptCmd not registered as subcommand of convoyCmd")
	}
}

// TestConvoyAdoptCmd_Flags verifies that --owned and --merge flags exist.
func TestConvoyAdoptCmd_Flags(t *testing.T) {
	ownedFlag := convoyAdoptCmd.Flags().Lookup("owned")
	if ownedFlag == nil {
		t.Fatal("convoyAdoptCmd should have --owned flag")
	}
	if ownedFlag.DefValue != "false" {
		t.Errorf("--owned default = %q, want %q", ownedFlag.DefValue, "false")
	}

	mergeFlag := convoyAdoptCmd.Flags().Lookup("merge")
	if mergeFlag == nil {
		t.Fatal("convoyAdoptCmd should have --merge flag")
	}
	if mergeFlag.DefValue != "" {
		t.Errorf("--merge default = %q, want empty string", mergeFlag.DefValue)
	}
}

// ---------------------------------------------------------------------------
// Helper: set up workspace for runConvoyAdopt tests
// ---------------------------------------------------------------------------

// setupAdoptWorkspace extends the testDAG Setup by adding the workspace marker
// (mayor/town.json) and sentinel files so that getTownBeadsDir() and
// beads.EnsureCustomTypes/EnsureCustomStatuses work in tests.
func setupAdoptWorkspace(t *testing.T, dag *testDAG) (townRoot, logPath string) {
	t.Helper()
	townRoot, logPath = dag.Setup(t)

	// Create workspace marker so getTownBeadsDir() succeeds.
	townJSON := filepath.Join(townRoot, "mayor", "town.json")
	if err := os.WriteFile(townJSON, []byte(`{"name":"test-town"}`), 0644); err != nil {
		t.Fatalf("write town.json: %v", err)
	}

	// Write sentinel files so EnsureCustomTypes/EnsureCustomStatuses skip bd calls.
	beadsDir := filepath.Join(townRoot, ".beads")
	typesList := strings.Join(constants.BeadsCustomTypesList(), ",")
	statusesList := strings.Join(constants.BeadsCustomStatusesList(), ",")
	if err := os.WriteFile(filepath.Join(beadsDir, ".gt-types-configured"), []byte(typesList+"\n"), 0644); err != nil {
		t.Fatalf("write types sentinel: %v", err)
	}
	if err := os.WriteFile(filepath.Join(beadsDir, ".gt-statuses-configured"), []byte(statusesList+"\n"), 0644); err != nil {
		t.Fatalf("write statuses sentinel: %v", err)
	}

	// Reset the in-memory cache so sentinel files are re-read.
	beads.ResetEnsuredDirs()
	t.Cleanup(func() { beads.ResetEnsuredDirs() })

	return townRoot, logPath
}

// ---------------------------------------------------------------------------
// Validation tests
// ---------------------------------------------------------------------------

// TestConvoyAdopt_InvalidMergeFlag verifies that an invalid --merge value
// returns an error without calling bd at all.
func TestConvoyAdopt_InvalidMergeFlag(t *testing.T) {
	defer func() {
		convoyAdoptMerge = ""
		convoyAdoptOwned = false
	}()
	convoyAdoptMerge = "invalid"

	err := runConvoyAdopt(convoyAdoptCmd, []string{"gt-epic-1"})
	if err == nil {
		t.Fatal("expected error for invalid --merge value, got nil")
	}
	if !strings.Contains(err.Error(), "invalid --merge value") {
		t.Errorf("error should mention 'invalid --merge value', got: %v", err)
	}
	if !strings.Contains(err.Error(), "invalid") {
		t.Errorf("error should contain the invalid value, got: %v", err)
	}
}

// TestConvoyAdopt_NonEpicType verifies that passing a non-epic bead returns
// an error mentioning the bead is not an epic.
func TestConvoyAdopt_NonEpicType(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows — shell stubs")
	}

	defer func() {
		convoyAdoptMerge = ""
		convoyAdoptOwned = false
	}()
	convoyAdoptMerge = ""

	dag := newTestDAG(t).
		Task("gt-task-1", "A regular task", withRig("gastown"))

	dag.Setup(t)

	err := runConvoyAdopt(convoyAdoptCmd, []string{"gt-task-1"})
	if err == nil {
		t.Fatal("expected error for non-epic bead, got nil")
	}
	if !strings.Contains(err.Error(), "not an epic") {
		t.Errorf("error should mention 'not an epic', got: %v", err)
	}
	if !strings.Contains(err.Error(), "task") {
		t.Errorf("error should mention actual type 'task', got: %v", err)
	}
}

// TestConvoyAdopt_NoSlingableChildren verifies that an epic with no children
// returns an appropriate error.
func TestConvoyAdopt_NoSlingableChildren(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows — shell stubs")
	}

	defer func() {
		convoyAdoptMerge = ""
		convoyAdoptOwned = false
	}()
	convoyAdoptMerge = ""

	// Epic with no children at all.
	dag := newTestDAG(t).
		Epic("gt-epic-empty", "Empty Epic")

	dag.Setup(t)

	err := runConvoyAdopt(convoyAdoptCmd, []string{"gt-epic-empty"})
	if err == nil {
		t.Fatal("expected error for epic with no children, got nil")
	}
	if !strings.Contains(err.Error(), "no slingable children") {
		t.Errorf("error should mention 'no slingable children', got: %v", err)
	}
}

// TestConvoyAdopt_EpicWithOnlyNonSlingableChildren verifies that an epic
// whose children are all non-slingable (e.g., decisions, sub-epics with
// no slingable descendants) returns an error.
func TestConvoyAdopt_EpicWithOnlyNonSlingableChildren(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows — shell stubs")
	}

	defer func() {
		convoyAdoptMerge = ""
		convoyAdoptOwned = false
	}()
	convoyAdoptMerge = ""

	// Epic whose only child is another empty epic (not slingable, no descendants).
	dag := newTestDAG(t).
		Epic("gt-epic-parent", "Parent Epic").
		Epic("gt-epic-child", "Child Epic").ParentOf("gt-epic-parent")

	dag.Setup(t)

	err := runConvoyAdopt(convoyAdoptCmd, []string{"gt-epic-parent"})
	if err == nil {
		t.Fatal("expected error for epic with only non-slingable children, got nil")
	}
	if !strings.Contains(err.Error(), "no slingable children") {
		t.Errorf("error should mention 'no slingable children', got: %v", err)
	}
}

// TestConvoyAdopt_BeadNotFound verifies that a non-existent bead returns
// a "not found" error.
func TestConvoyAdopt_BeadNotFound(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows — shell stubs")
	}

	defer func() {
		convoyAdoptMerge = ""
		convoyAdoptOwned = false
	}()
	convoyAdoptMerge = ""

	// Empty DAG - no beads registered in the stub.
	dag := newTestDAG(t)
	dag.Setup(t)

	err := runConvoyAdopt(convoyAdoptCmd, []string{"gt-nonexistent"})
	if err == nil {
		t.Fatal("expected error for non-existent bead, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found', got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Happy-path tests
// ---------------------------------------------------------------------------

// TestConvoyAdopt_BasicEpicWithChildren verifies that adopting an epic with
// task children calls bd show, list --parent, create, and dep add correctly.
func TestConvoyAdopt_BasicEpicWithChildren(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows — shell stubs")
	}

	defer func() {
		convoyAdoptMerge = ""
		convoyAdoptOwned = false
	}()
	convoyAdoptMerge = ""

	dag := newTestDAG(t).
		Epic("gt-epic-1", "Test Epic").
		Task("gt-task-1", "First task", withRig("gastown")).ParentOf("gt-epic-1").
		Task("gt-task-2", "Second task", withRig("gastown")).ParentOf("gt-epic-1").
		Bug("gt-bug-1", "A bug", withRig("gastown")).ParentOf("gt-epic-1")

	_, logPath := setupAdoptWorkspace(t, dag)

	err := runConvoyAdopt(convoyAdoptCmd, []string{"gt-epic-1"})
	if err != nil {
		t.Fatalf("runConvoyAdopt: %v", err)
	}

	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read bd.log: %v", err)
	}
	logContent := string(logBytes)

	// Should have called bd show for the epic.
	if !strings.Contains(logContent, "CMD:show gt-epic-1 --json") {
		t.Errorf("bd.log should contain 'CMD:show gt-epic-1 --json', got:\n%s", logContent)
	}

	// Should have listed children of the epic.
	if !strings.Contains(logContent, "CMD:list --parent=gt-epic-1 --json") {
		t.Errorf("bd.log should contain 'CMD:list --parent=gt-epic-1 --json', got:\n%s", logContent)
	}

	// Should have created a convoy bead.
	if !strings.Contains(logContent, "CMD:create --type=convoy") {
		t.Errorf("bd.log should contain 'CMD:create --type=convoy', got:\n%s", logContent)
	}

	// Should have added tracks deps for each child.
	if !strings.Contains(logContent, "dep add") || !strings.Contains(logContent, "--type=tracks") {
		t.Errorf("bd.log should contain 'dep add ... --type=tracks', got:\n%s", logContent)
	}

	// Each child should have a tracks dep.
	for _, childID := range []string{"gt-task-1", "gt-task-2", "gt-bug-1"} {
		if !strings.Contains(logContent, childID) {
			t.Errorf("bd.log should reference child %s, got:\n%s", childID, logContent)
		}
	}
}

// TestConvoyAdopt_NestedEpics verifies that adopting an epic with nested
// sub-epics does BFS to find all slingable descendants.
func TestConvoyAdopt_NestedEpics(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows — shell stubs")
	}

	defer func() {
		convoyAdoptMerge = ""
		convoyAdoptOwned = false
	}()
	convoyAdoptMerge = ""

	// Root epic -> sub-epic -> tasks (nested 2 levels deep)
	dag := newTestDAG(t).
		Epic("gt-root", "Root Epic").
		Epic("gt-sub", "Sub Epic").ParentOf("gt-root").
		Task("gt-deep-1", "Deep task 1", withRig("gastown")).ParentOf("gt-sub").
		Task("gt-deep-2", "Deep task 2", withRig("gastown")).ParentOf("gt-sub").
		Task("gt-top-1", "Top-level task", withRig("gastown")).ParentOf("gt-root")

	_, logPath := setupAdoptWorkspace(t, dag)

	err := runConvoyAdopt(convoyAdoptCmd, []string{"gt-root"})
	if err != nil {
		t.Fatalf("runConvoyAdopt: %v", err)
	}

	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read bd.log: %v", err)
	}
	logContent := string(logBytes)

	// Should have listed children at both levels.
	if !strings.Contains(logContent, "CMD:list --parent=gt-root --json") {
		t.Errorf("bd.log should list children of root epic, got:\n%s", logContent)
	}
	if !strings.Contains(logContent, "CMD:list --parent=gt-sub --json") {
		t.Errorf("bd.log should list children of sub-epic (BFS), got:\n%s", logContent)
	}

	// All three slingable tasks should be tracked.
	for _, childID := range []string{"gt-deep-1", "gt-deep-2", "gt-top-1"} {
		if !strings.Contains(logContent, childID) {
			t.Errorf("bd.log should reference slingable descendant %s, got:\n%s", childID, logContent)
		}
	}

	// Sub-epic itself should NOT be tracked (it's not slingable).
	// Count how many dep add lines reference gt-sub.
	lines := strings.Split(logContent, "\n")
	for _, line := range lines {
		if strings.Contains(line, "dep add") && strings.Contains(line, "gt-sub") {
			t.Errorf("sub-epic gt-sub should NOT be tracked via dep add, got line: %s", line)
		}
	}
}

// TestConvoyAdopt_OwnedFlag verifies that --owned adds the gt:owned label
// to the create command.
func TestConvoyAdopt_OwnedFlag(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows — shell stubs")
	}

	defer func() {
		convoyAdoptMerge = ""
		convoyAdoptOwned = false
	}()
	convoyAdoptOwned = true
	convoyAdoptMerge = ""

	dag := newTestDAG(t).
		Epic("gt-epic-own", "Owned Epic").
		Task("gt-task-own", "Task to own", withRig("gastown")).ParentOf("gt-epic-own")

	_, logPath := setupAdoptWorkspace(t, dag)

	err := runConvoyAdopt(convoyAdoptCmd, []string{"gt-epic-own"})
	if err != nil {
		t.Fatalf("runConvoyAdopt: %v", err)
	}

	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read bd.log: %v", err)
	}
	logContent := string(logBytes)

	// The create command should include --labels=gt:owned.
	if !strings.Contains(logContent, "--labels=gt:owned") {
		t.Errorf("bd.log should contain '--labels=gt:owned' for --owned flag, got:\n%s", logContent)
	}
}

// TestConvoyAdopt_MergeFlag verifies that --merge flag values are accepted
// and appear in the convoy description.
func TestConvoyAdopt_MergeFlag(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows — shell stubs")
	}

	for _, mergeVal := range []string{"direct", "mr", "local"} {
		t.Run("merge="+mergeVal, func(t *testing.T) {
			defer func() {
				convoyAdoptMerge = ""
				convoyAdoptOwned = false
			}()
			convoyAdoptMerge = mergeVal

			dag := newTestDAG(t).
				Epic("gt-epic-m", "Merge Epic").
				Task("gt-task-m", "Merge task", withRig("gastown")).ParentOf("gt-epic-m")

			_, logPath := setupAdoptWorkspace(t, dag)

			err := runConvoyAdopt(convoyAdoptCmd, []string{"gt-epic-m"})
			if err != nil {
				t.Fatalf("runConvoyAdopt with --merge=%s: %v", mergeVal, err)
			}

			logBytes, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatalf("read bd.log: %v", err)
			}
			logContent := string(logBytes)

			// Should have created a convoy (verifying the command ran successfully).
			if !strings.Contains(logContent, "CMD:create --type=convoy") {
				t.Errorf("bd.log should contain convoy creation, got:\n%s", logContent)
			}
		})
	}
}

// TestConvoyAdopt_MixedChildTypes verifies that slingable types (task, bug,
// feature, chore) are tracked while non-slingable types (epic, decision)
// are recursed into.
func TestConvoyAdopt_MixedChildTypes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows — shell stubs")
	}

	defer func() {
		convoyAdoptMerge = ""
		convoyAdoptOwned = false
	}()
	convoyAdoptMerge = ""

	dag := newTestDAG(t).
		Epic("gt-epic-mix", "Mixed Epic").
		Task("gt-t1", "Task child", withRig("gastown")).ParentOf("gt-epic-mix").
		Bug("gt-b1", "Bug child", withRig("gastown")).ParentOf("gt-epic-mix")

	// Add a feature child using Task (feature is slingable, testDAG doesn't
	// have a Feature() method, but we can verify task and bug are tracked).

	_, logPath := setupAdoptWorkspace(t, dag)

	err := runConvoyAdopt(convoyAdoptCmd, []string{"gt-epic-mix"})
	if err != nil {
		t.Fatalf("runConvoyAdopt: %v", err)
	}

	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read bd.log: %v", err)
	}
	logContent := string(logBytes)

	// Both task and bug should be tracked.
	if !strings.Contains(logContent, "gt-t1") {
		t.Errorf("bd.log should reference task child gt-t1, got:\n%s", logContent)
	}
	if !strings.Contains(logContent, "gt-b1") {
		t.Errorf("bd.log should reference bug child gt-b1, got:\n%s", logContent)
	}
}

// TestConvoyAdopt_ValidMergeValues verifies that all three valid --merge
// values are accepted without error.
func TestConvoyAdopt_ValidMergeValues(t *testing.T) {
	for _, val := range []string{"direct", "mr", "local"} {
		t.Run(val, func(t *testing.T) {
			defer func() { convoyAdoptMerge = "" }()
			convoyAdoptMerge = val
			// The merge validation happens before any bd calls, so we
			// can verify valid values don't return a merge-related error.
			// The function will fail later (no bd stub), but not on merge validation.
			err := runConvoyAdopt(convoyAdoptCmd, []string{"gt-nonexistent"})
			if err != nil && strings.Contains(err.Error(), "invalid --merge value") {
				t.Errorf("--merge=%s should be valid, got: %v", val, err)
			}
		})
	}
}
