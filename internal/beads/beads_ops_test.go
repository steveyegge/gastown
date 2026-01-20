package beads

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// BeadsOpsTestEnv holds the test environment for BeadsOps conformance tests.
type BeadsOpsTestEnv struct {
	Ops      BeadsOps
	AddBead  func(beadID, status string, labels []string) // Add a bead to the store
	TownRoot string                                       // Town root path (for Real implementations)
	RigPath  string                                       // Rig path (for Real implementations)
	Cleanup  func()
}

// BeadsOpsFactory creates a BeadsOps implementation for testing.
type BeadsOpsFactory func(t *testing.T) *BeadsOpsTestEnv

// =============================================================================
// Conformance Test Suite - runs against ALL implementations
// =============================================================================

// RunConformanceTests runs the BeadsOps interface conformance test suite.
// Both the fake implementation and real implementation must pass these tests.
// This verifies the test double accurately models real bd behavior.
func RunConformanceTests(t *testing.T, factory BeadsOpsFactory) {
	t.Run("IsTownLevelBead", func(t *testing.T) {
		runIsTownLevelBeadTests(t, factory)
	})

	t.Run("GetRigForBead", func(t *testing.T) {
		runGetRigForBeadTests(t, factory)
	})

	t.Run("LabelAdd", func(t *testing.T) {
		runLabelAddTests(t, factory)
	})

	t.Run("LabelRemove", func(t *testing.T) {
		runLabelRemoveTests(t, factory)
	})

	t.Run("ListByLabelAllRigs", func(t *testing.T) {
		runListByLabelAllRigsTests(t, factory)
	})
}

// --- IsTownLevelBead tests ---

func runIsTownLevelBeadTests(t *testing.T, factory BeadsOpsFactory) {
	t.Run("returns true for hq- prefix", func(t *testing.T) {
		env := factory(t)
		defer env.Cleanup()

		if !env.Ops.IsTownLevelBead("hq-abc") {
			t.Error("expected hq-abc to be town-level")
		}
		if !env.Ops.IsTownLevelBead("hq-cv-test") {
			t.Error("expected hq-cv-test to be town-level")
		}
	})

	t.Run("returns false for rig prefixes", func(t *testing.T) {
		env := factory(t)
		defer env.Cleanup()

		if env.Ops.IsTownLevelBead("gt-abc") {
			t.Error("expected gt-abc to not be town-level")
		}
		if env.Ops.IsTownLevelBead("tr-xyz") {
			t.Error("expected tr-xyz to not be town-level")
		}
	})

	t.Run("returns false for empty string", func(t *testing.T) {
		env := factory(t)
		defer env.Cleanup()

		if env.Ops.IsTownLevelBead("") {
			t.Error("expected empty string to not be town-level")
		}
	})
}

// --- GetRigForBead tests ---

func runGetRigForBeadTests(t *testing.T, factory BeadsOpsFactory) {
	t.Run("returns empty for unrouted prefix", func(t *testing.T) {
		env := factory(t)
		defer env.Cleanup()

		rigName := env.Ops.GetRigForBead("xx-abc")
		if rigName != "" {
			t.Errorf("expected empty for unrouted prefix, got %q", rigName)
		}
	})

	t.Run("returns empty for empty bead ID", func(t *testing.T) {
		env := factory(t)
		defer env.Cleanup()

		rigName := env.Ops.GetRigForBead("")
		if rigName != "" {
			t.Errorf("expected empty for empty bead ID, got %q", rigName)
		}
	})

	t.Run("returns empty for bead without prefix", func(t *testing.T) {
		env := factory(t)
		defer env.Cleanup()

		rigName := env.Ops.GetRigForBead("noprefix")
		if rigName != "" {
			t.Errorf("expected empty for bead without prefix, got %q", rigName)
		}
	})
}

// --- LabelAdd tests ---

