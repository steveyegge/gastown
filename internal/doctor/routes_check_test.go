package doctor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRoutesCheck_MissingTownRoute(t *testing.T) {
	t.Run("detects missing town root route", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create .beads directory with routes.jsonl missing the hq- route
		beadsDir := filepath.Join(tmpDir, ".beads")
		if err := os.MkdirAll(beadsDir, 0755); err != nil {
			t.Fatal(err)
		}

		// Create routes.jsonl with only a rig route (no hq- or hq-cv- routes)
		routesPath := filepath.Join(beadsDir, "routes.jsonl")
		routesContent := `{"prefix": "gt-", "path": "gastown/mayor/rig"}
`
		if err := os.WriteFile(routesPath, []byte(routesContent), 0644); err != nil {
			t.Fatal(err)
		}

		// Create mayor directory
		if err := os.MkdirAll(filepath.Join(tmpDir, "mayor"), 0755); err != nil {
			t.Fatal(err)
		}

		check := NewRoutesCheck()
		ctx := &CheckContext{TownRoot: tmpDir}
		result := check.Run(ctx)

		if result.Status != StatusWarning {
			t.Errorf("expected StatusWarning, got %v: %s", result.Status, result.Message)
		}
		// When no rigs.json exists, the message comes from the early return path
		if result.Message != "Required town routes are missing" {
			t.Errorf("expected 'Required town routes are missing', got %s", result.Message)
		}
	})

	t.Run("passes when town root route exists", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create .beads directory with valid routes.jsonl
		beadsDir := filepath.Join(tmpDir, ".beads")
		if err := os.MkdirAll(beadsDir, 0755); err != nil {
			t.Fatal(err)
		}

		// Create routes.jsonl with both hq- and hq-cv- routes
		routesPath := filepath.Join(beadsDir, "routes.jsonl")
		routesContent := `{"prefix": "hq-", "path": "."}
{"prefix": "hq-cv-", "path": "."}
`
		if err := os.WriteFile(routesPath, []byte(routesContent), 0644); err != nil {
			t.Fatal(err)
		}

		// Create mayor directory
		if err := os.MkdirAll(filepath.Join(tmpDir, "mayor"), 0755); err != nil {
			t.Fatal(err)
		}

		check := NewRoutesCheck()
		ctx := &CheckContext{TownRoot: tmpDir}
		result := check.Run(ctx)

		if result.Status != StatusOK {
			t.Errorf("expected StatusOK, got %v: %s", result.Status, result.Message)
		}
	})
}

func TestRoutesCheck_FixRestoresTownRoute(t *testing.T) {
	t.Run("fix adds missing town root route", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create .beads directory with empty routes.jsonl
		beadsDir := filepath.Join(tmpDir, ".beads")
		if err := os.MkdirAll(beadsDir, 0755); err != nil {
			t.Fatal(err)
		}

		// Create empty routes.jsonl
		routesPath := filepath.Join(beadsDir, "routes.jsonl")
		if err := os.WriteFile(routesPath, []byte(""), 0644); err != nil {
			t.Fatal(err)
		}

		// Create mayor directory (no rigs.json needed for this test)
		if err := os.MkdirAll(filepath.Join(tmpDir, "mayor"), 0755); err != nil {
			t.Fatal(err)
		}

		check := NewRoutesCheck()
		ctx := &CheckContext{TownRoot: tmpDir}

		// Run fix
		if err := check.Fix(ctx); err != nil {
			t.Fatalf("Fix failed: %v", err)
		}

		// Verify routes.jsonl now contains both hq- and hq-cv- routes
		content, err := os.ReadFile(routesPath)
		if err != nil {
			t.Fatalf("Failed to read routes.jsonl: %v", err)
		}

		if len(content) == 0 {
			t.Error("routes.jsonl is still empty after fix")
		}

		contentStr := string(content)
		if contentStr != `{"prefix":"hq-","path":"."}
{"prefix":"hq-cv-","path":"."}
` {
			t.Errorf("unexpected routes.jsonl content: %s", contentStr)
		}

		// Verify the check now passes
		result := check.Run(ctx)
		if result.Status != StatusOK {
			t.Errorf("expected StatusOK after fix, got %v: %s", result.Status, result.Message)
		}
	})

	t.Run("fix preserves existing routes while adding town route", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create .beads directory
		beadsDir := filepath.Join(tmpDir, ".beads")
		if err := os.MkdirAll(beadsDir, 0755); err != nil {
			t.Fatal(err)
		}

		// Create rig directory structure for route validation
		rigPath := filepath.Join(tmpDir, "myrig", "mayor", "rig", ".beads")
		if err := os.MkdirAll(rigPath, 0755); err != nil {
			t.Fatal(err)
		}

		// Create routes.jsonl with only a rig route (no hq- or hq-cv- routes)
		routesPath := filepath.Join(beadsDir, "routes.jsonl")
		routesContent := `{"prefix": "my-", "path": "myrig/mayor/rig"}
`
		if err := os.WriteFile(routesPath, []byte(routesContent), 0644); err != nil {
			t.Fatal(err)
		}

		// Create mayor directory
		if err := os.MkdirAll(filepath.Join(tmpDir, "mayor"), 0755); err != nil {
			t.Fatal(err)
		}

		check := NewRoutesCheck()
		ctx := &CheckContext{TownRoot: tmpDir}

		// Run fix
		if err := check.Fix(ctx); err != nil {
			t.Fatalf("Fix failed: %v", err)
		}

		// Verify routes.jsonl now contains all routes
		content, err := os.ReadFile(routesPath)
		if err != nil {
			t.Fatalf("Failed to read routes.jsonl: %v", err)
		}

		contentStr := string(content)
		// Should have the original rig route plus both hq- and hq-cv- routes
		if contentStr != `{"prefix":"my-","path":"myrig/mayor/rig"}
{"prefix":"hq-","path":"."}
{"prefix":"hq-cv-","path":"."}
` {
			t.Errorf("unexpected routes.jsonl content: %s", contentStr)
		}
	})

	t.Run("fix does not duplicate existing town route", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create .beads directory with valid routes.jsonl
		beadsDir := filepath.Join(tmpDir, ".beads")
		if err := os.MkdirAll(beadsDir, 0755); err != nil {
			t.Fatal(err)
		}

		// Create routes.jsonl with both hq- and hq-cv- routes already present
		routesPath := filepath.Join(beadsDir, "routes.jsonl")
		originalContent := `{"prefix": "hq-", "path": "."}
{"prefix": "hq-cv-", "path": "."}
`
		if err := os.WriteFile(routesPath, []byte(originalContent), 0644); err != nil {
			t.Fatal(err)
		}

		// Create mayor directory
		if err := os.MkdirAll(filepath.Join(tmpDir, "mayor"), 0755); err != nil {
			t.Fatal(err)
		}

		check := NewRoutesCheck()
		ctx := &CheckContext{TownRoot: tmpDir}

		// Run fix (should be a no-op)
		if err := check.Fix(ctx); err != nil {
			t.Fatalf("Fix failed: %v", err)
		}

		// Verify routes.jsonl is unchanged (no duplicate)
		content, err := os.ReadFile(routesPath)
		if err != nil {
			t.Fatalf("Failed to read routes.jsonl: %v", err)
		}

		// File should be unchanged - fix doesn't write when no modifications needed
		if string(content) != originalContent {
			t.Errorf("routes.jsonl was modified when it shouldn't have been: %s", string(content))
		}
	})
}

