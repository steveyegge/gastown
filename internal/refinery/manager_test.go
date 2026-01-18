package refinery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/rig"
)

func setupTestManager(t *testing.T) (*Manager, string) {
	t.Helper()

	// Create temp directory structure
	tmpDir := t.TempDir()
	rigPath := filepath.Join(tmpDir, "testrig")
	if err := os.MkdirAll(filepath.Join(rigPath, ".runtime"), 0755); err != nil {
		t.Fatalf("mkdir .runtime: %v", err)
	}

	r := &rig.Rig{
		Name: "testrig",
		Path: rigPath,
	}

	return NewManager(r), rigPath
}

func TestManager_GetMR(t *testing.T) {
	mgr, _ := setupTestManager(t)

	// Create a test MR in the pending queue
	mr := &MergeRequest{
		ID:       "gt-mr-abc123",
		Branch:   "polecat/Toast/gt-xyz",
		Worker:   "Toast",
		IssueID:  "gt-xyz",
		Status:   MROpen,
		Error:    "test failure",
	}

	if err := mgr.RegisterMR(mr); err != nil {
		t.Fatalf("RegisterMR: %v", err)
	}

	t.Run("find existing MR", func(t *testing.T) {
		found, err := mgr.GetMR("gt-mr-abc123")
		if err != nil {
			t.Errorf("GetMR() unexpected error: %v", err)
		}
		if found == nil {
			t.Fatal("GetMR() returned nil")
		}
		if found.ID != mr.ID {
			t.Errorf("GetMR() ID = %s, want %s", found.ID, mr.ID)
		}
	})

	t.Run("MR not found", func(t *testing.T) {
		_, err := mgr.GetMR("nonexistent-mr")
		if err != ErrMRNotFound {
			t.Errorf("GetMR() error = %v, want %v", err, ErrMRNotFound)
		}
	})
}

func TestManager_Retry(t *testing.T) {
	t.Run("retry failed MR clears error", func(t *testing.T) {
		mgr, _ := setupTestManager(t)

		// Create a failed MR
		mr := &MergeRequest{
			ID:       "gt-mr-failed",
			Branch:   "polecat/Toast/gt-xyz",
			Worker:   "Toast",
			Status:   MROpen,
			Error:    "merge conflict",
		}

		if err := mgr.RegisterMR(mr); err != nil {
			t.Fatalf("RegisterMR: %v", err)
		}

		// Retry without processing
		err := mgr.Retry("gt-mr-failed", false)
		if err != nil {
			t.Errorf("Retry() unexpected error: %v", err)
		}

		// Verify error was cleared
		found, _ := mgr.GetMR("gt-mr-failed")
		if found.Error != "" {
			t.Errorf("Retry() error not cleared, got %s", found.Error)
		}
	})

	t.Run("retry non-failed MR fails", func(t *testing.T) {
		mgr, _ := setupTestManager(t)

		// Create a successful MR (no error)
		mr := &MergeRequest{
			ID:     "gt-mr-success",
			Branch: "polecat/Toast/gt-abc",
			Worker: "Toast",
			Status: MROpen,
			Error:  "", // No error
		}

		if err := mgr.RegisterMR(mr); err != nil {
			t.Fatalf("RegisterMR: %v", err)
		}

		err := mgr.Retry("gt-mr-success", false)
		if err != ErrMRNotFailed {
			t.Errorf("Retry() error = %v, want %v", err, ErrMRNotFailed)
		}
	})

	t.Run("retry nonexistent MR fails", func(t *testing.T) {
		mgr, _ := setupTestManager(t)

		err := mgr.Retry("nonexistent", false)
		if err != ErrMRNotFound {
			t.Errorf("Retry() error = %v, want %v", err, ErrMRNotFound)
		}
	})
}

func TestManager_Status_ReconcilesStaleTmuxState(t *testing.T) {
	mgr, rigPath := setupTestManager(t)

	// Write a state file that claims "running" even though no tmux session exists
	now := time.Now()
	staleState := &Refinery{
		RigName:   "testrig",
		State:     StateRunning, // Lie: claims running
		StartedAt: &now,
	}

	stateFile := filepath.Join(rigPath, ".runtime", "refinery.json")
	data, err := json.Marshal(staleState)
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	if err := os.WriteFile(stateFile, data, 0644); err != nil {
		t.Fatalf("write state file: %v", err)
	}

	// Status() should reconcile with reality - no tmux session means stopped
	status, err := mgr.Status()
	if err != nil {
		t.Fatalf("Status() unexpected error: %v", err)
	}

	// The session name would be "gt-testrig-refinery" which doesn't exist
	// So Status() should return stopped, not running
	if status.State != StateStopped {
		t.Errorf("Status().State = %v, want %v (should reconcile stale state)", status.State, StateStopped)
	}
}

func TestManager_RegisterMR(t *testing.T) {
	mgr, rigPath := setupTestManager(t)

	mr := &MergeRequest{
		ID:           "gt-mr-new",
		Branch:       "polecat/Cheedo/gt-123",
		Worker:       "Cheedo",
		IssueID:      "gt-123",
		TargetBranch: "main",
		CreatedAt:    time.Now(),
		Status:       MROpen,
	}

	if err := mgr.RegisterMR(mr); err != nil {
		t.Fatalf("RegisterMR: %v", err)
	}

	// Verify it was saved to disk
	stateFile := filepath.Join(rigPath, ".runtime", "refinery.json")
	data, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("reading state file: %v", err)
	}

	var ref Refinery
	if err := json.Unmarshal(data, &ref); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}

	if ref.PendingMRs == nil {
		t.Fatal("PendingMRs is nil")
	}

	saved, ok := ref.PendingMRs["gt-mr-new"]
	if !ok {
		t.Fatal("MR not found in PendingMRs")
	}

	if saved.Worker != "Cheedo" {
		t.Errorf("saved MR worker = %s, want Cheedo", saved.Worker)
	}
}