func runLabelAddTests(t *testing.T, factory BeadsOpsFactory) {
	t.Run("adds label to existing bead", func(t *testing.T) {
		env := factory(t)
		defer env.Cleanup()

		// Create a bead first
		env.AddBead("tr-abc", "open", []string{})

		// Add a label
		err := env.Ops.LabelAdd("tr-abc", "queued")
		if err != nil {
			t.Fatalf("LabelAdd failed: %v", err)
		}
	})

	t.Run("is idempotent for existing label", func(t *testing.T) {
		env := factory(t)
		defer env.Cleanup()

		// Create bead with label
		env.AddBead("tr-def", "open", []string{"queued"})

		// Adding same label again should not error
		err := env.Ops.LabelAdd("tr-def", "queued")
		if err != nil {
			t.Errorf("LabelAdd should be idempotent: %v", err)
		}
	})
}

// --- LabelRemove tests ---

func runLabelRemoveTests(t *testing.T, factory BeadsOpsFactory) {
	t.Run("removes label from existing bead", func(t *testing.T) {
		env := factory(t)
		defer env.Cleanup()

		// Create bead with label
		env.AddBead("tr-abc", "open", []string{"queued"})

		// Remove the label
		err := env.Ops.LabelRemove("tr-abc", "queued")
		if err != nil {
			t.Fatalf("LabelRemove failed: %v", err)
		}
	})

	t.Run("is idempotent for missing label", func(t *testing.T) {
		env := factory(t)
		defer env.Cleanup()

		// Create bead without label
		env.AddBead("tr-def", "open", []string{})

		// Removing non-existent label should not error
		err := env.Ops.LabelRemove("tr-def", "queued")
		if err != nil {
			t.Errorf("LabelRemove should be idempotent: %v", err)
		}
	})
}

// --- ListByLabelAllRigs tests ---

func runListByLabelAllRigsTests(t *testing.T, factory BeadsOpsFactory) {
	t.Run("returns empty map when no beads have label", func(t *testing.T) {
		env := factory(t)
		defer env.Cleanup()

		result, err := env.Ops.ListByLabelAllRigs("queued")
		if err != nil {
			t.Fatalf("ListByLabelAllRigs failed: %v", err)
		}
		if len(result) != 0 {
			t.Errorf("expected empty map, got %v", result)
		}
	})

	t.Run("returns beads with matching label", func(t *testing.T) {
		env := factory(t)
		defer env.Cleanup()

		// Add beads with and without the label
		env.AddBead("tr-queued", "open", []string{"queued"})
		env.AddBead("tr-other", "open", []string{"other"})

		result, err := env.Ops.ListByLabelAllRigs("queued")
		if err != nil {
			t.Fatalf("ListByLabelAllRigs failed: %v", err)
		}

		// Should have at least one rig with beads
		totalBeads := 0
		for _, beadList := range result {
			totalBeads += len(beadList)
		}

		if totalBeads == 0 {
			t.Error("expected at least one bead with 'queued' label")
		}
	})

	t.Run("only returns open beads", func(t *testing.T) {
		env := factory(t)
		defer env.Cleanup()

		// Add open and closed beads with the label
		env.AddBead("tr-open", "open", []string{"queued"})
		env.AddBead("tr-closed", "closed", []string{"queued"})

		result, err := env.Ops.ListByLabelAllRigs("queued")
		if err != nil {
			t.Fatalf("ListByLabelAllRigs failed: %v", err)
		}

		// Check that closed bead is not in results
		for _, beadList := range result {
			for _, bead := range beadList {
				if bead.ID == "tr-closed" {
					t.Error("closed bead should not be in results")
				}
			}
		}
	})
}

// =============================================================================
// Test Entry Points - Same suite, different implementations
// =============================================================================

// TestBeadsOps_Conformance runs the conformance suite against all implementations.
func TestBeadsOps_Conformance(t *testing.T) {
	t.Run("Fake", func(t *testing.T) {
		RunConformanceTests(t, newFakeBeadsOpsFactory)
	})

	t.Run("Real/FromRig", func(t *testing.T) {
		if !hasBdCLI() {
			t.Skip("bd CLI not available")
		}
		RunConformanceTests(t, newRealBeadsOpsFromRigFactory)
	})

	t.Run("Real/FromTown", func(t *testing.T) {
		if !hasBdCLI() {
			t.Skip("bd CLI not available")
		}
		RunConformanceTests(t, newRealBeadsOpsFromTownFactory)
	})
}

// =============================================================================
// Cross-Context Conformance Tests
// =============================================================================

