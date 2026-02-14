package doctor

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
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

func TestRoutesCheck_DirectLayoutRig(t *testing.T) {
	t.Run("Run matches direct-layout rig correctly", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create town-level .beads
		beadsDir := filepath.Join(tmpDir, ".beads")
		if err := os.MkdirAll(beadsDir, 0755); err != nil {
			t.Fatal(err)
		}

		// Create direct-layout rig: .beads at rig root, no redirect
		rigBeadsDir := filepath.Join(tmpDir, "myrig", ".beads")
		if err := os.MkdirAll(rigBeadsDir, 0755); err != nil {
			t.Fatal(err)
		}
		// No redirect file — this is a direct layout

		// Create mayor dir and rigs.json
		if err := os.MkdirAll(filepath.Join(tmpDir, "mayor"), 0755); err != nil {
			t.Fatal(err)
		}
		rigsPath := filepath.Join(tmpDir, "mayor", "rigs.json")
		rigsContent := `{
			"version": 1,
			"rigs": {
				"myrig": {
					"git_url": "https://github.com/example/myrig",
					"beads": { "prefix": "mr" }
				}
			}
		}`
		if err := os.WriteFile(rigsPath, []byte(rigsContent), 0644); err != nil {
			t.Fatal(err)
		}

		// Create routes.jsonl with the direct-layout path (myrig, not myrig/mayor/rig)
		routesPath := filepath.Join(beadsDir, "routes.jsonl")
		routesContent := `{"prefix":"hq-","path":"."}
{"prefix":"hq-cv-","path":"."}
{"prefix":"mr-","path":"myrig"}
`
		if err := os.WriteFile(routesPath, []byte(routesContent), 0644); err != nil {
			t.Fatal(err)
		}

		check := NewRoutesCheck()
		ctx := &CheckContext{TownRoot: tmpDir}
		result := check.Run(ctx)

		if result.Status != StatusOK {
			t.Errorf("expected StatusOK for direct-layout rig, got %v: %s (details: %v)", result.Status, result.Message, result.Details)
		}
	})

	t.Run("Fix writes direct-layout path for rig without redirect", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create town-level .beads with empty routes
		beadsDir := filepath.Join(tmpDir, ".beads")
		if err := os.MkdirAll(beadsDir, 0755); err != nil {
			t.Fatal(err)
		}
		routesPath := filepath.Join(beadsDir, "routes.jsonl")
		if err := os.WriteFile(routesPath, []byte(""), 0644); err != nil {
			t.Fatal(err)
		}

		// Create direct-layout rig: .beads at rig root, no redirect
		rigDir := filepath.Join(tmpDir, "myrig")
		rigBeadsDir := filepath.Join(rigDir, ".beads")
		if err := os.MkdirAll(rigBeadsDir, 0755); err != nil {
			t.Fatal(err)
		}

		// Create mayor dir and rigs.json
		if err := os.MkdirAll(filepath.Join(tmpDir, "mayor"), 0755); err != nil {
			t.Fatal(err)
		}
		rigsPath := filepath.Join(tmpDir, "mayor", "rigs.json")
		rigsContent := `{
			"version": 1,
			"rigs": {
				"myrig": {
					"git_url": "https://github.com/example/myrig",
					"beads": { "prefix": "mr" }
				}
			}
		}`
		if err := os.WriteFile(rigsPath, []byte(rigsContent), 0644); err != nil {
			t.Fatal(err)
		}

		check := NewRoutesCheck()
		ctx := &CheckContext{TownRoot: tmpDir}

		if err := check.Fix(ctx); err != nil {
			t.Fatalf("Fix failed: %v", err)
		}

		content, err := os.ReadFile(routesPath)
		if err != nil {
			t.Fatalf("Failed to read routes.jsonl: %v", err)
		}

		contentStr := string(content)
		// Should use "myrig" path (direct layout), not "myrig/mayor/rig"
		expected := `{"prefix":"hq-","path":"."}
{"prefix":"hq-cv-","path":"."}
{"prefix":"mr-","path":"myrig"}
`
		if contentStr != expected {
			t.Errorf("unexpected routes.jsonl content:\ngot:  %s\nwant: %s", contentStr, expected)
		}
	})
}

