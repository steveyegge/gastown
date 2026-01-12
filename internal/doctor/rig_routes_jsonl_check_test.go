package doctor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRigRoutesJSONLCheck_Run(t *testing.T) {
	t.Run("no rigs returns OK", func(t *testing.T) {
		tmpDir := t.TempDir()
		// Create minimal town structure
		if err := os.MkdirAll(filepath.Join(tmpDir, "mayor"), 0755); err != nil {
			t.Fatal(err)
		}

		check := NewRigRoutesJSONLCheck()
		ctx := &CheckContext{TownRoot: tmpDir}
		result := check.Run(ctx)

		if result.Status != StatusOK {
			t.Errorf("expected StatusOK, got %v: %s", result.Status, result.Message)
		}
	})

	t.Run("rig without routes.jsonl returns OK", func(t *testing.T) {
		tmpDir := t.TempDir()
		// Create rig with .beads but no routes.jsonl
		rigBeads := filepath.Join(tmpDir, "myrig", ".beads")
		if err := os.MkdirAll(rigBeads, 0755); err != nil {
			t.Fatal(err)
		}

		check := NewRigRoutesJSONLCheck()
		ctx := &CheckContext{TownRoot: tmpDir}
		result := check.Run(ctx)

		if result.Status != StatusOK {
			t.Errorf("expected StatusOK, got %v: %s", result.Status, result.Message)
		}
	})

	t.Run("rig with routes.jsonl warns", func(t *testing.T) {
		tmpDir := t.TempDir()
		rigBeads := filepath.Join(tmpDir, "myrig", ".beads")
		if err := os.MkdirAll(rigBeads, 0755); err != nil {
			t.Fatal(err)
		}

		// Create routes.jsonl (any content - will be deleted)
		if err := os.WriteFile(filepath.Join(rigBeads, "routes.jsonl"), []byte(`{"prefix":"x-","path":"."}`+"\n"), 0644); err != nil {
			t.Fatal(err)
		}

		check := NewRigRoutesJSONLCheck()
		ctx := &CheckContext{TownRoot: tmpDir}
		result := check.Run(ctx)

		if result.Status != StatusWarning {
			t.Errorf("expected StatusWarning, got %v: %s", result.Status, result.Message)
		}
		if len(result.Details) == 0 {
			t.Error("expected details about the issue")
		}
	})

	t.Run("multiple rigs with routes.jsonl reports all", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create two rigs with routes.jsonl
		for _, rigName := range []string{"rig1", "rig2"} {
			rigBeads := filepath.Join(tmpDir, rigName, ".beads")
			if err := os.MkdirAll(rigBeads, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(rigBeads, "routes.jsonl"), []byte(`{"prefix":"x-","path":"."}`+"\n"), 0644); err != nil {
				t.Fatal(err)
			}
		}

		check := NewRigRoutesJSONLCheck()
		ctx := &CheckContext{TownRoot: tmpDir}
		result := check.Run(ctx)

		if result.Status != StatusWarning {
			t.Errorf("expected StatusWarning, got %v", result.Status)
		}
		if len(result.Details) != 2 {
			t.Errorf("expected 2 details, got %d: %v", len(result.Details), result.Details)
		}
	})
}

func TestRigRoutesJSONLCheck_Fix(t *testing.T) {
	t.Run("deletes routes.jsonl unconditionally", func(t *testing.T) {
		tmpDir := t.TempDir()
		rigBeads := filepath.Join(tmpDir, "myrig", ".beads")
		if err := os.MkdirAll(rigBeads, 0755); err != nil {
			t.Fatal(err)
		}

		// Create routes.jsonl with any content
		routesPath := filepath.Join(rigBeads, "routes.jsonl")
		if err := os.WriteFile(routesPath, []byte(`{"id":"test-abc123","title":"Test Issue"}`+"\n"), 0644); err != nil {
			t.Fatal(err)
		}

		check := NewRigRoutesJSONLCheck()
		ctx := &CheckContext{TownRoot: tmpDir}

		// Run check first to populate affectedRigs
		result := check.Run(ctx)
		if result.Status != StatusWarning {
			t.Fatalf("expected StatusWarning, got %v", result.Status)
		}

		// Fix
		if err := check.Fix(ctx); err != nil {
			t.Fatalf("Fix() error: %v", err)
		}

		// Verify routes.jsonl is gone
		if _, err := os.Stat(routesPath); !os.IsNotExist(err) {
			t.Error("routes.jsonl should have been deleted")
		}
	})

	t.Run("fix is idempotent", func(t *testing.T) {
		tmpDir := t.TempDir()
		rigBeads := filepath.Join(tmpDir, "myrig", ".beads")
		if err := os.MkdirAll(rigBeads, 0755); err != nil {
			t.Fatal(err)
		}

		check := NewRigRoutesJSONLCheck()
		ctx := &CheckContext{TownRoot: tmpDir}

		// First run - should pass (no routes.jsonl)
		result := check.Run(ctx)
		if result.Status != StatusOK {
			t.Fatalf("expected StatusOK, got %v", result.Status)
		}

		// Fix should be no-op
		if err := check.Fix(ctx); err != nil {
			t.Fatalf("Fix() error on clean state: %v", err)
		}
	})
}

func TestRigRoutesJSONLCheck_FindRigDirectories(t *testing.T) {
	t.Run("finds rigs from multiple sources", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create mayor directory
		if err := os.MkdirAll(filepath.Join(tmpDir, "mayor"), 0755); err != nil {
			t.Fatal(err)
		}

		// Create town-level .beads with routes.jsonl
		townBeads := filepath.Join(tmpDir, ".beads")
		if err := os.MkdirAll(townBeads, 0755); err != nil {
			t.Fatal(err)
		}
		routes := `{"prefix":"rig1-","path":"rig1/mayor/rig"}` + "\n"
		if err := os.WriteFile(filepath.Join(townBeads, "routes.jsonl"), []byte(routes), 0644); err != nil {
			t.Fatal(err)
		}

		// Create rig1 (from routes.jsonl)
		if err := os.MkdirAll(filepath.Join(tmpDir, "rig1", ".beads"), 0755); err != nil {
			t.Fatal(err)
		}

		// Create rig2 (unregistered but has .beads)
		if err := os.MkdirAll(filepath.Join(tmpDir, "rig2", ".beads"), 0755); err != nil {
			t.Fatal(err)
		}

		check := NewRigRoutesJSONLCheck()
		rigs := check.findRigDirectories(tmpDir)

		if len(rigs) != 2 {
			t.Errorf("expected 2 rigs, got %d: %v", len(rigs), rigs)
		}
	})

	t.Run("excludes mayor and .beads directories", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create directories that should be excluded
		if err := os.MkdirAll(filepath.Join(tmpDir, "mayor", ".beads"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(tmpDir, ".beads"), 0755); err != nil {
			t.Fatal(err)
		}

		check := NewRigRoutesJSONLCheck()
		rigs := check.findRigDirectories(tmpDir)

		if len(rigs) != 0 {
			t.Errorf("expected 0 rigs (mayor and .beads should be excluded), got %d: %v", len(rigs), rigs)
		}
	})
}