// CrossContextEnv holds two BeadsOps for cross-context testing.
type CrossContextEnv struct {
	TownOps   BeadsOps
	RigOps    BeadsOps
	AddToBead func(beadID, status string, labels []string, toRig bool) // toRig=true adds to rig, false to town
	Cleanup   func()
}

type CrossContextFactory func(t *testing.T) *CrossContextEnv

// TestBeadsOps_CrossContext_Conformance verifies that beads created in one context
// (rig) are NOT visible via ListByLabel from another context (town), and vice versa.
func TestBeadsOps_CrossContext_Conformance(t *testing.T) {
	t.Run("Fake", func(t *testing.T) {
		runCrossContextTests(t, newFakeCrossContextEnv)
	})

	t.Run("Real/FromRig", func(t *testing.T) {
		if !hasBdCLI() {
			t.Skip("bd CLI not available")
		}
		runCrossContextTests(t, newRealCrossContextEnv)
	})

	t.Run("Real/FromTown", func(t *testing.T) {
		if !hasBdCLI() {
			t.Skip("bd CLI not available")
		}
		runCrossContextTests(t, newRealCrossContextEnv)
	})
}

func runCrossContextTests(t *testing.T, factory CrossContextFactory) {
	t.Run("rig beads visible from town ListByLabelAllRigs", func(t *testing.T) {
		env := factory(t)
		defer env.Cleanup()

		// Add bead to rig
		env.AddToBead("tr-rigbead", "open", []string{"queued"}, true)

		// ListByLabelAllRigs from town SHOULD see rig bead (it iterates all rigs)
		result, err := env.TownOps.ListByLabelAllRigs("queued")
		if err != nil {
			t.Fatalf("ListByLabelAllRigs failed: %v", err)
		}

		found := false
		for _, beads := range result {
			for _, b := range beads {
				if b.ID == "tr-rigbead" {
					found = true
				}
			}
		}
		if !found {
			t.Error("expected ListByLabelAllRigs to find rig bead (it iterates all rigs)")
		}
	})

	t.Run("town beads not visible from rig ListByLabelAllRigs", func(t *testing.T) {
		env := factory(t)
		defer env.Cleanup()

		// Add bead to town
		env.AddToBead("hq-townbead", "open", []string{"queued"}, false)

		// ListByLabelAllRigs from rig should NOT see town bead
		// (rig ops only knows about rigs, not town)
		result, err := env.RigOps.ListByLabelAllRigs("queued")
		if err != nil {
			t.Fatalf("ListByLabelAllRigs failed: %v", err)
		}

		for _, beads := range result {
			for _, b := range beads {
				if b.ID == "hq-townbead" {
					t.Error("rig ListByLabelAllRigs should not see town bead")
				}
			}
		}
	})
}

// =============================================================================
// Factory Functions - Fake Implementation
// =============================================================================

func newFakeBeadsOpsFactory(t *testing.T) *BeadsOpsTestEnv {
	ops := NewFakeBeadsOpsForRig("testrig")
	ops.AddRouteWithRig("tr-", "testrig", ops)

	return &BeadsOpsTestEnv{
		Ops: ops,
		AddBead: func(beadID, status string, labels []string) {
			ops.AddBead(beadID, status, labels)
		},
		Cleanup: func() {},
	}
}

func newFakeCrossContextEnv(t *testing.T) *CrossContextEnv {
	townOps := NewFakeBeadsOpsForRig("town")
	rigOps := NewFakeBeadsOpsForRig("testrig")

	// Set up routing: town routes tr- to rig
	townOps.AddRouteWithRig("hq-", "town", townOps)
	townOps.AddRouteWithRig("tr-", "testrig", rigOps)

	// Rig only knows about itself
	rigOps.AddRouteWithRig("tr-", "testrig", rigOps)

	return &CrossContextEnv{
		TownOps: townOps,
		RigOps:  rigOps,
		AddToBead: func(beadID, status string, labels []string, toRig bool) {
			if toRig {
				rigOps.AddBead(beadID, status, labels)
			} else {
				townOps.AddBead(beadID, status, labels)
			}
		},
		Cleanup: func() {},
	}
}

// =============================================================================
// Factory Functions - Real Implementation
// =============================================================================

func hasBdCLI() bool {
	_, err := exec.LookPath("bd")
	return err == nil
}

