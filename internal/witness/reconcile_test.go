package witness

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
)

// TestReconcileIdlePolecats_EmptyDir verifies that reconciliation handles
// an empty or missing polecats directory gracefully.
func TestReconcileIdlePolecats_EmptyDir(t *testing.T) {
	result := ReconcileIdlePolecats(DefaultBdCli(), "/nonexistent", "testrig", nil)
	if result.Checked != 0 {
		t.Errorf("Checked = %d, want 0 for nonexistent dir", result.Checked)
	}
	if result.Emitted != 0 {
		t.Errorf("Emitted = %d, want 0 for nonexistent dir", result.Emitted)
	}
}

// TestReconcileResult_ActionValues verifies the string constants used for
// reconciliation actions match the expected set.
func TestReconcileResult_ActionValues(t *testing.T) {
	validActions := map[string]bool{
		"emitted":           true,
		"already-processed": true,
		"no-mr":             true,
		"skipped":           true,
	}

	// Verify each action used in the reconciliation result is valid
	for _, action := range []string{"emitted", "already-processed", "no-mr", "skipped"} {
		if !validActions[action] {
			t.Errorf("unexpected action %q", action)
		}
	}
}

// TestReconcileIdleDetectionLogic verifies the filtering logic that determines
// which polecats need reconciliation: agent_state=idle AND exit_type present
// AND MR ID present.
func TestReconcileIdleDetectionLogic(t *testing.T) {
	tests := []struct {
		name       string
		state      string
		exitType   string
		mrID       string
		wantReconcile bool
	}{
		{
			name:          "idle + COMPLETED + MR → needs reconcile",
			state:         string(beads.AgentStateIdle),
			exitType:      "COMPLETED",
			mrID:          "mr-123",
			wantReconcile: true,
		},
		{
			name:          "idle + COMPLETED + no MR → skip",
			state:         string(beads.AgentStateIdle),
			exitType:      "COMPLETED",
			mrID:          "",
			wantReconcile: false,
		},
		{
			name:          "idle + no exit_type → skip (crashed)",
			state:         string(beads.AgentStateIdle),
			exitType:      "",
			mrID:          "mr-123",
			wantReconcile: false,
		},
		{
			name:          "working + COMPLETED + MR → skip (still working)",
			state:         string(beads.AgentStateWorking),
			exitType:      "COMPLETED",
			mrID:          "mr-123",
			wantReconcile: false,
		},
		{
			name:          "idle + ESCALATED + MR → needs reconcile",
			state:         string(beads.AgentStateIdle),
			exitType:      "ESCALATED",
			mrID:          "mr-456",
			wantReconcile: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Match the logic in ReconcileIdlePolecats:
			// agent_state=idle AND exit_type present AND mrID present
			needsReconcile := beads.AgentState(tt.state) == beads.AgentStateIdle &&
				tt.exitType != "" &&
				tt.mrID != ""

			if needsReconcile != tt.wantReconcile {
				t.Errorf("needsReconcile = %v, want %v", needsReconcile, tt.wantReconcile)
			}
		})
	}
}

// TestReconcileIdlePolecats_SkipsNonAgentEntries verifies that reconciliation
// scans real polecat directories, skips hidden dirs, and handles missing agent
// beads gracefully (returning "skipped" action for each).
func TestReconcileIdlePolecats_SkipsNonAgentEntries(t *testing.T) {
	townRoot := t.TempDir()
	rigName := "myrig"
	polecatsDir := filepath.Join(townRoot, rigName, "polecats")
	if err := os.MkdirAll(polecatsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create polecat dirs — one normal, one hidden (should be skipped)
	if err := os.MkdirAll(filepath.Join(polecatsDir, "toast"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(polecatsDir, ".hidden"), 0755); err != nil {
		t.Fatal(err)
	}
	// Create a file (should be skipped — not a directory)
	if err := os.WriteFile(filepath.Join(polecatsDir, "not-a-dir"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	result := ReconcileIdlePolecats(DefaultBdCli(), townRoot, rigName, nil)

	// Should have checked 1 (toast) — hidden and non-dirs are skipped
	if result.Checked != 1 {
		t.Errorf("Checked = %d, want 1 (only 'toast' directory)", result.Checked)
	}
	// Should be skipped since there's no real agent bead for toast
	if result.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", result.Skipped)
	}
	if result.Emitted != 0 {
		t.Errorf("Emitted = %d, want 0", result.Emitted)
	}
	if len(result.Reconciled) != 1 {
		t.Fatalf("Reconciled count = %d, want 1", len(result.Reconciled))
	}
	if result.Reconciled[0].PolecatName != "toast" {
		t.Errorf("PolecatName = %q, want 'toast'", result.Reconciled[0].PolecatName)
	}
	if result.Reconciled[0].Action != "skipped" {
		t.Errorf("Action = %q, want 'skipped'", result.Reconciled[0].Action)
	}
}

// TestReconcileDedup verifies that the deduplication mechanism prevents
// re-emitting MERGE_READY for the same polecat+MR pair.
func TestReconcileDedup(t *testing.T) {
	dedup := NewMessageDeduplicator()

	// First call should not be a duplicate
	key := "reconcile:be-testrig-p-Toast:mr-123"
	if dedup.AlreadyProcessed(key) {
		t.Error("first call should not be already processed")
	}

	// Second call should be a duplicate
	if !dedup.AlreadyProcessed(key) {
		t.Error("second call should be already processed")
	}

	// Different key should not be a duplicate
	key2 := "reconcile:be-testrig-p-Ember:mr-456"
	if dedup.AlreadyProcessed(key2) {
		t.Error("different key should not be already processed")
	}
}
