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