func TestDetermineRigBeadsPath_Containment(t *testing.T) {
	t.Run("redirect escaping town root falls back to default", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create rig with redirect that escapes the town root
		rigDir := filepath.Join(tmpDir, "myrig")
		rigBeadsDir := filepath.Join(rigDir, ".beads")
		if err := os.MkdirAll(rigBeadsDir, 0755); err != nil {
			t.Fatal(err)
		}
		// Write a redirect that escapes via ..
		if err := os.WriteFile(filepath.Join(rigBeadsDir, "redirect"), []byte("../../outside/.beads\n"), 0644); err != nil {
			t.Fatal(err)
		}

		result := determineRigBeadsPath(tmpDir, "myrig")
		expected := "myrig/mayor/rig"
		if result != expected {
			t.Errorf("expected fallback %q for escaped redirect, got %q", expected, result)
		}
	})

	t.Run("valid redirect within town root resolves correctly", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create rig with redirect to mayor/rig/.beads
		rigDir := filepath.Join(tmpDir, "myrig")
		rigBeadsDir := filepath.Join(rigDir, ".beads")
		mayorBeadsDir := filepath.Join(rigDir, "mayor", "rig", ".beads")
		if err := os.MkdirAll(rigBeadsDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(mayorBeadsDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(rigBeadsDir, "redirect"), []byte("mayor/rig/.beads\n"), 0644); err != nil {
			t.Fatal(err)
		}

		result := determineRigBeadsPath(tmpDir, "myrig")
		expected := "myrig/mayor/rig"
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})

	t.Run("direct layout with no redirect returns rig name", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create direct-layout rig
		rigBeadsDir := filepath.Join(tmpDir, "myrig", ".beads")
		if err := os.MkdirAll(rigBeadsDir, 0755); err != nil {
			t.Fatal(err)
		}

		result := determineRigBeadsPath(tmpDir, "myrig")
		expected := "myrig"
		if result != expected {
			t.Errorf("expected %q for direct layout, got %q", expected, result)
		}
	})
}

