package beads

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindTownRoot(t *testing.T) {
	// Create a temporary town structure
	tmpDir := t.TempDir()
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mayorDir, "town.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create nested directories
	deepDir := filepath.Join(tmpDir, "rig1", "crew", "worker1")
	if err := os.MkdirAll(deepDir, 0755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		startDir string
		expected string
	}{
		{"from town root", tmpDir, tmpDir},
		{"from mayor dir", mayorDir, tmpDir},
		{"from deep nested dir", deepDir, tmpDir},
		{"from non-town dir", t.TempDir(), ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := FindTownRoot(tc.startDir)
			if result != tc.expected {
				t.Errorf("FindTownRoot(%q) = %q, want %q", tc.startDir, result, tc.expected)
			}
		})
	}
}

func TestResolveRoutingTarget(t *testing.T) {
	// Create a temporary town with routes
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create mayor/town.json for FindTownRoot
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mayorDir, "town.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create routes.jsonl
	routesContent := `{"prefix": "gt-", "path": "gastown/mayor/rig"}
{"prefix": "hq-", "path": "."}
`
	if err := os.WriteFile(filepath.Join(beadsDir, "routes.jsonl"), []byte(routesContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create the rig beads directory
	rigBeadsDir := filepath.Join(tmpDir, "gastown", "mayor", "rig", ".beads")
	if err := os.MkdirAll(rigBeadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	fallback := "/fallback/.beads"

	tests := []struct {
		name     string
		townRoot string
		beadID   string
		expected string
	}{
		{
			name:     "rig-level bead routes to rig",
			townRoot: tmpDir,
			beadID:   "gt-gastown-polecat-Toast",
			expected: rigBeadsDir,
		},
		{
			name:     "town-level bead routes to town",
			townRoot: tmpDir,
			beadID:   "hq-mayor",
			expected: beadsDir,
		},
		{
			name:     "unknown prefix falls back",
			townRoot: tmpDir,
			beadID:   "xx-unknown",
			expected: fallback,
		},
		{
			name:     "empty townRoot falls back",
			townRoot: "",
			beadID:   "gt-gastown-polecat-Toast",
			expected: fallback,
		},
		{
			name:     "no prefix falls back",
			townRoot: tmpDir,
			beadID:   "noprefixid",
			expected: fallback,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ResolveRoutingTarget(tc.townRoot, tc.beadID, fallback)
			if result != tc.expected {
				t.Errorf("ResolveRoutingTarget(%q, %q, %q) = %q, want %q",
					tc.townRoot, tc.beadID, fallback, result, tc.expected)
			}
		})
	}
}

func TestEnsureCustomTypes(t *testing.T) {
	// Reset the in-memory cache before testing
	ResetEnsuredDirs()

	t.Run("empty beads dir returns error", func(t *testing.T) {
		err := EnsureCustomTypes("")
		if err == nil {
			t.Error("expected error for empty beads dir")
		}
	})

	t.Run("non-existent beads dir returns error", func(t *testing.T) {
		err := EnsureCustomTypes("/nonexistent/path/.beads")
		if err == nil {
			t.Error("expected error for non-existent beads dir")
		}
	})

	t.Run("sentinel file triggers cache hit", func(t *testing.T) {
		tmpDir := t.TempDir()
		beadsDir := filepath.Join(tmpDir, ".beads")
		if err := os.MkdirAll(beadsDir, 0755); err != nil {
			t.Fatal(err)
		}

		// Create sentinel file
		sentinelPath := filepath.Join(beadsDir, typesSentinel)
		if err := os.WriteFile(sentinelPath, []byte("v1\n"), 0644); err != nil {
			t.Fatal(err)
		}

		// Reset cache to ensure we're testing sentinel detection
		ResetEnsuredDirs()

		// This should succeed without running bd (sentinel exists)
		err := EnsureCustomTypes(beadsDir)
		if err != nil {
			t.Errorf("expected success with sentinel file, got: %v", err)
		}
	})

	t.Run("in-memory cache prevents repeated calls", func(t *testing.T) {
		tmpDir := t.TempDir()
		beadsDir := filepath.Join(tmpDir, ".beads")
		if err := os.MkdirAll(beadsDir, 0755); err != nil {
			t.Fatal(err)
		}

		// Create sentinel to avoid bd call
		sentinelPath := filepath.Join(beadsDir, typesSentinel)
		if err := os.WriteFile(sentinelPath, []byte("v1\n"), 0644); err != nil {
			t.Fatal(err)
		}

		ResetEnsuredDirs()

		// First call
		if err := EnsureCustomTypes(beadsDir); err != nil {
			t.Fatal(err)
		}

		// Remove sentinel - second call should still succeed due to in-memory cache
		os.Remove(sentinelPath)

		if err := EnsureCustomTypes(beadsDir); err != nil {
			t.Errorf("expected cache hit, got: %v", err)
		}
	})
}

func TestBeads_getTownRoot(t *testing.T) {
	// Create a temporary town
	tmpDir := t.TempDir()
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mayorDir, "town.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create nested directory
	rigDir := filepath.Join(tmpDir, "myrig", "mayor", "rig")
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}

	b := New(rigDir)

	// First call should find town root
	root1 := b.getTownRoot()
	if root1 != tmpDir {
		t.Errorf("first getTownRoot() = %q, want %q", root1, tmpDir)
	}

	// Second call should return cached value
	root2 := b.getTownRoot()
	if root2 != root1 {
		t.Errorf("second getTownRoot() = %q, want cached %q", root2, root1)
	}

	// Verify caching works (sync.Once ensures single execution)
	if b.townRoot != tmpDir {
		t.Errorf("expected townRoot to be cached as %q, got %q", tmpDir, b.townRoot)
	}
}