func TestRoutesCheck_CorruptedRoutesJsonl(t *testing.T) {
	t.Run("corrupted routes.jsonl results in empty routes", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create .beads directory with corrupted routes.jsonl
		beadsDir := filepath.Join(tmpDir, ".beads")
		if err := os.MkdirAll(beadsDir, 0755); err != nil {
			t.Fatal(err)
		}

		// Create corrupted routes.jsonl (malformed lines are skipped by LoadRoutes)
		routesPath := filepath.Join(beadsDir, "routes.jsonl")
		if err := os.WriteFile(routesPath, []byte("not valid json"), 0644); err != nil {
			t.Fatal(err)
		}

		// Create mayor directory
		if err := os.MkdirAll(filepath.Join(tmpDir, "mayor"), 0755); err != nil {
			t.Fatal(err)
		}

		check := NewRoutesCheck()
		ctx := &CheckContext{TownRoot: tmpDir}
		result := check.Run(ctx)

		// Corrupted/malformed lines are skipped, resulting in empty routes
		// This triggers the "Required town routes are missing" warning
		if result.Status != StatusWarning {
			t.Errorf("expected StatusWarning, got %v: %s", result.Status, result.Message)
		}
		if result.Message != "Required town routes are missing" {
			t.Errorf("expected 'Required town routes are missing', got %s", result.Message)
		}
	})

	t.Run("fix regenerates corrupted routes.jsonl with town route", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create .beads directory with corrupted routes.jsonl
		beadsDir := filepath.Join(tmpDir, ".beads")
		if err := os.MkdirAll(beadsDir, 0755); err != nil {
			t.Fatal(err)
		}

		// Create corrupted routes.jsonl
		routesPath := filepath.Join(beadsDir, "routes.jsonl")
		if err := os.WriteFile(routesPath, []byte("not valid json"), 0644); err != nil {
			t.Fatal(err)
		}

		// Create mayor directory
		if err := os.MkdirAll(filepath.Join(tmpDir, "mayor"), 0755); err != nil {
			t.Fatal(err)
		}

		check := NewRoutesCheck()
		ctx := &CheckContext{TownRoot: tmpDir}

		// Run fix
		if err := check.Fix(ctx); err != nil {
			t.Fatalf("Fix failed: %v", err)
		}

		// Verify routes.jsonl now contains both hq- and hq-cv- routes
		content, err := os.ReadFile(routesPath)
		if err != nil {
			t.Fatalf("Failed to read routes.jsonl: %v", err)
		}

		contentStr := string(content)
		if contentStr != `{"prefix":"hq-","path":"."}
{"prefix":"hq-cv-","path":"."}
` {
			t.Errorf("unexpected routes.jsonl content after fix: %s", contentStr)
		}

		// Verify the check now passes
		result := check.Run(ctx)
		if result.Status != StatusOK {
			t.Errorf("expected StatusOK after fix, got %v: %s", result.Status, result.Message)
		}
	})
}