func TestRoutesCheck_SuboptimalRoutes(t *testing.T) {
	// Helper to set up a town with a legacy rig whose route points to the rig root
	// instead of the canonical mayor/rig path.
	setupLegacyRig := func(t *testing.T) (tmpDir string) {
		t.Helper()
		tmpDir = t.TempDir()

		// Town-level .beads
		beadsDir := filepath.Join(tmpDir, ".beads")
		if err := os.MkdirAll(beadsDir, 0755); err != nil {
			t.Fatal(err)
		}

		// Legacy rig: redirect at rig root points to mayor/rig/.beads
		rigBeadsDir := filepath.Join(tmpDir, "crom", ".beads")
		if err := os.MkdirAll(rigBeadsDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(rigBeadsDir, "redirect"), []byte("mayor/rig/.beads\n"), 0644); err != nil {
			t.Fatal(err)
		}
		// The canonical target must exist with a real .beads directory
		canonicalBeads := filepath.Join(tmpDir, "crom", "mayor", "rig", ".beads")
		if err := os.MkdirAll(canonicalBeads, 0755); err != nil {
			t.Fatal(err)
		}

		// rigs.json registers the rig
		if err := os.MkdirAll(filepath.Join(tmpDir, "mayor"), 0755); err != nil {
			t.Fatal(err)
		}
		rigsContent := `{
			"version": 1,
			"rigs": {
				"crom": {
					"git_url": "https://github.com/example/crom",
					"beads": { "prefix": "cr" }
				}
			}
		}`
		if err := os.WriteFile(filepath.Join(tmpDir, "mayor", "rigs.json"), []byte(rigsContent), 0644); err != nil {
			t.Fatal(err)
		}

		// Legacy route: prefix cr- points to rig root "crom" (suboptimal)
		routesContent := `{"prefix":"hq-","path":"."}
{"prefix":"hq-cv-","path":"."}
{"prefix":"cr-","path":"crom"}
`
		if err := os.WriteFile(filepath.Join(beadsDir, "routes.jsonl"), []byte(routesContent), 0644); err != nil {
			t.Fatal(err)
		}

		return tmpDir
	}

	t.Run("Run detects legacy rig-root route as suboptimal", func(t *testing.T) {
		tmpDir := setupLegacyRig(t)

		check := NewRoutesCheck()
		ctx := &CheckContext{TownRoot: tmpDir}
		result := check.Run(ctx)

		if result.Status != StatusWarning {
			t.Errorf("expected StatusWarning, got %v: %s", result.Status, result.Message)
		}
		if len(result.Details) == 0 {
			t.Fatal("expected details about suboptimal route, got none")
		}
		found := false
		for _, d := range result.Details {
			if strings.Contains(d, "cr-") && strings.Contains(d, "crom/mayor/rig") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected detail mentioning cr- and crom/mayor/rig, got: %v", result.Details)
		}
	})

	t.Run("Fix rewrites suboptimal route to canonical path", func(t *testing.T) {
		tmpDir := setupLegacyRig(t)

		check := NewRoutesCheck()
		ctx := &CheckContext{TownRoot: tmpDir}

		if err := check.Fix(ctx); err != nil {
			t.Fatalf("Fix failed: %v", err)
		}

		// Read routes.jsonl and verify cr- now points to crom/mayor/rig
		content, err := os.ReadFile(filepath.Join(tmpDir, ".beads", "routes.jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		contentStr := string(content)
		if !strings.Contains(contentStr, `"path":"crom/mayor/rig"`) {
			t.Errorf("expected route rewritten to crom/mayor/rig, got:\n%s", contentStr)
		}
		if strings.Contains(contentStr, `"path":"crom"}`) {
			t.Error("old suboptimal route path 'crom' still present after fix")
		}

		// Run should now pass
		result := check.Run(ctx)
		if result.Status != StatusOK {
			t.Errorf("expected StatusOK after fix, got %v: %s (details: %v)", result.Status, result.Message, result.Details)
		}
	})

	t.Run("Fix skips rewrite when canonical target has no .beads directory", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Town-level .beads
		beadsDir := filepath.Join(tmpDir, ".beads")
		if err := os.MkdirAll(beadsDir, 0755); err != nil {
			t.Fatal(err)
		}

		// Rig with redirect, but the canonical target directory has NO .beads
		rigBeadsDir := filepath.Join(tmpDir, "crom", ".beads")
		if err := os.MkdirAll(rigBeadsDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(rigBeadsDir, "redirect"), []byte("mayor/rig/.beads\n"), 0644); err != nil {
			t.Fatal(err)
		}
		// Create the canonical directory but WITHOUT .beads inside it
		if err := os.MkdirAll(filepath.Join(tmpDir, "crom", "mayor", "rig"), 0755); err != nil {
			t.Fatal(err)
		}
		// Intentionally no .beads here — this makes the target beads-invalid

		// rigs.json
		if err := os.MkdirAll(filepath.Join(tmpDir, "mayor"), 0755); err != nil {
			t.Fatal(err)
		}
		rigsContent := `{
			"version": 1,
			"rigs": {
				"crom": {
					"git_url": "https://github.com/example/crom",
					"beads": { "prefix": "cr" }
				}
			}
		}`
		if err := os.WriteFile(filepath.Join(tmpDir, "mayor", "rigs.json"), []byte(rigsContent), 0644); err != nil {
			t.Fatal(err)
		}

		// Route pointing to rig root
		routesContent := `{"prefix":"hq-","path":"."}
{"prefix":"hq-cv-","path":"."}
{"prefix":"cr-","path":"crom"}
`
		if err := os.WriteFile(filepath.Join(beadsDir, "routes.jsonl"), []byte(routesContent), 0644); err != nil {
			t.Fatal(err)
		}

		check := NewRoutesCheck()
		ctx := &CheckContext{TownRoot: tmpDir}

		// Capture stderr to verify the warning message
		oldStderr := os.Stderr
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		os.Stderr = w

		if err := check.Fix(ctx); err != nil {
			os.Stderr = oldStderr
			t.Fatalf("Fix failed: %v", err)
		}

		w.Close()
		os.Stderr = oldStderr
		var stderrBuf bytes.Buffer
		if _, err := stderrBuf.ReadFrom(r); err != nil {
			t.Fatal(err)
		}
		stderrOutput := stderrBuf.String()

		if !strings.Contains(stderrOutput, "Warning: cannot rewrite route cr-") {
			t.Errorf("expected stderr warning about cannot rewrite route, got: %q", stderrOutput)
		}

		// Route should remain unchanged — "crom" not rewritten because target is not beads-valid
		content, err := os.ReadFile(filepath.Join(beadsDir, "routes.jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		contentStr := string(content)
		if strings.Contains(contentStr, `"path":"crom/mayor/rig"`) {
			t.Error("route was rewritten to crom/mayor/rig despite missing .beads directory at canonical target")
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