func newRealBeadsOpsFromRigFactory(t *testing.T) *BeadsOpsTestEnv {
	townRoot, rigPath, cleanup := setupTestTownWithBd(t)

	ops := NewRealBeadsOps(townRoot)

	return &BeadsOpsTestEnv{
		Ops:      ops,
		TownRoot: townRoot,
		RigPath:  rigPath,
		AddBead: func(beadID, status string, labels []string) {
			addBeadWithBd(t, rigPath, beadID, status, labels)
		},
		Cleanup: cleanup,
	}
}

func newRealBeadsOpsFromTownFactory(t *testing.T) *BeadsOpsTestEnv {
	townRoot, rigPath, cleanup := setupTestTownWithBd(t)

	ops := NewRealBeadsOps(townRoot)

	return &BeadsOpsTestEnv{
		Ops:      ops,
		TownRoot: townRoot,
		RigPath:  rigPath,
		AddBead: func(beadID, status string, labels []string) {
			// Add beads to the rig (tr- prefix routes there)
			addBeadWithBd(t, rigPath, beadID, status, labels)
		},
		Cleanup: cleanup,
	}
}

func newRealCrossContextEnv(t *testing.T) *CrossContextEnv {
	townRoot, rigPath, cleanup := setupTestTownWithBd(t)

	townOps := NewRealBeadsOps(townRoot)
	rigOps := NewRealBeadsOps(townRoot)

	return &CrossContextEnv{
		TownOps: townOps,
		RigOps:  rigOps,
		AddToBead: func(beadID, status string, labels []string, toRig bool) {
			if toRig {
				addBeadWithBd(t, rigPath, beadID, status, labels)
			} else {
				addBeadWithBd(t, townRoot, beadID, status, labels)
			}
		},
		Cleanup: cleanup,
	}
}

// =============================================================================
// Test Environment Setup
// =============================================================================

// setupTestTownWithBd creates a production-like test environment:
//
//	tmpdir/town/
//	├── .beads/
//	│   ├── config.yaml (prefix: hq)
//	│   ├── issues.jsonl
//	│   └── routes.jsonl (tr- -> testrig)
//	├── mayor/
//	│   └── rigs.json
//	└── testrig/
//	    └── .beads/
//	        ├── config.yaml (prefix: tr)
//	        └── issues.jsonl
func setupTestTownWithBd(t *testing.T) (townRoot, rigPath string, cleanup func()) {
	t.Helper()

	tmpdir, err := os.MkdirTemp("", "beadsops-test-*")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}

	townRoot = filepath.Join(tmpdir, "town")
	rigPath = filepath.Join(townRoot, "testrig")

	// Create directory structure
	if err := os.MkdirAll(townRoot, 0755); err != nil {
		t.Fatalf("creating town dir: %v", err)
	}
	if err := os.MkdirAll(rigPath, 0755); err != nil {
		t.Fatalf("creating rig dir: %v", err)
	}

	// Create mayor directory for rigs.json
	mayorDir := filepath.Join(townRoot, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatalf("creating mayor dir: %v", err)
	}

	// Initialize bd in town with hq- prefix
	initBd(t, townRoot, "hq")

	// Initialize bd in rig with tr- prefix
	initBd(t, rigPath, "tr")

	// Add route from town to rig (relative path)
	addRoute(t, townRoot, "tr-", "testrig")

	// Write rigs.json for RealBeadsOps
	rigsContent := `{"version":1,"rigs":{"testrig":{"git_url":"","added_at":"2024-01-01T00:00:00Z"}}}`
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"), []byte(rigsContent), 0644); err != nil {
		t.Fatalf("writing rigs.json: %v", err)
	}

	cleanup = func() {
		os.RemoveAll(tmpdir)
	}

	return townRoot, rigPath, cleanup
}

func initBd(t *testing.T, dir, prefix string) {
	t.Helper()
	cmd := exec.Command("bd", "init", "--prefix", prefix)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bd init failed in %s: %v\n%s", dir, err, output)
	}
}

func addRoute(t *testing.T, dir, prefix, targetPath string) {
	t.Helper()
	// Write route directly to routes.jsonl (bd doesn't have a route command)
	routesFile := filepath.Join(dir, ".beads", "routes.jsonl")
	routeEntry := fmt.Sprintf(`{"prefix":"%s","path":"%s"}`+"\n", prefix, targetPath)

	f, err := os.OpenFile(routesFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("opening routes.jsonl: %v", err)
	}
	defer f.Close()

	if _, err := f.WriteString(routeEntry); err != nil {
		t.Fatalf("writing route: %v", err)
	}
}

