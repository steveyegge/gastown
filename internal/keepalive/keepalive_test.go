package keepalive

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTouchInWorkspace(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	// Touch the keepalive
	TouchInWorkspace(tmpDir, "gt status")

	// Read back
	state := Read(tmpDir)
	if state == nil {
		t.Fatal("expected state to be non-nil")
	}

	if state.LastCommand != "gt status" {
		t.Errorf("expected last_command 'gt status', got %q", state.LastCommand)
	}

	// Check timestamp is recent
	if time.Since(state.Timestamp) > time.Minute {
		t.Errorf("timestamp too old: %v", state.Timestamp)
	}
}

func TestReadNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	state := Read(tmpDir)
	if state != nil {
		t.Error("expected nil state for non-existent file")
	}
}

func TestStateAge(t *testing.T) {
	// Test nil state returns very large age
	var nilState *State
	if nilState.Age() < 24*time.Hour {
		t.Error("nil state should have very large age")
	}

	// Test fresh state returns accurate age
	freshState := &State{Timestamp: time.Now().Add(-30 * time.Second)}
	age := freshState.Age()
	if age < 29*time.Second || age > 31*time.Second {
		t.Errorf("expected ~30s age, got %v", age)
	}

	// Test older state returns accurate age
	olderState := &State{Timestamp: time.Now().Add(-5 * time.Minute)}
	age = olderState.Age()
	if age < 4*time.Minute+55*time.Second || age > 5*time.Minute+5*time.Second {
		t.Errorf("expected ~5m age, got %v", age)
	}

	// NOTE: IsFresh(), IsStale(), IsVeryStale() were removed as part of ZFC cleanup.
	// Staleness classification belongs in Deacon molecule, not Go code.
}

func TestDirectoryCreation(t *testing.T) {
	tmpDir := t.TempDir()
	workDir := filepath.Join(tmpDir, "some", "nested", "workspace")

	// Touch should create .runtime directory
	TouchInWorkspace(workDir, "gt test")

	// Verify directory was created
	runtimeDir := filepath.Join(workDir, ".runtime")
	if _, err := os.Stat(runtimeDir); os.IsNotExist(err) {
		t.Error("expected .runtime directory to be created")
	}
}

// Example functions demonstrate keepalive usage patterns.

func ExampleTouchInWorkspace() {
	// TouchInWorkspace signals agent activity in a specific workspace.
	// This is the core function - use it when you know the workspace root.

	workspaceRoot := "/path/to/workspace"

	// Signal that "gt status" was run
	TouchInWorkspace(workspaceRoot, "gt status")

	// Signal a command with arguments
	TouchInWorkspace(workspaceRoot, "gt sling bd-abc123 ai-platform")

	// All errors are silently ignored (best-effort design).
	// This is intentional - keepalive failures should never break commands.
}

func ExampleRead() {
	// Read retrieves the current keepalive state for a workspace.
	// Returns nil if no keepalive file exists or it can't be read.

	workspaceRoot := "/path/to/workspace"
	state := Read(workspaceRoot)

	if state == nil {
		// No keepalive found - agent may not have run any commands yet
		return
	}

	// Access the last command that was run
	_ = state.LastCommand // e.g., "gt status"

	// Access when the command was run
	_ = state.Timestamp // time.Time in UTC
}

func ExampleState_Age() {
	// Age() returns how long ago the keepalive was updated.
	// This is useful for detecting idle or stuck agents.

	workspaceRoot := "/path/to/workspace"
	state := Read(workspaceRoot)

	// Age() is nil-safe - returns ~1 year for nil state
	age := state.Age()

	// Check if agent was active recently (within 5 minutes)
	if age < 5*time.Minute {
		// Agent is active
		_ = "active"
	}

	// Check if agent might be stuck (no activity for 30+ minutes)
	if age > 30*time.Minute {
		// Agent may need attention
		_ = "possibly stuck"
	}
}
