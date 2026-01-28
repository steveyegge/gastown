package doctor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewStaleAgentBeadsCheck(t *testing.T) {
	check := NewStaleAgentBeadsCheck()

	if check.Name() != "stale-agent-beads" {
		t.Errorf("expected name 'stale-agent-beads', got %q", check.Name())
	}

	if !check.CanFix() {
		t.Error("expected CanFix to return true")
	}

	if check.Description() != "Detect agent beads for removed crew members" {
		t.Errorf("unexpected description: %q", check.Description())
	}

	if check.Category() != CategoryRig {
		t.Errorf("expected category %q, got %q", CategoryRig, check.Category())
	}
}

func TestStaleAgentBeadsCheck_NoRoutes(t *testing.T) {
	tmpDir := t.TempDir()

	// No .beads dir at all — LoadRoutes returns empty, so check returns OK (no rigs)
	check := NewStaleAgentBeadsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	// With no routes, there are no rigs to check, so result is OK or Warning
	if result.Status != StatusOK && result.Status != StatusWarning {
		t.Errorf("expected StatusOK or StatusWarning, got %v: %s", result.Status, result.Message)
	}
}

func TestStaleAgentBeadsCheck_NoRigs(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .beads dir with empty routes.jsonl
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(beadsDir, "routes.jsonl"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewStaleAgentBeadsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for no rigs, got %v: %s", result.Status, result.Message)
	}
}

func TestStaleAgentBeadsCheck_CrewOnDisk(t *testing.T) {
	tmpDir := t.TempDir()

	// Set up routes pointing to a rig
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	routesContent := `{"prefix":"gt-","path":"myrig/mayor/rig"}` + "\n"
	if err := os.WriteFile(filepath.Join(beadsDir, "routes.jsonl"), []byte(routesContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create rig beads directory
	rigBeadsDir := filepath.Join(tmpDir, "myrig", "mayor", "rig", ".beads")
	if err := os.MkdirAll(rigBeadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create crew on disk
	crewDir := filepath.Join(tmpDir, "myrig", "crew")
	for _, name := range []string{"alice", "bob"} {
		if err := os.MkdirAll(filepath.Join(crewDir, name), 0755); err != nil {
			t.Fatal(err)
		}
	}

	check := NewStaleAgentBeadsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	// Without a running bd daemon, List() will fail gracefully
	// The check should handle this and not crash
	result := check.Run(ctx)
	t.Logf("Stale agent beads check: status=%v, message=%s", result.Status, result.Message)
}
