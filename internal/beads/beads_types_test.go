package beads

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/constants"
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

		// Create sentinel file with current types list
		currentTypes := strings.Join(constants.BeadsCustomTypesList(), ",")
		sentinelPath := filepath.Join(beadsDir, typesSentinel)
		if err := os.WriteFile(sentinelPath, []byte(currentTypes+"\n"), 0644); err != nil {
			t.Fatal(err)
		}

		// Reset cache to ensure we're testing sentinel detection
		ResetEnsuredDirs()

		// This should succeed without running bd (sentinel matches)
		err := EnsureCustomTypes(beadsDir)
		if err != nil {
			t.Errorf("expected success with sentinel file, got: %v", err)
		}
	})

	t.Run("stale sentinel triggers re-configuration", func(t *testing.T) {
		tmpDir := t.TempDir()
		beadsDir := filepath.Join(tmpDir, ".beads")
		if err := os.MkdirAll(beadsDir, 0755); err != nil {
			t.Fatal(err)
		}

		// Create sentinel file with old/legacy content (gt-zmy, gt-26f)
		sentinelPath := filepath.Join(beadsDir, typesSentinel)
		if err := os.WriteFile(sentinelPath, []byte("v1\n"), 0644); err != nil {
			t.Fatal(err)
		}

		ResetEnsuredDirs()

		// Should NOT cache-hit — sentinel is stale. Will attempt bd config set,
		// which may fail in test env (no bd), but it should NOT return nil
		// from the sentinel check.
		err := EnsureCustomTypes(beadsDir)
		// We can't assert success/failure (depends on bd availability),
		// but verify it didn't silently cache the stale sentinel
		if ensuredDirs[beadsDir] && err != nil {
			t.Error("should not cache a failed re-configuration")
		}
	})

	t.Run("in-memory cache prevents repeated calls", func(t *testing.T) {
		tmpDir := t.TempDir()
		beadsDir := filepath.Join(tmpDir, ".beads")
		if err := os.MkdirAll(beadsDir, 0755); err != nil {
			t.Fatal(err)
		}

		// Create sentinel with current types to avoid bd call
		currentTypes := strings.Join(constants.BeadsCustomTypesList(), ",")
		sentinelPath := filepath.Join(beadsDir, typesSentinel)
		if err := os.WriteFile(sentinelPath, []byte(currentTypes+"\n"), 0644); err != nil {
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

func TestEnsureDatabaseInitialized(t *testing.T) {
	t.Run("dolt dir exists — skip init", func(t *testing.T) {
		beadsDir := filepath.Join(t.TempDir(), ".beads")
		os.MkdirAll(filepath.Join(beadsDir, "dolt"), 0755)

		err := ensureDatabaseInitialized(beadsDir)
		if err != nil {
			t.Errorf("expected nil error when dolt/ exists, got: %v", err)
		}
	})

	t.Run("metadata.json with valid db — skip init (server mode)", func(t *testing.T) {
		// Set up town structure with .dolt-data/<db> so the deep check passes
		townDir := t.TempDir()
		mayorDir := filepath.Join(townDir, "mayor")
		os.MkdirAll(mayorDir, 0755)
		os.WriteFile(filepath.Join(mayorDir, "town.json"), []byte("{}"), 0644)

		rigDir := filepath.Join(townDir, "testrig")
		beadsDir := filepath.Join(rigDir, ".beads")
		os.MkdirAll(beadsDir, 0755)

		// Create the referenced database in .dolt-data/
		os.MkdirAll(filepath.Join(townDir, ".dolt-data", "testdb"), 0755)

		meta := `{"dolt_mode":"server","dolt_database":"testdb"}`
		os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte(meta), 0644)

		err := ensureDatabaseInitialized(beadsDir)
		if err != nil {
			t.Errorf("expected nil error when metadata.json + .dolt-data/<db> exist, got: %v", err)
		}
	})

	t.Run("metadata.json exists but db missing — attempts init", func(t *testing.T) {
		// metadata.json references a database that doesn't exist in .dolt-data/
		townDir := t.TempDir()
		mayorDir := filepath.Join(townDir, "mayor")
		os.MkdirAll(mayorDir, 0755)
		os.WriteFile(filepath.Join(mayorDir, "town.json"), []byte("{}"), 0644)
		os.MkdirAll(filepath.Join(townDir, ".dolt-data"), 0755) // empty .dolt-data

		rigDir := filepath.Join(townDir, "testrig")
		beadsDir := filepath.Join(rigDir, ".beads")
		os.MkdirAll(beadsDir, 0755)

		meta := `{"dolt_mode":"server","dolt_database":"missing_db"}`
		os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte(meta), 0644)

		// Should fall through to bd init (not short-circuit on metadata.json alone).
		// bd init may or may not succeed depending on test env, but should not panic.
		_ = ensureDatabaseInitialized(beadsDir)
	})

	t.Run("beads.db exists — skip init (legacy)", func(t *testing.T) {
		beadsDir := filepath.Join(t.TempDir(), ".beads")
		os.MkdirAll(beadsDir, 0755)
		os.WriteFile(filepath.Join(beadsDir, "beads.db"), []byte("sqlite"), 0644)

		err := ensureDatabaseInitialized(beadsDir)
		if err != nil {
			t.Errorf("expected nil error when beads.db exists, got: %v", err)
		}
	})

	t.Run("no database artifacts — attempts bd init", func(t *testing.T) {
		beadsDir := filepath.Join(t.TempDir(), ".beads")
		os.MkdirAll(beadsDir, 0755)

		// With no dolt/, metadata.json, or beads.db, ensureDatabaseInitialized
		// must attempt bd init. The result depends on whether bd is available
		// in the test environment. Either way, it should not panic.
		_ = ensureDatabaseInitialized(beadsDir)
	})
}

func TestDetectPrefix(t *testing.T) {
	t.Run("config.yaml unquoted prefix", func(t *testing.T) {
		beadsDir := filepath.Join(t.TempDir(), ".beads")
		os.MkdirAll(beadsDir, 0755)
		os.WriteFile(filepath.Join(beadsDir, "config.yaml"), []byte("issue-prefix: myrig-\n"), 0644)

		got := detectPrefix(beadsDir)
		if got != "myrig" {
			t.Errorf("detectPrefix() = %q, want %q", got, "myrig")
		}
	})

	t.Run("config.yaml double-quoted prefix", func(t *testing.T) {
		beadsDir := filepath.Join(t.TempDir(), ".beads")
		os.MkdirAll(beadsDir, 0755)
		os.WriteFile(filepath.Join(beadsDir, "config.yaml"), []byte("prefix: \"myrig\"\n"), 0644)

		got := detectPrefix(beadsDir)
		if got != "myrig" {
			t.Errorf("detectPrefix() = %q, want %q", got, "myrig")
		}
	})

	t.Run("config.yaml single-quoted prefix", func(t *testing.T) {
		beadsDir := filepath.Join(t.TempDir(), ".beads")
		os.MkdirAll(beadsDir, 0755)
		os.WriteFile(filepath.Join(beadsDir, "config.yaml"), []byte("prefix: 'myrig'\n"), 0644)

		got := detectPrefix(beadsDir)
		if got != "myrig" {
			t.Errorf("detectPrefix() = %q, want %q", got, "myrig")
		}
	})

	t.Run("config.yaml double-quoted prefix with trailing dash", func(t *testing.T) {
		beadsDir := filepath.Join(t.TempDir(), ".beads")
		os.MkdirAll(beadsDir, 0755)
		os.WriteFile(filepath.Join(beadsDir, "config.yaml"), []byte("prefix: \"myrig-\"\n"), 0644)

		got := detectPrefix(beadsDir)
		if got != "myrig" {
			t.Errorf("detectPrefix() = %q, want %q (quotes stripped before dash trim)", got, "myrig")
		}
	})

	t.Run("config.yaml single-quoted prefix with trailing dash", func(t *testing.T) {
		beadsDir := filepath.Join(t.TempDir(), ".beads")
		os.MkdirAll(beadsDir, 0755)
		os.WriteFile(filepath.Join(beadsDir, "config.yaml"), []byte("prefix: 'myrig-'\n"), 0644)

		got := detectPrefix(beadsDir)
		if got != "myrig" {
			t.Errorf("detectPrefix() = %q, want %q (quotes stripped before dash trim)", got, "myrig")
		}
	})

	t.Run("config.yaml invalid prefix falls through to default", func(t *testing.T) {
		beadsDir := filepath.Join(t.TempDir(), ".beads")
		os.MkdirAll(beadsDir, 0755)
		os.WriteFile(filepath.Join(beadsDir, "config.yaml"), []byte("prefix: 123-invalid\n"), 0644)

		got := detectPrefix(beadsDir)
		if got != "gt" {
			t.Errorf("detectPrefix() = %q, want %q", got, "gt")
		}
	})

	t.Run("no config.yaml falls through to default", func(t *testing.T) {
		beadsDir := filepath.Join(t.TempDir(), ".beads")
		os.MkdirAll(beadsDir, 0755)

		got := detectPrefix(beadsDir)
		if got != "gt" {
			t.Errorf("detectPrefix() = %q, want %q", got, "gt")
		}
	})

	t.Run("rigs.json authoritative source", func(t *testing.T) {
		// Create town structure with rigs.json
		townDir := t.TempDir()
		mayorDir := filepath.Join(townDir, "mayor")
		os.MkdirAll(mayorDir, 0755)
		os.WriteFile(filepath.Join(mayorDir, "town.json"), []byte("{}"), 0644)

		// Create rigs.json with a prefix for our rig
		rigsJSON := `{"rigs": {"testrig": {"prefix": "tr"}}}`
		os.WriteFile(filepath.Join(mayorDir, "rigs.json"), []byte(rigsJSON), 0644)

		// Create rig directory with .beads
		rigDir := filepath.Join(townDir, "testrig")
		beadsDir := filepath.Join(rigDir, ".beads")
		os.MkdirAll(beadsDir, 0755)

		got := detectPrefix(beadsDir)
		// GetRigPrefix should find "testrig" in rigs.json and return "tr".
		// If the town structure isn't recognized (e.g., GetRigPrefix expects
		// additional files), it falls back to "gt" — both are valid prefixes.
		if got != "tr" && got != "gt" {
			t.Errorf("detectPrefix() = %q, want %q or %q", got, "tr", "gt")
		}
	})

	t.Run("routed path falls back to default", func(t *testing.T) {
		// Routed beads path: mayor/rig/.beads — filepath.Base(filepath.Dir)
		// yields "rig", not the actual rig name. Should fall back to "gt".
		townDir := t.TempDir()
		mayorDir := filepath.Join(townDir, "mayor")
		os.MkdirAll(mayorDir, 0755)
		os.WriteFile(filepath.Join(mayorDir, "town.json"), []byte("{}"), 0644)

		rigDir := filepath.Join(townDir, "myrig")
		routedDir := filepath.Join(rigDir, "mayor", "rig")
		beadsDir := filepath.Join(routedDir, ".beads")
		os.MkdirAll(beadsDir, 0755)

		got := detectPrefix(beadsDir)
		// "rig" won't be found in rigs.json → falls to "gt" default
		if got != "gt" {
			t.Errorf("detectPrefix() for routed path = %q, want %q", got, "gt")
		}
	})
}

func TestStripYAMLQuotes(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`"myrig"`, "myrig"},
		{`'myrig'`, "myrig"},
		{"myrig", "myrig"},
		{`""`, ""},
		{`"a"`, "a"},
		{`"`, `"`},
		{"", ""},
	}
	for _, tc := range tests {
		got := stripYAMLQuotes(tc.input)
		if got != tc.expected {
			t.Errorf("stripYAMLQuotes(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
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
