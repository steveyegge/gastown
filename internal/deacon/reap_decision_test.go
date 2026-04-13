package deacon

import (
	"os"
	"path/filepath"
	"testing"
)

// Tests for decision logging in ScanCompletedPolecatsCtx.
// Every scanned polecat must produce a ReapDecision entry explaining
// whether it was eligible for reaping and why.

func TestReapDecision_Fields(t *testing.T) {
	d := &ReapDecision{
		Rig:     "gastown-prime",
		Polecat: "obsidian",
		Reason:  "no_session",
	}

	if d.Rig != "gastown-prime" {
		t.Errorf("Rig = %q, want gastown-prime", d.Rig)
	}
	if d.Reason != "no_session" {
		t.Errorf("Reason = %q, want no_session", d.Reason)
	}
	if d.Eligible {
		t.Error("Eligible should default to false")
	}
}

func TestReapDecision_EligibleWithBeadInfo(t *testing.T) {
	d := &ReapDecision{
		Rig:        "gastown-prime",
		Polecat:    "obsidian",
		Eligible:   true,
		Reason:     "bead_closed",
		BeadID:     "sbx-gastown-abc",
		BeadStatus: "closed",
		HasSession: true,
	}

	if !d.Eligible {
		t.Error("Eligible should be true")
	}
	if d.BeadID != "sbx-gastown-abc" {
		t.Errorf("BeadID = %q, want sbx-gastown-abc", d.BeadID)
	}
	if d.BeadStatus != "closed" {
		t.Errorf("BeadStatus = %q, want closed", d.BeadStatus)
	}
}

func TestReapScanResult_HasDecisions(t *testing.T) {
	result := &ReapScanResult{
		Results:   make([]*ReapResult, 0),
		Decisions: make([]*ReapDecision, 0),
	}

	if result.Decisions == nil {
		t.Error("Decisions should be initialized")
	}
	if len(result.Decisions) != 0 {
		t.Errorf("Decisions should be empty, got %d", len(result.Decisions))
	}
}

func TestScanCompletedPolecats_DecisionsForNoSession(t *testing.T) {
	// Polecats with directories but no tmux session should produce
	// a decision entry with reason "no_session".
	townRoot := t.TempDir()

	rigDir := filepath.Join(townRoot, "testrig", "polecats")
	if err := os.MkdirAll(filepath.Join(rigDir, "alpha"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(rigDir, "beta"), 0755); err != nil {
		t.Fatal(err)
	}

	cfg := DefaultReapConfig()
	result, err := ScanCompletedPolecats(townRoot, cfg)
	if err != nil {
		t.Fatalf("ScanCompletedPolecats error: %v", err)
	}

	if len(result.Decisions) != 2 {
		t.Fatalf("expected 2 decisions, got %d", len(result.Decisions))
	}

	for _, d := range result.Decisions {
		if d.Reason != "no_session" {
			t.Errorf("polecat %s: reason = %q, want no_session", d.Polecat, d.Reason)
		}
		if d.Eligible {
			t.Errorf("polecat %s: should not be eligible (no session)", d.Polecat)
		}
		if d.HasSession {
			t.Errorf("polecat %s: HasSession should be false", d.Polecat)
		}
	}
}

func TestScanCompletedPolecats_DecisionCountMatchesTotal(t *testing.T) {
	// The number of decisions must equal TotalPolecats — every polecat gets a decision.
	townRoot := t.TempDir()

	for _, name := range []string{"alpha", "beta", "gamma"} {
		dirPath := filepath.Join(townRoot, "testrig", "polecats", name)
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			t.Fatal(err)
		}
	}

	cfg := DefaultReapConfig()
	result, err := ScanCompletedPolecats(townRoot, cfg)
	if err != nil {
		t.Fatalf("ScanCompletedPolecats error: %v", err)
	}

	if len(result.Decisions) != result.TotalPolecats {
		t.Errorf("len(Decisions) = %d, TotalPolecats = %d — every polecat must have a decision",
			len(result.Decisions), result.TotalPolecats)
	}
}

func TestScanCompletedPolecats_EmptyTownNoDecisions(t *testing.T) {
	townRoot := t.TempDir()

	cfg := DefaultReapConfig()
	result, err := ScanCompletedPolecats(townRoot, cfg)
	if err != nil {
		t.Fatalf("ScanCompletedPolecats error: %v", err)
	}

	if len(result.Decisions) != 0 {
		t.Errorf("expected 0 decisions for empty town, got %d", len(result.Decisions))
	}
}