func addBeadWithBd(t *testing.T, dir, beadID, status string, labels []string) {
	t.Helper()

	// Create the bead - running from dir uses dir/.beads
	args := []string{"--no-daemon", "create", "--id", beadID, "--title", "Test bead " + beadID}
	cmd := exec.Command("bd", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bd create failed for %s: %v\n%s", beadID, err, output)
	}

	// Set status if not open (default)
	if status != "open" && status != "" {
		cmd = exec.Command("bd", "--no-daemon", "update", beadID, "--status", status)
		cmd.Dir = dir
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("bd update status failed for %s: %v\n%s", beadID, err, output)
		}
	}

	// Add labels
	for _, label := range labels {
		cmd = exec.Command("bd", "--no-daemon", "update", beadID, "--add-label", label)
		cmd.Dir = dir
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("bd update label failed for %s: %v\n%s", beadID, err, output)
		}
	}
}

// =============================================================================
// Additional Fake-specific tests (for test double verification)
// =============================================================================

func TestBeadsOps_Fake_GetRigForBead_WithRouting(t *testing.T) {
	ops := NewFakeBeadsOpsForRig("gastown")
	ops.AddRouteWithRig("gt-", "gastown", ops)
	ops.AddRouteWithRig("gp-", "greenplace", ops)

	tests := []struct {
		beadID      string
		expectedRig string
	}{
		{"gt-abc", "gastown"},
		{"gp-xyz", "greenplace"},
		{"xx-unknown", ""},
		{"hq-town", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.beadID, func(t *testing.T) {
			got := ops.GetRigForBead(tt.beadID)
			if got != tt.expectedRig {
				t.Errorf("GetRigForBead(%q) = %q, want %q", tt.beadID, got, tt.expectedRig)
			}
		})
	}
}

func TestBeadsOps_Fake_LabelAdd_CreatesLabels(t *testing.T) {
	ops := NewFakeBeadsOpsForRig("gastown")
	ops.AddRouteWithRig("gt-", "gastown", ops)
	ops.AddBead("gt-abc", "open", []string{})

	// Add label
	if err := ops.LabelAdd("gt-abc", "queued"); err != nil {
		t.Fatalf("LabelAdd failed: %v", err)
	}

	// Verify via GetLabels (fake-specific)
	labels := ops.GetLabels("gt-abc")
	if len(labels) != 1 || labels[0] != "queued" {
		t.Errorf("expected [queued], got %v", labels)
	}

	// Add another label
	if err := ops.LabelAdd("gt-abc", "priority"); err != nil {
		t.Fatalf("LabelAdd failed: %v", err)
	}

	labels = ops.GetLabels("gt-abc")
	if len(labels) != 2 {
		t.Errorf("expected 2 labels, got %v", labels)
	}
}

func TestBeadsOps_Fake_ListByLabelAllRigs_MultiRig(t *testing.T) {
	// Create two separate fake ops (simulating two rigs)
	gasOps := NewFakeBeadsOpsForRig("gastown")
	greenOps := NewFakeBeadsOpsForRig("greenplace")

	// Set up routing on gasOps (the "main" ops we'll use)
	gasOps.AddRouteWithRig("gt-", "gastown", gasOps)
	gasOps.AddRouteWithRig("gp-", "greenplace", greenOps)

	// Add beads to each rig
	gasOps.AddBead("gt-abc", "open", []string{"queued"})
	greenOps.AddBead("gp-xyz", "open", []string{"queued"})

	// List should find beads from both rigs
	result, err := gasOps.ListByLabelAllRigs("queued")
	if err != nil {
		t.Fatalf("ListByLabelAllRigs failed: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("expected 2 rigs, got %d: %v", len(result), result)
	}

	if len(result["gastown"]) != 1 {
		t.Errorf("expected 1 bead in gastown, got %d", len(result["gastown"]))
	}

	if len(result["greenplace"]) != 1 {
		t.Errorf("expected 1 bead in greenplace, got %d", len(result["greenplace"]))
	}
}
